DROP INDEX IF EXISTS uniq_room_members_active_user_per_room;
DROP INDEX IF EXISTS idx_room_members_active_room_id;
DROP INDEX IF EXISTS idx_room_members_user_id;
DROP INDEX IF EXISTS idx_room_members_room_id;

DROP INDEX IF EXISTS idx_rooms_destroy_after;
DROP INDEX IF EXISTS idx_rooms_status;
DROP INDEX IF EXISTS idx_rooms_media_episode_id;
DROP INDEX IF EXISTS idx_rooms_host_user_id;

DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;
