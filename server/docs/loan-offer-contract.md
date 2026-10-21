# Loan Offer / Contract Confirmation (PR 2)

## Scope

A personal salary loan that has passed review (`approved`) must have an immutable
loan offer before it can be newly disbursed. The existing bank credit and repayment
schedule mechanisms remain unchanged. This implementation does **not** provide a
qualified electronic signature or a downloadable legally binding contract.

## Workflow

1. Staff issues the offer: `POST /api/loans/{applicationId}/offer/issue`.
   The server recomputes the rate from verified evidence, cross-checks the
   previously recorded assessment, and persists `LoanOfferIssued`.
   **This is an explicit staff action, not automatic upon approval.**
2. Owner views the offer: `GET /api/loans/{applicationId}/offer`.
3. Owner accepts or rejects: `POST .../offer/accept` or `POST .../offer/reject`.
4. After acceptance, owner requests email OTP: `POST .../contract/otp`.
   OTP is sent to the verified account email, stored in Redis with isolated
   `loan-contract:*` keys and 5-minute validity.
5. Owner confirms with `POST .../contract/confirm`:
   `{"otp":"123456","acceptTerms":true}`. A successful OTP verification
   precedes writing `LoanContractConfirmed` to EventStoreDB. Rejected or
   unaccepted offers cannot be confirmed.
6. Staff executes the existing `POST .../disburse` route. The system refuses
   newly approved loans without a confirmed offer. The confirmed interest rate
   is frozen at offer time and is reused to prepare/disburse.

`LoanOfferIssued`, `LoanOfferAccepted`, `LoanOfferRejected`, and
`LoanContractConfirmed` are recorded in the loan EventStoreDB stream and
replayed into the `Application.offer` projection. A rejected offer is terminal
in this MVP: a new application is needed to start a new offer.

## Rollout and compatibility

Existing completed loans do not need an Offer. Already prepared old loans
(`LoanDisbursementPrepared` exists) may still finalize a previously credited
bank transaction via reconciliation; new disbursements without preparation
require the confirmed offer. Never replay or force-write a confirmation.

SMTP must be configured. Missing SMTP/Redis fails closed, not bypassed.
The email OTP is an application-level confirmation for the **research demo**,
not a legally qualified electronic signature. Confirm any production signing,
contract artifact, legal disclosures, and audit requirements separately.

## Test commands

From `server`:

```
go test ./... -count=1
go test -tags=e2e ./internal/infrastructure/eventstore -run '^TestPersonalLoanHTTPE2E$' -count=1 -v
```

The E2E test uses its own fake OTP service; the production OTP integration uses
Redis and SMTP. Test SMTP error handling and OTP delivery separately before any
production use.
