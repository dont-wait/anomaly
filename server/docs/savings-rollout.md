# Savings shared-bank-stream rollout (mandatory)

`SavingsOpened` and `SavingsSettled` are new event types in the canonical shared banking stream. An API/projector binary that predates them **cannot decode these records** and may fail all subsequent banking reads or projection checkpoints. Updating just one replica is not sufficient.

1. Deploy a version with updated `DecodeRecord`, `bankState.apply`, and `BankProjection` support to **every** API, bank projector, worker or other consumer of `BankStream`. Keep `SAVINGS_EVENT_WRITE_ENABLED` unset/false during this deployment. Verify all processes are upgraded, healthy and have advanced their projections to the stream head.
2. Confirm Mongo's currently deployed `account_transaction_feed` validator accepts the generic `savings` feed type, and that ledger/checkpoint projection works on a staging stream. Do not alter the validator to accept unreviewed new types.
3. Explicitly set `SAVINGS_EVENT_WRITE_ENABLED=true` only after step 1 and 2, restarting any writer instances as needed. The default fails closed: new savings events cannot be appended until the flag is set. Disable writes again **before** rolling back consumers.
4. Avoid deploying older binaries while Savings writes are enabled. Rolling back code after new financial events have already been persisted does not undo those events.

The feature flag is a deployment interlock, not proof that every external consumer has upgraded. Confirming compatibility remains an operator responsibility. Full concurrent-request and Mongo-validator E2E testing is required before production use.

## Replay-performance boundary

`BankRepository.load` now keeps a process-local, revisioned snapshot and reads only new events from the EventStoreDB leader between requests. This is a warm-instance optimization **not a durable snapshot**: restarting an API instance still replays the entire banking stream once. Large production streams will require persisted, verified snapshots (or a different authoritative write-state partition) and load testing before launch. Never rely on an eventually consistent Mongo balance for the final debit/credit decision.

## Mandatory integration verification

Before enabling writers beyond a staging environment, verify on the deployed Mongo validator that both Savings movements write feed type `savings`, projection checkpoints advance, reopening/rerunning projectors is idempotent, and two parallel withdrawal requests cannot produce more than one credit. `go test ./...` alone does not prove the external integration paths.

## Catalog failure isolation

If the Savings catalog cannot be seeded/validated during API startup, the
Savings handler is disabled (503) while account/loan/transfer routes remain
available. Fix the Mongo catalog and restart the API; do **not** treat this as
permission to enable `SAVINGS_EVENT_WRITE_ENABLED`. When Savings is disabled,
both Quote and Command are disabled on that API instance.

## Extra GĐ2/GĐ3 review verification

- HTTP regression tests cover one response per `fail()` invocation, correct
  409/503 mapping, and diagnostic logs for unexpected failures.
- `go test -tags=e2e -run TestSavingsCommandsExactlyOnceWithEventStoreDB ./internal/infrastructure/eventstore -count=1` runs against a disposable EventStoreDB via Testcontainers (requires Docker).
- `go test -tags=savings_e2e -run TestSavingsProjectionReplayWithMongoValidator ./internal/infrastructure/mongo -count=1` starts a disposable MongoDB replica set via Testcontainers and checks strict validators, stable balances and replay.
- The Mongo E2E test verifies the **projection**; the EventStore E2E test verifies the **canonical Banking Stream**. A fully HTTP-to-Redis-to-EventStore-to-Mongo E2E is still a separate release gate, not implicitly covered by either test.
- Go Race Detector requires CGO and a supported C compiler. `go test -race` failing before test execution is an **incomplete check**, never a PASS.
