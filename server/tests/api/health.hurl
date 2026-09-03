GET {{base_url}}/health
HTTP 200
[Asserts]
body == "OK"
duration < 2000

GET {{base_url}}/api/auth/me
HTTP 401
Content-Type: application/json; charset=utf-8
[Asserts]
jsonpath "$.error" == "unauthorized"
