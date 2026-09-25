"""Disposable Docker integration environment. Never points at the deployed server."""
import json
import os
import secrets
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:18088"
APP = "storyforge-ci-app"
DB = "storyforge-ci-db"
NETWORK = "storyforge-ci-net"
VOLUME = "storyforge-ci-data"


def docker(*args, check=True):
    return subprocess.run(["docker", *args], check=check, capture_output=True, text=True).stdout.strip()


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
        with open(os.environ["GITHUB_ENV"], "a") as env:
            env.write("PGPASSWORD=" + os.environ["PGPASSWORD"] + "\n")
    docker("network", "create", NETWORK)
    docker("volume", "create", VOLUME)
    docker("run", "-d", "--name", DB, "--network", NETWORK,
           "-v", VOLUME + ":/var/lib/postgresql/data", "-e", "POSTGRES_USER=storyforge",
           "-e", "POSTGRES_DB=storyforge", "-e", "POSTGRES_PASSWORD", "postgres:17-alpine")
    start_app()
    assert api("/healthz")["status"] == "ok"
    assert all(not v for v in api("/api/workspace").values())
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
    assert len(snapshot["events"]) == 2
    with urllib.request.urlopen(BASE, timeout=5) as response:
        assert b"<div id=\"root\">" in response.read()
    print("Database API checks passed: relations, assignments, validation, conflicts, overlap, revisions.")


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
