# UI/Product Spec — Anomaly Admin Console

---

## 1. Tổng quan

| Hạng mục | Chi tiết |
|----------|----------|
| **Sản phẩm** | Desktop admin console cho ngân hàng Anomaly |
| **Mục tiêu** | Theo dõi giao dịch bất thường, điều tra tài khoản, quản lý khoản vay và duyệt KYC |
| **Form factor** | Tauri desktop app, màn hình tối thiểu 1280x800 |
| **Ngôn ngữ UI** | Tiếng Việt; mã, enum và field API dùng tiếng Anh |
| **Đối tượng** | Risk officer, loan officer, loan approver, KYC officer, auditor, manager, admin |
| **Trạng thái** | Draft - UI/API contract |

Tài liệu này chỉ mô tả:

- Cấu trúc màn hình và navigation.
- Nội dung, component và trạng thái UI.
- User interaction và permission ở mức giao diện.
- Các API cần gọi.
- Response data tối thiểu mà backend phải trả để render UI.

Tài liệu này **không mô tả** database schema, migration, Go package, event projection, Kafka, KurrentDB implementation hoặc anomaly detection engine.

---

## 2. Phạm vi màn hình

### 2.1 P0

1. Admin login.
2. Anomaly monitoring dashboard.
3. Alert queue.
4. Alert detail và triage.
5. Account investigation/event timeline.
6. Time-travel và replay status.
7. Loan package list/detail/form.
8. Loan application queue/detail/review.
9. Repayment schedule.
10. Manual KYC queue/detail.
11. Transaction/risk reports.

### 2.2 P2

1. Customer list/detail.
2. Staff list/detail.
3. Role/permission matrix.
4. Settings.

### 2.3 Ngoài phạm vi UI

- Màn hình cấu hình anomaly rules.
- Màn hình xây anomaly detection engine.
- Màn hình sửa event gốc.
- Màn hình quản trị database hoặc projection worker.
- Mobile responsive layout. Admin tối ưu desktop; tablet chỉ cần không vỡ layout.

---

## 3. Information architecture

```text
Admin Console
├── Tổng quan rủi ro
│   ├── Dashboard
│   ├── Cảnh báo
│   └── Tài khoản rủi ro cao
├── Điều tra
│   ├── Event explorer
│   ├── Account timeline
│   ├── Time-travel / Replay
│   └── Projection failures
├── Khoản vay
│   ├── Gói vay
│   ├── Hồ sơ vay
│   └── Quá hạn
├── KYC
│   └── Hàng đợi duyệt KYC
├── Báo cáo
│   ├── Giao dịch
│   ├── Rủi ro
│   └── Danh mục khoản vay
└── Quản trị (P2)
    ├── Khách hàng
    ├── Nhân viên
    └── Vai trò & quyền
```

### 3.1 Desktop shell

- Sidebar trái rộng 248–264px, có trạng thái expanded/collapsed.
- Topbar cao 64px: breadcrumb, global search, time range, notification, staff menu.
- Main content có max-width khoảng 1360px và grid 12 cột.
- Alert detail và account context mở bằng right drawer 400–480px khi phù hợp.
- Table dùng sticky header, toolbar cố định khi scroll.
- Không dùng bottom navigation hoặc mobile bottom sheet.

### 3.2 Navigation item

Mỗi item có `label`, `icon`, `route`, `permission`, `badge` tùy chọn. Badge cảnh báo hiển thị số alert `new`/`critical`, không hiển thị số quá lớn trực tiếp; từ 1000 trở lên dùng `999+`.

---

## 4. Quy ước UI chung

### 4.1 Trạng thái bắt buộc

Mỗi màn hình data-driven phải có:

- Loading: skeleton theo đúng hình dạng content.
- Empty: giải thích nguyên nhân + action tiếp theo.
- Error: message ngắn + retry.
- Permission denied: nêu quyền còn thiếu, không render dữ liệu nhạy cảm.
- Stale data: hiển thị thời điểm cập nhật gần nhất và nút refresh.

### 4.2 Table conventions

- Cột tiền căn phải, có phân tách hàng nghìn và đơn vị `₫`.
- Số liệu dùng tabular numerals.
- Status dùng badge có text, không chỉ dùng màu.
- Row click mở detail; action menu không được trigger row click.
- Filter state phải giữ được khi back/forward.
- Hỗ trợ search, filter, sort, column visibility và cursor pagination.
- Bulk action luôn có số lượng item và confirm dialog.

### 4.3 Date/time

