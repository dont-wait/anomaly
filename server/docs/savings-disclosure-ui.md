# Savings UI disclosure (UC-10) — integration note

The existing Savings opening command is unchanged: `POST /api/savings/contracts`
requires a valid JWT, `confirm: true`, a still-valid quote and a UUID-v4
`Idempotency-Key`. The financial event, ledger and projection shapes are **not**
modified by this UI enhancement.

## New read endpoint

`GET /api/savings/terms` (JWT required) returns a versioned, informational
description of the current savings product rules. The response `version` and text are read from `savings_terms` in MongoDB.
Provision the collection with migration 16 or the local Compass bootstrap script
(see `server/docs/savings-terms-mongodb.md`). There is **no** hardcoded Go
fallback. If the collection has no valid active document the endpoint returns
`503 SAVINGS_TERMS_UNAVAILABLE`. Amount, rate, maturity and contract identity
must continue to come from the server quote/open responses.

## UI behaviour

- After requesting an opening preview, the authenticated mobile client loads
  the terms before obtaining its live quote, avoiding extra time within the
  5-minute quote TTL.
- The client shows a registration summary and a dialog with the terms. The
  customer must explicitly mark the terms reviewed, then check consent before
  the existing opening call is enabled.
- After opening, contract detail reads the existing backend fields and offers
  `GET /api/savings/contracts/{id}/history` on demand, showing debit and
  settlement entries (if any).
- The development-only mock preview route remains unchanged.

## Important scope boundary

The existing `confirm` and recorded `SavingsOpened` event establish that a
user submitted and completed an opening command. **This patch does not store
an immutable, versioned legal consent artifact** in EventStoreDB, does not add
an OTP, and does not generate a PDF. If the group requires legally auditable
terms acceptance, define a reviewed terms-document/version/signature contract,
then design the compatible event migration and acceptance rules separately.

## Validate

From repository root, with a clean branch and patch applied:

```powershell
cd server
go test -count=1 ./internal/presentation/http/handler/savings ./internal/application/savings/commands
cd ..\client
$env:PATH = "$((Get-Location).Path)\node_modules\.bin;$env:PATH"
yarn workspace anomaly-mobile typecheck
yarn workspace anomaly-mobile lint
yarn workspace anomaly-mobile test
```

After starting the local test stack and authenticating as a customer, use the
real Savings tab (not `#/preview/savings`) to review the new dialog, confirm an
opening **only on isolated demo data**, and inspect contract history. Check
Swagger `GET /api/savings/terms` with and without JWT (200 and 401).
