ALTER TABLE characters ADD COLUMN target_voice text NOT NULL DEFAULT ''
    CHECK (length(target_voice) <= 120);

-- Views expand SELECT * when created; expose the new field to workspace reads.
CREATE OR REPLACE VIEW active_characters AS
SELECT c.* FROM characters c JOIN active_projects p ON p.id=c.project_id
WHERE c.deleted_at IS NULL;
