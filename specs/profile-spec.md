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

The initial list above does not mean every product currently requires proof of income/address. The UI should show those upload slots only when backend policy/configuration says they are required or supported.

### 3.6 Change password

- Require current password, new password, and confirmation in the UI. Never prefill or persist password fields.
- Server verifies the current password and applies the existing password policy before changing the hash.
- Do not return the password, hash, or a new token in the response.
- On success, clear password inputs and show confirmation. Password change must be audit-logged without recording secrets.
- Token revocation/session invalidation policy must be explicitly implemented and documented. Until revocation support exists, keep the current token behavior and rely on its expiration; do not claim other sessions were logged out.

## 4. Data ownership and storage

| Data | System of record | Storage rule |
| --- | --- | --- |
| Name, DOB, identity data, contact and address | Existing account/customer domain and persistence/event path | Structured fields; never RustFS |
| Password | Existing account credential field | bcrypt hash only; never RustFS |
| Avatar/document bytes | RustFS | Private objects, server-generated keys |
| Media metadata and ownership/type/status | Account/profile persistence associated with current account | No raw bytes, credentials, or public object URL |

Account writes must go through the established account write path so EventStoreDB/account state and Mongo projections remain consistent. Do not update a projection directly as the source of truth. The exact event/schema migration strategy is a backend design decision and must preserve older account records.

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

All endpoints require the normal account bearer token, use the standard success/error envelope, return `Cache-Control: no-store` for sensitive profile responses, and derive the account ID from auth claims. Clients cannot choose an account ID.

### 5.1 Read current profile

`GET /api/auth/me`

Extend the existing response with `avatar` and `documents` metadata while retaining existing field names and meanings. Mask identity values in the customer-facing profile response unless a screen has a justified need for the full value.

### 5.2 Update editable profile fields

`PATCH /api/auth/me/profile`

Request fields are partial and limited to:

```json
{
  "phone": "+84901234567",
  "email": "customer@example.com",
  "address": {
    "line": "12 Nguyen Trai",
    "provinceCode": "HCM"
  }
}
```

Reject unknown/protected fields (including identity, role, account number, balance, and KYC status). Response returns the updated profile representation. Email uniqueness/verification semantics must follow the existing account policy; if email changes require verification, represent it as pending and do not treat it as verified contact data.

### 5.3 Change password

`PUT /api/auth/me/password`

Request:

```json
{
  "currentPassword": "...",
  "newPassword": "..."
}
```

Return a generic success message. Never echo either password. Use the same password strength rules as account creation and return a generic invalid-current-password error.

### 5.4 Upload avatar

`POST /api/auth/me/avatar` (`multipart/form-data`, field `file`)

Returns the new avatar metadata. Replacement is atomic from the client's perspective: do not publish the new reference until RustFS upload and account metadata update succeed.

### 5.5 Upload personal document

`POST /api/auth/me/documents` (`multipart/form-data`, fields `type`, `file`)

Allowed initial types: `IDENTITY_FRONT`, `IDENTITY_BACK`, `INCOME_PROOF`, `ADDRESS_PROOF`. Server-side state/KYC policy determines whether upload, replacement, or deletion is allowed.

### 5.6 List/get/delete media

- `GET /api/auth/me/documents` returns the caller's metadata list.
- `GET /api/auth/me/media/{mediaId}` streams the file with safe content headers and owner checks. Preview is inline only for allow-listed image/PDF types; force attachment otherwise.
- `DELETE /api/auth/me/documents/{mediaId}` is allowed only for replaceable, unverified documents. Avatar removal may be a separate `DELETE /api/auth/me/avatar` operation.

Use opaque media IDs and account-scoped authorization. Do not add a generic “download arbitrary key” endpoint.

### 5.7 Error behavior

Use consistent error envelopes and distinguish at least: invalid input (400), unauthenticated (401), forbidden mutation/state (403), missing media/profile (404), file too large (413), unsupported or detected type (415), conflict (409), and storage/internal failure (500/503). Avoid exposing bucket/key or storage-provider errors.

## 6. Frontend PR scope

- Update the profile drawer into a profile overview using the Savings visual language (Be Vietnam Pro, pale gradient background, rounded white cards, clear status chips and pill actions).
- Add focused edit screens/sheets for contact/address, avatar selection/preview, document upload/preview, and password change.
- Keep form state local to the screen; do not persist passwords or document bytes in localStorage/sessionStorage.
- Use the API client and token handling already present. Refresh profile state after successful mutations.
- Add loading, empty, upload progress, retry, validation, and success/error states.
- Do not include unrelated loan/savings changes in this PR.

## 7. Backend PR scope

- Add authenticated self-service routes through the `openapi.Registry`; never accept an account ID from the customer request.
- Add account application commands for editable profile updates and password change, using the established account persistence/event path.
- Add private profile-media metadata and repository operations backed by the existing RustFS client.
- Add strict multipart limits, file signature validation, server-generated keys, safe response headers, owner checks, and cleanup behavior for failed/replaced objects.
- Reuse existing verified KYC media metadata where appropriate without weakening the current verification/download rules.
- Ensure private responses are not cached and sensitive values are excluded from logs/audit payloads.
- Preserve compatibility with existing `/api/auth/me`, registration, login, KYC, and loan document contracts.

## 8. Acceptance criteria

### Frontend

- Opening/closing profile works as an overlay and does not unexpectedly navigate away from the current screen.
- Customer sees only their own profile and masked identity/account values.
- Editable contact/address fields save through the API and show server-confirmed results.
- Verified identity fields cannot be edited through the normal form.
- Avatar and document uploads show accepted types/limits, preview/progress, and actionable errors.
- Password change requires current password and matching new-password confirmation; secrets are cleared after submit.
- No success state appears when the backend rejects or fails a mutation.

### Backend

- Self-service routes always operate on the authenticated account, including when a request attempts to inject another account ID.
- Protected account, financial, role, identity/KYC status, and verification fields cannot be changed through profile update.
- RustFS objects remain private; another user cannot read, replace, or delete them by guessing an ID or key.
- Invalid/oversized/mislabelled files are rejected before storage; failed metadata writes do not leave untracked objects indefinitely.
- Passwords are verified and rehashed using the existing password mechanism; secrets never appear in response/log/event payloads.
- OpenAPI and focused handler/application/storage tests cover success, validation, authz, storage failure, and compatibility paths.

Runtime acceptance must separately verify the mobile UI and the running API/RustFS path. A successful build or unit test alone does not prove file access or upload behavior in the app.

## 9. Out of scope for the first delivery

- Editing verified legal identity data without a correction/reverification workflow.
- Changing account number, balance, role, KYC result, or loan/credit decisions from the profile screen.
- Storing profile fields or passwords in RustFS.
- Public or permanent RustFS links.
- Admin profile management, account recovery, password reset by email/SMS, MFA, session-device management, or global logout unless separately specified.
- Extending loan application document requirements. Loan underwriting documents remain owned by the loan API unless a separate product decision moves them into the general profile document vault.

## 10. Decisions to confirm before implementation

1. Are phone, email, and address the only fields customers can edit directly in the first release?
2. Does changing email require OTP verification before the new email becomes active?
3. Which personal documents are required in the profile independently of a loan application? The current proposal keeps income/address proofs optional and workflow-driven.
4. Should customers be able to delete an avatar or document, or only replace them?
5. Should password change revoke all other sessions? Current JWT/session infrastructure must be checked before promising this behavior.
6. Is avatar cropping required for launch, or is centered square/round display of the uploaded image sufficient?
