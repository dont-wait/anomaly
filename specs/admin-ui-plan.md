# UI-First Implementation Plan — Anomaly Admin Console

---

## 1. Mục tiêu

Xây dựng giao diện admin desktop trước khi kết nối backend thật.

UI giai đoạn đầu sử dụng mock data và mock API nhưng response shape bám theo
`specs/admin-spec.md`. Khi backend hoàn thành, chỉ thay implementation của API
adapter; page và component không cần viết lại.

### 1.1 Nguyên tắc

- Tạo admin app riêng trong `admin/`, không trộn với consumer app trong `client/`.
- UI không truy cập trực tiếp MongoDB, Redis, EventStoreDB hoặc Kafka.
- Mock API mô phỏng cả Query API và Command API.
- Mọi màn hình data-driven phải có loading, empty, error, stale-data và permission-denied state.
- Xây vertical slice hoàn chỉnh trước, không làm tất cả màn hình rời rạc cùng lúc.
- Desktop là form factor chính: chuẩn `1440x900`, tối thiểu `1280x800`.

### 1.2 Không nằm trong UI phase

- Kết nối backend thật.
- Database schema hoặc migration.
- Projection worker và Kafka consumer.
- Anomaly detection engine.
- Sửa event gốc trong EventStoreDB.
- Logic authorization thật ở server.

---

## 2. Thứ tự triển khai

```text
1. Visual direction và design tokens
2. Stitch prototype cho các màn hình lõi
3. Scaffold admin Tauri app
4. Admin shell và mock authentication
5. Shared UI primitives
6. Anomaly monitoring vertical slice
7. Investigation/audit vertical slice
8. Loan UI
9. KYC UI
10. Reports
11. P2 customer/staff UI
12. Polish, testing và API handoff
```

Mốc review chính là sau bước 6. Nếu dashboard, alert queue và alert detail đã
ổn về visual language, các màn hình còn lại sẽ tái sử dụng được phần lớn shell,
table, filter, drawer và status components.

---

## 3. Phase 1 — Visual direction

### 3.1 Deliverable

Tạo `admin/DESIGN.md` với:

- Desktop shell.
- Color và surface tokens.
- Typography.
- Spacing và radius.
- Table density.
- Chart style.
- Severity/risk/status colors.
- Component states.
- Accessibility rules.
- Responsive behavior ở 1280–1920px.

### 3.2 Visual foundation

- Giữ Be Vietnam Pro.
- Giữ nhận diện navy `#1e3a8a` và violet `#8b5cf6`.
- Dùng surface phẳng, tương phản rõ, ưu tiên khả năng đọc bảng.
- Chỉ dùng glass effect ở login hoặc topbar nếu không ảnh hưởng readability.
- Radius chính khoảng 8–14px, nhỏ hơn consumer app.
- Severity luôn có text, không dùng màu đơn độc:
  - Low: blue.
  - Medium: amber.
  - High: orange.
  - Critical: red.
- Số tiền dùng tabular numerals và căn phải.

### 3.3 Shell layout

- Sidebar trái: 248–264px, có trạng thái collapsed 72px.
- Topbar: 64px.
- Main content: grid 12 cột, max-width khoảng 1360px.
- Right drawer: 400–480px cho context detail.
- Table có sticky header và toolbar cố định khi scroll.
- Không dùng bottom navigation hoặc mobile bottom sheet.

---

## 4. Phase 2 — Stitch prototype

### 4.1 Batch đầu tiên

Sinh và review 6 màn hình:

1. Admin login.
2. Anomaly monitoring dashboard.
3. Alert queue.
4. Alert detail.
5. Account investigation/event timeline.
6. Loan application review.

### 4.2 Batch thứ hai

Sau khi duyệt batch đầu:

1. Loan package list/form.
2. Repayment schedule.
3. KYC compare view.
4. Transaction report.
5. Replay/time-travel.
6. Customer/staff screens P2.

### 4.3 Mục tiêu Stitch

Stitch dùng để chốt:

- Layout.
- Content hierarchy.
- Density.
- Chart/table style.
- Drawer/dialog pattern.
- Status và severity presentation.

