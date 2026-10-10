POST {{base_url}}/api/media/upload
[Multipart]
kind: id-card-front
file: file,fixtures/media.txt; text/plain
HTTP 404

POST {{base_url}}/api/kyc/media
HTTP 404

POST {{base_url}}/api/kyc/verify
HTTP 404

GET {{base_url}}/api/media/download
[Query]
key: any-object
HTTP 401
Content-Type: application/json
