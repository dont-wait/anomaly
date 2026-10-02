# Register writes the account aggregate directly to MongoDB.
POST {{base_url}}/api/auth/register
{
  "username": "hurl-{{newUuid}}",
  "cccdNumber": "{{cccd}}",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "email": "Hurl User <hurl-{{newUuid}}@example.com>",
  "password": "hurl-password-123"
}
HTTP 201
Content-Type: application/json
[Captures]
account_id: jsonpath "$.data.id"
account_username: jsonpath "$.data.username"
account_email: jsonpath "$.data.email"
[Asserts]
jsonpath "$.status" == 201
jsonpath "$.data.id" matches /^[0-9a-f]{24}$/
jsonpath "$.data.username" startsWith "hurl-"
jsonpath "$.data.email" endsWith "@example.com"
jsonpath "$.data.email" not contains "<"
jsonpath "$.data.role" == "user"
jsonpath "$.data.isVerify" == false
jsonpath "$.data.amount" == 0

# Retry briefly while the API and MongoDB complete the registration request.
POST {{base_url}}/api/auth/login
[Options]
retry: 20
retry-interval: 500ms
{
  "cccdNumber": "{{cccd}}",
  "password": "hurl-password-123"
}
HTTP 200
Content-Type: application/json
[Captures]
access_token: jsonpath "$.data.token" redact
[Asserts]
jsonpath "$.data.token" isString
jsonpath "$.data.expiresAt" isIsoDate
jsonpath "$.data.user.id" == "{{account_id}}"
jsonpath "$.data.user.username" == "{{account_username}}"
jsonpath "$.data.user.email" == "{{account_email}}"
jsonpath "$.data.user.role" == "user"

POST {{base_url}}/api/auth/login
{
  "cccdNumber": "{{admin_cccd}}",
  "password": "{{admin_password}}"
}
HTTP 200
Content-Type: application/json
[Captures]
admin_access_token: jsonpath "$.data.token" redact
admin_account_id: jsonpath "$.data.user.id"
[Asserts]
jsonpath "$.data.user.role" == "admin"

GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.email" == "{{account_email}}"

GET {{base_url}}/api/accounts/{{account_id}}
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.idCardFrontUrl" not exists
jsonpath "$.data.idCardBackUrl" not exists
jsonpath "$.data.liveVideoUrl" not exists

GET {{base_url}}/api/accounts/by-email/{{account_email}}
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"

GET {{base_url}}/api/accounts/{{admin_account_id}}
Authorization: Bearer {{access_token}}
HTTP 404
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "ACCOUNT_NOT_FOUND"

GET {{base_url}}/api/accounts
Authorization: Bearer {{access_token}}
HTTP 403
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "FORBIDDEN"

GET {{base_url}}/api/accounts
Authorization: Bearer {{admin_access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data[*].id" contains "{{account_id}}"

GET {{base_url}}/api/accounts/{{account_id}}
Authorization: Bearer {{admin_access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"

POST {{base_url}}/api/accounts/{{account_id}}/verify
Authorization: Bearer {{access_token}}
{
  "idCardFrontUrl": "media/hurl-id-front.jpg",
  "idCardBackUrl": "media/hurl-id-back.jpg",
  "liveVideoUrl": "media/hurl-live-video.mp4"
}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.isVerify" == true
jsonpath "$.data.idCardFrontUrl" == "media/hurl-id-front.jpg"

# Re-KYC creates a new session and exposes the latest verified media.
POST {{base_url}}/api/accounts/{{account_id}}/verify
Authorization: Bearer {{access_token}}
{
  "idCardFrontUrl": "media/hurl-id-front-v2.jpg",
  "idCardBackUrl": "media/hurl-id-back-v2.jpg",
  "liveVideoUrl": "media/hurl-live-video-v2.mp4"
}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.isVerify" == true
jsonpath "$.data.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
jsonpath "$.data.idCardBackUrl" == "media/hurl-id-back-v2.jpg"
jsonpath "$.data.liveVideoUrl" == "media/hurl-live-video-v2.mp4"

GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
[Options]
retry: 20
retry-interval: 500ms
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.isVerify" == true
jsonpath "$.data.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
jsonpath "$.data.idCardBackUrl" == "media/hurl-id-back-v2.jpg"
jsonpath "$.data.liveVideoUrl" == "media/hurl-live-video-v2.mp4"

# Once the projection exists, registering the same identity is rejected.
POST {{base_url}}/api/auth/register
{
  "username": "{{account_username}}",
  "cccdNumber": "{{cccd}}",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "email": "{{account_email}}",
  "password": "hurl-password-123"
}
HTTP 409
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "USER_ALREADY_EXISTS"

POST {{base_url}}/api/auth/register
{
  "username": "valid-user",
  "email": "not-an-email",
  "cccdNumber": "123456789012",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "password": "hurl-password-123"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_EMAIL"

POST {{base_url}}/api/auth/register
{
  "username": "valid-user",
  "cccdNumber": "123456789012",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "email": "hurl-{{newUuid}}@example.com",
  "password": "short"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "WEAK_PASSWORD"

POST {{base_url}}/api/auth/register
{
  "username": "   ",
  "cccdNumber": "123456789012",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "email": "hurl-{{newUuid}}@example.com",
  "password": "hurl-password-123"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_USERNAME"

POST {{base_url}}/api/auth/register
{
  "username": "valid-user",
  "email": "hurl-{{newUuid}}@example.com",
  "cccdNumber": "123",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "password": "hurl-password-123"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_CCCD"

POST {{base_url}}/api/auth/register
{
  "unexpected": true
}
HTTP 400
Content-Type: application/json

POST {{base_url}}/api/auth/login
{
  "cccdNumber": "{{cccd}}",
  "password": "wrong-password"
}
HTTP 401
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_CREDENTIALS"

GET {{base_url}}/api/accounts/{{newUuid}}
Authorization: Bearer {{access_token}}
HTTP 404
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "ACCOUNT_NOT_FOUND"

GET {{base_url}}/api/accounts/by-email/missing-{{newUuid}}@example.com
Authorization: Bearer {{access_token}}
HTTP 404
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "ACCOUNT_NOT_FOUND"

POST {{base_url}}/api/accounts/{{newUuid}}/verify
Authorization: Bearer {{access_token}}
{
  "idCardFrontUrl": "media/hurl-id-front.jpg",
  "idCardBackUrl": "media/hurl-id-back.jpg",
  "liveVideoUrl": "media/hurl-live-video.mp4"
}
HTTP 403
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "USER_MISMATCH"

POST {{base_url}}/api/accounts/{{account_id}}/verify
Authorization: Bearer {{access_token}}
{
  "idCardFrontUrl": "",
  "idCardBackUrl": "media/hurl-id-back.jpg",
  "liveVideoUrl": "media/hurl-live-video.mp4"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_VERIFY_PAYLOAD"

# Verify that the latest KYC session is projected back into MongoDB.
GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
[Options]
retry: 20
retry-interval: 500ms
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.data.id" == "{{account_id}}"
jsonpath "$.data.isVerify" == true
jsonpath "$.data.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