Stitch không phải source code chính; code UI sẽ được viết theo design tokens và
component rules đã duyệt.

---

## 5. Phase 3 — Scaffold admin app

### 5.1 Cấu trúc dự kiến

```text
admin/
├── package.json
├── vite.config.ts
├── tsconfig.json
├── src/
│   ├── app/
│   │   ├── App.tsx
│   │   ├── routes.ts
│   │   └── app.css
│   ├── components/
│   ├── features/
│   │   ├── auth/
│   │   ├── monitor/
│   │   ├── alerts/
│   │   ├── investigation/
│   │   ├── loans/
│   │   ├── kyc/
│   │   └── reports/
│   ├── mock/
│   ├── shared/
│   │   ├── api/
│   │   ├── formatters/
│   │   └── permissions/
│   ├── index.css
│   └── main.tsx
└── src-tauri/
```

### 5.2 Stack

- React 19.
- TypeScript.
- Vite.
- Tailwind CSS 4.
- Tauri 2.
- Font Awesome theo convention hiện tại.
- Recharts hoặc chart library tương đương cho dashboard/report.
- Testing Library + Vitest.

Không thêm table/form/state library lớn ngay từ đầu. Trước tiên xây primitive
nhỏ phù hợp với các screen thực tế; chỉ thêm dependency khi có nhu cầu rõ ràng.

### 5.3 Route strategy

Admin dùng route namespace riêng:

```text
#/admin/login
#/admin/monitor
#/admin/alerts
#/admin/alerts/:id
#/admin/audit/events
#/admin/audit/accounts/:id
#/admin/audit/replay
#/admin/loan-packages
#/admin/loan-applications
#/admin/loan-applications/:no
#/admin/kyc
#/admin/kyc/:sessionId
#/admin/reports/transactions
#/admin/403
```

Router phải hỗ trợ:

- Nested route.
- Route guard.
- Back/forward.
- Deep link khi chạy trong Tauri WebView.
- Giữ filter state trên URL/hash khi phù hợp.

---

## 6. Phase 4 — Admin shell và mock authentication

### 6.1 Shell components

- `AdminShell`.
- `Sidebar`.
- `Topbar`.
- `Breadcrumb`.
- `GlobalSearch`.
- `NotificationMenu`.
- `StaffMenu`.
- `PermissionGate`.
- `PageHeader`.

### 6.2 Mock auth behavior

Mô phỏng:

- Login thành công.
- Login thất bại.
- Session expired.
- Staff profile.
- Permission list.
- Logout.
- Route `403`.

Mock staff:

```ts
{
  id: "staff_001",
  employeeCode: "EMP-0001",
  fullName: "Tran Thi B",
  role: "risk_officer",
  permissions: [
    "alert.read",
    "alert.triage",
    "alert.escalate",
    "audit.read",
    "report.read"
  ]
}
```

Permission chỉ dùng để điều khiển UI trong prototype. Backend sẽ enforce quyền
thật ở phase tích hợp API.

---

## 7. Phase 5 — Shared UI primitives

### 7.1 Foundation

- Button.
- Icon button.
- Input.
- Textarea.
- Select.
- Date range picker.
- Tabs.
- Dropdown.
- Tooltip.
- Badge.
- Modal.
- Drawer.
- Confirm dialog.
- Toast.

### 7.2 Data display

- `DataTable`.
- `TableToolbar`.
- `FilterBar`.
- `Pagination`.
- `StatusBadge`.
- `SeverityBadge`.
- `RiskScore`.
- `AmountCell`.
- `DateTimeCell`.
- `EmptyState`.
- `ErrorState`.
- `Skeleton`.
- `StaleDataBanner`.

### 7.3 Domain components

- `StatCard`.
- `RiskTrendChart`.
- `SeverityChart`.
- `CategoryBarChart`.
- `MiniSparkline`.
- `Timeline`.
- `DiffViewer`.
- `PayloadViewer`.
- `AccountSummaryCard`.
- `LoanStatusStepper`.
- `KycComparisonField`.
- `ReplayDiffTable`.

