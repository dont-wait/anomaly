# UC-11 GĐ3 — Final API validation and local smoke-test gate

## Scope

Savings Go backend only, on top of GĐ1 (PR #148) and GĐ2 (PR #149).
No changes to Kafka, Loan, Frontend, source design JSON, interest policy,
or financial settlement/write path. This phase adds regression assertions and
an explicit acceptance checklist; it does not claim a production rollout.

UC-11 supports **full withdrawal at/after maturity**, returning exactly the
principal and maturity interest to the original source account. No OTP,
partial/early withdrawal, renewal, or automatic credit is supported.

## Automated validation (run from `server/`)

Docker Desktop must be running for the tagged E2E suites. Tests use disposable
Testcontainers resources and must not touch local/project banking data.

```powershell
$files = @(git diff --name-only --relative -- '*.go')
& "$(go env GOPATH)\bin\gofumpt.exe" -l $files
golangci-lint run
go test -count=1 ./...
go test -tags=e2e -count=1 -v -run '^(TestSavingsCommandsExactlyOnceWithEventStoreDB|TestSavingsHTTPProjectionFullFlow)$' ./internal/infrastructure/eventstore
go test -tags=savings_e2e -count=1 -v -run '^(TestSavingsProjectionReplayWithMongoValidator|TestSavingsReadAndMaturityNoticesWithMongo)$' ./internal/infrastructure/mongo
go test -tags=savings_e2e -count=1 -v -run '^TestSavingsReadRedisCacheTTLWithDocker$' ./internal/infrastructure/cache
git diff --check
```

Record **actual** PASS/FAIL output before creating the PR. Running the tests
without `-tags` does not execute the Docker E2E suites.

## HTTP/API assertions

The full-flow test exercises the routes through actual JWT middleware.
Missing JWT is rejected by the shared middleware with HTTP 401 and the
`httpx.ErrorResponse` envelope (`errors[0].code=MISSING_AUTH_HEADER`), whereas
Savings business errors use the flat `{ "code": "...", "error": "..." }` shape.

| API | Acceptance expectation |
| --- | --- |
| `POST /api/savings/contracts/{id}/withdrawal-quotes` | `200` full payout and original destination; `401` without JWT; `404` for another owner; `400 UNSUPPORTED_WITHDRAWAL_TYPE` for partial; `422 SAVINGS_NOT_MATURED` before maturity; `409 SAVINGS_ALREADY_SETTLED` after settlement |
| `POST /api/savings/contracts/{id}/withdrawals` | `200` on success and same-key/same-payload retry; `401` without JWT; `404` for another owner/unknown quote; `400 CONFIRMATION_REQUIRED` or `INVALID_IDEMPOTENCY_KEY`; `409 IDEMPOTENCY_CONFLICT` on same key/different quote; `409 SAVINGS_ALREADY_SETTLED` on a new key after settlement |
| `GET /api/savings/contracts/{id}/settlement` | `409 SAVINGS_NOT_SETTLED` before credit; `404` for another owner; `200` with authoritative transaction ID/amount after settlement |
| `GET /api/savings/notifications` | Only owned notifications; retains delivered maturity history; `canWithdraw=false` after projected settlement |

The test also asserts that failed withdrawal requests produce **zero**
`SavingsSettled` events, successful withdrawal plus retries produce exactly
**one** such event, and the balance changes only by the expected payout.

## Manual smoke gate — local/test services only

The full-flow E2E test calls the real projection `Apply` method synchronously;
it does **not** verify live subscription, scheduled process startup, crash
recovery, or live deployment configuration. Those need separate manual checks:

1. In isolated local/test infrastructure, confirm current migrations and
   supported SavingsOpened/SavingsSettled banking consumers. Keep Savings
   writes disabled until all consumers are compatible.
2. Start `cmd/projector`, `cmd/worker`, and `cmd/api` with the test
   EventStoreDB, MongoDB replica set, and Redis. Enable Savings writes **only
   for this isolated test setup** after prerequisite checks.
3. With a test account/JWT, open a Savings contract using the approved term and
   amount. Verify one debit in Banking Stream and the projected balance,
   ledger, feed, and contract. Restart the projector; verify catch-up and no
   duplicate entries.
4. Use the automated historical EventStoreDB fixture to exercise maturity;
   **do not** change a real deployment clock, rewrite historical production
   events, or bypass the domain policy to force maturity.
5. Verify the scheduler sends at most one in-app maturity notice; it never
   credits funds automatically. Check an owned matured contract can be quoted
   and withdrawn even if the Mongo status projection is temporarily behind.
6. Confirm withdrawal returns money **only** to the original source account;
   repeat with the same idempotency key and confirm no second credit.
   Check the authoritative `/settlement` receipt and the 2-entry ledger.
7. Restart the projector/worker. Verify no duplicate credits/notifications,
   notification history remains visible, and the settled notice has
   `canWithdraw=false`. Check cross-owner access is denied.
8. If EventStoreDB reports an ambiguous commit, treat `503 SETTLEMENT_UNAVAILABLE`
   as **unknown**, not rollback. Query `/settlement` or retry with the same
   key; never blindly send a new key. A closed/locked destination account must
   not be credited or silently redirected.

## Release gate and known limitations

- Do not merge until tagged Docker E2E tests, Go unit tests, lint, and PR
  review/CI pass. A completed PR is **not** an authorization to deploy.
- Monitor banking stream commit timeouts, unresolved settlement confirmation,
  projection lag, Redis outages, and failed maturity notices before rollout.
- `canWithdraw` comes from the Mongo read projection and may briefly be stale;
  **only EventStoreDB may authorize a withdrawal**.
- A Mongo read cache miss or lag must never cause a duplicate financial append.
- Quote withdrawal TTL currently follows code behavior (five minutes); the
  UC-11 v0.2 spec treats this as a provisional implementation choice, not a
  final business-policy decision.
- Real process startup, restart/resume, infrastructure fault injection,
  performance/load testing, and Kafka consumers are **not** covered by this
  patch's automated tests. Kafka remains owned by another workstream.
