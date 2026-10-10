POST {{base_url}}/api/auth/register
{
  "username": "hurl-{{newUuid}}",
  "cccdNumber": "{{cccd}}",
  "cccdIssuedDate": "2020-06-15T00:00:00Z",
  "dob": "1995-03-20T00:00:00Z",
  "email": "hurl-{{newUuid}}@example.com",
  "password": "hurl-password-123"
}
HTTP 201
Content-Type: application/json
[Captures]
account_id: jsonpath "$.data.user.id"
kyc_token: jsonpath "$.data.kycToken" redact
[Asserts]
jsonpath "$.data.user.isVerify" == false
jsonpath "$.data.kycToken" isString
jsonpath "$.data.kycExpiresAt" isIsoDate

POST {{base_url}}/api/auth/login
{
  "cccdNumber": "{{cccd}}",
  "password": "hurl-password-123"
}
HTTP 403
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "KYC_REQUIRED"

POST {{base_url}}/api/kyc/session
{
  "cccdNumber": "{{cccd}}",
  "password": "hurl-password-123"
}
HTTP 200
Content-Type: application/json
[Captures]
resumed_kyc_token: jsonpath "$.data.kycToken" redact
[Asserts]
jsonpath "$.data.user.id" == "{{account_id}}"
jsonpath "$.data.kycToken" isString
jsonpath "$.data.kycExpiresAt" isIsoDate

POST {{base_url}}/api/kyc/session
{
  "cccdNumber": "{{cccd}}",
  "password": "wrong-password"
}
HTTP 401
Content-Type: application/json
[Asserts]
jsonpath "$.errors[0].code" == "INVALID_CREDENTIALS"

# Invalid signatures are rejected before forwarding or storage.
POST {{base_url}}/api/kyc/complete
Authorization: Bearer {{resumed_kyc_token}}
[Multipart]
idCardFront: file,fixtures/media.txt; image/jpeg
idCardBack: file,fixtures/media.txt; image/jpeg
liveVideo: file,fixtures/media.txt; video/mp4
challengeType: TURN_HEAD_LEFT_RIGHT_BLINK
HTTP 400
Content-Type: application/json

POST {{base_url}}/api/kyc/media
Authorization: Bearer {{kyc_token}}
HTTP 404

POST {{base_url}}/api/kyc/verify
Authorization: Bearer {{kyc_token}}
{
  "idCardFrontUrl": "kyc/other/front",
  "idCardBackUrl": "kyc/other/back",
  "liveVideoUrl": "kyc/other/video"
}
HTTP 404
