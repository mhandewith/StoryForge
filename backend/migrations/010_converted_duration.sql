-- Old conversions retain their existing raw-duration fallback until regenerated.
ALTER TABLE voice_jobs ADD COLUMN duration_ms integer CHECK (duration_ms > 0);