- Backend trả RFC 3339 UTC.
- UI hiển thị theo `Asia/Ho_Chi_Minh`.
- Mọi chart có time range rõ ràng.
- Khoảng thời gian mặc định của dashboard: 24 giờ gần nhất.

### 4.4 Permission behavior

Permission ảnh hưởng đến UI như sau:

| Tình huống | UI behavior |
|------------|-------------|
| Không có quyền xem route | Chuyển tới `#/admin/403` |
| Không có quyền xem một field | Ẩn hoặc mask field |
| Không có quyền action | Ẩn action hoặc disable kèm tooltip |
| Có quyền xem nhưng không có quyền export | Ẩn nút export |
| Action nhạy cảm | Bắt buộc reason + confirmation |

UI không thay thế backend authorization.

---

## 5. API conventions

### 5.1 Base path

Các API admin dùng prefix `/api/admin`.

### 5.2 Common response

List response:

```json
{
  "items": [],
  "nextCursor": "opaque-cursor-or-null",
  "total": 120,
  "updatedAt": "2026-10-01T08:00:00Z"
}
```

`total` có thể bỏ qua với queue lớn nếu backend không tính hiệu quả; UI không được phụ thuộc bắt buộc vào field này.

Error response:

```json
{
  "error": "Không thể thực hiện thao tác ở trạng thái hiện tại",
  "code": "INVALID_STATE",
  "requestId": "req_01J..."
}
```

### 5.3 Field conventions

- JSON request/response dùng camelCase.
- ID là string.
- Tiền là integer VND, không dùng float.
- Giá trị phần trăm trả dạng string decimal, ví dụ `"12.50"`.
- `null` dùng cho dữ liệu chưa có; không dùng chuỗi rỗng thay cho null.
- Enum trả bằng lowercase snake case.

### 5.4 Query conventions

```text
?q=&status=&from=&to=&sort=&direction=asc&limit=20&cursor=
```

`limit` mặc định 20, tối đa 100. Filter nhiều giá trị có thể truyền comma-separated.

### 5.5 API freshness

Các response dashboard/list nên trả `updatedAt` hoặc `lastEventAt` để UI hiển thị "Cập nhật lúc...". Nếu dữ liệu eventual consistency, backend cần trả `dataStatus: "fresh" | "stale"` khi cần.

---

## 6. Authentication và session UI

### 6.1 Màn hình `#/admin/login`

**Mục đích:** Cho nhân viên đăng nhập admin.

**UI:**

- Logo/brand.
- Input email hoặc username.
- Password input có show/hide.
- TOTP input chỉ hiển thị khi backend yêu cầu.
- Remember device tùy policy.
- Error inline và error banner cho lỗi hệ thống.

**API:**

```text
POST /api/admin/auth/login
POST /api/admin/auth/refresh
POST /api/admin/auth/logout
GET  /api/admin/auth/me
GET  /api/admin/auth/permissions
```

**Login response tối thiểu:**

```json
{
  "accessToken": "<token>",
  "expiresAt": "2026-10-01T08:30:00Z",
  "refreshToken": "<token>",
  "mfaRequired": false,
  "staff": {
    "id": "staff_001",
    "employeeCode": "EMP-0001",
    "fullName": "Nguyen Van A",
    "email": "risk@example.com",
    "roles": ["risk_officer"],
    "permissions": ["alert.read", "alert.triage", "audit.read"],
    "branch": { "id": "HCM-01", "name": "Chi nhanh Ho Chi Minh" },
    "avatarUrl": null
  }
}
```

**Session UI cần biết:**

- `expiresAt` để cảnh báo session sắp hết hạn.
- `mfaRequired` để chuyển sang bước OTP.
- `permissions` để render navigation/action.
- `staff` để render topbar và audit context.

---

## 7. Dashboard giám sát bất thường

### 7.1 Route

`#/admin/monitor`

### 7.2 Mục đích

Cho risk officer biết hệ thống đang có bao nhiêu rủi ro, rủi ro tăng/giảm như thế nào và alert nào cần xử lý ngay.

### 7.3 Layout

1. Header: title, time range, refresh, live/polling indicator.
2. KPI row: 4–5 stat cards.
3. Risk trend chart: line/area theo thời gian.
4. Alert severity distribution.
5. Alert type distribution.
6. Top risky accounts table.
7. Recent alert feed.

### 7.4 KPI cần hiển thị

- Alert mới trong khoảng thời gian.
- Alert critical/high đang mở.
- Tổng giá trị giao dịch liên quan.
- Số tài khoản đang điều tra.
- Tỷ lệ false positive, nếu backend có dữ liệu.

### 7.5 API

