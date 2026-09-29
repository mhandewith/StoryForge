-- Arrangement data is deliberately separate from recordings and script revisions.
-- A group is a flat container; members remain ordinary script events.
CREATE TABLE dialogue_groups (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 scene_id uuid NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE dialogue_group_members (
 group_id uuid NOT NULL REFERENCES dialogue_groups(id) ON DELETE CASCADE,
 event_id uuid PRIMARY KEY REFERENCES script_events(id) ON DELETE CASCADE,
 offset_ms integer NOT NULL DEFAULT 0 CHECK (offset_ms BETWEEN -300000 AND 300000),
 PRIMARY KEY(group_id,event_id)
);
-- Element keys are either "line:<uuid>" or "group:<uuid>". Keeping them as
-- keys makes transitions survive a selected-take change and avoids nested groups.
CREATE TABLE dialogue_transitions (
 scene_id uuid NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
 predecessor text NOT NULL CHECK (predecessor ~ '^(line|group):[0-9a-f-]{36}$'),
 successor text NOT NULL CHECK (successor ~ '^(line|group):[0-9a-f-]{36}$'),
 offset_ms integer NOT NULL CHECK (offset_ms BETWEEN -300000 AND 300000),
 PRIMARY KEY(scene_id,predecessor,successor),
 CHECK(predecessor <> successor)
);
CREATE INDEX dialogue_groups_scene_idx ON dialogue_groups(scene_id);
