# Savings GĐ4 — Mongo read model, Redis and maturity notifications

- Banking EventStoreDB remains authoritative for balance and contract settlement.
- Query APIs use `savings_contracts` projected by the Banking Projector; the JWT account is resolved to `customer_id` from Mongo `accounts`. The same customer ID is required in every contract query. `404` hides the existence of other customers' contracts.
- Query pagination: `GET /api/savings/contracts?status=all|active|matured|settled&limit=20&cursor=...`. Maximum 50, `opened_at DESC, _id DESC`. The cursor carries the Banking Projection checkpoint revision **and classification timestamp (`asOf`)**. Subsequent pages reuse `asOf` so contracts do not move between active/matured pages due to clock movement. A changed checkpoint returns HTTP 409 and requires restarting pagination. Cursors from before this revision lack `asOf` and must be restarted.
- `matured` is evaluated from `maturity_at <= now` on fresh reads, or from the cursor's fixed classification time on subsequent pages. It does not depend on the scheduler or a mutable `status` projection field. Commands always use EventStoreDB's authoritative status.
- Redis caches only settled list/detail results, keyed by projected checkpoint and customer, for 15 seconds. Time-dependent `active`, `matured`, `all` are intentionally uncached. Redis outages fall back to Mongo, not stale state, and GET/SET failures are logged without exposing Redis keys. The cache does not certify that the projector has caught up with EventStoreDB.
- `GET /api/savings/contracts/{id}/history` reads the two possible savings `ledger_entries` by deterministic transaction IDs after a customer-ownership gate. `GET /api/savings/notifications` lists in-app notices scoped to the customer.
- Startup ensures indexes for contract owner, maturity polling and customer notifications. If indexes are unavailable, read endpoints return 503 and the scheduler is not started; unrelated banking endpoints remain available.
- Every minute (and on initial start) `SavingsMaturityWorker` looks for projected contracts due but not settled. It atomically claims a `maturity_notice_at` marker and upserts a single notice with `_id=savings-maturity:<savingsID>` in a Mongo transaction. Multiple workers may run; duplicate _ids are prevented. Malformed rows and individual notification transaction failures are logged and skipped during a streaming scan (100-document batches); a scan reports aggregated failures without preventing later rows from being considered. Failed transactions do not mark a contract notified. It **never** updates banking balances or auto-settles an account.
- Mongo is eventually consistent with EventStoreDB; a settlement committed to the canonical event stream but not yet projected may briefly appear as active to a scheduler or reader. The notice worker serializes its eligibility with the Mongo projection, not with the EventStoreDB append. Do not interpret a notice as proof a contract can still be withdrawn; the command must enforce its authoritative checks.
- Current Savings `savings_contracts` projection uses string `_id`/`customer_id`, `package_id`, `term_months` and `annual_rate_bps` (see `savings-projection-schema.md`). Historical `earlyWithdrawalRateAtOpening` is returned as `null` because the MVP contract does not store such a snapshot; do not invent an early-withdrawal policy. The two original analysis/design JSON files are unchanged.
- Requires Mongo replica set for scheduler transactions. Redis and Mongo read integrations can be tested with the `savings_e2e` tag using isolated Testcontainers, never production data.

## Verification (run in `server/`)

```powershell
golangci-lint run
go test -count=1 ./...
go test -tags=savings_e2e -count=1 -v -run '^TestSavingsReadAndMaturityNoticesWithMongo$' ./internal/infrastructure/mongo
go test -tags=savings_e2e -count=1 -v -run '^TestSavingsReadRedisCacheTTLWithDocker$' ./internal/infrastructure/cache
```

Backend only. No Kafka, frontend, push messages, automatic renewal or automatic payout.
