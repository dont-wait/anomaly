BASE_URL ?= http://localhost:8080

.PHONY: start-app test test-api

start-app:
	docker compose --profile app up --build

test:
	go test -count=1 ./...

test-api:
	hurl --test --variable base_url="$(BASE_URL)" tests/api/*.hurl
