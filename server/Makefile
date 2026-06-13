MIGRATIONS_DIR := ./migrations
IDENTITY_MIGRATIONS_DIR := ./identity_migrations
MEDIA_MIGRATIONS_DIR := ./media_migrations
TIMELINE_MIGRATIONS_DIR := ./timeline_migrations
PROGRESS_MIGRATIONS_DIR := ./progress_migrations
ROOM_MIGRATIONS_DIR := ./room_migrations

.PHONY: migration-create migration-up migration-down migration-version identity-migration-up identity-migration-down identity-migration-version media-migration-up media-migration-down media-migration-version timeline-migration-up timeline-migration-down timeline-migration-version progress-migration-up progress-migration-down progress-migration-version room-migration-up room-migration-down room-migration-version proto-lint proto-generate verify-phase8 verify-phase9 verify-phase10 verify-phase11 verify-phase16 verify-phase23

migration-create:
	@test -n "$(name)" || (echo "usage: make migration-create name=create_users_table" && exit 1)
	@./scripts/new_migration.sh "$(name)"

migration-up:
	@./scripts/migrate.sh up

migration-down:
	@./scripts/migrate.sh down 1

migration-version:
	@./scripts/migrate.sh version

identity-migration-up:
	@test -n "$$IDENTITY_DATABASE_URL" || (echo "IDENTITY_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(IDENTITY_MIGRATIONS_DIR) DATABASE_URL="$$IDENTITY_DATABASE_URL" ./scripts/migrate.sh up

identity-migration-down:
	@test -n "$$IDENTITY_DATABASE_URL" || (echo "IDENTITY_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(IDENTITY_MIGRATIONS_DIR) DATABASE_URL="$$IDENTITY_DATABASE_URL" ./scripts/migrate.sh down 1

identity-migration-version:
	@test -n "$$IDENTITY_DATABASE_URL" || (echo "IDENTITY_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(IDENTITY_MIGRATIONS_DIR) DATABASE_URL="$$IDENTITY_DATABASE_URL" ./scripts/migrate.sh version

media-migration-up:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh up

media-migration-down:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh down 1

media-migration-version:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh version

timeline-migration-up:
	@test -n "$$TIMELINE_DATABASE_URL" || (echo "TIMELINE_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(TIMELINE_MIGRATIONS_DIR) DATABASE_URL="$$TIMELINE_DATABASE_URL" ./scripts/migrate.sh up

timeline-migration-down:
	@test -n "$$TIMELINE_DATABASE_URL" || (echo "TIMELINE_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(TIMELINE_MIGRATIONS_DIR) DATABASE_URL="$$TIMELINE_DATABASE_URL" ./scripts/migrate.sh down 1

timeline-migration-version:
	@test -n "$$TIMELINE_DATABASE_URL" || (echo "TIMELINE_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(TIMELINE_MIGRATIONS_DIR) DATABASE_URL="$$TIMELINE_DATABASE_URL" ./scripts/migrate.sh version

progress-migration-up:
	@test -n "$$PROGRESS_DATABASE_URL" || (echo "PROGRESS_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(PROGRESS_MIGRATIONS_DIR) DATABASE_URL="$$PROGRESS_DATABASE_URL" ./scripts/migrate.sh up

progress-migration-down:
	@test -n "$$PROGRESS_DATABASE_URL" || (echo "PROGRESS_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(PROGRESS_MIGRATIONS_DIR) DATABASE_URL="$$PROGRESS_DATABASE_URL" ./scripts/migrate.sh down 1

progress-migration-version:
	@test -n "$$PROGRESS_DATABASE_URL" || (echo "PROGRESS_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(PROGRESS_MIGRATIONS_DIR) DATABASE_URL="$$PROGRESS_DATABASE_URL" ./scripts/migrate.sh version

room-migration-up:
	@test -n "$$ROOM_DATABASE_URL" || (echo "ROOM_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(ROOM_MIGRATIONS_DIR) DATABASE_URL="$$ROOM_DATABASE_URL" ./scripts/migrate.sh up

room-migration-down:
	@test -n "$$ROOM_DATABASE_URL" || (echo "ROOM_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(ROOM_MIGRATIONS_DIR) DATABASE_URL="$$ROOM_DATABASE_URL" ./scripts/migrate.sh down 1

room-migration-version:
	@test -n "$$ROOM_DATABASE_URL" || (echo "ROOM_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(ROOM_MIGRATIONS_DIR) DATABASE_URL="$$ROOM_DATABASE_URL" ./scripts/migrate.sh version

proto-lint:
	@cd .. && buf lint

proto-generate:
	@cd .. && buf generate

verify-phase8:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase8.ps1

verify-phase9:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase9.ps1

verify-phase10:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase10.ps1

verify-phase11:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase11.ps1

verify-phase16:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase16.ps1

verify-phase23:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase23.ps1
