MIGRATIONS_DIR := ./migrations
MEDIA_MIGRATIONS_DIR := ./media_migrations

.PHONY: migration-create migration-up migration-down migration-version media-migration-up media-migration-down media-migration-version proto-lint proto-generate verify-phase8 verify-phase9

migration-create:
	@test -n "$(name)" || (echo "usage: make migration-create name=create_users_table" && exit 1)
	@./scripts/new_migration.sh "$(name)"

migration-up:
	@./scripts/migrate.sh up

migration-down:
	@./scripts/migrate.sh down 1

migration-version:
	@./scripts/migrate.sh version

media-migration-up:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh up

media-migration-down:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh down 1

media-migration-version:
	@test -n "$$MEDIA_DATABASE_URL" || (echo "MEDIA_DATABASE_URL is required" && exit 1)
	@MIGRATIONS_DIR=$(MEDIA_MIGRATIONS_DIR) DATABASE_URL="$$MEDIA_DATABASE_URL" ./scripts/migrate.sh version

proto-lint:
	@cd .. && buf lint

proto-generate:
	@cd .. && buf generate

verify-phase8:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase8.ps1

verify-phase9:
	@powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify_phase9.ps1
