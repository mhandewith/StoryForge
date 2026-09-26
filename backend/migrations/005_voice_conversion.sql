ALTER TABLE characters ADD COLUMN eleven_voice_id text NOT NULL DEFAULT '';
CREATE OR REPLACE VIEW active_characters AS SELECT c.* FROM characters c JOIN active_projects p ON p.id=c.project_id WHERE c.deleted_at IS NULL;

CREATE TABLE voice_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 scene_id uuid NOT NULL REFERENCES scenes(id),
 snapshot_hash text NOT NULL,
 request_id uuid UNIQUE NOT NULL,
 forced_event text NOT NULL DEFAULT '',
 requested_by text NOT NULL,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','complete','failed')),
 error text NOT NULL DEFAULT '',
 audio_key text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX voice_runs_one_active ON voice_runs(scene_id) WHERE state IN ('queued','running');
CREATE TABLE voice_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 run_id uuid NOT NULL REFERENCES voice_runs(id),
 event_id uuid NOT NULL REFERENCES script_events(id),
 take_id uuid NOT NULL REFERENCES takes(id),
 voice_id text NOT NULL,
 position integer NOT NULL,
 source_key text NOT NULL,
 state text NOT NULL CHECK(state IN ('queued','running','complete','failed')),
 audio_key text NOT NULL DEFAULT '',
 error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 UNIQUE(run_id,event_id)
);
CREATE INDEX voice_jobs_reuse ON voice_jobs(take_id,voice_id,completed_at DESC) WHERE state='complete';
CREATE INDEX voice_jobs_run ON voice_jobs(run_id,position);
