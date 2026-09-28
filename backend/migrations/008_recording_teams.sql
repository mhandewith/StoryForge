CREATE TABLE recording_teams (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 120),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE recording_team_members (
 team_id uuid NOT NULL REFERENCES recording_teams(id) ON DELETE CASCADE,
 actor_id uuid NOT NULL REFERENCES actors(id),
 PRIMARY KEY(team_id,actor_id)
);
