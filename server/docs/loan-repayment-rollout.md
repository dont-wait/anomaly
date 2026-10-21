# Loan repayment rollout (Go server only)

The new `LoanInstallmentPaid` event is **not understood by older Bank Projector builds**.

1. Back up MongoDB and confirm database migration 10 (`dirty=false`). Do not reset or delete the Banking Stream.
2. Stop every running old `cmd/projector` instance; ensure only compatible Bank Projectors remain.
3. Deploy the compatible Bank Projector with `LoanInstallmentPaid` handling, replay-safe balance versions, and generation-based repayment cache invalidation. Verify its EventStoreDB subscription and MongoDB checkpoint progress on a test event.
4. Run `loan-projector --backfill-loan-repayment` for existing disbursed loans and verify the logged loan count. This must finish before enabling writes.
5. Only after step 3 and the backfill, allow the new repayment HTTP writes. Backend and projector should use compatible versions during rollout.
6. During checkpoint-only `--replay`, balance writes are guarded by `financial_version` so a historical event cannot overwrite a newer balance. A full rebuild of all read models remains a separate migration/operations procedure; do not delete collections or run replay on shared/production databases without a backup and a recovery plan.
7. MongoDB reads and Redis caches are projections, not the source of truth. Bank EventStoreDB journal decides canonical debits/idempotency. Cache generation counters deliberately persist to prevent an in-flight stale read from repopulating a newly invalidated key.

Kafka is not changed by the loan repayment rollout.
