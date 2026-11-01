# Profile — Product and API Specification

**Status:** Draft for review  
**Scope:** Customer profile UI, profile updates, avatar and personal-document media, password change  
**Related implementation:** `client/mobile/src/shared/layout/ProfileSidebar.tsx`, `server/internal/presentation/http/handler/account`, `server/internal/infrastructure/rustfs`

## 1. Purpose

Give an authenticated customer one place to review and manage their personal profile. The profile must make it clear which information can be edited directly, which is verified identity data, which files are stored, and whether a change succeeded.

This specification is for a separately delivered frontend PR and backend PR. The frontend must consume the backend contract in this document; it must not simulate successful saves or upload media directly to RustFS.

## 2. Current state and constraints

- The mobile app opens `ProfileSidebar` from the dashboard. It currently shows the account name and logout action only.
- `GET /api/auth/me` returns the current private account response. The frontend calls it through `getInfo` when restoring a saved session.
- Account data already distinguishes customer profile, customer identity, and verified KYC sessions. KYC images/video are stored in RustFS and downloaded through an authenticated backend endpoint that checks ownership and verified-session membership.
- The existing account domain stores structured account/profile data. RustFS is an object store and should contain file bytes, not structured profile records or credentials.
- Passwords are represented by `PasswordHash` and checked with bcrypt during login. Plain passwords must never be stored, logged, or sent to RustFS.
- The API's account write path is `eventstore.BankRepository`. Its `Save` appends one `AccountUpdated` event holding the full account snapshot, returns a conflict unless `Version` is exactly the current version + 1, and **returns success without writing when `Version` is unchanged**.
- `toAccountResponsePrivate` currently returns RustFS storage keys in `idCardFrontUrl`, `idCardBackUrl`, and `liveVideoUrl`. This response is also used by registration, login, and KYC handlers.
- Email exists in both `UserAccount.Email` and `Customer.Profile.Email`. Login lookups and loan-contract OTP delivery use `UserAccount.Email`.
- Bearer tokens are validated without an account lookup, default to a 24-hour expiry, and have no revocation mechanism. Password verification has no throttling or lockout.
- The mobile `requestApi` client allows only `GET`, `POST`, and `DELETE`.

## 3. Product behavior

### 3.1 Profile entry and navigation

- Tapping **Cá nhân** opens the profile as a drawer/sidebar over the current page.
- The drawer has a clear close action and retains the current page beneath it.
- Profile sections are reachable from the drawer without creating duplicate navigation entries or fake profile rows.
- On small Android screens, the drawer may occupy the full viewport while preserving the same header, spacing, typography, and card styling as the Savings screen.

### 3.2 Profile overview

Show:

- Avatar, display name, account number (masked except the last four digits), and verification status.
- Personal details: full name, date of birth, phone, email, residential address, and province/region where available.
- Identity details: CCCD/identity number (masked), issue date/place, and permanent address where available.
- A document section showing each required document's label, status, upload date, and preview/download action when available.
- Separate actions for **Cập nhật thông tin**, **Đổi mật khẩu**, **Đổi ảnh đại diện**, and document updates.

Do not display internal account/customer IDs, RustFS keys, storage URLs, token values, password hashes, or backend implementation details.

### 3.3 Editing profile information

- Editable fields in the first release: phone, email, residential address, and province/region.
- Full name, date of birth, CCCD number, CCCD issue data, and permanent address are read-only after KYC verification. The UI explains that correction requires a review flow and supporting proof; it must not imply that changing text updates verified identity.
- Before KYC verification, whether identity fields can be edited is governed by the existing KYC/session flow; this profile feature must not bypass KYC.
- Validate required values and formats before submission. The server remains authoritative.
- Save is explicit. Show loading state, field-level validation where possible, and a success/error message. Do not optimistically claim a save before the server confirms it.
- Email is changed through its own verification flow (section 5.3), not the contact form. While a change is pending, show the new address as pending and keep the current address as the active email.

### 3.4 Avatar

- Accept JPEG, PNG, or WebP images up to 5 MiB. Reject SVG and other active content.
- Show a local preview before upload, with **Lưu ảnh** and **Hủy** actions.
- Crop/resize is optional for the first release; if omitted, document the accepted aspect ratio and server behavior.
- Replacing an avatar updates the profile only after object upload and metadata update succeed. Remove the previous object after the new reference is committed; provide retry/cleanup for orphaned uploads.
- Avatar reads must use an authenticated backend URL or short-lived authorized response, not a public RustFS URL.

### 3.5 Personal documents

Initial document categories:

