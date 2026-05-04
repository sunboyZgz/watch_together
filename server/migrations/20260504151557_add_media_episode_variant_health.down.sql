-- Write rollback migration here.
ALTER TABLE media_episode_variants
    DROP COLUMN IF EXISTS average_segment_ms,
    DROP COLUMN IF EXISTS segment_count;
