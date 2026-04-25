ALTER TABLE media_items
    ADD COLUMN original_title TEXT,
    ADD COLUMN production_team TEXT,
    ADD COLUMN search_aliases JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE media_items
    ADD CONSTRAINT media_items_original_title_not_blank CHECK (
        original_title IS NULL OR length(btrim(original_title)) > 0
    ),
    ADD CONSTRAINT media_items_production_team_not_blank CHECK (
        production_team IS NULL OR length(btrim(production_team)) > 0
    ),
    ADD CONSTRAINT media_items_search_aliases_is_array CHECK (
        jsonb_typeof(search_aliases) = 'array'
    );

CREATE INDEX idx_media_items_original_title ON media_items(original_title);
CREATE INDEX idx_media_items_production_team ON media_items(production_team);
CREATE INDEX idx_media_items_search_aliases_gin ON media_items USING GIN(search_aliases);
