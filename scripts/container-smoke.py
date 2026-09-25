"""Disposable Docker integration environment. Never points at the deployed server."""
import json
import os
import secrets
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:18088"
APP = "storyforge-ci-app"
DB = "storyforge-ci-db"
NETWORK = "storyforge-ci-net"
VOLUME = "storyforge-ci-data"


def docker(*args, check=True):
    result = subprocess.run(["docker", *args], check=check, capture_output=True, text=True)
    return (result.stdout + (result.stderr if args[0] == "logs" else "")).strip()


def api(path, body=None, method=None, expected=200):
    request = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None,
                                     method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=8) as response:
            status, raw = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read()
    assert status == expected, (path, status, raw)
    return json.loads(raw)


def ready():
    for _ in range(90):
        try:
            assert api("/readyz")["database"] == "connected"
            return
        except Exception:
            time.sleep(1)
    raise RuntimeError("Service did not become ready: " + docker("logs", APP, check=False))


def start_app():
    docker("run", "-d", "--name", APP, "--network", NETWORK, "-p", "127.0.0.1:18088:8080",
           "-e", "PGHOST=" + DB, "-e", "PGUSER=storyforge", "-e", "PGDATABASE=storyforge",
           "-e", "PGSSLMODE=disable", "-e", "PGPASSWORD", os.environ["SMOKE_IMAGE"])
    ready()


