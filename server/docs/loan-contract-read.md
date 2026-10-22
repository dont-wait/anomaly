# Loan Contract Read API (PR 3)

Two authenticated read-only endpoints expose contracts that have been disbursed:

- `GET /api/loans/contracts?limit=20&cursor=...` — customer-owned contracts, paginated by application creation date (newest first); limit 1–50, default 20.
- `GET /api/loans/contracts/detail/{loanId}` — contract details, including the signed annual rate, outstanding principal, current status and full repayment schedule.

`loanId` is the EventStoreDB loan/application UUID already used by repayment endpoints. It is **not** the MongoDB `ObjectID` of `loans._id`.

Both routes use the authenticated JWT customer identifier. Unlike staff application endpoints, an admin cannot read another customer's contracts through these routes. Unknown/non-owned/not-yet-disbursed contracts return 404. Invalid IDs, limits or cursors return 400. A disbursed application with missing or invalid contract projection returns 503 instead of an incomplete success response.

The list response has `{ "items": [], "nextCursor": null, "eventualConsistency": true }`. The cursor is opaque, scoped to the authenticated customer and valid only while that customer remains the requester. Ordering uses the creation time of the underlying application, not the installment due date.

No new write events or banking transactions are created. The source of truth remains EventStoreDB; the contracts are derived from MongoDB `loan_applications_read` and `loans`. MongoDB queries do not recalculate rates or EMI. An existing `completed` contract stays visible even though all installments have been paid. For the UI, repayment-history endpoints and repayment quotes remain separate and unchanged.

## Tests

- `go test ./... -count=1`
- `go test -tags=loan_contract_e2e ./internal/infrastructure/mongo -run '^TestLoanContractMongoE2E$' -count=1 -v` (requires `LOAN_CONTRACT_E2E_MONGO_URI` pointing to a disposable test MongoDB instance; SKIP does not count as success)
- `go test -tags=e2e ./internal/infrastructure/eventstore -run '^TestPersonalLoanHTTPE2E$' -count=1 -v` (Docker/Testcontainers)

PR 3 is based on `main` commit `85c917a` and is independent of pending PRs for product quotes and contract acceptance. Rebase and re-run tests after those PRs merge, especially around `composition/loan.go` and `loan_handler.go`.