```text
GET /api/admin/risk/overview?from=&to=
GET /api/admin/risk/timeseries?from=&to=&bucket=1h
GET /api/admin/risk/distribution?from=&to=&groupBy=severity
GET /api/admin/risk/top-accounts?from=&to=&limit=10
GET /api/admin/alerts?status=new,investigating&limit=8
```

**`GET /risk/overview` response:**

```json
{
  "range": { "from": "2026-10-01T00:00:00Z", "to": "2026-10-01T08:00:00Z" },
  "updatedAt": "2026-10-01T08:00:05Z",
  "metrics": {
    "newAlerts": { "value": 38, "changePercent": 12.5, "trend": "up" },
    "openHighRiskAlerts": { "value": 7, "changePercent": -8.2, "trend": "down" },
    "amountAtRisk": { "value": 128500000, "currency": "VND", "changePercent": 4.1, "trend": "up" },
    "openInvestigations": { "value": 12, "changePercent": 0, "trend": "flat" },
    "falsePositiveRate": { "value": 8.4, "unit": "percent", "changePercent": -2.1, "trend": "down" }
  }
}
```

**`GET /risk/timeseries` response:**

```json
{
  "bucket": "1h",
  "series": [
    { "timestamp": "2026-10-01T07:00:00Z", "alertCount": 12, "highRiskCount": 3, "riskScoreAverage": 61 },
    { "timestamp": "2026-10-01T08:00:00Z", "alertCount": 18, "highRiskCount": 5, "riskScoreAverage": 68 }
  ]
}
```

**Top account item:**

```json
{
  "accountId": "acc_001",
  "accountNo": "ACC-001234",
  "customerName": "Nguyen Van A",
  "riskScore": 92,
  "openAlertCount": 4,
  "amountAtRisk": 45000000,
  "lastAlertAt": "2026-10-01T07:55:00Z"
}
```

---

## 8. Alert queue và alert detail

### 8.1 Alert queue

**Route:** `#/admin/alerts`

**UI:**

- Filter: severity, status, alert type, account, date range, assignee.
- Search theo alert ID, account number, transaction ID.
- Table columns: severity, alert type, account, amount, risk score, occurred time, status, assignee.
- Bulk assign/status cho các alert được chọn.
- Row click mở `#/admin/alerts/:id`.

**API:**

```text
GET /api/admin/alerts?severity=&status=&alertType=&q=&accountId=&assigneeId=&from=&to=&limit=&cursor=
PATCH /api/admin/alerts/{id}
POST /api/admin/alerts/{id}/notes
POST /api/admin/alerts/{id}/escalate
```

**Alert list item:**

```json
{
  "id": "alert_001",
  "severity": "critical",
  "status": "investigating",
  "alertType": "velocity",
  "ruleCode": "VELOCITY_10M",
  "title": "Nhiều giao dịch liên tiếp trong thời gian ngắn",
  "account": { "id": "acc_001", "accountNo": "ACC-001234", "customerName": "Nguyen Van A" },
  "riskScore": 92,
  "amount": 45000000,
  "currency": "VND",
  "transactionCount": 8,
  "occurredAt": "2026-10-01T07:55:00Z",
  "detectedAt": "2026-10-01T07:56:02Z",
  "assignee": { "id": "staff_002", "name": "Tran Thi B" },
  "lastUpdatedAt": "2026-10-01T08:01:00Z"
}
```

### 8.2 Alert detail

**Route:** `#/admin/alerts/:id`

**Layout:**

1. Header: alert ID, severity, status, created time.
2. Summary card: account, customer, risk score, amount.
3. Risk factors/evidence.
4. Related transactions.
5. Alert timeline.
6. Notes.
7. Sticky action panel: acknowledge, assign, investigate, escalate, resolve, false positive.

**API:**

```text
GET /api/admin/alerts/{id}
GET /api/admin/alerts/{id}/timeline
GET /api/admin/alerts/{id}/transactions
```

**Detail response:**

```json
{
  "id": "alert_001",
  "severity": "critical",
  "status": "investigating",
  "title": "Nhiều giao dịch liên tiếp trong thời gian ngắn",
  "description": "8 giao dịch trong 10 phút vượt ngưỡng thông thường.",
  "riskScore": 92,
  "account": {
    "id": "acc_001",
    "accountNo": "ACC-001234",
    "customerId": "cus_001",
    "customerName": "Nguyen Van A",
    "kycStatus": "verified",
    "accountStatus": "active"
  },
  "metrics": { "amount": 45000000, "currency": "VND", "transactionCount": 8, "windowMinutes": 10 },
  "riskFactors": [
    { "code": "HIGH_VELOCITY", "label": "Tần suất giao dịch cao", "severity": "high", "value": "8 giao dịch / 10 phút" }
  ],
  "relatedTransactions": [],
  "timeline": [],
  "notes": [],
  "assignee": null,
  "availableActions": ["acknowledge", "assign", "escalate", "resolve", "false_positive"]
}
```

