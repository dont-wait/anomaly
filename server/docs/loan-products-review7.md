# Loan Products / Pre-Application Quote – Sourcery review 7 fixes

- `POST /api/loans` checks the same active, unique package data as pre-application quote **before** appending `LoanApplicationCreated`.
- Retries with the same idempotency key and same request return an already-created application before consulting the current package configuration.
- `loan_packages.package_code` has a unique index, installed at API startup. If an existing database contains duplicate `package_code` values, **API startup fails**; reconcile those documents manually before deploying. Do not silently delete rows or arbitrarily select a winner.
- Status is normalized on read, including `Active`/`ACTIVE`/`active`; new seed/configuration should write `ACTIVE` or `INACTIVE` consistently.
- Legacy already-disbursed contracts with a retired/missing product retain the source package code instead of linking to an unrelated document. The existing contract interest rate is not recalculated.
- Mongo package reads are bounded by a 5-second context deadline; no result is returned as an empty catalog after a timeout.
- The development seeder creates `PERSONAL_UNSECURED_SALARY` when absent (1,000,000–500,000,000 VND; 6, 12, 24, 36, 48, or 60 months). It does not overwrite an existing record or configure a fixed interest rate; rates are calculated from verified evidence and loan terms.
- Run `make seed` from `server/` to run all registered seeders, including the catalog seeder. The authenticated current-admin endpoint `POST /api/admin/loans/catalog/seed` runs the catalog seeder alone. Both require `APP_ENV=development|dev|local` and `SEED_DEMO_ENABLED=true`; Docker Compose passes these variables to `anomaly-server` and defaults to production/false.

Local verification after applying patch: `go test ./... -count=1`, Mongo E2E (`loan_products_e2e`), and HTTP loan E2E (`e2e`). Check for `SKIP` in E2E logs.