Mỗi primitive phải có trạng thái disabled, loading hoặc error phù hợp trước khi
dùng trong domain screen.

---

## 8. Phase 6 — Anomaly monitoring vertical slice

Đây là vertical slice đầu tiên và phải hoàn chỉnh từ route đến mock data.

### 8.1 Dashboard

Route:

```text
#/admin/monitor
```

UI:

- Time range selector.
- Refresh button.
- Last updated/stale indicator.
- KPI cards.
- Risk trend chart.
- Severity distribution.
- Alert type distribution.
- Top risky accounts.
- Recent alert feed.

Mock API:

```text
getRiskOverview()
getRiskTimeseries()
getRiskDistribution()
getTopRiskyAccounts()
listAlerts()
```

### 8.2 Alert queue

Route:

```text
#/admin/alerts
```

UI:

- Search alert ID, account number, transaction ID.
- Filter severity, status, type, account, assignee, date.
- Sortable table.
- Cursor pagination mock.
- Row selection.
- Bulk assignment/status action.
- Loading, empty, error và stale states.

Mock API:

```text
listAlerts(params)
assignAlert(id, input)
updateAlertStatus(id, input)
```

### 8.3 Alert detail

Route:

```text
#/admin/alerts/:id
```

UI:

- Alert header.
- Account summary.
- Risk score và risk factors.
- Related transactions.
- Alert timeline.
- Notes.
- Sticky action panel.

Mock API:

```text
getAlertDetail(id)
getAlertTimeline(id)
getRelatedTransactions(id)
addAlertNote(id, input)
escalateAlert(id, input)
```

Sau mỗi command mock, trả về detail mới nhất để UI cập nhật mà không cần giả
định response `204`.

---

## 9. Phase 7 — Investigation vertical slice

### 9.1 Event explorer

Route:

```text
#/admin/audit/events
```

UI:

- Search theo account, transaction, event ID.
- Filter event type, stream, actor, date.
- Event table.
- Expand payload đã mask.

Mock API:

```text
searchEvents(params)
getStreamEvents(streamId, params)
```

### 9.2 Account investigation

Route:

```text
#/admin/audit/accounts/:id
```

Layout 3 pane:

- Account summary.
- Event timeline.
- Selected event/detail/diff/state tabs.

Mock API:

```text
getAccountSummary(id)
getAccountEvents(id, params)
getProjectionDiff(id)
getHistoricalState(id, params)
```

### 9.3 Time-travel và replay

Route:

```text
#/admin/audit/replay
```

UI:

- Chọn account.
- Chọn revision/time range.
- Dry-run.
- Diff preview.
- Apply confirmation.
- Reason input.
- Replay progress/status/history.

Mock API:

```text
createReplayRun(input)
getReplayStatus(runId)
listReplayRuns(params)
```

Replay UI phải hiển thị rõ đây là historical/read-model operation, không phải
giao dịch tài chính.

---

## 10. Phase 8 — Loan UI

### 10.1 Loan package

Routes:

```text
#/admin/loan-packages
#/admin/loan-packages/new
#/admin/loan-packages/:code/edit
```

Thứ tự làm:

1. List với status tabs.
2. Detail.
3. Form nhiều section.
4. Repayment preview.
5. Publish/archive confirmation.

Mock API:

```text
listLoanPackages(params)
getLoanPackage(code)
createLoanPackage(input)
updateLoanPackage(code, input)
previewAmortization(input)
publishLoanPackage(code)
archiveLoanPackage(code)
```

### 10.2 Loan application

Routes:

```text
#/admin/loan-applications
#/admin/loan-applications/:no
#/admin/loan-applications/:no/review
#/admin/loan-applications/:no/schedule
```

UI:

- Pipeline tabs.
- Queue filters.
- Customer/risk context.
- Checklist.
- Reviewer note.
- Approval/rejection dialog.
- Loan lifecycle stepper.
- Repayment schedule.

Mock API:

```text
listLoanApplications(params)
getLoanApplication(applicationNo)
assignLoanApplication(applicationNo, input)
updateChecklist(applicationNo, itemId, input)
submitLoanReview(applicationNo, input)
approveLoanApplication(applicationNo, input)
rejectLoanApplication(applicationNo, input)
getRepaymentSchedule(applicationNo)
```