| Type | Purpose | Editability |
| --- | --- | --- |
| `IDENTITY_FRONT` | Front of the identity document | Read-only after KYC verification; correction/replacement requires review |
| `IDENTITY_BACK` | Back of the identity document | Read-only after KYC verification; correction/replacement requires review |
| `INCOME_PROOF` | Income evidence for services that request it | Customer may replace while the related workflow permits it |
| `ADDRESS_PROOF` | Address evidence when requested | Customer may replace while the related workflow permits it |

- Reuse KYC media where it is already available; do not upload duplicate copies merely to populate the profile.
- Accept PDF, JPEG, and PNG up to 10 MiB per document. Validate declared and sniffed content type, extension, size, and file signature. Reject archives, executables, HTML, and SVG.
- Never expose bucket names, object keys, or permanent public links to the client.
- A successful upload returns metadata and a document identifier, not file bytes.
- Downloads/previews require the current authenticated owner. Administrative access remains governed by existing authorization/audit rules.
- Replacing or deleting verified KYC evidence must not silently rewrite the verified session or its evidence. Use an explicit correction/reverification workflow.

The initial list above does not mean every product currently requires proof of income/address. The UI shows only the slots returned by `GET /api/auth/me/documents` (section 5.7) and must not hard-code which types are supported or required.

Each account has at most one active document per type. Uploading to a type that already has an active, replaceable document replaces it.

### 3.6 Change password

- Require current password, new password, and confirmation in the UI. Never prefill or persist password fields.
- Server verifies the current password and applies the existing password policy before changing the hash.
- Do not return the password, hash, or a new token in the response.
- On success, clear password inputs and show confirmation. Password change must be audit-logged without recording secrets.
- A successful password change invalidates every existing token for the account, including the one used for the request. The client clears its token and returns to login, where the customer signs in with the new password.
- Failed current-password attempts are throttled and audit-logged (section 5.4).

## 4. Data ownership and storage

| Data | System of record | Storage rule |
| --- | --- | --- |
| Name, DOB, identity data, contact and address | Existing account/customer domain and persistence/event path | Structured fields; never RustFS |
| Password | Existing account credential field | bcrypt hash only; never RustFS. The hash is persisted in protected account events so replay restores the current credential; it never appears in responses, logs, or audit payloads |
| Email | `UserAccount.Email` is authoritative | `Customer.Profile.Email` is a copy, always set to the same value in the same account event |
| Avatar/document bytes | RustFS | Private objects, server-generated keys |
| Media metadata and ownership/type/status | Account/profile persistence associated with current account | No raw bytes, credentials, or public object URL |

Account writes must go through the established account write path so EventStoreDB/account state and Mongo projections remain consistent. Do not update a projection directly as the source of truth. The exact event/schema migration strategy is a backend design decision and must preserve older account records.

Every profile, email, and password command must increment `UserAccount.Version` and set `UpdatedAt` before calling `Save`; otherwise `Save` reports success without persisting anything. A version conflict returns 409 and the client reloads the profile before retrying. It must not be retried automatically with the stale snapshot.

Suggested media metadata:

```json
{
  "mediaId": "opaque-id",
  "type": "AVATAR",
  "fileName": "avatar.jpg",
  "contentType": "image/jpeg",
  "sizeBytes": 248120,
  "uploadedAt": "2026-11-01T10:00:00Z",
  "status": "ACTIVE"
}
```

Do not include the RustFS key in customer-facing responses. Store it server-side and scope it to the owning account, e.g. `profile/{accountID}/{mediaID}`. Keys must be generated by the server, not accepted from the request.

## 5. Proposed API contract

All endpoints require the normal account bearer token, return `Cache-Control: no-store` for sensitive profile responses, and derive the account ID from auth claims. Clients cannot choose an account ID. All endpoints use the standard success/error envelope, except that a successful media download returns raw bytes (section 5.7).

### 5.1 Read current profile

`GET /api/auth/me`

Extend the existing response with `avatar` metadata and `pendingEmail` (null when no change is pending). Document metadata comes from `GET /api/auth/me/documents`. Mask identity values in the customer-facing profile response unless a screen has a justified need for the full value.

Remove `idCardFrontUrl`, `idCardBackUrl`, and `liveVideoUrl` from `toAccountResponsePrivate`, because they contain storage keys. This applies to every response using that mapper, and the mobile `profile.ts` type drops the same fields. Other existing fields keep their names and meanings.

### 5.2 Update editable profile fields

`PATCH /api/auth/me/profile`

Request fields are partial and limited to:

```json
{
  "phone": "+84901234567",
  "address": {
    "line": "12 Nguyen Trai",
    "provinceCode": "HCM"
  }
}
```

