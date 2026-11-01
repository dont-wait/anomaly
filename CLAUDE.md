# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository

Banking app with transfer anomaly detection. Monorepo; the root has no Makefile, so `cd` into an app or use `make -C <app> <target>`. Each app has its own `.env.example` to copy to `.env`.

- `server/`: Go 1.25 backend
- `client/`: Yarn 1 workspace with two Tauri 2 + React 19 apps, `mobile/` (consumer, Android + Linux) and `desktop/` (admin). Vite, Vitest, ESLint, TypeScript, Nix and the lockfile are shared at `client/`.
- `kyc-service/`: Python 3.11 FastAPI face verification and liveness
- `anomaly-service/`: Python 3.11 Kafka worker running the GATv2 model
- `consumer-mock/`: standalone Go consumer that mocks the model service
- `deploy/swarm/`: Docker Swarm ML stack, autoscaler, k6 load test
- `docs/` (`ARCHITECTURE.md`, `MODEL_FEATURES.md`, `INFRA.md`, `FEATURE.md`), `specs/` (internal design notes), `server/README.md` (the main backend reference)

Toolchains come from Nix: `nix develop` in `server/`, `client/` (also `.#android`, `.#windows`) or `kyc-service/`.

## Commands

### Backend (`server/`)

Run from `server/`; the migration runner reads `migrations/` from the working directory.

```bash
docker compose --profile app up --build   # full local stack (or ./dev-up.sh)
make start-api                            # API on :8080
make start-worker                         # banking projector (cmd/projector)
make start-relay                          # EventStoreDB -> Kafka relay
make seed                                 # needs APP_ENV=development, SEED_DEMO_ENABLED=true
make migrate                              # also: migrate-status, migrate-down
make migrate-new NAME=<name>

make test                                 # go test -count=1 ./... (no real databases needed)
go test -count=1 -run '^TestName$' ./internal/path/to/pkg   # single test
make test-account-e2e                     # Testcontainers; e2e targets use -tags=e2e
make test-transaction-e2e
make test-loan-e2e
make test-api BASE_URL=http://localhost:8080   # Hurl, needs a running API and the seeded admin
make test-loan-api                        # Hurl, needs a verified customer and admin

gofumpt -l .                              # CI format gate, must print nothing
golangci-lint run --timeout=5m            # CI pins v1.64.8
```

### Frontend (`client/`)

```bash
corepack enable && yarn install
yarn dev:mobile                 # Vite on :1420 (strictPort)
yarn dev:desktop                # Vite on :1422 (strictPort)
make desktop                    # Tauri Linux desktop via Nix
make android                    # Tauri Android via Nix (ANDROID_DEVICE=SERIAL to select)

yarn lint && yarn typecheck && yarn test    # all workspaces, same as CI
yarn format

# Single test, from client/mobile (from client/desktop with --mode desktop for the admin app)
../node_modules/.bin/vitest run --config ../vitest.config.ts --mode mobile src/path/to/file.test.ts
```

Vitest and Vite require `--mode mobile|desktop`; the mode selects the target root and the `@` -> `<target>/src` alias.

### Python services

```bash
cd kyc-service                  # make start: Nix + venv + uvicorn on :8090
ruff check . && ruff format --check . && pytest      # tests need pip install -e '.[vision]'

cd anomaly-service              # make install, then make start (requires .env)
make check                      # ruff + pytest
pytest -q tests/path/to/test_file.py::test_name      # single test
```

Both use Ruff with line length 100 and rules `E`, `F`, `I`.

## Architecture

### Event sourcing is the write path

EventStoreDB (KurrentDB client) is the source of truth for accounts, KYC, balances and transfers. MongoDB holds rebuildable projections only.

