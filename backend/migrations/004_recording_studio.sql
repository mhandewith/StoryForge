CREATE TABLE actor_logins (
 actor_id uuid PRIMARY KEY REFERENCES actors(id),
 email text UNIQUE NOT NULL CHECK (length(email) BETWEEN 3 AND 254)
);
CREATE TABLE audio_assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 storage_key text UNIQUE NOT NULL,
 sha256 text NOT NULL,
 mime_type text NOT NULL,
 size_bytes bigint NOT NULL CHECK (size_bytes > 0),
 duration_ms integer NOT NULL CHECK (duration_ms BETWEEN 1 AND 300000),
 sample_rate integer NOT NULL,
 channels integer NOT NULL,
 asset_class text NOT NULL DEFAULT 'SOURCE' CHECK (asset_class IN ('SOURCE','DERIVED','CACHE')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE takes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL,
 revision integer NOT NULL,
 actor_id uuid NOT NULL REFERENCES actors(id),
 asset_id uuid NOT NULL UNIQUE REFERENCES audio_assets(id),
 take_number integer NOT NULL CHECK (take_number > 0),
 preferred boolean NOT NULL DEFAULT false,
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,revision) REFERENCES script_event_revisions(event_id,revision),
 UNIQUE(event_id,actor_id,take_number),
 UNIQUE(actor_id,request_id)
);
CREATE UNIQUE INDEX takes_preferred ON takes(event_id,actor_id) WHERE preferred;
CREATE INDEX takes_actor ON takes(actor_id);
