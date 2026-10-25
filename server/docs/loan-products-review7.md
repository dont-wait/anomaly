# Loan Products / Pre-Application Quote – Sourcery review 7 fixes

- `POST /api/loans` checks the same active, unique package data as pre-application quote **before** appending `LoanApplicationCreated`.
- Retries with the same idempotency key and same request return an already-created application before consulting the current package configuration.
- `loan_packages.package_code` has a unique index, installed at API startup. If an existing database contains duplicate `package_code` values, **API startup fails**; reconcile those documents manually before deploying. Do not silently delete rows or arbitrarily select a winner.
- Status is normalized on read, including `Active`/`ACTIVE`/`active`; new seed/configuration should write `ACTIVE` or `INACTIVE` consistently.
- Legacy already-disbursed contracts with a retired/missing product retain the source package code instead of linking to an unrelated document. The existing contract interest rate is not recalculated.
- Mongo package reads are bounded by a 5-second context deadline; no result is returned as an empty catalog after a timeout.
- These changes do **not** create or seed `PERSONAL_UNSECURED_SALARY`. `loan_packages` must contain a real, unique active configuration before customers can create new loans or quotes.

Local verification after applying patch: `go test ./... -count=1`, Mongo E2E (`loan_products_e2e`), and HTTP loan E2E (`e2e`). Check for `SKIP` in E2E logs.
