BASE_URL ?= http://localhost:8080

ifeq ($(OS),Windows_NT)
MIGRATE_CMD := powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/windows/migrate.ps1
else
MIGRATE_CMD := go run ./cmd/migrate
endif

.PHONY: help start-app migrate migrate-up migrate-up-one migrate-version migrate-down test test-api test-account-e2e

start-app:
	docker compose --profile app up --build

help:
	@echo "Usage: make <target> [BASE_URL=http://localhost:8080]"
	@echo ""
	@echo "  help              Show available commands"
	@echo "  start-app         Build and start the Docker Compose app profile"
	@echo "  migrate           Alias for migrate-up"
	@echo "  migrate-up        Apply all pending migrations"
	@echo "  migrate-up-one    Apply the next pending migration"
	@echo "  migrate-version   Show migration version and dirty state"
	@echo "  migrate-down      Roll back the latest migration"
	@echo "  test              Run Go tests"
	@echo "  test-api          Run Hurl API tests against BASE_URL"
	@echo "  test-account-e2e  Run the concurrent registration E2E test"

migrate: migrate-up

migrate-up:
	$(MIGRATE_CMD) up

migrate-up-one:
	$(MIGRATE_CMD) up-by-one

migrate-version:
	$(MIGRATE_CMD) version

migrate-down:
	$(MIGRATE_CMD) down

test:
	go test -count=1 ./...

test-api:
	hurl --test --variable base_url="$(BASE_URL)" --variable cccd="$$(date +%s%N | cut -c1-12)" tests/api/*.hurl

test-account-e2e:
	go test -tags=e2e -count=1 -run '^TestConcurrentRegistrationPersistsOneAccountWithoutEventStore$$' ./cmd/api
