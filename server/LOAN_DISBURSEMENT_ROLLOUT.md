
# Loan Disbursement - Deployment Order

## Problem

The LoanDisbursementCredited event is not supported by
older versions of the banking event decoder.

Older API replicas may fail to replay the banking stream
after the first loan disbursement event is written.

## Deployment strategy

Use a controlled deployment to prevent old banking-stream
readers from running alongside the new event writer.

## Deployment steps

Run commands from the server directory.

### 1. Stop the old API

```powershell
docker compose --profile app stop anomaly-server
```

Confirm that no older API replicas or other banking-stream
readers remain active outside this Compose deployment.

### 2. Build updated services

```powershell
docker compose --profile app build anomaly-server anomaly-worker anomaly-loan-projector
```
Before starting the updated services, run the database
migrations and verify that migration 000007
(allow_loan_disbursement_feed) has been applied.

Do not enable loan disbursement until the migration
has completed successfully.
### 3. Start updated projectors

```powershell
docker compose --profile app up -d --no-deps --force-recreate anomaly-worker anomaly-loan-projector
```

### 4. Verify reader versions

Confirm that all deployed banking-stream readers support
LoanDisbursementCredited.

Do not enable loan disbursement while any old reader is active.

### 5. Start the updated API

```powershell
docker compose --profile app up -d --no-deps --force-recreate anomaly-server
```

### 6. Verify deployment

```powershell
docker compose --profile app ps anomaly-server anomaly-worker anomaly-loan-projector
```

Check the service logs and confirm that the updated services
are running before allowing new loan disbursements.

## Rollback warning

After LoanDisbursementCredited has been written to EventStoreDB,
do not roll back to a version that cannot decode this event.

Rollback must use a version that supports the new banking event.

## Scope

This procedure covers the repository's single-API Docker
Compose deployment.

Multi-replica deployments require coordination across all
hosts and replicas before enabling the new event writer.

## Loan Projection Checkpoint Recovery

The Loan Projector stores a checkpoint containing:

- Commit position
- Prepare position
- Event ID

When starting, the projector verifies that the checkpoint
matches the corresponding event in EventStoreDB.

If the checkpoint is missing its Event ID or does not match
EventStoreDB, the projector stops instead of automatically
deleting MongoDB data.

### Recovery requirements

Before performing a reset:

1. Stop all Loan Projector instances.
2. Confirm EventStoreDB is available.
3. Verify that EventStoreDB contains the complete Loan
   event history required to rebuild the read model.
4. Back up the MongoDB loan_applications_read collection,
   the loan-projection-all-v1 checkpoint, and the
   loan_projection_failures collection.
5. Arrange a maintenance window because Loan queries
   may return incomplete results during rebuilding.

If EventStoreDB has lost its original event history,
restore EventStoreDB from backup first.

Do not reset the Loan Read Model when the source events
are missing.

### Manual reset

Stop the Loan Projector:

```powershell
docker compose --profile app stop anomaly-loan-projector
```

After completing the recovery requirements, execute:

```powershell
docker compose --profile app run --rm --no-deps -e LOAN_PROJECTION_RESET_CONFIRM=I_HAVE_BACKED_UP_LOAN_READ_MODEL anomaly-loan-projector /app/loan-projector --reset-loan-projection
```

This command clears the Loan Read Model and checkpoint
so they can be rebuilt from EventStoreDB.

The confirmation variable is a safeguard against
accidental execution. It does not replace backups.

### Restart projector

After resetting, start the updated Loan Projector:

```powershell
docker compose --profile app up -d --no-deps anomaly-loan-projector
```

Check the logs:

```powershell
docker compose --profile app logs --tail=100 anomaly-loan-projector
```

Verify that the projector replays the events and
rebuilds the Loan Read Model successfully.

Do not resume normal operations until the read model
has been validated.

### Retry quarantined loan

If only one Loan application has a projection failure,
fix the underlying cause before retrying it.

Use:

```powershell
docker compose --profile app run --rm --no-deps anomaly-loan-projector /app/loan-projector --retry-loan <loan-id>
```

This command retries the specified Loan application
without resetting the entire Loan Read Model.
