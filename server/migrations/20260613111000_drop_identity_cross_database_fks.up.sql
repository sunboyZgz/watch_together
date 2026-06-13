ALTER TABLE rooms
    DROP CONSTRAINT IF EXISTS rooms_host_user_id_fkey;

ALTER TABLE room_members
    DROP CONSTRAINT IF EXISTS room_members_user_id_fkey;

ALTER TABLE user_media_progress
    DROP CONSTRAINT IF EXISTS user_media_progress_user_id_fkey;
