# Issue #113 — Loan repayment reminder, stage 1

This stage implements a **read-only** scheduler; it does **not** send email, write EventStoreDB, or mark reminders as sent.

## Data and time model

- Read `loans` documents in MongoDB and inspect their `repayment_schedule` entries.
- Only loan `status: active` and installment `status: upcoming` are eligible.
- Match `T-5` and `T-0` with a Vietnam calendar date (`Asia/Ho_Chi_Minh` / fixed UTC+7).
- Signed EMI `repayment_schedule.due_date` values are stored as *UTC-midnight date-only values*; query the matching UTC calendar day instead of converting due dates to midnight in Vietnam.
- Keep `installment_no`, `total_due`, and `loan_no` from the read model in memory. Never recompute EMI amounts.
- Never log loan numbers, repayment amounts, due dates, or idempotency keys. Only aggregate statistics and reminder type are logged.
- Migration `000020_loan_reminder_due_index` adds a compound multikey index for `status`, `repayment_schedule.status`, and `repayment_schedule.due_date`, so the active/upcoming due-date query can use an index as the loan collection grows. Run migrations through the team's approved process and inspect current migration status first; do not blindly apply earlier, possibly destructive migrations. Check query plans with MongoDB `explain` against representative data.

## Run

From `server/`, with MongoDB reachable and the read model populated:

```powershell
go test -count=1 ./internal/application/loan/reminder ./internal/infrastructure/mongo ./cmd/loan-reminder
go run ./cmd/loan-reminder --once
go run ./cmd/loan-reminder
```

The first command runs tests. The `--once` command performs a single scan (exits nonzero when scanning fails). The final command runs an immediate scan and then scans every hour until Ctrl+C. Each scan has a 2-minute timeout. Both run modes log only non-sensitive candidate types and aggregate counts; neither sends reminders.

## Stage 2 integration contract

`reminder.Candidate` and `Candidate.IdempotencyKey()` provide stable IDs (`loan-reminder:<loan_id>:<installment_no>:<type>`, with type `T-5` or `T-0`). In stage 2, connect the notification/mail-outbox adapter to `reminder.Handler`. Add an **atomic unique-key reservation/enqueue** in the durable outbox; a deterministic string or in-process `seen` map alone does not prevent duplicates across restart or multiple replicas. Verify the installment is still unpaid at enqueue time and avoid publishing while the read model is rebuilding. Implement outbox claim/send/retry and observe `sent/skipped/failed` as distinct delivery outcomes.

No migration, HTTP route, or notification implementation is added in stage 1. The main stage-1 process is deliberately dry-run-only.