Action response trả alert detail mới nhất, không chỉ trả `204`, để UI cập nhật ngay.

---

## 9. Account investigation và audit trail

### 9.1 Event explorer

**Route:** `#/admin/audit/events`

**UI:**

- Search account number, customer ID, transaction ID, event ID.
- Filter event type, stream, actor, date range.
- Table: timestamp, event type, stream, actor, revision, correlation ID.
- Expand row để xem payload đã mask.

**API:**

```text
GET /api/admin/audit/events?q=&streamId=&eventType=&actorId=&from=&to=&limit=&cursor=
GET /api/admin/audit/streams/{streamId}
```

**Event item:**

```json
{
  "eventId": "evt_001",
  "eventType": "TransactionCompleted",
  "streamId": "account-acc_001",
  "aggregateId": "acc_001",
  "revision": 42,
  "occurredAt": "2026-10-01T07:55:00Z",
  "actor": { "type": "customer", "id": "cus_001", "displayName": "Nguyen Van A" },
  "correlationId": "req_001",
  "summary": "Chuyển tiền 5,000,000₫",
  "payload": { "amount": 5000000, "currency": "VND", "status": "success" },
  "payloadMasked": false
}
```

### 9.2 Account investigation workspace

**Route:** `#/admin/audit/accounts/:id`

**Layout 3 pane:**

- Left: account summary, customer, KYC, balance/status, risk score.
- Center: chronological event timeline with revision marker.
- Right: selected event detail, projection diff, state-at-time tabs.

**API:**

```text
GET /api/admin/accounts/{id}/summary
GET /api/admin/audit/accounts/{id}/events?fromRevision=&toRevision=&limit=&cursor=
GET /api/admin/audit/accounts/{id}/diff
GET /api/admin/audit/accounts/{id}/state-at?revision=&at=
```

**Summary response:**

```json
{
  "account": {
    "id": "acc_001",
    "accountNo": "ACC-001234",
    "customerName": "Nguyen Van A",
    "status": "active",
    "balance": 128500000,
    "currency": "VND",
    "kycStatus": "verified",
    "openedAt": "2026-01-10T08:00:00Z"
  },
  "risk": { "score": 92, "openAlertCount": 4, "lastAlertAt": "2026-10-01T07:55:00Z" },
  "projection": { "version": 42, "lastProjectedAt": "2026-10-01T08:00:01Z", "status": "in_sync" },
  "availableActions": ["view_state_at", "dry_run_replay"]
}
```

### 9.3 Time-travel

**UI:**

- Revision/timestamp slider.
- Event marker trên timeline.
- State snapshot tại mốc đã chọn.
- So sánh current state và historical state.
- Label rõ `Trạng thái lịch sử - không dùng để thực hiện giao dịch`.

**API:**

```text
GET /api/admin/audit/accounts/{id}/state-at?revision=42
GET /api/admin/audit/accounts/{id}/state-at?at=2026-10-01T07:55:00Z
```

**Response:**

```json
{
  "accountId": "acc_001",
  "asOf": { "revision": 42, "timestamp": "2026-10-01T07:55:00Z" },
  "state": {
    "status": "active",
    "balance": 128500000,
    "currency": "VND",
    "kycStatus": "verified"
  },
  "eventCountApplied": 42,
  "isHistorical": true
}
```

### 9.4 Replay console

**Route:** `#/admin/audit/replay`

**UI:**

- Chọn account.
- Chọn revision range.
- Nút `Dry run`.
- Diff preview trước/sau.
- Nút `Apply replay` chỉ hiện khi có quyền.
- Reason bắt buộc.
- Confirmation dialog cảnh báo đây là thao tác sửa read model.
- Replay history và status.

**API:**

```text
POST /api/admin/audit/replay
GET  /api/admin/audit/replay/{runId}
GET  /api/admin/audit/replay?scopeId=&status=&limit=&cursor=
```

**Create replay request:**

```json
{
  "scope": "account",
  "scopeId": "acc_001",
  "mode": "dry_run",
  "fromRevision": 0,
  "toRevision": 42,
  "reason": "Đối chiếu projection sau cảnh báo lệch số dư"
}
```

**Replay status response:**