def setup():
    os.environ["PGPASSWORD"] = secrets.token_hex(24)
    os.environ["POSTGRES_PASSWORD"] = os.environ["PGPASSWORD"]
    # Carry the generated credential between CI steps without committing it or printing it.
    if os.environ.get("GITHUB_ENV"):
        print("::add-mask::" + os.environ["PGPASSWORD"], flush=True)
        with open(os.environ["GITHUB_ENV"], "a") as env:
            env.write("PGPASSWORD=" + os.environ["PGPASSWORD"] + "\n")
    docker("network", "create", NETWORK)
    docker("volume", "create", VOLUME)
    docker("run", "-d", "--name", DB, "--network", NETWORK,
           "-v", VOLUME + ":/var/lib/postgresql/data", "-e", "POSTGRES_USER=postgres",
           "-e", "POSTGRES_DB=postgres", "-e", "POSTGRES_PASSWORD", "postgres:15-alpine")
    for attempt in range(60):
        if 'accepting connections' in docker('exec', DB, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', check=False):
            break
        time.sleep(1)
    # Match the shared Unraid database: a dedicated non-superuser owns this database.
    subprocess.run(['docker', 'exec', '-i', DB, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1'],
                   input="CREATE ROLE storyforge LOGIN PASSWORD '" + os.environ['PGPASSWORD'] + "';\nCREATE DATABASE storyforge OWNER storyforge;\n",
                   text=True, capture_output=True, check=True)
    # Upgrade an existing milestone-2 database, rather than only testing fresh installs.
    baseline = Path('backend/migrations/001_scripts.sql').read_text()
    baseline += """
CREATE TABLE schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
INSERT INTO schema_migrations(name) VALUES ('001_scripts.sql');
INSERT INTO projects(id,name) VALUES ('00000000-0000-0000-0000-000000000001','Existing script');
INSERT INTO characters(id,project_id,name) VALUES ('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','Existing character');
INSERT INTO scenes(id,project_id,name,position) VALUES ('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000001','Existing scene',1);
INSERT INTO script_events(project_id,scene_id,character_id,text,position) VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002','Keep this original line.',1);
"""
    subprocess.run(['docker','exec','-i',DB,'psql','-U','storyforge','-d','storyforge','-v','ON_ERROR_STOP=1'], input=baseline,text=True,capture_output=True,check=True)
    start_app()
    assert api("/healthz")["status"] == "ok"
    assert api('/api/workspace')['events'][0]['text'] == 'Keep this original line.'
    project = api("/api/projects", {"name": "Integration project"}, expected=201)
    other = api("/api/projects", {"name": "Another project"}, expected=201)
    actor = api("/api/actors", {"name": "Integration actor"}, expected=201)
    char = api("/api/characters", {"name": "Guide", "project_id": project["id"]}, expected=201)
    other_char = api("/api/characters", {"name": "Other guide", "project_id": other["id"]}, expected=201)
    scene = api("/api/scenes", {"name": "Forest", "position": 1, "project_id": project["id"]}, expected=201)
    api("/api/assignments/" + char["id"], {"actor_id": actor["id"]}, "PUT")
    line = {"project_id": project["id"], "scene_id": scene["id"], "character_id": char["id"],
            "text": "Welcome home.", "direction": "Warmly", "position": 1, "start_ms": 0}
    created = api("/api/events", line, expected=201)
    assert created["revision"] == 1
    api("/api/events", line, expected=409)  # duplicate position
    api("/api/events", dict(line, position=2, character_id=other_char["id"]), expected=400)
    api("/api/events", dict(line, position=2, start_ms=-1), expected=400)
    overlapping = api("/api/events", dict(line, position=2), expected=201)
    assert overlapping["start_ms"] == created["start_ms"]
    update = dict(line, revision=1, text="Welcome to our forest.")
    updated = api("/api/events/" + created["id"], update, "PUT")
    assert updated["revision"] == 2
    api("/api/events/" + created["id"], update, "PUT", expected=409)
    revisions = docker("exec", DB, "psql", "-U", "storyforge", "-Atc",
                       "SELECT count(*) FROM script_event_revisions WHERE event_id='" + created["id"] + "'")
    assert revisions == "2", revisions
    snapshot = api("/api/workspace")
    assert len(snapshot["events"]) == 3
    with urllib.request.urlopen(BASE, timeout=5) as response:
        assert b"<div id=\"root\">" in response.read()
    print("Database API checks passed: relations, assignments, validation, conflicts, overlap, revisions.")
    tools_checks()


def tools_checks():
    source = '''[script: Imported test]
[cast: Fox | Reader]
[cast: Bird | Reader]
[scene: Arrival]
[Fox]
[direction: Softly]
First paragraph.
Second paragraph.
[Bird]
I hear you.
[scene: Departure]
[Fox]
Goodbye.'''
    before = api('/api/workspace')
    plan = api('/api/import/preview', {'text': source})
    assert plan['line_count'] == 3 and len(plan['scenes']) == 2
    assert api('/api/workspace') == before, 'Preview wrote to database'
    api('/api/import', {'text': '[script: invalid]\n[unknown: tag]', 'request_id': '1'*32}, expected=400)
    assert api('/api/workspace') == before, 'Invalid import partially saved'
    imported = api('/api/import', {'text': source, 'request_id': '2'*32}, expected=201)
    assert api('/api/import', {'text': source, 'request_id': '2'*32}, expected=201)['id'] == imported['id']
    api('/api/import', {'text': source+'\nDifferent text', 'request_id': '2'*32}, expected=409)
    snapshot = api('/api/workspace')
    scenes = [s for s in snapshot['scenes'] if s['project_id']==imported['id']]
    characters = [c for c in snapshot['characters'] if c['project_id']==imported['id']]
    actor = next(a for a in snapshot['actors'] if a['name']=='Reader')
    assert len([a for a in snapshot['assignments'] if a['actor_id']==actor['id']]) == 2
    first = scenes[0]
    lines = [e for e in snapshot['events'] if e['scene_id']==first['id']]
    assert lines[0]['text']=='First paragraph.\nSecond paragraph.'
    ids = [s['id'] for s in scenes]
    api('/api/projects/'+imported['id']+'/scene-order', {'ids':ids[::-1], 'expected':ids}, 'PUT')
    api('/api/projects/'+imported['id']+'/scene-order', {'ids':ids, 'expected':ids}, 'PUT', expected=409)
    foreign = next(s['id'] for s in before['scenes'])
    api('/api/projects/'+imported['id']+'/scene-order', {'ids':[foreign,ids[0]], 'expected':ids[::-1]}, 'PUT', expected=400)
    line_ids = [e['id'] for e in lines]
    api('/api/scenes/'+first['id']+'/line-order', {'ids':line_ids[::-1], 'expected':line_ids}, 'PUT')
    now = [e for e in api('/api/workspace')['events'] if e['scene_id']==first['id']]
    assert [e['id'] for e in now]==line_ids[::-1] and [e['position'] for e in now]==[1,2]
    assert all(e['start_ms']==0 for e in now)
    api('/api/scenes/'+first['id']+'/line-order', {'ids':[line_ids[0]]*2,'expected':line_ids[::-1]}, 'PUT', expected=400)
    api('/api/characters/'+characters[0]['id'], {'confirm':True}, 'DELETE', expected=409)
    api('/api/actors/'+actor['id'], {'confirm':False}, 'DELETE', expected=400)
    api('/api/actors/'+actor['id'], {'confirm':True}, 'DELETE')
    assert not any(a['actor_id']==actor['id'] for a in api('/api/workspace')['assignments'])
    api('/api/assignments/'+characters[0]['id'], {'actor_id':actor['id']}, 'PUT', expected=404)
    api('/api/events/'+now[0]['id'], {'confirm':True,'revision':1}, 'DELETE', expected=409)
    api('/api/events/'+now[0]['id'], {'confirm':True,'revision':now[0]['revision']}, 'DELETE')
    api('/api/scenes/'+first['id'], {'confirm':True}, 'DELETE')
    snapshot = api('/api/workspace')
    assert not any(e['scene_id']==first['id'] for e in snapshot['events'])
    assert not any(s['id']==first['id'] for s in snapshot['scenes'])
    bird = next(c for c in characters if c['name']=='Bird')
    api('/api/characters/'+bird['id'], {'confirm':True}, 'DELETE')
    api('/api/projects/'+imported['id'], {'confirm':True}, 'DELETE')
    snapshot = api('/api/workspace')
    assert not any(p['id']==imported['id'] for p in snapshot['projects'])
    assert not any(e['project_id']==imported['id'] for e in snapshot['events'])
    api('/api/scenes', {'project_id':imported['id'],'name':'Hidden write','position':4}, expected=404)
    api('/api/import', {'text':source,'request_id':'2'*32}, expected=409)
    count = docker('exec',DB,'psql','-U','storyforge','-Atc',"SELECT count(*) FROM script_events WHERE project_id='"+imported['id']+"'")
    assert count == '3', 'Removal destroyed historical records'
    # Ambiguous actor names fail inside the import transaction with no partial project.
    api('/api/actors',{'name':'Duplicate reader'},expected=201)
    api('/api/actors',{'name':'Duplicate reader'},expected=201)
    before = api('/api/workspace')
    api('/api/import',{'text':source.replace('Reader','Duplicate reader'),'request_id':'3'*32},expected=409)
    assert api('/api/workspace')==before
    print('Import preview, rollback, retries, archive preservation, removal rules and atomic reorder passed.')


def restart():
    before = api("/api/workspace")
    docker("stop", APP)
    assert docker("inspect", "--format={{.State.ExitCode}}", APP) == "0"
    docker("rm", APP)
    docker("restart", DB)
    start_app()  # reruns migrations without recreating or duplicating data
    assert api("/api/workspace") == before, "Records changed after restart"
    docker("stop", DB)
    api("/readyz", expected=503)
    assert api("/healthz")["status"] == "ok"
    docker("start", DB)
    ready()
    assert api("/api/workspace") == before
    print("Persistence, migration reentry, graceful shutdown, outage and recovery checks passed.")


def cleanup():
    for name in (APP, DB):
        print(docker("logs", name, check=False))
        docker("rm", "-f", name, check=False)
    docker("volume", "rm", VOLUME, check=False)
    docker("network", "rm", NETWORK, check=False)


if __name__ == "__main__":
    {"setup": setup, "restart": restart, "cleanup": cleanup}[sys.argv[1]]()
