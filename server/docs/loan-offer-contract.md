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

## Rollout and compatibility (mandatory, no mixed binaries)

**Do not deploy these writers with a rolling mixed-version API or projector.**
Old API binaries cannot replay the four new offer events and may still write
unconfirmed `LoanDisbursementPrepared`/bank credits. Old projectors can
quarantine a loan and advance a checkpoint past unknown events.

1. Keep `LOAN_OFFER_WRITES_ENABLED` unset/`false` on the new binary (fail closed).
2. Stop **every** old API instance and the old loan-projector instance on every
   host. Drain in-flight disbursements and make sure no old instance can start.
3. Record an operator-verified exclusive UTC cutoff after the old writers have
   stopped. Audit any older credited loans that were not finalized. **Never
   choose a cutoff while old writers can still credit new loans.**
4. Deploy updated API and projector binaries together; verify readiness and
   read-model checkpoint/projection failure status. For previously quarantined
   loans, use the existing `--retry-loan` recovery workflow after an upgrade.
5. Configure `LOAN_OFFER_LEGACY_CREDIT_CUTOFF=YYYY-MM-DDTHH:MM:SSZ` **on the
   reconciler only** for previously credited loans older than that cutoff.
   An unset value disables the legacy exemption and leaves such loans for
   operator investigation, rather than automatically declaring them confirmed.
6. Only after old writers/readers have been drained and the new projector is
   healthy, restart the updated API with `LOAN_OFFER_WRITES_ENABLED=true`.
   The API returns 503 for offer mutation endpoints until this is enabled.

**Compatibility policy:** The ordinary disbursement command now always requires
an Offer with `confirmed` status (including for an old preparation). Reconciliation
can finalize a previously credited unconfirmed loan only when bank credit time
(and preparation time, if any) is strictly before the configured cutoff. Such
credits must be independently audited; this cutoff is a deployment safety control,
not evidence of customer consent. Reconciliation never credits the bank account.
New loan credits must follow Offer → acceptance → OTP confirmation → disbursement.
Already recorded historical `LoanDisbursed` events remain replayable, but replay
of a committed fact must not be used as a way to authorize a new credit.

New readers stop at unknown future loan events rather than checkpointing past them;
**this does not retrofit old running binaries**. Do not roll back to older
readers after any new offer event has been written.

SMTP and Redis must be configured. Missing SMTP/Redis fails closed, not bypassed.
Email OTP is an application-level confirmation for the **research demo**,
not a legally qualified electronic signature. Production deployment requires
separate review of contract artifacts, disclosures, signing, and audit rules.

## Test commands

From `server`:

```
go test ./... -count=1
go test -tags=e2e ./internal/infrastructure/eventstore -run '^TestPersonalLoanHTTPE2E$' -count=1 -v
```

The E2E test uses its own fake OTP service; the production OTP integration uses
Redis and SMTP. Test SMTP error handling and OTP delivery separately before any
production use.