```json
{
  "runId": "replay_001",
  "status": "succeeded",
  "mode": "dry_run",
  "scope": { "type": "account", "id": "acc_001", "accountNo": "ACC-001234" },
  "progress": { "processed": 42, "total": 42, "percent": 100 },
  "diff": [
    { "field": "balance", "currentValue": 125000000, "replayedValue": 128500000, "change": 3500000 }
  ],
  "requestedBy": { "id": "staff_001", "name": "Tran Thi B" },
  "reason": "Đối chiếu projection sau cảnh báo lệch số dư",
  "createdAt": "2026-10-01T08:01:00Z",
  "completedAt": "2026-10-01T08:01:03Z"
}
```

### 9.5 Projection failures

**Route:** `#/admin/audit/projection-failures`

**API:**

```text
GET  /api/admin/audit/projection-failures?status=&eventType=&from=&to=&limit=&cursor=
POST /api/admin/audit/projection-failures/{eventId}/retry
```

Table cần: event ID, stream, event type, error summary, failed time, attempts, status, retry action.

---

## 10. Loan package screens

### 10.1 Package list

**Route:** `#/admin/loan-packages`

**UI:**

- Header + `Tạo gói vay`.
- Tabs/status: tất cả, draft, active, archived.
- Search theo code/name.
- Table hoặc card-grid tùy density.
- Columns: code, name, amount range, term range, interest, active applications, status, updated time.

**API:**

```text
GET /api/admin/loan-packages?status=&q=&limit=&cursor=
GET /api/admin/loan-packages/{code}
```

**List item:**

```json
{
  "code": "VAY-TIEU-DUNG-01",
  "name": "Vay tiêu dùng linh hoạt",
  "status": "active",
  "amount": { "min": 5000000, "max": 100000000, "currency": "VND" },
  "termMonths": { "min": 3, "max": 24 },
  "interest": { "annualRate": "12.50", "method": "declining_balance" },
  "fee": { "processing": 100000, "rate": "0.50" },
  "usage": { "applicationCount": 128, "approvedCount": 74, "disbursedAmount": 4200000000 },
  "effectiveFrom": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-09-30T08:00:00Z"
}
```

### 10.2 Package form

**Routes:** `#/admin/loan-packages/new`, `#/admin/loan-packages/:code/edit`

**Sections:**

1. Basic info: code, name, description.
2. Amount/term limits.
3. Interest method/rate.
4. Fee and late fee.
5. Required documents.
6. Effective period.
7. Live repayment preview.

**API:**

```text
POST  /api/admin/loan-packages
PATCH /api/admin/loan-packages/{code}
POST  /api/admin/loan-packages/preview-amortization
POST  /api/admin/loan-packages/{code}/publish
POST  /api/admin/loan-packages/{code}/archive
```

Preview request/response:

```json
{
  "amount": 30000000,
  "termMonths": 12,
  "annualRate": "12.50",
  "method": "declining_balance",
  "processingFee": 100000
}
```

```json
{
  "summary": { "principal": 30000000, "interest": 2100000, "fees": 100000, "totalPayable": 32200000 },
  "installments": [
    { "no": 1, "dueDate": "2026-11-01", "principal": 2400000, "interest": 312500, "fee": 0, "total": 2712500, "remaining": 27600000 }
  ]
}
```

---

## 11. Loan application screens

### 11.1 Application queue

**Route:** `#/admin/loan-applications`

**UI:**

- Pipeline tabs: tất cả, chờ thẩm định, chờ phê duyệt, đã duyệt, đã giải ngân, đang trả, quá hạn.
- Filter: amount, package, officer, date, delinquency.
- Table: application no, customer, package, requested amount, stage, assigned officer, submitted time, risk badge.
- Quick action tùy permission: assign, open review.

**API:**

```text
GET /api/admin/loan-applications?status=&stage=&packageCode=&officerId=&delinquency=&from=&to=&q=&limit=&cursor=
GET /api/admin/loan-applications/stats
GET /api/admin/loan-applications/approvals/pending
```

**List item:**

```json
{
  "applicationNo": "APP-2026-000001",
  "customer": { "id": "cus_001", "name": "Nguyen Van A", "accountNo": "ACC-001234" },
  "package": { "code": "VAY-TIEU-DUNG-01", "name": "Vay tiêu dùng linh hoạt" },
  "requestedAmount": 30000000,
  "approvedAmount": null,
  "termMonths": 12,
  "status": "under_review",
  "stage": "review",
  "delinquencyStatus": null,
  "assignedOfficer": { "id": "staff_003", "name": "Le Van C" },
  "riskScore": 42,
  "submittedAt": "2026-09-30T08:00:00Z",
  "updatedAt": "2026-10-01T07:00:00Z",
  "availableActions": ["open_review", "reassign"]
}
```

