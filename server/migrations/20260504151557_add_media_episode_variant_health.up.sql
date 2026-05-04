-- Write forward migration here.
ALTER TABLE media_episode_variants
    ADD COLUMN segment_count INTEGER CHECK (segment_count IS NULL OR segment_count > 0),
    ADD COLUMN average_segment_ms INTEGER CHECK (average_segment_ms IS NULL OR average_segment_ms > 0);
