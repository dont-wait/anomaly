# Requires a running API and `make seed` (demo customer + SYSTEM_TEST notification).

POST {{base_url}}/api/auth/login
Content-Type: application/json
{
  "cccdNumber": "{{customer_cccd}}",
  "password": "{{customer_password}}"
}
HTTP 200
[Captures]
customer_token: jsonpath "$.data.token" redact

GET {{base_url}}/api/notifications
HTTP 401

GET {{base_url}}/api/notifications?limit=51
Authorization: Bearer {{customer_token}}
HTTP 400

GET {{base_url}}/api/notifications?limit=20
Authorization: Bearer {{customer_token}}
HTTP 200
[Captures]
notification_id: jsonpath "$.data.items[0].id"
[Asserts]
jsonpath "$.data.items" count >= 1
jsonpath "$.data.items[0].title" exists
jsonpath "$.data.nextCursor" exists

GET {{base_url}}/api/notifications/unread-count
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.data.count" >= 0

PATCH {{base_url}}/api/notifications/{{notification_id}}/read
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.data.read" == true

PATCH {{base_url}}/api/notifications/does-not-exist/read
Authorization: Bearer {{customer_token}}
HTTP 404

POST {{base_url}}/api/notifications/read-all
Authorization: Bearer {{customer_token}}
Content-Type: application/json
{}
HTTP 200

GET {{base_url}}/api/notifications/unread-count
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.data.count" == 0
