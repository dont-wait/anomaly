# Loan HTTP E2E

Run from `server/` with Go 1.25+ and a working Docker daemon:

```sh
make test-loan-e2e
```

The test uses the `e2e` build tag and creates isolated Testcontainers for
MongoDB 7.0 (replica set), EventStoreDB 23.10.8 and RustFS (the same image as
Compose). Docker may download images on the first run. No running development
API, database, Redis or SMTP server is required. Containers use dynamic ports
and are removed after the test; development volumes and data are not used.

`TestPersonalLoanHTTPE2E` exercises the production HTTP router, signed JWTs,
loan composition, repositories, migrations and media adapter. Accounts and KYC
are prepared through real account commands; the reviewer role is seeded before
issuing its JWT.

Covered flows:

- Create and retry the same application using `Idempotency-Key`.
- Reject unauthenticated, non-owner and non-admin requests.
- Require documents before submission; upload all seven document types and
  download the stored bytes from RustFS.
- Reject assessment for each incomplete verification flag without advancing
  the loan stream, then accept corrected evidence and assess the same loan.
- Automatically approve, route to manual approval, or automatically reject
  according to verified repayment history.
- Generate a provisional quote and persist the official repayment schedule.
- Retry disbursement through fresh handlers without duplicating the bank credit
  or changing the loan revision/disbursement timestamp.
- Simulate failure writing `LoanDisbursed` after a real bank credit, then finalize
  through the production reconciler and retry safely.
- Replay banking projection with duplicate delivery; verify balances, one ledger
  entry/feed row per loan, transaction history and the loan list through HTTP.
- Delete a registered document from RustFS and verify HTTP 404.

Projection and reconciliation are invoked synchronously for deterministic
assertions. This suite does not start the worker binaries or test their
subscription, scheduling or checkpoint recovery loops. It also does not drive
the mobile UI; the client regression tests cover rendering the new feed kind.

The suite is opt-in: the normal `go test ./...` command does not run it.

## Hurl API lifecycle

The Hurl file is `tests/api/loans/lifecycle.hurl`. It logs in as a verified
customer and admin, creates a loan, uploads all seven documents, downloads and
checks the document hash, submits the application, and starts review. It proves
incomplete evidence can be corrected before assessment, then follows manual
approval and disbursement. It also checks permissions, idempotent creation,
duplicate disbursement, account balance, transaction history and the loan list.
Only the final read requests retry while waiting for projections.

To run against an existing development API, ensure migration 000009, RustFS,
the banking projector and the loan projector are running. Use a verified personal
payment account and a verified admin account with their actual credentials:

```sh
make test-loan-api BASE_URL=http://localhost:8080 \
  LOAN_CUSTOMER_CCCD=079123456789 LOAN_CUSTOMER_PASSWORD='DemoLocal@123' \
  ADMIN_CCCD=001234567890 ADMIN_PASSWORD="$ANOMALY_DEMO_ADMIN_PASSWORD"
```

The customer defaults match the demo seeder. The admin must have been seeded
with `ANOMALY_DEMO_ADMIN_PASSWORD` or provisioned separately; use that password.
The Make target keeps CCCD values as strings so leading zeroes survive Hurl's
variable type inference. Each run creates a new loan and credits 10,000,000 VND
to the supplied customer. Use a development/test environment and avoid concurrent
balance changes for that account during the test.

To validate Hurl using isolated containers and prepared accounts instead:

```sh
make test-loan-hurl-e2e
```

This requires Hurl on PATH in addition to Go and Docker. It runs the Go E2E suite
and then the same Hurl file against its real HTTP server, with asynchronous
banking/loan projections using production adapters. It does not start worker
binaries or verify their checkpoint/restart loops. The original `test-api` target
is unchanged and does not include this separate loan suite.