### 11.2 Application detail/review

**Routes:**

- `#/admin/loan-applications/:no`
- `#/admin/loan-applications/:no/review`

**Detail tabs:**

1. Tổng quan.
2. Hồ sơ và checklist.
3. Risk context.
4. Lịch trả.
5. Lịch sử quyết định.
6. Event/audit history.

**Review layout:**

- Left: customer/application summary.
- Center: checklist và evidence.
- Right: reviewer note, risk summary, submit review action.

**API:**

```text
GET  /api/admin/loan-applications/{no}
POST /api/admin/loan-applications/{no}/assign
POST /api/admin/loan-applications/{no}/checklist/{itemId}
POST /api/admin/loan-applications/{no}/review-submit
POST /api/admin/loan-applications/{no}/approve
POST /api/admin/loan-applications/{no}/reject
POST /api/admin/loan-applications/{no}/disburse
```

**Detail response tối thiểu:**

```json
{
  "applicationNo": "APP-2026-000001",
  "status": "under_review",
  "stage": "review",
  "customer": {
    "id": "cus_001",
    "name": "Nguyen Van A",
    "accountNo": "ACC-001234",
    "kycStatus": "verified",
    "creditScore": 742
  },
  "package": { "code": "VAY-TIEU-DUNG-01", "name": "Vay tiêu dùng linh hoạt", "version": 2 },
  "request": { "amount": 30000000, "termMonths": 12, "purpose": "Mua thiết bị" },
  "checklist": [
    { "id": "identity", "label": "KYC hợp lệ", "status": "passed", "required": true },
    { "id": "income", "label": "Chứng minh thu nhập", "status": "pending", "required": true }
  ],
  "risk": { "score": 42, "level": "medium", "openAlertCount": 0, "flags": [] },
  "review": { "officer": { "id": "staff_003", "name": "Le Van C" }, "note": null },
  "approval": null,
  "availableActions": ["update_checklist", "submit_review", "reject"]
}
```

Reject/approve/disburse response trả lại application detail mới nhất để tránh UI phải gọi lại detail.

### 11.3 Repayment schedule

**Route:** `#/admin/loan-applications/:no/schedule`

**API:**

```text
GET /api/admin/loan-applications/{no}/schedule
GET /api/admin/loan-applications/{no}/schedule/summary
```

**Response:**

```json
{
  "applicationNo": "APP-2026-000001",
  "summary": {
    "principal": 30000000,
    "interest": 2100000,
    "fees": 100000,
    "paid": 5425000,
    "outstanding": 26775000,
    "nextDueDate": "2026-11-01",
    "delinquencyStatus": "current"
  },
  "installments": [
    {
      "no": 1,
      "dueDate": "2026-11-01",
      "principal": 2400000,
      "interest": 312500,
      "fee": 0,
      "total": 2712500,
      "paidAmount": 2712500,
      "status": "paid",
      "paidAt": "2026-10-30T10:00:00Z"
    }
  ]
}
```

---

## 12. Manual KYC screens

### 12.1 KYC queue

**Route:** `#/admin/kyc`

**UI:**

- Tabs: pending, escalated, recently reviewed.
- Filter: submitted time, confidence, reason, branch.
- Table: session ID, customer, submitted time, OCR status, face match score, liveness, priority.
- Row click mở compare view.

**API:**

```text
GET /api/admin/kyc-reviews?status=&q=&from=&to=&priority=&limit=&cursor=
```

**List item:**

```json
{
  "sessionId": "kyc_001",
  "customer": { "id": "cus_001", "name": "Nguyen Van A", "emailMasked": "ng***@mail.com" },
  "status": "pending",
  "priority": "high",
  "submittedAt": "2026-10-01T07:30:00Z",
  "verification": { "ocr": "passed", "liveness": "review", "faceMatch": "review", "faceMatchScore": 0.71 },
  "reasonCodes": ["LOW_FACE_MATCH"],
  "availableActions": ["open"]
}
```

### 12.2 KYC compare view

**Route:** `#/admin/kyc/:sessionId`

**Layout:**

- Left: CCCD front/back media.
- Center: liveness video/face image.
- Right: declared customer fields vs OCR fields.
- Bottom: provider scores, failure reasons, reviewer notes, approve/reject/escalate.

**API:**

