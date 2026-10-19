# Requires a verified personal customer, an active verified admin, both projectors,
# and migration 000009. Each run creates a new loan and credits 10,000,000 VND.
# Account/auth responses wrap data; loan and transaction responses are direct JSON.
POST {{base_url}}/api/auth/login
Content-Type: application/json
{
  "cccdNumber": "{{customer_cccd}}",
  "password": "{{customer_password}}"
}
HTTP 200
[Captures]
customer_token: jsonpath "$.data.token" redact
customer_id: jsonpath "$.data.user.id"
balance_before: jsonpath "$.data.user.amount"
[Asserts]
jsonpath "$.data.user.role" == "user"
jsonpath "$.data.user.isVerify" == true

POST {{base_url}}/api/auth/login
Content-Type: application/json
{
  "cccdNumber": "{{admin_cccd}}",
  "password": "{{admin_password}}"
}
HTTP 200
[Captures]
admin_token: jsonpath "$.data.token" redact
admin_id: jsonpath "$.data.user.id"
[Asserts]
jsonpath "$.data.user.role" == "admin"
jsonpath "$.data.user.isVerify" == true

GET {{base_url}}/api/loans
HTTP 401

POST {{base_url}}/api/loans
Authorization: Bearer {{customer_token}}
Content-Type: application/json
Idempotency-Key: {{loan_key}}
[Options]
variable: loan_key={{newUuid}}
{
  "loanPackageId": "PERSONAL_UNSECURED_SALARY",
  "disbursementAccountId": "{{customer_id}}",
  "requestedAmount": 10000000,
  "termMonths": 12
}
HTTP 201
[Captures]
loan_id: jsonpath "$.id"
[Asserts]
jsonpath "$.status" == "draft"
jsonpath "$.customerId" == "{{customer_id}}"
jsonpath "$.requestedAmount" == 10000000

# Reuse the captured key, not newUuid, to verify create idempotency.
POST {{base_url}}/api/loans
Authorization: Bearer {{customer_token}}
Content-Type: application/json
Idempotency-Key: {{loan_key}}
{
  "loanPackageId": "PERSONAL_UNSECURED_SALARY",
  "disbursementAccountId": "{{customer_id}}",
  "requestedAmount": 10000000,
  "termMonths": 12
}
HTTP 201
[Asserts]
jsonpath "$.id" == "{{loan_id}}"

POST {{base_url}}/api/loans/{{loan_id}}/review
Authorization: Bearer {{customer_token}}
HTTP 403

POST {{base_url}}/api/loans/{{loan_id}}/disburse
Authorization: Bearer {{customer_token}}
HTTP 403

POST {{base_url}}/api/loans/{{loan_id}}/disburse
Authorization: Bearer {{admin_token}}
HTTP 422

POST {{base_url}}/api/loans/{{loan_id}}/submit
Authorization: Bearer {{customer_token}}
HTTP 422
[Asserts]
jsonpath "$.error" contains "missing required documents"

# Loan uploads inspect content bytes, so use a PDF fixture rather than media.txt.
POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: IDENTITY_FRONT
file: file,fixtures/loan.pdf; application/pdf
HTTP 201
[Captures]
document_id: jsonpath "$.documentId"
document_sha256: jsonpath "$.sha256"
[Asserts]
jsonpath "$.mimeType" == "application/pdf"
jsonpath "$.verificationStatus" == "pending"

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: IDENTITY_BACK
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: VNEID_PERSONAL
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: VNEID_RESIDENCE
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: VSSID_PARTICIPATION
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: VSSID_DETAIL
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/documents
Authorization: Bearer {{customer_token}}
[Multipart]
type: SALARY_STATEMENT_3_MONTHS
file: file,fixtures/loan.pdf; application/pdf
HTTP 201

GET {{base_url}}/api/loans/{{loan_id}}/documents/{{document_id}}
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
header "Content-Type" == "application/octet-stream"
header "X-Content-Type-Options" == "nosniff"
sha256 toHex == "{{document_sha256}}"

GET {{base_url}}/api/loans/{{loan_id}}/documents/{{newUuid}}
Authorization: Bearer {{customer_token}}
HTTP 404

POST {{base_url}}/api/loans/{{loan_id}}/submit
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "submitted"
jsonpath "$.documents" count == 7

