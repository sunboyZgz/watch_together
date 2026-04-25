CREATE TABLE media_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (
        length(btrim(slug)) > 0 AND btrim(slug) = lower(btrim(slug))
    ),
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    is_featured BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE media_item_tags (
    media_item_id UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    media_tag_id UUID NOT NULL REFERENCES media_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (media_item_id, media_tag_id)
);

CREATE INDEX idx_media_tags_active_sort
    ON media_tags(is_active, sort_order);

CREATE INDEX idx_media_tags_featured_active_sort
    ON media_tags(is_featured, is_active, sort_order);

CREATE INDEX idx_media_item_tags_media_tag_id
    ON media_item_tags(media_tag_id);
