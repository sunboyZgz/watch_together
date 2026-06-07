MIGRATIONS_DIR := ./migrations

.PHONY: migration-create migration-up migration-down migration-version proto-lint proto-generate

migration-create:
	@test -n "$(name)" || (echo "usage: make migration-create name=create_users_table" && exit 1)
	@./scripts/new_migration.sh "$(name)"

migration-up:
	@./scripts/migrate.sh up

migration-down:
	@./scripts/migrate.sh down 1

migration-version:
	@./scripts/migrate.sh version

proto-lint:
	@cd .. && buf lint

proto-generate:
	@cd .. && buf generate
