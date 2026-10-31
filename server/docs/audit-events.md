# Audit events

The API writes non-financial audit events to EventStoreDB. They are kept out of
the `banking` stream so OTP and authentication history cannot affect banking
replay, balances, or projector checkpoints.

## Streams

- `audit-account-{accountId}` for events with a trusted account ID.
- `audit-global` for events that happen before an account can be identified,
  such as email OTP verification and an invalid login attempt.

The audit repository appends optimistically and treats the deterministic event
ID as the idempotency key for a retry of the same operation.

## Event types

| Type | Meaning |
| --- | --- |
| `TransferOTPVerificationSucceeded` | Transfer OTP accepted before transfer confirmation |
| `TransferOTPVerificationFailed` | Transfer OTP rejected; includes remaining attempts |
| `TransferOTPExpired` | Expiry was detected during verification |
| `TransferOTPAttemptsExceeded` | Verification limit was reached |
| `EmailOTPVerificationSucceeded` | Email OTP accepted |
| `EmailOTPVerificationFailed` | Email OTP rejected; includes remaining attempts |
| `EmailOTPExpired` | Missing/expired email challenge was detected |
| `EmailOTPAttemptsExceeded` | Email OTP verification limit was reached |
| `LoginSucceeded` | Credentials and login conditions passed |
| `LoginFailed` | Credentials or an account login condition was rejected |

## Security rules

Audit payloads do not contain passwords, password hashes, OTP values, OTP
hashes, access tokens, refresh tokens, or raw identity numbers. Email OTP
events use a normalized SHA-256 email correlation value because they may happen
before an account exists.

`Expired` means expiry was detected by a verification request. EventStoreDB
does not receive a separate event merely because a Redis TTL elapsed.

## Failure semantics

OTP state remains managed by Redis. The API appends the audit event
synchronously after the Redis verification result. There is intentionally no
pending queue or worker in this change. If EventStoreDB is unavailable, the
request returns an infrastructure error; Redis changes are not rolled back and
the missing audit event is not reconstructed automatically.

`TransferCancelled` now carries an optional `cancellationReason` in the
banking event payload. Existing events without that field remain valid.