Reject unknown/protected fields (including email, identity, role, account number, balance, and KYC status). Response returns the updated profile representation.

Merge rules apply at every level, including inside `address`: an omitted field keeps its current value. `null` or an empty string is rejected with 400, because these fields cannot be cleared in the first release. A request with no fields is also rejected with 400.

### 5.3 Change email

`POST /api/auth/me/email-change` with `{ "newEmail": "..." }` starts the change and sends one OTP to the current email and one to the new email. The account's `Email` does not change.

`POST /api/auth/me/email-change/confirm` with `{ "currentEmailOtp": "...", "newEmailOtp": "..." }` completes it. Both codes are required: verifying only the new address would let someone holding a stolen token move the account to an inbox they control.

- Until confirmation, login, notifications, and sensitive OTPs, including loan-contract OTP, keep using the current email.
- The new email must be unique when the change starts and again at confirmation; otherwise return 409.
- Pending changes expire with their OTPs. Starting a new change replaces the previous one. OTP issue and attempt limits follow the existing OTP service.
- Confirmation updates both email copies in one account event, as defined in section 4, and sends a notice to the previous address.

### 5.4 Change password

`PUT /api/auth/me/password`

Request:

```json
{
  "currentPassword": "...",
  "newPassword": "..."
}
```

Return a generic success message. Never echo either password. Use the same password strength rules as account creation and return a generic invalid-current-password error with status 400, so the client does not treat it as an expired session.

**Throttling.** After 5 failed attempts for an account within 15 minutes, reject further attempts with 429 and `Retry-After` without checking the password, even if the password is correct. Apply a separate per-source limit. Limits are configurable, and every failed or throttled attempt is audit-logged without secrets.

**Token invalidation.** Add a token version to the account and to JWT claims. A password change increments it in the same account event as the new hash. `RequireAuth` rejects a token with 401 when its version does not match the account's current version. Tokens issued before this feature and accounts created before it both count as version 0. This adds an account read to authenticated requests, so its latency must be measured before release.

### 5.5 Upload avatar

`POST /api/auth/me/avatar` (`multipart/form-data`, field `file`)

Returns the new avatar metadata. Replacement is atomic from the client's perspective: do not publish the new reference until RustFS upload and account metadata update succeed.

### 5.6 Upload personal document

`POST /api/auth/me/documents` (`multipart/form-data`, fields `type`, `file`)

Allowed initial types: `IDENTITY_FRONT`, `IDENTITY_BACK`, `INCOME_PROOF`, `ADDRESS_PROOF`. Server-side state/KYC policy determines whether upload, replacement, or deletion is allowed.

Upload is safe to retry: because each type has one active document, repeating a request whose response was lost replaces the document rather than adding a second one. Avatar upload behaves the same way. A retry against a type that is no longer replaceable returns 403.

### 5.7 List/get/delete media

- `GET /api/auth/me/documents` returns one slot for each type the backend policy supports for this account, whether or not a file has been uploaded:

  ```json
  {
    "slots": [
      {
        "type": "INCOME_PROOF",
        "required": false,
        "replaceable": true,
        "document": null
      }
    ]
  }
  ```

  `document` uses the media metadata from section 4 when a file exists. A type missing from `slots` is unsupported and uploads to it are rejected.
- `GET /api/auth/me/media/{mediaId}` streams the file with owner checks. Success returns raw bytes without the JSON envelope, with `Content-Type`, `Content-Length`, `Content-Disposition`, `X-Content-Type-Options: nosniff`, and `Cache-Control: no-store`. Preview is inline only for allow-listed image/PDF types; force attachment otherwise. Errors found before the first byte use the standard error envelope; a storage failure during streaming aborts the connection so a partial file is not treated as complete.
- `DELETE /api/auth/me/documents/{mediaId}` is allowed only for replaceable, unverified documents. Avatar removal may be a separate `DELETE /api/auth/me/avatar` operation.

Use opaque media IDs and account-scoped authorization. Do not add a generic “download arbitrary key” endpoint.

### 5.8 Error behavior

Use consistent error envelopes and distinguish at least: invalid input or wrong current password (400), unauthenticated or invalidated token (401), forbidden mutation/state (403), missing media/profile (404), version or email conflict (409), file too large (413), unsupported or detected type (415), throttled (429), and storage/internal failure (500/503). Avoid exposing bucket/key or storage-provider errors.

## 6. Frontend PR scope

