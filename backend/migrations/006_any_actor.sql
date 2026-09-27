-- A present assignment with no actor means an open role; no row is unassigned.
ALTER TABLE assignments ALTER COLUMN actor_id DROP NOT NULL;
CREATE OR REPLACE VIEW active_assignments AS
 SELECT a.* FROM assignments a
 JOIN active_characters c ON c.id=a.character_id
 LEFT JOIN active_actors actor ON actor.id=a.actor_id
 WHERE a.actor_id IS NULL OR actor.id IS NOT NULL;

-- Preferred is a selection for the line, across all performers.
WITH ranked AS (
 SELECT id,row_number() OVER(PARTITION BY event_id ORDER BY created_at DESC,id DESC) AS n
 FROM takes WHERE preferred
)
UPDATE takes SET preferred=false WHERE id IN(SELECT id FROM ranked WHERE n>1);
DROP INDEX takes_preferred;
CREATE UNIQUE INDEX takes_preferred ON takes(event_id) WHERE preferred;
