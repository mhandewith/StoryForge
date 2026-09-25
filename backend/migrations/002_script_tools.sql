ALTER TABLE projects ADD COLUMN deleted_at timestamptz;
ALTER TABLE actors ADD COLUMN deleted_at timestamptz;
ALTER TABLE characters ADD COLUMN deleted_at timestamptz;
ALTER TABLE scenes ADD COLUMN deleted_at timestamptz;
ALTER TABLE script_events ADD COLUMN deleted_at timestamptz;

ALTER TABLE scenes DROP CONSTRAINT scenes_project_id_position_key;
ALTER TABLE script_events DROP CONSTRAINT script_events_scene_id_position_key;
ALTER TABLE characters DROP CONSTRAINT characters_project_id_name_key;
ALTER TABLE scenes ALTER COLUMN position TYPE bigint;
ALTER TABLE script_events ALTER COLUMN position TYPE bigint;
ALTER TABLE script_event_revisions ALTER COLUMN position TYPE bigint;
CREATE UNIQUE INDEX scenes_active_position ON scenes(project_id,position) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX events_active_position ON script_events(scene_id,position) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX characters_active_name ON characters(project_id,name) WHERE deleted_at IS NULL;

-- Temporary positions used inside reorder transactions do not create revisions.
CREATE OR REPLACE FUNCTION record_script_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.revision = OLD.revision THEN RETURN NEW; END IF;
    END IF;
    INSERT INTO script_event_revisions(event_id, revision, character_id, text, direction, position, start_ms)
    VALUES (NEW.id, NEW.revision, NEW.character_id, NEW.text, NEW.direction, NEW.position, NEW.start_ms);
    RETURN NEW;
END;
$$;

CREATE TABLE script_imports (
    request_id text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id),
    source_hash text NOT NULL
);

-- Active views keep archived scripts, removed entities, and their history out of the workspace.
CREATE VIEW active_projects AS SELECT * FROM projects WHERE deleted_at IS NULL;
CREATE VIEW active_actors AS SELECT * FROM actors WHERE deleted_at IS NULL;
CREATE VIEW active_scenes AS SELECT s.* FROM scenes s JOIN active_projects p ON p.id=s.project_id WHERE s.deleted_at IS NULL;
CREATE VIEW active_characters AS SELECT c.* FROM characters c JOIN active_projects p ON p.id=c.project_id WHERE c.deleted_at IS NULL;
CREATE VIEW active_events AS SELECT e.* FROM script_events e JOIN active_scenes s ON s.id=e.scene_id WHERE e.deleted_at IS NULL;
CREATE VIEW active_assignments AS SELECT a.* FROM assignments a JOIN active_characters c ON c.id=a.character_id JOIN active_actors actor ON actor.id=a.actor_id;