- Update the profile drawer into a profile overview using the Savings visual language (Be Vietnam Pro, pale gradient background, rounded white cards, clear status chips and pill actions).
- Add focused edit screens/sheets for contact/address, avatar selection/preview, document upload/preview, and password change.
- Keep form state local to the screen; do not persist passwords or document bytes in localStorage/sessionStorage.
- Use the API client and token handling already present, extending `HttpMethod` and `requestApi` to send `PATCH` and `PUT` with bodies. Refresh profile state after successful mutations.
- Make `AuthProvider.refreshProfile` ignore a response when a newer request has started, using a request generation or cancellation, so an older read cannot replace a newer profile.
- Add the email-change screen with both OTP inputs and the pending state. On 409 from a profile save, reload the profile and keep the customer's input for review.
- After a successful password change, clear the token and return to login.
- Add loading, empty, upload progress, retry, validation, and success/error states.
- Do not include unrelated loan/savings changes in this PR.

## 7. Backend PR scope

- Add authenticated self-service routes through the `openapi.Registry`; never accept an account ID from the customer request.
- Add account application commands for editable profile updates, email change, and password change, using the established account persistence/event path and incrementing `Version` as required by section 4.
- Add password-attempt throttling, the token version claim, and its check in `RequireAuth`.
- Add private profile-media metadata and repository operations backed by the existing RustFS client.
- Add strict multipart limits, file signature validation, server-generated keys, safe response headers, owner checks, and cleanup behavior for failed/replaced objects.
- Reuse existing verified KYC media metadata where appropriate without weakening the current verification/download rules.
- Ensure private responses are not cached and sensitive values are excluded from logs/audit payloads.
- Preserve compatibility with existing `/api/auth/me`, registration, login, KYC, and loan document contracts, apart from removing the three storage-key fields (section 5.1).

## 8. Acceptance criteria

### Frontend

- Opening/closing profile works as an overlay and does not unexpectedly navigate away from the current screen.
- Customer sees only their own profile and masked identity/account values.
- Editable contact/address fields save through the API and show server-confirmed results.
- Verified identity fields cannot be edited through the normal form.
- Avatar and document uploads show accepted types/limits, preview/progress, and actionable errors.
- Password change requires current password and matching new-password confirmation; secrets are cleared after submit, and success returns the customer to login.
- No success state appears when the backend rejects or fails a mutation.
- A pending email is never shown as the active email.
- An older in-flight profile read cannot replace the profile shown after a confirmed save.

### Backend

- Self-service routes always operate on the authenticated account, including when a request attempts to inject another account ID.
- Protected account, financial, role, identity/KYC status, and verification fields cannot be changed through profile update.
- RustFS objects remain private; another user cannot read, replace, or delete them by guessing an ID or key.
- Invalid/oversized/mislabelled files are rejected before storage; failed metadata writes do not leave untracked objects indefinitely.
- Passwords are verified and rehashed using the existing password mechanism. Plaintext passwords never appear in response, log, audit, or event payloads; the bcrypt hash appears only in protected account events.
- After a password change, every token issued earlier gets 401, and replaying the account stream restores the new credential.
- Repeated wrong current passwords get 429, even when a later guess is correct.
- Loan-contract OTP keeps going to the current email until both email-change OTPs are confirmed; afterwards both email copies match.
- Customer-facing responses contain no storage keys, including register, login, and KYC responses.
- Tests confirm that a profile save is present after reload, and that a stale `Version` returns 409 without overwriting newer data.
- Retrying an upload leaves one active document per type.
- OpenAPI and focused handler/application/storage tests cover success, validation, authz, storage failure, and compatibility paths.

Runtime acceptance must separately verify the mobile UI and the running API/RustFS path. A successful build or unit test alone does not prove file access or upload behavior in the app.

## 9. Out of scope for the first delivery

- Editing verified legal identity data without a correction/reverification workflow.
- Changing account number, balance, role, KYC result, or loan/credit decisions from the profile screen.
- Storing profile fields or passwords in RustFS.
- Public or permanent RustFS links.
- Admin profile management, account recovery, password reset by email/SMS, MFA, session-device management, or a customer-triggered global logout unless separately specified. Token invalidation after a password change is in scope.
- Extending loan application document requirements. Loan underwriting documents remain owned by the loan API unless a separate product decision moves them into the general profile document vault.

## 10. Decisions to confirm before implementation

1. Are phone, email, and address the only fields customers can edit directly in the first release?
2. Which personal documents are required in the profile independently of a loan application? The current proposal keeps income/address proofs optional and workflow-driven.
3. Should customers be able to delete an avatar or document, or only replace them?
4. Is avatar cropping required for launch, or is centered square/round display of the uploaded image sufficient?

Decided in this draft: email changes require OTPs to both the current and the new address (section 5.3), and a password change invalidates all existing tokens (section 5.4).
