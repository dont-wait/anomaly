# PR #116 review resolution checklist

| Review | Fix/verification |
|---|---|
| RV1 + RV6 | Guard writes with `LOAN_OFFER_WRITES_ENABLED` (off by default); stop old writers/readers before enabling. Future unknown events fail closed without quarantine/checkpoint. Run the manual retry procedure for preexisting quarantines. |
| RV2 | No new banking disbursement from an unconfirmed offer, even with `DisbursementPreparedAt`; historical prepared credits use the reconciler only. |
| RV3 | Domain blocks new disbursement without Offer; reconciliation requires confirmation or an explicit trusted pre-rollout credit cutoff; committed historical disbursement events replay separately. |
| RV4 + RV5 | Command retries of accept/reject/confirm return existing terminal state without re-appending; HTTP confirmation handles a confirmed contract after owner/auth checks. |
| RV7 | Typed gRPC Unavailable/DeadlineExceeded mapped to 503/504. |
| RV8 | `GET /offer` and contract OTP reads go straight to EventStoreDB with ownership checks, not async Mongo projection. |
| RV9 | Correct OTP OpenAPI success response and 410/423/429 failure codes. |
| RV10 | Application port and OTP response/error contract moved to `internal/application/otp`; infra keeps type/error aliases for transfer compatibility. |
| RV11 | Assert full aggregate snapshot unchanged when a rejected transition fails, including replay. |

## Required checks before merging

From `server` with project Go 1.25+ and dependencies installed:

```powershell
go test ./... -count=1
go test -tags=e2e ./internal/infrastructure/eventstore -run '^TestPersonalLoanHTTPE2E$' -count=1 -v
git diff --check
```

Because an EventStore change affects replay and banking, do not deploy until the
reviewers validate the legacy cutoff using real archived credit timestamps and
confirm there are no old API/projector binaries left. E2E tests are a separate
check that requires Docker and configured integration services.
