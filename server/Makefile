BASE_URL ?= http://localhost:8080

ifeq ($(OS),Windows_NT)
MIGRATE_CMD := powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/windows/migrate.ps1
else
MIGRATE_CMD := go run ./cmd/migrate
endif

.PHONY: start-app migrate migrate-up migrate-up-one migrate-version migrate-down test test-api test-account-e2e

start-app:
	docker compose --profile app up --build

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
