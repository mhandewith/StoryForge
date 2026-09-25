CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE actors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE scenes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    position integer NOT NULL CHECK (position > 0),
    UNIQUE (project_id, position),
    UNIQUE (id, project_id)
);
CREATE TABLE characters (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    UNIQUE (project_id, name),
    UNIQUE (id, project_id)
);
CREATE TABLE assignments (
    character_id uuid PRIMARY KEY REFERENCES characters(id),
    actor_id uuid NOT NULL REFERENCES actors(id)
);
CREATE TABLE script_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    scene_id uuid NOT NULL,
    character_id uuid NOT NULL,
    kind text NOT NULL DEFAULT 'dialogue' CHECK (kind = 'dialogue'),
    text text NOT NULL CHECK (length(trim(text)) BETWEEN 1 AND 10000),
    direction text NOT NULL DEFAULT '' CHECK (length(direction) <= 2000),
    position integer NOT NULL CHECK (position > 0),
    start_ms bigint NOT NULL DEFAULT 0 CHECK (start_ms BETWEEN 0 AND 86400000),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (scene_id, project_id) REFERENCES scenes(id, project_id),
    FOREIGN KEY (character_id, project_id) REFERENCES characters(id, project_id),
    UNIQUE (scene_id, position)
);
-- Preserve every dialogue version. Future takes can reference (event_id, revision).
CREATE TABLE script_event_revisions (
    event_id uuid NOT NULL REFERENCES script_events(id),
    revision integer NOT NULL,
    character_id uuid NOT NULL REFERENCES characters(id),
    text text NOT NULL,
    direction text NOT NULL,
    position integer NOT NULL,
    start_ms bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, revision)
);
CREATE FUNCTION record_script_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO script_event_revisions(event_id, revision, character_id, text, direction, position, start_ms)
    VALUES (NEW.id, NEW.revision, NEW.character_id, NEW.text, NEW.direction, NEW.position, NEW.start_ms);
    RETURN NEW;
END;
$$;
CREATE TRIGGER script_revision_history AFTER INSERT OR UPDATE ON script_events
FOR EACH ROW EXECUTE FUNCTION record_script_revision();
CREATE INDEX scenes_project_idx ON scenes(project_id);
CREATE INDEX characters_project_idx ON characters(project_id);
CREATE INDEX script_events_project_idx ON script_events(project_id);
CREATE INDEX assignments_actor_idx ON assignments(actor_id);