Mọi action mock phải thay đổi status hoặc detail response để kiểm tra được
transition UI.

---

## 11. Phase 9 — KYC UI

### 11.1 KYC queue

Route:

```text
#/admin/kyc
```

UI:

- Pending/escalated/recently reviewed tabs.
- Filter priority, score, reason, date.
- Queue table.
- Priority và confidence indicators.

### 11.2 KYC compare view

Route:

```text
#/admin/kyc/:sessionId
```

UI:

- CCCD front/back.
- Liveness media.
- OCR vs declared fields.
- Face match/liveness/OCR scores.
- Approve/reject/escalate actions.
- Media loading/error/expired state.

Mock API:

```text
listKycReviews(params)
getKycReview(sessionId)
getKycMedia(sessionId, kind)
approveKyc(sessionId, input)
rejectKyc(sessionId, input)
escalateKyc(sessionId, input)
```

Media URL trong mock phải có `expiresAt` để test expired-media state.

---

## 12. Phase 10 — Reports

Routes:

```text
#/admin/reports/transactions
#/admin/reports/risk
#/admin/reports/loan-portfolio
```

### 12.1 Transaction report

- Date range.
- Group by account/time/type.
- Summary KPI.
- Chart.
- Result table.
- Export mock state.

### 12.2 Risk report

- Severity/type/account grouping.
- Alert trend.
- Resolution breakdown.
- Top risky accounts.

### 12.3 Loan portfolio

- Total disbursed.
- Outstanding principal.
- Approval rate.
- Repayment rate.
- Delinquency buckets.

Mock API:

```text
getTransactionReport(params)
exportTransactionReport(params)
getRiskReport(params)
getLoanPortfolioReport(params)
```

---

## 13. Phase 11 — P2 UI

Chỉ làm sau khi P0 hoàn chỉnh:

- Customer list/detail.
- Staff list/detail.
- Role/permission matrix.
- Settings.

Các màn hình này phải tái sử dụng shell, table, drawer, badge, permission gate
và detail layout đã có.

---

## 14. Mock API architecture

Tách component khỏi data source ngay từ đầu:

```text
admin/src/
├── features/alerts/
│   ├── api/
│   │   ├── alerts.ts
│   │   └── alerts.mock.ts
│   ├── model/
│   └── pages/
└── shared/api/
    ├── types.ts
    ├── mockTransport.ts
    └── errors.ts
```

API function giữ interface giống API thật:

```ts
listAlerts(params): Promise<AlertListResponse>
getAlertDetail(id): Promise<AlertDetail>
updateAlertStatus(id, input): Promise<AlertDetail>
```

Mock transport cần hỗ trợ:

- Delay giả lập.
- Ép loading lâu.
- Ép 4xx/5xx.
- Empty result.
- Stale data.
- Cursor pagination.
- Command success trả về resource mới nhất.

Khi backend sẵn sàng, thay:

```text
alerts.mock.ts → alerts.http.ts
```

Không thay đổi page/component contract.

---

## 15. Testing và verification

### 15.1 Mỗi phase

```text
npm run lint
npm run build
npm run test
```

### 15.2 UI test tối thiểu

- Route rendering.
- Navigation và deep link.
- Loading/empty/error states.
- Filter và pagination.
- Permission gating.
- Alert status action.
- Replay confirmation/status.
- Loan checklist/action.
- KYC decision action.
- Report filter/export state.

### 15.3 Visual review

Kiểm tra ở:

- `1280x800`.
- `1440x900`.
- `1920x1080`.

Kiểm tra thêm:

- Table không tràn ngang ngoài ý muốn.
- Sidebar collapsed không làm mất context.
- Drawer không che action chính.
- Dialog có thể đóng bằng Escape.
- Focus ring và keyboard navigation hoạt động.
- Chart có text/table fallback.
- Severity/status không phụ thuộc duy nhất vào màu.

---

## 16. API handoff sau UI prototype

