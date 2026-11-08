# Issue #212 — Profile-change review hardening

## Deployment prerequisites

1. **Back up MongoDB** before deploying or migrating. Existing `customers.profile_change` values are migrated to the dedicated `profile_change_requests` collection by migration 23. Historical requests already overwritten *before this migration* cannot be recovered automatically.
2. **Stop/drain older API writers before migration and rollout.** Earlier server versions replace whole customer documents and do not know the review collection. Newer code uses field-level updates, and authoritative review records remain independent of customer documents. Do not keep old writers running alongside the new review contract.
3. MongoDB **replica set is mandatory** for transactions (provided by the repository's `docker-compose.yml`: `rs0`). Run `go run ./cmd/migrate status` and then `go run ./cmd/migrate up` from `server/`; verify version >= 23, `dirty=false` before starting a new API instance. Migration 20 is also required for `notifications` startup.
4. Deploy new API then Desktop. `/api/admin/profile-changes` still returns the original array for older Desktop clients, while `/api/admin/profile-changes/page` returns the paginated object. New Desktop falls back to the array API on a 404 while older API instances are being drained. The fallback does not provide cursor snapshot guarantees.
5. **Do not roll migration 23 down on a running system**: the down script drops the history collection and permanently destroys review history. Restore from backup when a rollback is necessary.

## Behavior

- Each submitted request is stored as an individual document in `profile_change_requests` (Mongo unique request ID). Submitting another request supersedes the previous pending request, preserving audit history; admin decisions require `PENDING`.
- Submission and decisions update the review document and customer profile/UI mirror in a single Mongo transaction. Two concurrent decisions cannot both succeed.
- Before-values are captured on new submissions and remain unchanged after approval. Backfilled older requests have `has_previous=false` because the original before-image was not stored in the legacy schema; the UI marks the comparison as unavailable rather than incorrectly displaying identical values.
- The new page API supports `status`, `search`, `pageSize`, and `cursor`; the cursor binds to the search/status filters. The UI uses `nextCursor` for stable page boundaries when new requests arrive, with a reset on filter/search/refresh.
- The Mongo query counts and pages review documents directly, instead of loading every account. Status and chronological pagination are indexed. Arbitrary substring search uses an escaped regex on the review document's denormalized `search_text`; it may need a dedicated search index if the dataset becomes very large.

## Test plan

- Existing and new Go unit tests (Mongo record and command/handler review), Desktop typecheck/test/lint.
- Live with Mongo replica set + migration 23: submit, approve/reject, re-submit after approval, filter complete history, compare prior values, refresh, and test two concurrent decisions.
- With at least 11 review records, page forward/back, submit a newer request between pages, ensure cursor navigation doesn't repeat an earlier item. Verify legacy array and new object routes independently.
