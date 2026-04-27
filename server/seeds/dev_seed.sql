-- Development seed data for local API testing.
-- Password for both users is "secret" when testing /auth/login.

INSERT INTO users (
    id,
    account,
    password_hash,
    nickname,
    avatar_seed,
    avatar_url,
    bio
) VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        'xingye',
        '$2a$10$o0dEjgsq4M0iEgmcacydve.SdFAEkAANnbZ3guVqB0fvFMttRs3He',
        'Xingye',
        'xingye',
        NULL,
        '今晚也想和朋友一起看点什么。'
    ),
    (
        '00000000-0000-0000-0000-000000000002',
        'tazi',
        '$2a$10$o0dEjgsq4M0iEgmcacydve.SdFAEkAANnbZ3guVqB0fvFMttRs3He',
        '搭子',
        'tazi',
        NULL,
        '同步观影测试用户。'
    )
ON CONFLICT (id) DO UPDATE SET
    account = EXCLUDED.account,
    password_hash = EXCLUDED.password_hash,
    nickname = EXCLUDED.nickname,
    avatar_seed = EXCLUDED.avatar_seed,
    avatar_url = EXCLUDED.avatar_url,
    bio = EXCLUDED.bio,
    updated_at = NOW();

INSERT INTO media_items (
    id,
    title,
    subtitle,
    description,
    cover_url,
    media_url,
    category,
    tags,
    duration_ms,
    status,
    original_title,
    production_team,
    search_aliases,
    season_label,
    episode_label
) VALUES
    (
        '10000000-0000-0000-0000-000000000001',
        '紫罗兰永恒花园',
        '和搭子一起继续看到第 09 集',
        '治愈系剧场感作品，适合夜晚慢慢看。',
        'https://example.com/covers/violet-evergarden.jpg',
        'https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8',
        'anime',
        '["治愈", "剧场版"]'::jsonb,
        1458000,
        'active',
        'Violet Evergarden',
        'Kyoto Animation',
        '["紫罗兰", "薇尔莉特", "京阿尼"]'::jsonb,
        '第 1 季',
        '第 09 集'
    ),
    (
        '10000000-0000-0000-0000-000000000002',
        '孤独摇滚!',
        '上次看到第 06 集',
        '轻松日常，气氛感很好。',
        'https://example.com/covers/bocchi.jpg',
        'https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8',
        'anime',
        '["搞笑", "群像"]'::jsonb,
        1440000,
        'active',
        'Bocchi the Rock!',
        'CloverWorks',
        '["孤独摇滚", "波奇", "乐队"]'::jsonb,
        '第 1 季',
        '第 06 集'
    ),
    (
        '10000000-0000-0000-0000-000000000003',
        '葬送的芙莉莲',
        '治愈冒险，适合慢慢看',
        '旅途、回忆和很温柔的冒险。',
        'https://example.com/covers/frieren.jpg',
        'https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8',
        'anime',
        '["治愈", "奇幻"]'::jsonb,
        1500000,
        'active',
        'Frieren: Beyond Journey''s End',
        'Madhouse',
        '["芙莉莲", "葬送", "魔法"]'::jsonb,
        '第 1 季',
        '第 03 集'
    )
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    subtitle = EXCLUDED.subtitle,
    description = EXCLUDED.description,
    cover_url = EXCLUDED.cover_url,
    media_url = EXCLUDED.media_url,
    category = EXCLUDED.category,
    tags = EXCLUDED.tags,
    duration_ms = EXCLUDED.duration_ms,
    status = EXCLUDED.status,
    original_title = EXCLUDED.original_title,
    production_team = EXCLUDED.production_team,
    search_aliases = EXCLUDED.search_aliases,
    season_label = EXCLUDED.season_label,
    episode_label = EXCLUDED.episode_label,
    updated_at = NOW();

INSERT INTO media_tags (
    id,
    slug,
    name,
    sort_order,
    is_featured,
    is_active
) VALUES
    ('20000000-0000-0000-0000-000000000001', 'all', '全部', 0, true, true),
    ('20000000-0000-0000-0000-000000000002', 'hot-blooded', '热血', 10, true, true),
    ('20000000-0000-0000-0000-000000000003', 'healing', '治愈', 20, true, true),
    ('20000000-0000-0000-0000-000000000004', 'campus', '校园', 30, true, true),
    ('20000000-0000-0000-0000-000000000005', 'theatrical', '剧场版', 40, true, true),
    ('20000000-0000-0000-0000-000000000006', 'romance', '恋爱', 50, false, true),
    ('20000000-0000-0000-0000-000000000007', 'fantasy', '奇幻', 60, false, true),
    ('20000000-0000-0000-0000-000000000008', 'sci-fi', '科幻', 70, false, true),
    ('20000000-0000-0000-0000-000000000009', 'comedy', '搞笑', 80, false, true),
    ('20000000-0000-0000-0000-000000000010', 'slice-of-life', '日常', 90, false, true)
ON CONFLICT (id) DO UPDATE SET
    slug = EXCLUDED.slug,
    name = EXCLUDED.name,
    sort_order = EXCLUDED.sort_order,
    is_featured = EXCLUDED.is_featured,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

INSERT INTO media_item_tags (media_item_id, media_tag_id) VALUES
    ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003'),
    ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000005'),
    ('10000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000009'),
    ('10000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000010'),
    ('10000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000003'),
    ('10000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000007')
ON CONFLICT (media_item_id, media_tag_id) DO NOTHING;

INSERT INTO user_media_progress (
    user_id,
    media_item_id,
    last_position_seconds,
    duration_seconds,
    last_watched_at,
    completed,
    completion_source
) VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000001',
        564,
        1458,
        NOW() - INTERVAL '1 hour',
        false,
        NULL
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000002',
        360,
        1440,
        NOW() - INTERVAL '1 day',
        false,
        NULL
    )
ON CONFLICT (user_id, media_item_id) DO UPDATE SET
    last_position_seconds = EXCLUDED.last_position_seconds,
    duration_seconds = EXCLUDED.duration_seconds,
    last_watched_at = EXCLUDED.last_watched_at,
    completed = EXCLUDED.completed,
    completion_source = EXCLUDED.completion_source,
    updated_at = NOW();
