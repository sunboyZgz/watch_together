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
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    sort_order = EXCLUDED.sort_order,
    is_featured = EXCLUDED.is_featured,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

INSERT INTO media_seasons (
    id,
    slug,
    title,
    original_title,
    description,
    cover_url,
    category,
    production_team,
    search_aliases,
    season_number,
    season_label,
    sort_order,
    status
) VALUES
    (
        '30000000-0000-0000-0000-000000000001',
        'violet-evergarden-season-01',
        '紫罗兰永恒花园',
        'Violet Evergarden',
        '治愈系剧场感作品，适合夜晚慢慢看。',
        'https://example.com/covers/violet-evergarden.jpg',
        'anime',
        'Kyoto Animation',
        '["紫罗兰", "薇尔莉特", "京阿尼"]'::jsonb,
        1,
        '第 1 季',
        10,
        'active'
    ),
    (
        '30000000-0000-0000-0000-000000000002',
        'bocchi-the-rock-season-01',
        '孤独摇滚!',
        'Bocchi the Rock!',
        '轻松日常，气氛感很好。',
        'https://example.com/covers/bocchi.jpg',
        'anime',
        'CloverWorks',
        '["孤独摇滚", "波奇", "乐队"]'::jsonb,
        1,
        '第 1 季',
        20,
        'active'
    ),
    (
        '30000000-0000-0000-0000-000000000003',
        'frieren-season-01',
        '葬送的芙莉莲',
        'Frieren: Beyond Journey''s End',
        '旅途、回忆和很温柔的冒险。',
        'https://example.com/covers/frieren.jpg',
        'anime',
        'Madhouse',
        '["芙莉莲", "葬送", "魔法"]'::jsonb,
        1,
        '第 1 季',
        30,
        'active'
    )
ON CONFLICT (id) DO UPDATE SET
    slug = EXCLUDED.slug,
    title = EXCLUDED.title,
    original_title = EXCLUDED.original_title,
    description = EXCLUDED.description,
    cover_url = EXCLUDED.cover_url,
    category = EXCLUDED.category,
    production_team = EXCLUDED.production_team,
    search_aliases = EXCLUDED.search_aliases,
    season_number = EXCLUDED.season_number,
    season_label = EXCLUDED.season_label,
    sort_order = EXCLUDED.sort_order,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO media_episodes (
    id,
    season_id,
    title,
    subtitle,
    description,
    cover_url,
    media_url,
    duration_ms,
    episode_number,
    episode_label,
    source_key,
    source_hash,
    sort_order,
    status
) VALUES
    (
        '40000000-0000-0000-0000-000000000001',
        '30000000-0000-0000-0000-000000000001',
        '紫罗兰永恒花园',
        '和搭子一起继续看到第 09 集',
        '治愈系剧场感作品，适合夜晚慢慢看。',
        'https://example.com/covers/violet-evergarden.jpg',
        'http://10.0.2.2:9000/media/tmp/media/sample-show/season-01/episode-01-720p/hls/index.m3u8',
        1458000,
        9,
        '第 09 集',
        'dev/violet-evergarden/season-01/episode-09.mp4',
        NULL,
        90,
        'active'
    ),
    (
        '40000000-0000-0000-0000-000000000002',
        '30000000-0000-0000-0000-000000000002',
        '孤独摇滚!',
        '上次看到第 06 集',
        '轻松日常，气氛感很好。',
        'https://example.com/covers/bocchi.jpg',
        'https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8',
        1440000,
        6,
        '第 06 集',
        'dev/bocchi-the-rock/season-01/episode-06.mp4',
        NULL,
        60,
        'active'
    ),
    (
        '40000000-0000-0000-0000-000000000003',
        '30000000-0000-0000-0000-000000000003',
        '葬送的芙莉莲',
        '治愈冒险，适合慢慢看',
        '旅途、回忆和很温柔的冒险。',
        'https://example.com/covers/frieren.jpg',
        'https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8',
        1500000,
        3,
        '第 03 集',
        'dev/frieren/season-01/episode-03.mp4',
        NULL,
        30,
        'active'
    )
ON CONFLICT (id) DO UPDATE SET
    season_id = EXCLUDED.season_id,
    title = EXCLUDED.title,
    subtitle = EXCLUDED.subtitle,
    description = EXCLUDED.description,
    cover_url = EXCLUDED.cover_url,
    media_url = EXCLUDED.media_url,
    duration_ms = EXCLUDED.duration_ms,
    episode_number = EXCLUDED.episode_number,
    episode_label = EXCLUDED.episode_label,
    source_key = EXCLUDED.source_key,
    source_hash = EXCLUDED.source_hash,
    sort_order = EXCLUDED.sort_order,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO media_season_tags (season_id, media_tag_id)
SELECT season_id, tag.id
FROM (
    VALUES
        ('30000000-0000-0000-0000-000000000001'::uuid, 'healing'),
        ('30000000-0000-0000-0000-000000000001'::uuid, 'theatrical'),
        ('30000000-0000-0000-0000-000000000002'::uuid, 'comedy'),
        ('30000000-0000-0000-0000-000000000002'::uuid, 'slice-of-life'),
        ('30000000-0000-0000-0000-000000000003'::uuid, 'healing'),
        ('30000000-0000-0000-0000-000000000003'::uuid, 'fantasy')
) AS seed_tags(season_id, tag_slug)
INNER JOIN media_tags AS tag ON tag.slug = seed_tags.tag_slug
ON CONFLICT (season_id, media_tag_id) DO NOTHING;

INSERT INTO user_media_progress (
    id,
    user_id,
    media_episode_id,
    last_position_seconds,
    duration_seconds,
    last_watched_at,
    completed,
    completion_source
) VALUES
    (
        '50000000-0000-0000-0000-000000000001',
        '00000000-0000-0000-0000-000000000001',
        '40000000-0000-0000-0000-000000000001',
        564,
        1458,
        NOW() - INTERVAL '1 hour',
        false,
        NULL
    ),
    (
        '50000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '40000000-0000-0000-0000-000000000002',
        360,
        1440,
        NOW() - INTERVAL '1 day',
        false,
        NULL
    )
ON CONFLICT (user_id, media_episode_id) DO UPDATE SET
    id = EXCLUDED.id,
    user_id = EXCLUDED.user_id,
    media_episode_id = EXCLUDED.media_episode_id,
    last_position_seconds = EXCLUDED.last_position_seconds,
    duration_seconds = EXCLUDED.duration_seconds,
    last_watched_at = EXCLUDED.last_watched_at,
    completed = EXCLUDED.completed,
    completion_source = EXCLUDED.completion_source,
    updated_at = NOW();