Sau khi UI hoàn chỉnh, bàn giao cho BE:

1. Danh sách route UI sử dụng.
2. API method/path/query.
3. Request payload.
4. Response DTO tối thiểu.
5. Error code mà UI cần map.
6. Permission cần cho từng route/action.
7. Loading/stale/empty assumptions.
8. Tần suất refresh dashboard/alert queue.
9. Contract cho async replay/export job.
10. Mock fixture để BE dùng làm contract test.

Nguồn API contract chính vẫn là `specs/admin-spec.md`; file này chỉ mô tả thứ
tự và cách triển khai UI.

---

## 17. Definition of Done

- Admin app chạy độc lập bằng Tauri/Vite.
- Shell desktop hoàn chỉnh ở 1280x800 trở lên.
- P0 routes có thể navigate bằng mock data.
- Dashboard, alert queue và alert detail hoạt động như một vertical slice.
- Investigation có timeline, historical state, diff và replay status.
- Loan package/application có list, detail, review và schedule.
- KYC có queue và compare view.
- Reports có chart, table, filter và export mock state.
- Tất cả P0 screen có loading/empty/error/permission state.
- Mock API interfaces bám API contract trong `specs/admin-spec.md`.
- `lint`, `build` và `test` chạy thành công.
- Không có page nào phụ thuộc trực tiếp vào MongoDB, Redis, EventStoreDB hoặc Kafka.

---

*Phiên bản: 0.1*
*Trạng thái: UI-first implementation plan*

---

## 18. GitHub Issue/PR workflow

Quy ước của project: tạo một GitHub Issue trước khi bắt đầu, sau đó mỗi Issue
được triển khai bằng đúng một Pull Request.

### 18.1 Nguyên tắc chia Issue

Một Issue phải đại diện cho một kết quả UI có thể review được:

> Sau khi merge PR này, user thấy hoặc thao tác được thêm điều gì?

Không chia Issue theo từng component nhỏ hoặc từng API riêng lẻ. Cũng không gom
nhiều domain độc lập như loan, KYC và reports vào cùng một PR.

Mỗi Issue cần ghi rõ:

- Goal.
- Scope.
- Out of scope.
- Routes/screens.
- Mock API và fixture cần dùng.
- Acceptance criteria.
- Dependencies.
- Link tới design/Stitch nếu có.

### 18.2 Milestone M0 — Foundation

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Admin UI design foundation | Design tokens, layout, typography, table/chart rules, `admin/DESIGN.md` | Design document và link Stitch prototype | Không |
| Admin app scaffold | Tạo `admin/` với React/Vite/Tauri/Tailwind | App chạy được, có build/lint/test scripts | Design foundation |
| Admin shell and routing | Sidebar, topbar, breadcrumb, route guard, mock login | Shell và route placeholders | App scaffold |
| Admin UI primitives | Button, card, table base, filter bar, badge, drawer, modal, skeleton, error/empty state, mock transport | Shared primitives dùng được trong feature | Shell/routing |

Issue primitives chỉ làm các component cần cho P0 đầu tiên. Không xây toàn bộ
component library nếu chưa có màn hình sử dụng.

### 18.3 Milestone M1 — Anomaly monitoring

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Anomaly monitoring dashboard | KPI, risk trend, severity/type chart, top risky accounts | `#/admin/monitor` với mock data | UI primitives |
| Alert queue | Search, filter, sort, table, pagination, bulk selection | `#/admin/alerts` | UI primitives |
| Alert detail and triage | Risk factors, transactions, timeline, notes, action panel | `#/admin/alerts/:id` và mock command actions | Alert queue |

Sau milestone này cần review lại visual language trước khi mở rộng sang loan,
KYC và reports.

### 18.4 Milestone M2 — Investigation

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Event explorer and account investigation | Event table, account summary, timeline, payload viewer | `#/admin/audit/events`, `#/admin/audit/accounts/:id` | UI primitives |
| Time travel and replay console | Historical state, diff viewer, dry-run/apply, job status | `#/admin/audit/replay` | Investigation |