```text
GET  /api/admin/kyc-reviews/{sessionId}
GET  /api/admin/kyc-reviews/{sessionId}/media/{kind}
POST /api/admin/kyc-reviews/{sessionId}/approve
POST /api/admin/kyc-reviews/{sessionId}/reject
POST /api/admin/kyc-reviews/{sessionId}/escalate
```

**Detail response:**

```json
{
  "sessionId": "kyc_001",
  "status": "pending",
  "customer": {
    "id": "cus_001",
    "declared": { "fullName": "Nguyen Van A", "identityNumber": "079*********", "dateOfBirth": "1999-01-01" }
  },
  "ocr": { "fullName": "NGUYEN VAN A", "identityNumber": "079*********", "dateOfBirth": "1999-01-01", "confidence": 0.98 },
  "verification": {
    "ocrStatus": "passed",
    "livenessStatus": "review",
    "faceMatchStatus": "review",
    "faceMatchScore": 0.71,
    "provider": "kyc-provider"
  },
  "media": {
    "identityFront": { "url": "<short-lived-signed-url>", "expiresAt": "2026-10-01T08:10:00Z", "mimeType": "image/jpeg" },
    "identityBack": { "url": "<short-lived-signed-url>", "expiresAt": "2026-10-01T08:10:00Z", "mimeType": "image/jpeg" },
    "liveness": { "url": "<short-lived-signed-url>", "expiresAt": "2026-10-01T08:10:00Z", "mimeType": "video/mp4" }
  },
  "diffs": [
    { "field": "fullName", "declared": "Nguyen Van A", "ocr": "NGUYEN VAN A", "status": "match" }
  ],
  "availableActions": ["approve", "reject", "escalate"]
}
```

Media URL phải ngắn hạn và chỉ được trả sau khi API đã kiểm tra permission.

---

## 13. Reports

### 13.1 Transaction report

**Route:** `#/admin/reports/transactions`

**UI:**

- Date range bắt buộc.
- Group by: tài khoản, thời gian, loại giao dịch.
- Filter: account, transaction type, status, direction.
- KPI: tổng giao dịch, tổng tiền vào, tổng tiền ra, số giao dịch bất thường.
- Chart + table.
- Export CSV.

**API:**

```text
GET /api/admin/reports/transactions?from=&to=&groupBy=account|time|type&accountId=&transactionType=&status=&limit=&cursor=
GET /api/admin/reports/transactions/export?from=&to=&groupBy=&format=csv
```

**Response:**

```json
{
  "range": { "from": "2026-09-01T00:00:00Z", "to": "2026-10-01T00:00:00Z", "timezone": "Asia/Ho_Chi_Minh" },
  "groupBy": "type",
  "summary": {
    "transactionCount": 18240,
    "totalIn": 8500000000,
    "totalOut": 7900000000,
    "anomalousCount": 38,
    "currency": "VND"
  },
  "series": [
    { "key": "transfer", "label": "Chuyển tiền", "count": 12000, "amount": 5400000000, "anomalousCount": 21 }
  ],
  "rows": []
}
```

### 13.2 Risk report

**Route:** `#/admin/reports/risk`

```text
GET /api/admin/reports/risk-summary?from=&to=&groupBy=severity|type|account
```

Response cần có summary, chart series, top accounts và resolution breakdown.

### 13.3 Loan portfolio report

```text
GET /api/admin/reports/loan-portfolio?from=&to=&packageCode=&branchId=
```

Response cần có total disbursed, outstanding principal, approval rate, repayment rate, delinquency buckets và series theo thời gian.

---

## 14. P2 screens

### 14.1 Customer list/detail

```text
GET /api/admin/customers?q=&kycStatus=&status=&limit=&cursor=
GET /api/admin/customers/{id}
GET /api/admin/customers/{id}/accounts
GET /api/admin/customers/{id}/alerts
GET /api/admin/customers/{id}/loans
GET /api/admin/customers/{id}/audit-trail
```

UI cần hiển thị: profile, KYC status, account balances, risk alerts, loan applications, audit summary và freeze/unfreeze action nếu có quyền.

### 14.2 Staff list/detail

```text
GET   /api/admin/staff?q=&status=&role=&limit=&cursor=
GET   /api/admin/staff/{id}
POST  /api/admin/staff
PATCH /api/admin/staff/{id}
POST  /api/admin/staff/{id}/suspend
POST  /api/admin/staff/{id}/reactivate
GET   /api/admin/staff/roles
```

Staff list response tối thiểu: ID, employee code, name, email masked, roles, branch, status, last login, created time.

### 14.3 Role matrix

**Route:** `#/admin/staff/roles`

```text
GET /api/admin/staff/roles
```

