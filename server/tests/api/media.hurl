POST {{base_url}}/api/media/upload
[Multipart]
key: hurl/{{newUuid}}.txt
file: file,fixtures/media.txt; text/plain
HTTP 201
Content-Type: application/json
[Captures]
media_key: jsonpath "$.data.key"
[Asserts]
jsonpath "$.data.key" startsWith "hurl/"
jsonpath "$.data.key" endsWith ".txt"

GET {{base_url}}/api/media/download
[Query]
key: {{media_key}}
HTTP 200
Content-Type: text/plain
file,fixtures/media.txt;
