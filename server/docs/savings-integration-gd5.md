# GĐ5 — Savings integration validation and release gate

## Scope

Backend Savings only. No Kafka, frontend, legacy design JSON, or automatic
settlement/renewal changes. `EventStoreDB` owns every debit/credit; MongoDB
is a derived read model; Redis holds short-lived quotes and optional read cache.

The automated test added in this phase uses **disposable** Testcontainers:

- EventStoreDB: canonical SavingsOpened / SavingsSettled banking stream.
- MongoDB 7 replica set: real migrations, atomic BankProjection, read model,
  ledger, and one-time in-app maturity notices.
- Redis: persisted opening and withdrawal quotes, cached settled reads, and
  degraded Redis GET/SET fallback to Mongo after a simulated outage.
- `httptest` HTTP routes: real JWT and Savings handlers, same app composition
  as `cmd/api`, with no mocked financial repository.

**Limit:** To make assertions deterministic, the automated test explicitly
calls `BankProjection.Apply` on newly persisted records. It exercises the real
projection **implementation** and checkpoint but **not** `cmd/projector`'s
long-running subscription, reconnection, or process bootstrapping. Those
remain manual smoke-test gates (below). The historical matured savings opening
is appended to the test's isolated stream only; production rates and clock
remain unchanged.

## Automated test commands (from `server/`)

```powershell
& "$(go env GOPATH)\bin\gofumpt.exe" -l internal/infrastructure/eventstore/savings_full_flow_e2e_test.go
golangci-lint run
go test -count=1 ./...
go test -tags=e2e -count=1 -v -run '^TestSavingsHTTPProjectionFullFlow$' ./internal/infrastructure/eventstore
go test -tags=e2e -count=1 -v -run '^TestSavingsCommandsExactlyOnceWithEventStoreDB$' ./internal/infrastructure/eventstore
go test -tags=savings_e2e -count=1 -v -run '^TestSavingsProjectionReplayWithMongoValidator$|^TestSavingsReadAndMaturityNoticesWithMongo$' ./internal/infrastructure/mongo
go test -tags=savings_e2e -count=1 -v -run '^TestSavingsReadRedisCacheTTLWithDocker$' ./internal/infrastructure/cache
```

Record PASS/FAIL **from actual execution**; writing this document or applying
the patch is not proof the commands passed. Docker Desktop must be running.
The full-flow test owns unique, disposable databases/streams and does **not**
connect to your running AnomalyBank database. Running it does not require
setting `SAVINGS_EVENT_WRITE_ENABLED` in your shell or deployment.

## Assertions covered by the full-flow test

1. Anonymous read is 401; cross-customer detail/withdrawal is denied.
2. Opening quote is non-financial; HTTP open plus retry records one debit.
3. EventStoreDB journal is projected through BankProjection into Mongo read API,
   with exactly one active contract and correct owner and principal.
4. A historical **test-only** 3-month contract appears as matured; scheduler
   creates exactly one in-app notice and never changes funds.
5. Withdrawal quote, HTTP withdrawal, and idempotent retry produce one credit,
   one settled contract/receipt, and an accurate two-entry ledger.
6. After closing the disposable Redis client, a settled read still succeeds
   from MongoDB (read cache is not an authority for financial decisions).

## Manual smoke test — actual running projector subscription

On **isolated local/test** infrastructure only:

1. Apply latest Mongo migrations, start EventStoreDB, MongoDB replica set and
   Redis; verify all startup connectivity. Deploy/build all Banking Stream
   consumers that will see SavingsOpened/SavingsSettled; keep Kafka outside
   this feature's changes.
2. Start `go run ./cmd/projector` separately from `go run ./cmd/api`.
3. Before enabling Savings writes, confirm `SAVINGS_EVENT_WRITE_ENABLED` is
   **disabled**. Check API GET `/api/savings/packages` and health endpoints.
4. After all consumers are compatible, enable Savings writes **only in this
   disposable local/test environment**, restart API and use an owned funded
   test account to create a quote and open one contract.
5. Confirm the actual running projector advances `banking-projection` in
   MongoDB checkpoints. Query `/api/savings/contracts` and `/history` as owner;
   cross-user access must return 404. Verify one SavingsOpened journal record,
   one debit ledger entry, one feed item, and corresponding balance decrement.
6. Restart projector, confirm resume from its checkpoint and no duplicate
   ledger/feed rows. Optionally stop it briefly, create **one additional test
   contract** with a fresh key, then restart projector; verify read-model catch-up
   without recharging the payment account.
7. For maturity testing, use the automated historical fixture rather than
   changing the deployed server clock or directly modifying production events.
   `GET /api/savings/notifications` is in-app only, not mobile push.
8. Before enabling this feature beyond local/test, audit all Banking Stream
   consumers and implement operational monitoring for projector lag, unexpected
   cache outages, EventStoreDB commit timeout, and undeliverable maturity
   notices. An E2E PASS alone does not authorize rollout.

## Limitations / deferred hardening

- Mongo projection is asynchronous: briefly stale reads are expected. Do not
  infer transaction failure from an absent read model; command idempotency and
  authoritative receipts live in EventStoreDB.
- The maturity worker fails closed unless it can verify each candidate against
  EventStoreDB. Projecting SavingsSettled removes earlier Mongo notices in the
  same transaction; notice queries exclude contracts projected as settled.
- The EventStoreDB guard and the MongoDB write are not an atomic cross-store
  transaction: a concurrent settlement may occur just after verification.
  The projector compensates by removing the notice, but a transient stale
  notice remains possible until projection catches up. Do not describe this
  as strict linearizable notification delivery.
- Full process startup, real subscription/restart and Kafka consumers are **not**
  automatically tested here. No Kafka code is modified in this phase.
- Race detector and load/concurrency soak testing remain separate; the
  repository-level `TestSavingsCommandsExactlyOnceWithEventStoreDB` sends
  competing keys and asserts exactly one persisted winning key and one credit.
- When settlement confirmation cannot be read after bounded retries, the HTTP
  response uses `SETTLEMENT_UNAVAILABLE` (503): the result is unknown,
  never a declaration of rollback. Query `/settlement` or retry with the same key.