### 18.5 Milestone M3 — Loan và KYC

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Loan package management | List, detail, create/edit form, repayment preview | Loan package screens | UI primitives |
| Loan application review | Queue, detail, checklist, review/approve/reject flow | Loan application screens | Loan package |
| Repayment schedule | Schedule table, summary, overdue indicators | Repayment schedule screen | Loan application |
| Manual KYC review | Queue, CCCD comparison, OCR diff, media states, decisions | KYC screens | UI primitives |

### 18.6 Milestone M4 — Reports và P2

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Admin reports | Transaction, risk, loan portfolio, filters, charts, export mock | Report screens | UI primitives |
| Customer management P2 | Customer list/detail, accounts, alerts, loans | Customer screens | Shell + primitives |
| Staff and permissions P2 | Staff list/detail, role matrix, permission gate | Staff screens | Shell + primitives |

### 18.7 Milestone M5 — API integration

Các Issue này chỉ mở sau khi UI mock đã ổn và backend API contract đã được xác
nhận.

| Issue | Nội dung | PR deliverable | Phụ thuộc |
|-------|----------|----------------|-----------|
| Connect admin UI to query APIs | Thay mock Query API bằng HTTP adapter | Dashboard/list/detail dùng API thật | P0 UI + BE query APIs |
| Connect admin commands | Kết nối alert/loan/KYC/replay actions | Command actions dùng API thật | Query integration |

### 18.8 Dependency graph

```text
Design foundation
        ↓
Admin scaffold
        ↓
Shell + routing
        ↓
UI primitives + mock API
        ↓
 ┌──────┼────────┬─────────┐
 ↓      ↓        ↓         ↓
Risk  Alerts  Audit     Loans
        ↓       ↓         ↓
     Detail   Replay   Applications
                            ↓
                         Schedule

KYC và Reports có thể làm song song sau UI primitives.
```

### 18.9 Issue template

```md
## Goal

Mô tả kết quả user sẽ có sau khi Issue hoàn thành.

## Scope

- Route/screen:
- Components:
- Mock API:
- User interactions:

## Out of scope

- Không làm trong Issue này:

## Dependencies

- Depends on #...

## Acceptance criteria

- [ ] Route hoạt động ở desktop 1280x800 và 1440x900
- [ ] Có loading state
- [ ] Có empty state
- [ ] Có error state và retry
- [ ] Có permission state nếu màn hình có action bảo vệ
- [ ] Mock data bám `specs/admin-spec.md`
- [ ] Có test cho behavior chính
- [ ] Lint/build/test pass

## Design reference

Link tới `admin/DESIGN.md`, Stitch screen hoặc screenshot.
```

### 18.10 PR checklist

Mỗi PR phải:

- Gắn đúng một Issue.
- Dùng `Closes #<issue-number>` trong PR description.
- Chỉ chứa một feature slice.
- Có screenshot ở `1280x800` và `1440x900`.
- Có test cho interaction chính.
- Chạy thành công `lint`, `build`, `test`.
- Không kết nối backend thật trong UI phase nếu Issue chưa yêu cầu.
- Không thêm component tổng quát nếu chỉ dùng một lần.
- Không chứa thay đổi không liên quan.
- Ghi rõ mock assumptions nếu response chưa có backend thật.

### 18.11 Branch và commit convention

Branch đề xuất:

```text
feat/admin-<issue-number>-short-description
fix/admin-<issue-number>-short-description
chore/admin-<issue-number>-short-description
```

Ví dụ:

```text
feat/admin-101-alert-queue
```

Commit nên giữ scope rõ ràng:

```text
feat(admin): add alert queue (#101)
```

### 18.12 Thứ tự tạo Issue

Không cần tạo toàn bộ Issue ngay từ đầu. Nên tạo theo batch:

1. M0 Foundation.
2. M1 Anomaly monitoring.
3. M2 Investigation.
4. M3 Loan/KYC.
5. M4 Reports/P2.
6. M5 API integration sau khi BE chốt contract.

Mỗi Issue mới nên được tạo sau khi acceptance criteria và dependency của Issue
trước đó đã rõ, tránh tạo backlog quá chi tiết rồi phải sửa lại theo design.