- Commands rebuild the canonical aggregate from the single `banking` stream, validate, then append with an expected revision. That shared revision is the concurrency boundary: it is what prevents double spending, and a conflicting command rebuilds and revalidates before retrying.
- Never decide funds or command state from Mongo. Paginated history and summaries read eventually consistent projections; confirm and detail responses use canonical state, so they do not depend on the projector catching up.
- `TransferCompleted` commits debit and credit in one event. Retry an uncertain append with the same idempotency key so replay resolves whether it committed.
- Projectors write the projection and its checkpoint in one Mongo transaction, which is why Mongo needs replica set `rs0` (Compose provides it).
- Loans differ: each application has its own stream (`loanStreamName` in `internal/infrastructure/eventstore/loan_repository.go`), projected by `cmd/loan-projector`. Loan payments and savings money movements live on the banking stream (`bank_loan_repayment.go`, `bank_savings.go`).

### Processes

- `cmd/api`: HTTP API. Depends on EventStoreDB, MongoDB, Redis (OTP) and RustFS (media).
- `cmd/projector`: banking read models. Compose runs it as a separate service, along with `cmd/loan-projector` and `cmd/relay`.
- `cmd/worker`: legacy account projector, not part of the write path. Do not run it alongside `cmd/projector`.
- `cmd/import-bank`: offline cutover into an empty banking stream, with all writers stopped.

### Risk pipeline

1. `cmd/relay` holds the `kafka-publisher` persistent subscription on `banking`. It enriches `TransferCreated` with `model_features` from an incremental Mongo feature state and publishes to `anomaly.bank.events.v1`, acknowledging only after Kafka confirms.
2. `anomaly-service` treats that Kafka message as a trigger only. It reads the raw event and prior transfer history from EventStoreDB, then runs the local GATv2 bundle.
3. It publishes to `anomaly.bank.risk-decisions.v1`, HMAC-signed with `KAFKA_RISK_DECISION_SIGNING_KEY`, and training records to `anomaly.model.training.v1`.
4. The API verifies the signature and stores assessments in `risk_decisions` for the admin dashboard (served over SSE).

`consumer-mock` and `anomaly-service` consume the same trigger, so stop the mock when testing the real model. Contract details are in `docs/MODEL_FEATURES.md`.

### Go layering (`server/internal/`)

`domain` -> `application` (use cases split into commands and queries) -> `infrastructure` (Mongo, EventStoreDB, Kafka, RustFS, Redis adapters) -> `presentation/http`, wired together in `composition`. Application and HTTP code must not depend on adapter errors: `composition` translates them into application errors (see `savings_command_adapter.go`).

- Register HTTP routes through `openapi.Registry` in `*_routes.go`. It registers the route and generates the OpenAPI document from its metadata, so never hand-edit `openapi.json`.
- Responses: `{status, message, data}` on success, `{status, title, errors}` on error; binary media downloads have no envelope.
- Add seeders in `internal/seeder/` with `init()` registration.
- Migrations are numbered JSON up/down pairs in `migrations/`. The runner stores only a version number, so it cannot detect a changed migration under an already applied number. Do not renumber applied migrations.

### Frontend

Code is organized by feature under `src/features/<feature>`, with shared code in `src/shared` and routing in `src/app`; `mobile` also has route-level components in `src/pages`. The two apps do not share source, so a change needed in both must be made in each. API endpoints come from `VITE_API_ENDPOINT` and `VITE_KYC_ENDPOINT` (HTTPS required outside loopback). The desktop dev origin `http://localhost:1422` must be in the server's `CORS_ALLOWED_ORIGINS`.

## Gotchas

- Model weights, bundles and calibrated thresholds are not in the repo. KYC real mode and anomaly inference need external artifacts.
- `src-tauri/` is ignored by ESLint and the Vite watcher. Android and mobile desktop share port 1420, so stop one before starting the other.
- CI installs only `pytest` and `prometheus-client` for `anomaly-service`, so its CI tests must stay offline.
- CI runs on Node 24 while the Nix shells mostly provide Node 22.