```json
{
  "roles": [
    {
      "key": "risk_officer",
      "label": "Giám sát rủi ro",
      "permissions": ["alert.read", "alert.triage", "alert.escalate", "audit.read", "report.read"]
    }
  ],
  "permissions": [
    { "key": "alert.read", "label": "Xem cảnh báo", "group": "Anomaly" }
  ]
}
```

---

## 15. Các component cần thiết

### 15.1 Foundation

- Admin shell/sidebar/topbar.
- Breadcrumb.
- Button, icon button, input, textarea, select.
- Date range picker.
- Tabs, dropdown, tooltip.
- Badge/status pill.
- Modal, drawer, confirmation dialog.

### 15.2 Data display

- Data table với sorting/filter/selection/pagination.
- Stat card có trend.
- Timeline event.
- Stepper cho loan lifecycle.
- Diff viewer.
- JSON/payload viewer đã mask.
- Currency/percentage/date cells.
- Skeleton, empty state, error state.

### 15.3 Data visualization

- Risk line/area chart.
- Severity stacked bar/area.
- Category bar chart.
- Donut chỉ dùng khi số category ít.
- Chart tooltip phải hiển thị label, value, timestamp.
- Mỗi chart có text summary hoặc table fallback.

### 15.4 Domain components

- `AlertSeverityBadge`.
- `AlertStatusBadge`.
- `RiskScoreBadge`.
- `AccountSummaryCard`.
- `TransactionAmountCell`.
- `LoanStatusStepper`.
- `KycComparisonField`.
- `ReplayDiffTable`.
- `PermissionGate`.

---

## 16. Design direction

- Giữ nhận diện Ethereal Neo-Banking: navy `#1e3a8a`, violet accent `#8b5cf6`, font Be Vietnam Pro.
- Admin ưu tiên data density và readability hơn glassmorphism.
- Surface chính sáng, tương phản rõ; glass chỉ dùng ở login/topbar nếu không ảnh hưởng khả năng đọc.
- Radius nhỏ hơn consumer app: khoảng 8–14px.
- Severity colors phải có text đi kèm:
  - Low: blue.
  - Medium: amber.
  - High: orange.
  - Critical: red.
- Trạng thái success dùng green; không dùng green cho trạng thái đang xử lý.
- Focus ring rõ, keyboard navigation cho table/action/dialog.
- Không biểu diễn rủi ro chỉ bằng màu sắc.

Chi tiết token, spacing, typography, responsive behavior và component visual sẽ nằm trong `admin/DESIGN.md`.

---

## 17. Definition of Done cho UI/API contract

1. Mỗi P0 route có wireframe/layout và screen states.
2. Mỗi table có API list item schema, filter và pagination contract.
3. Mỗi detail/action screen có API detail schema và action response.
4. Dashboard có đủ data cho KPI, chart và top account table.
5. Alert detail có đủ evidence, transaction links, timeline và available actions.
6. Investigation có event timeline, state-at-time, diff và replay status.
7. Loan review có checklist, customer context, risk context và approval state.
8. KYC detail có media metadata, OCR comparison và decision actions.
9. Reports có summary, series, rows và export contract.
10. Error/loading/empty/permission states được thiết kế cho toàn bộ P0.
11. Backend có thể implement API theo contract mà không cần phụ thuộc vào cách UI lưu state.
12. UI không giả định backend storage, event store hay anomaly engine cụ thể.

---

## 18. Các điểm cần BE xác nhận

| # | Câu hỏi | Ảnh hưởng UI |
|---|---------|--------------|
| API-01 | Alert engine gửi alert qua endpoint nào và format evidence ra sao? | Alert detail/ingestion |
| API-02 | Dữ liệu transaction thật đã sẵn sàng chưa? | Dashboard/report |
| API-03 | Dashboard cần polling hay SSE? | Refresh indicator |
| API-04 | Response media dùng signed URL hay authenticated stream? | KYC compare |
| API-05 | Replay apply trả diff trước khi confirm hay tạo job dry-run? | Replay console |
| API-06 | Loan schedule tính theo flat hay declining balance trong MVP? | Preview/schedule |
| API-07 | Quyền export PII thuộc role nào? | Reports/customer detail |
| API-08 | Có branch filter trong P0 không? | Global filter và report |
| API-09 | Tên transaction types/status thực tế là gì? | Filter/badge/report |
| API-10 | Chuẩn error code chung của backend là gì? | Toast/inline error mapping |

---

*Phiên bản: 0.2 — tập trung vào UI, UX và API response contract.*
*Trạng thái: Draft — chờ BE xác nhận API và tiếp tục sang `admin/DESIGN.md`.*
