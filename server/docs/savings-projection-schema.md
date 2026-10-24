# Savings projection v1: storage and replay contract

This describes the **implemented** MongoDB read projection in GĐ2/GĐ3. It does
not amend the original analysis/design JSON files. Before implementing GĐ4
list/detail queries, the team must approve one canonical DTO/mapping and a
migration strategy for existing records. **Do not silently convert ID types in
place**; that would break existing documents and cursors.

`EventStoreDB` (`banking` stream) is the only source of truth for savings
contracts and bank balances. `savings_contracts` is disposable/rebuildable.

| Field in current projection | Storage type | Source |
|---|---|---|
| `_id` | string (contract UUID) | Savings Event `Contract.ID` |
| `customer_id` | string | Event `Contract.CustomerID` |
| `package_id` | string (`savings_standard`) | Event `Contract.PackageID` |
| `source_account_id` | string | Event `Contract.SourceAccountID` |
| `term_months` | integer | Event `Contract.TermMonths` |
| `annual_rate_bps` | integer basis points | Contract rate **at opening**, not current catalog |
| `financial_revision` | int64 | **banking stream event number + 1** |
| `settlement` | subdocument, only when settled | Event `Contract.Settlement` |

The original design document uses ObjectId references and names such as
`savings_package_id`, `term.months`, and `interest_rate` (Decimal128). These
are **not automatically interchangeable** with the current event DTO and
MongoDB fields. The GĐ4 query adapter must explicitly reconcile them, or a
reviewed migration must change storage and all dependent consumers together.

## Replay guarantee

- A `SavingsOpened` event projects an **active** contract without a settlement.
- A `SavingsSettled` event projects a **settled** contract with a settlement.
- Only an event whose `financial_revision` exceeds the stored revision can
  replace a read document. Old events replayed after resetting a checkpoint
  must not overwrite the newer contract state.
- A full **replacement** removes fields absent in the newer canonical event,
  avoiding a stale `settlement` left on an Active document.
- Projection writes and banking checkpoint are in the same MongoDB transaction.
  EventStoreDB append and MongoDB projection are *eventually consistent*; no
  claim of cross-database atomicity is made.

Test this behavior with
`go test -tags=savings_e2e -run TestSavingsProjectionReplayWithMongoValidator ./internal/infrastructure/mongo -count=1`
with Docker running. The test starts a **disposable MongoDB replica set** via
Testcontainers, creates an isolated database and removes the container.
