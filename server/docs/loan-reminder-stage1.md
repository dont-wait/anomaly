# Issue #113 — Loan repayment reminder, stages 1 and 2

The worker uses the shared `internal/scheduler` runner. It defaults to **read-only dry-run** (stage 1). Stage 2 can persist in-app notifications through the existing Notification service when explicitly enabled. Neither mode sends SMTP email or writes EventStoreDB events.

## Data and time model

- Read `loans` documents in MongoDB and inspect their `repayment_schedule` entries.
- Only loan `status: active` and installment `status: upcoming` are eligible.
- Match `T-5` and `T-0` with a Vietnam calendar date (`Asia/Ho_Chi_Minh` / fixed UTC+7).
- Signed EMI `repayment_schedule.due_date` values are stored as *UTC-midnight date-only values*; query the matching UTC calendar day instead of converting due dates to midnight in Vietnam.
- Keep `installment_no`, `total_due`, and `loan_no` from the read model in memory. Never recompute EMI amounts.
- Never log loan numbers, repayment amounts, due dates, or idempotency keys. Only aggregate statistics and reminder type are logged.
- Stage 1 **does include** migration `000021_loan_reminder_due_index`: it adds a compound multikey index for `status`, `repayment_schedule.status`, and `repayment_schedule.due_date`, so the active/upcoming due-date query can use an index as the loan collection grows. Run migrations through the team's approved process and inspect current migration status first; do not blindly apply earlier, possibly destructive migrations. Check query plans with MongoDB `explain` against representative data.

## Run

From `server/`, with MongoDB reachable and the read model populated:

```powershell
go test -count=1 ./internal/application/loan/reminder ./internal/infrastructure/mongo ./cmd/loan-reminder
go run ./cmd/loan-reminder --once
go run ./cmd/loan-reminder
```

The first command runs tests. The `--once` command performs a single scan (exits nonzero when scanning fails). The final command runs an immediate scan and then scans every hour until Ctrl+C. Each scan has a 2-minute timeout, supplied by the reusable `internal/scheduler` package (`RunOnce` aggregates failures; `Start` runs immediately, repeats on a ticker, and stops on cancellation). Both run modes log only non-sensitive candidate types or aggregate counts. With the default flag they never send reminders; notification mode is separately opt-in.

## Stage 2 — in-app notifications (opt-in)

The Notification subsystem merged from PR #205 exposes `composition.NewNotifier`,
`notification.TypeLoanRepaymentDue` and the `notifications` MongoDB collection.
This worker now supports direct `Notifier.Notify` when **explicitly enabled**:

```powershell
# Use a disposable MongoDB database with realistic loan and owner projections.
# Do NOT enable notifications against production during a smoke test.
$env:MONGO_DB = "anomaly_loan_reminder_stage2_test"
$env:LOAN_REMINDER_NOTIFICATIONS_ENABLED = "true"
go run ./cmd/loan-reminder --once
Remove-Item Env:LOAN_REMINDER_NOTIFICATIONS_ENABLED
Remove-Item Env:MONGO_DB
```

By default (`LOAN_REMINDER_NOTIFICATIONS_ENABLED=false`) the existing worker
remains **dry-run only**. Stage 2 inserts **in-app notifications**, not SMTP
emails. API change streams/SSE propagate them to mobile; the mobile route for
`LOAN_REPAYMENT_DUE` opens `#/loan/contracts/:loanId`.

Before enabling, apply migrations through the team's approved deployment flow:
`000020_create_notifications` creates the notification collection and indexes;
`000021_loan_reminder_due_index` indexes the loan due-date query. **Do not run
`migrate up` blindly on an existing database** with earlier pending migrations.
The worker also checks that the notifications store already exists before it
will start in notification mode.

- **Recipient:** `loan_applications_read.owner_id` is the JWT user ID. Do not
  use `accounts.customer_id`, which is a different MongoDB identity.
- **Eligibility:** Before each Notify, recheck `loans.status=active`, the
  signed installment's `status=upcoming`, due date and amount, the deterministic
  application link, and that the loan projection is not rebuilding. Missing
  ownership projection causes a retriable error, not an unowned notification.
- **Idempotency:** The `SourceEventID` is
  `loan-reminder:<loan_id>:<installment_no>:<T-5|T-0>`. The Notification service
  derives `_id` from `(SourceEventID, Type, RecipientID)` and upserts via
  `$setOnInsert`, so restarts and repeated hourly runs do not create duplicates.
  T-5 and T-0 are intentionally separate notifications.
- **No PII in logs or push-visible text:** no account number, loan number,
  amount, due date, owner, or deterministic source key is logged. Notification
  text is generic; `data.loanId` is used only for authenticated navigation.
- **Errors:** a paid/cancelled installment is counted as skipped. MongoDB or
  Notify errors are returned to the scheduler (which retries on its next hourly
  run), while other candidates continue processing. Notification insertions
  are idempotent; they are not proof of push delivery or emails being sent.

**Consistency limit:** the pre-send status check and notification insert are
separate MongoDB operations against an eventually consistent loan read model.
A payment racing between these operations could still produce a notification.
Eliminating that window strictly requires a transaction/authoritative event
integration, not merely the current idempotent notification store.

## Validation

From `server/`:

```powershell
go test -count=1 ./internal/application/loan/reminder ./internal/application/notification ./internal/infrastructure/mongo ./internal/scheduler ./cmd/loan-reminder

# Optional integration test against a TEST MongoDB instance only:
$env:LOAN_REMINDER_E2E_MONGO_URI = "mongodb://<test-host>:27017/?replicaSet=rs0"
go test -count=1 -v -run TestLoanReminderNotificationE2E ./internal/infrastructure/mongo
Remove-Item Env:LOAN_REMINDER_E2E_MONGO_URI
```

The optional Go E2E test creates and drops its own uniquely named database,
seeds two loan installments and an owner, and checks durable de-duplication
and paid-period filtering. It is skipped unless the TEST URI is supplied.

For a **manual smoke test against an isolated database only**, insert realistic `loans` and
`loan_applications_read` records (including matching deterministic application
IDs and `owner_id`), then run `--once` twice with notifications enabled and
verify that each recipient has at most one T-5 and one T-0 item in
`notifications`, with `category=LOAN`. Mark the test installment `paid`, rerun
and confirm no new notification is inserted. Check the mobile Notifications page
and routing. A database containing only the older four synthetic `loans`
records, without the owner/application projections, cannot exercise stage 2.