POST {{base_url}}/api/loans/{{loan_id}}/review
Authorization: Bearer {{admin_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "under_review"
jsonpath "$.reviewStartedBy" == "{{admin_id}}"

# Incomplete evidence must leave the loan writable, without an assessment event.
POST {{base_url}}/api/loans/{{loan_id}}/evidence
Authorization: Bearer {{admin_token}}
Content-Type: application/json
{
  "monthlySalaries": [20000000, 20000000, 20000000],
  "regionCode": "KV01",
  "existingMonthlyDebt": 0,
  "insuranceMonths": 24,
  "repaymentHistory": "previously_late",
  "insuranceVerified": true,
  "historyVerified": true,
  "financialDataVerified": true,
  "documentsVerified": false,
  "sources": ["hurl-salary", "hurl-insurance", "hurl-repayment-history"]
}
HTTP 201

POST {{base_url}}/api/loans/{{loan_id}}/assess
Authorization: Bearer {{admin_token}}
HTTP 422
[Asserts]
jsonpath "$.error" == "loan assessment requires complete verified evidence"

GET {{base_url}}/api/loans/{{loan_id}}
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "under_review"
jsonpath "$.assessedAt" == null

POST {{base_url}}/api/loans/{{loan_id}}/evidence
Authorization: Bearer {{admin_token}}
Content-Type: application/json
{
  "monthlySalaries": [20000000, 20000000, 20000000],
  "regionCode": "KV01",
  "existingMonthlyDebt": 0,
  "insuranceMonths": 24,
  "repaymentHistory": "previously_late",
  "insuranceVerified": true,
  "historyVerified": true,
  "financialDataVerified": true,
  "documentsVerified": true,
  "sources": ["hurl-salary", "hurl-insurance", "hurl-repayment-history"]
}
HTTP 201
[Asserts]
jsonpath "$.verifiedBy" == "{{admin_id}}"

GET {{base_url}}/api/loans/{{loan_id}}/quote
Authorization: Bearer {{customer_token}}
HTTP 200
[Captures]
quoted_rate: jsonpath "$.annualRateBps"
[Asserts]
jsonpath "$.official" == false
jsonpath "$.provisionalSchedule" count == 12

POST {{base_url}}/api/loans/{{loan_id}}/assess
Authorization: Bearer {{customer_token}}
HTTP 403

# Verified late repayment requires a staff decision instead of auto approval.
POST {{base_url}}/api/loans/{{loan_id}}/assess
Authorization: Bearer {{admin_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "under_review"
jsonpath "$.assessmentDecision" == "manual_review"
jsonpath "$.assessedBy" == "{{admin_id}}"
jsonpath "$.assessedAt" isIsoDate

POST {{base_url}}/api/loans/{{loan_id}}/assess
Authorization: Bearer {{admin_token}}
HTTP 409

POST {{base_url}}/api/loans/{{loan_id}}/evidence
Authorization: Bearer {{admin_token}}
Content-Type: application/json
{}
HTTP 409

POST {{base_url}}/api/loans/{{loan_id}}/decision
Authorization: Bearer {{customer_token}}
Content-Type: application/json
{
  "decision": "approved",
  "reason": "Hurl permission check"
}
HTTP 403

POST {{base_url}}/api/loans/{{loan_id}}/decision
Authorization: Bearer {{admin_token}}
Content-Type: application/json
{
  "decision": "approved",
  "reason": "Hurl: verified late repayment has been resolved"
}
HTTP 200
[Asserts]
jsonpath "$.status" == "approved"
jsonpath "$.manualReviewerId" == "{{admin_id}}"

POST {{base_url}}/api/loans/{{loan_id}}/disburse
Authorization: Bearer {{admin_token}}
HTTP 200
[Captures]
disbursed_at: jsonpath "$.disbursedAt"
[Asserts]
jsonpath "$.status" == "disbursed"
jsonpath "$.annualRateBps" == {{quoted_rate}}
jsonpath "$.repaymentSchedule" count == 12
jsonpath "$.repaymentSchedule[11].RemainingBalance" == 0

GET {{base_url}}/api/auth/me
Authorization: Bearer {{customer_token}}
HTTP 200
[Captures]
balance_after: jsonpath "$.data.amount"
[Asserts]
jsonpath "$.data.amount" > {{balance_before}}

# A lost response must be safe to retry without changing money or loan terms.
POST {{base_url}}/api/loans/{{loan_id}}/disburse
Authorization: Bearer {{admin_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "disbursed"
jsonpath "$.disbursedAt" == "{{disbursed_at}}"
jsonpath "$.annualRateBps" == {{quoted_rate}}

GET {{base_url}}/api/auth/me
Authorization: Bearer {{customer_token}}
HTTP 200
[Asserts]
jsonpath "$.data.amount" == {{balance_after}}

# Retry only read requests: projectors may lag; do not auto-retry writes.
GET {{base_url}}/api/transactions
Authorization: Bearer {{customer_token}}
[Options]
retry: 20
retry-interval: 250ms
[Query]
q: {{loan_id}}
limit: 100
HTTP 200
[Asserts]
jsonpath "$.items" count == 1
jsonpath "$.items[0].id" == "{{loan_id}}"
jsonpath "$.items[0].kind" == "loan_disbursement"
jsonpath "$.items[0].direction" == "in"
jsonpath "$.items[0].amount" == 10000000
jsonpath "$.items[0].balanceAfter" == {{balance_after}}

GET {{base_url}}/api/loans
Authorization: Bearer {{customer_token}}
[Options]
retry: 20
retry-interval: 250ms
[Query]
limit: 100
HTTP 200
[Asserts]
jsonpath "$.eventualConsistency" == true
jsonpath "$.items[?(@.id == '{{loan_id}}')].status" contains "disbursed"
