# Register writes the account aggregate directly to MongoDB.
POST {{base_url}}/api/auth/register
{
  "username": "hurl-{{newUuid}}",
  "email": "Hurl User <hurl-{{newUuid}}@example.com>",
  "password": "hurl-password-123"
}
HTTP 201
Content-Type: application/json
[Captures]
account_id: jsonpath "$.id"
account_username: jsonpath "$.username"
account_email: jsonpath "$.email"
[Asserts]
jsonpath "$.id" matches /^[0-9a-f]{24}$/
jsonpath "$.username" startsWith "hurl-"
jsonpath "$.email" endsWith "@example.com"
jsonpath "$.email" not contains "<"
jsonpath "$.isVerify" == false
jsonpath "$.amount" == 0

# Retry briefly while the API and MongoDB complete the registration request.
POST {{base_url}}/api/auth/login
[Options]
retry: 20
retry-interval: 500ms
{
  "login": "{{account_email}}",
  "password": "hurl-password-123"
}
HTTP 200
Content-Type: application/json
[Captures]
access_token: jsonpath "$.token" redact
[Asserts]
jsonpath "$.token" isString
jsonpath "$.expiresAt" isIsoDate
jsonpath "$.user.id" == "{{account_id}}"
jsonpath "$.user.username" == "{{account_username}}"
jsonpath "$.user.email" == "{{account_email}}"

GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.email" == "{{account_email}}"

GET {{base_url}}/api/accounts/{{account_id}}
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.idCardFrontUrl" not exists
jsonpath "$.idCardBackUrl" not exists
jsonpath "$.liveVideoUrl" not exists

GET {{base_url}}/api/accounts/by-email/{{account_email}}
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.id" == "{{account_id}}"

GET {{base_url}}/api/accounts
Authorization: Bearer {{access_token}}
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$[*].id" contains "{{account_id}}"

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
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.isVerify" == true
jsonpath "$.idCardFrontUrl" == "media/hurl-id-front.jpg"

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
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.isVerify" == true
jsonpath "$.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
jsonpath "$.idCardBackUrl" == "media/hurl-id-back-v2.jpg"
jsonpath "$.liveVideoUrl" == "media/hurl-live-video-v2.mp4"

GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
[Options]
retry: 20
retry-interval: 500ms
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.isVerify" == true
jsonpath "$.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
jsonpath "$.idCardBackUrl" == "media/hurl-id-back-v2.jpg"
jsonpath "$.liveVideoUrl" == "media/hurl-live-video-v2.mp4"

# Once the projection exists, registering the same identity is rejected.
POST {{base_url}}/api/auth/register
{
  "username": "{{account_username}}",
  "email": "{{account_email}}",
  "password": "hurl-password-123"
}
HTTP 409
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "user already exists"

POST {{base_url}}/api/auth/register
{
  "username": "valid-user",
  "email": "not-an-email",
  "password": "hurl-password-123"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "invalid email"

POST {{base_url}}/api/auth/register
{
  "username": "valid-user",
  "email": "hurl-{{newUuid}}@example.com",
  "password": "short"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "password must be at least 8 characters"

POST {{base_url}}/api/auth/register
{
  "username": "   ",
  "email": "hurl-{{newUuid}}@example.com",
  "password": "hurl-password-123"
}
HTTP 400
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "username must not be empty"

POST {{base_url}}/api/auth/register
{
  "unexpected": true
}
HTTP 400
Content-Type: application/json

POST {{base_url}}/api/auth/login
{
  "login": "{{account_email}}",
  "password": "wrong-password"
}
HTTP 401
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "invalid credentials"

GET {{base_url}}/api/accounts/{{newUuid}}
Authorization: Bearer {{access_token}}
HTTP 404
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "account not found"

GET {{base_url}}/api/accounts/by-email/missing-{{newUuid}}@example.com
Authorization: Bearer {{access_token}}
HTTP 404
Content-Type: application/json
[Asserts]
jsonpath "$.error" == "account not found"

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
jsonpath "$.error" == "user mismatch"

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
jsonpath "$.error" == "all identity URLs are required"

# Verify that the latest KYC session is projected back into MongoDB.
GET {{base_url}}/api/auth/me
Authorization: Bearer {{access_token}}
[Options]
retry: 20
retry-interval: 500ms
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.id" == "{{account_id}}"
jsonpath "$.isVerify" == true
jsonpath "$.idCardFrontUrl" == "media/hurl-id-front-v2.jpg"
