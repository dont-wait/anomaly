# Issue #212 — Profile-change review hardening

## Deployment prerequisites

1. **Back up MongoDB** before deploying or migrating. Existing `customers.profile_change` values are migrated to the dedicated `profile_change_requests` collection by migration 23. Historical requests already overwritten *before this migration* cannot be recovered automatically.
2. **Stop/drain older API writers before migration and rollout.** Earlier server versions replace whole customer documents and do not know the review collection. Newer code uses field-level updates, and authoritative review records remain independent of customer documents. Do not keep old writers running alongside the new review contract.
3. MongoDB **replica set is mandatory** for transactions (provided by the repository's `docker-compose.yml`: `rs0`). Run `go run ./cmd/migrate status` and then `go run ./cmd/migrate up` from `server/`; verify version >= **24**, `dirty=false` before using profile-change reviews. Migration 23 is the history collection/backfill; **migration 24** backfills indexable search terms and creates the search index. Migration 20 is also required for `notifications` startup. Do not edit an already-applied migration 23.
4. New API startup **warns** when migration 24 is missing rather than crashing unrelated endpoints. Review reads/writes remain unavailable until migration 24 is applied. Only migration commands create indexes; API never recreates them. Confirm the `idx_review_search_keys` index after migration.
5. Production `AccountAggregateRepository` implements `ProfileChangeStore`, so customer submissions/decisions use the transactional review collection, not the in-memory/testing fallback. `CustomerRepository.Save` intentionally does not overwrite the `profile_change` compatibility mirror; do not route production profile-change commands through a repository without `ProfileChangeStore`.
6. Deploy new API then Desktop. `/api/admin/profile-changes` still returns the original array for older Desktop clients, while `/api/admin/profile-changes/page` returns the paginated object. New Desktop falls back to the array API on a 404 while older API instances are being drained. The fallback does not provide cursor snapshot guarantees.
7. **Do not roll migration 23 down on a running system**: the down script drops the history collection and permanently destroys review history. Restore from backup when a rollback is necessary.

## Behavior

- Each submitted request is stored as an individual document in `profile_change_requests` (Mongo unique request ID). Submitting another request supersedes the previous pending request, preserving audit history; admin decisions require `PENDING`.
- Submission and decisions update the review document and customer profile/UI mirror in a single Mongo transaction. Two concurrent decisions cannot both succeed.
- Before-values are captured on new submissions and remain unchanged after approval. Backfilled older requests have `has_previous=false` because the original before-image was not stored in the legacy schema; the UI marks the comparison as unavailable rather than incorrectly displaying identical values.
- The new page API supports `status`, `search`, `pageSize`, and `cursor`; the cursor binds to the search/status filters. The UI uses `nextCursor` for stable page boundaries when new requests arrive, with a reset on filter/search/refresh.
- The Mongo query uses one `$facet` aggregation for the exact total and bounded page instead of separately counting/scanning all accounts. Status and chronology have indexes. Search is now **case-normalized, anchored prefix matching** against the indexed multi-key `search_keys` field (full name and word-start prefixes, username, account ID). This is deliberately **not arbitrary substring search**. Migration 24 backfills searchable keys for earlier requests; older backfilled keys may support fewer word combinations than newly submitted records.
- Approval updates search metadata in the same transaction as the approved profile values; an approved request is searchable by both its previous and new name.
- General `FindAll` batches new review-history enrichment rather than requesting it once per account; pre-existing customer/KYC hydration still issues its own queries.
- Review records snapshot the email, identity type and a **masked** identity number for submissions after this change. Existing historical records without these fields leave them blank; they are never silently reconstructed as a before-image. Do not log or expose unmasked identity numbers in review API responses.
- Legacy Desktop fallback is shown as an explicit limited mode. A structured API 404 is surfaced as an error rather than silently falling back. Review API availability still depends on migration 24.

## Test plan

- Existing and new Go unit tests (Mongo record and command/handler review), Desktop typecheck/test/lint.
- Live with Mongo replica set + migration 23: submit, approve/reject, re-submit after approval, filter complete history, compare prior values, refresh, and test two concurrent decisions.
- With at least 11 review records, page forward/back, submit a newer request between pages, ensure cursor navigation doesn't repeat an earlier item. Verify legacy array and new object routes independently. Repeat the same search, switch filters and verify no duplicate fetch loop; confirm masked identity and older-server warning.
- Verify MongoDB migration 24 on a *backup/staging database* before production: the backfill pipeline, exact-count `$facet`, and cursor behavior under concurrent inserts; review all changes to migration status before deploying.
