DO $$
BEGIN
    IF to_regclass('public.users') IS NOT NULL THEN
        IF to_regclass('public.rooms') IS NOT NULL
            AND NOT EXISTS (
                SELECT 1
                FROM rooms
                LEFT JOIN users ON users.id = rooms.host_user_id
                WHERE users.id IS NULL
            )
            AND NOT EXISTS (
                SELECT 1 FROM pg_constraint WHERE conname = 'rooms_host_user_id_fkey'
            ) THEN
            ALTER TABLE rooms
                ADD CONSTRAINT rooms_host_user_id_fkey
                FOREIGN KEY (host_user_id) REFERENCES users(id);
        END IF;

        IF to_regclass('public.room_members') IS NOT NULL
            AND NOT EXISTS (
                SELECT 1
                FROM room_members
                LEFT JOIN users ON users.id = room_members.user_id
                WHERE users.id IS NULL
            )
            AND NOT EXISTS (
                SELECT 1 FROM pg_constraint WHERE conname = 'room_members_user_id_fkey'
            ) THEN
            ALTER TABLE room_members
                ADD CONSTRAINT room_members_user_id_fkey
                FOREIGN KEY (user_id) REFERENCES users(id);
        END IF;

        IF to_regclass('public.user_media_progress') IS NOT NULL
            AND NOT EXISTS (
                SELECT 1
                FROM user_media_progress
                LEFT JOIN users ON users.id = user_media_progress.user_id
                WHERE users.id IS NULL
            )
            AND NOT EXISTS (
                SELECT 1 FROM pg_constraint WHERE conname = 'user_media_progress_user_id_fkey'
            ) THEN
            ALTER TABLE user_media_progress
                ADD CONSTRAINT user_media_progress_user_id_fkey
                FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
        END IF;
    END IF;
END $$;
