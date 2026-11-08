# Contract báo cáo giao dịch & rủi ro

Issue: [#223](https://github.com/dont-wait/anomaly/issues/223).

**Trạng thái:** Phase 1 đã chốt thiết kế; server và client dùng duy nhất contract
mới, thay thế hoàn toàn cách tính/response cũ theo yêu cầu triển khai. Không có
query chọn version, không giữ nhánh v1. Đây là chuẩn nghiệm thu cho Phase 2–6,
không phải khẳng định phiên bản đã được deploy hoặc mọi hạng mục UI/export đã
hoàn tất. Policy risk không thay đổi.

Phạm vi: admin desktop, tab **BÁO CÁO → Giao dịch & Rủi ro**, giao dịch
`type=transfer` nội bộ, đơn vị VND. Không gộp giải ngân, trả nợ, tiết kiệm hoặc
hai dòng debit/credit của cùng một giao dịch vào số lượng chuyển khoản.

## 1. Các quyết định đã chốt

1. Tách **giao dịch khởi tạo trong ngày** khỏi **giao dịch hoàn tất trong ngày**.
2. Doanh số và phí thực thu theo thời điểm hoàn tất, không theo lúc tạo intent.
3. Phân bổ trạng thái, risk, bảng giao dịch và top risk accounts cùng dùng tập
   giao dịch khởi tạo trong ngày.
4. Assessment thiếu là `null`, không phải `ALLOW`, score 0 hay threshold 0.
5. Quyết định đã áp dụng lấy từ canonical transaction snapshot; assessment mới
   nhất không được viết lại kết quả nghiệp vụ của giao dịch đã kết thúc.
6. Bộ lọc/search/phân trang chỉ áp dụng cho bảng. Cards/charts/top accounts là
   toàn ngày; giao diện và export phải ghi rõ phạm vi này.
7. Báo cáo là ảnh chụp read model hiện tại, không phải sổ đã khóa cuối ngày hoặc
   khả năng truy vấn lại mọi trạng thái lịch sử.

## 2. Nguồn dữ liệu và thời gian

- EventStoreDB là source of truth. API report đọc Mongo projection; không replay
  toàn journal trên mỗi request và không ghi canonical event khi đọc báo cáo.
- `date` bắt buộc, đúng định dạng `YYYY-MM-DD`; thiếu, rỗng, sai định dạng hoặc
  ngày không tồn tại trả 400. Ngày hợp lệ chưa có dữ liệu trả 200 với tập rỗng.
- `timezone` cố định `Asia/Ho_Chi_Minh`. Ngày D là `[start, end)` từ 00:00 D đến
  00:00 D+1 theo giờ Việt Nam; lưu/truy vấn timestamp UTC, trả RFC3339 UTC.
- `previousDate` là D-1 theo lịch Việt Nam, không phải ngày trước thời điểm mở UI.
- Server lấy một `asOf` cho request, dùng chung để xác định OTP hết hạn và giới
  hạn timestamp phát sinh/nhận dữ liệu. Client không tự tính lại bằng đồng hồ máy.
- Các phần trong một response phải đọc cùng Mongo snapshot, bao gồm ngày trước,
  để không ghép summary, bảng và assessment từ các thời điểm đọc khác nhau.
  `asOf` là mốc tham chiếu của request, không phải cam kết canonical store đã
  được project đầy đủ tới thời điểm đó. Không cung cấp tham số truy vấn lịch sử
  `asOf` trong phạm vi này.
- Event/assessment đến muộn, replay, hoàn tất hoặc cập nhật OTP có thể làm báo cáo
  ngày cũ thay đổi. Export giữ `asOf` và metadata của lần đọc, không gọi là báo
  cáo tài chính đã chốt.

### Metadata về độ mới

Contract trả `generatedAt`, `asOf`, `projection.status` và
`projection.checkpointRevision` (nullable). `generatedAt` là thời điểm hoàn tất
tạo response; `asOf` là mốc tham chiếu ở trên.

`projection.status` là `caught_up`, `lagging` hoặc `unknown`. Chỉ dùng hai giá trị
đầu khi so sánh được checkpoint banking với canonical banking head tương ứng.
Không có phép đo thì trả `unknown`, không gán mặc định `fresh`. Revision là
chuỗi thập phân để không làm mất độ chính xác trên JavaScript. Metadata này chỉ
phản ánh banking projector; không đảm bảo model/Kafka đã xử lý xong. Assessment
coverage được công bố riêng.

## 3. Hai tập giao dịch và các chỉ số

### 3.1. Tập khởi tạo C(D)

Mỗi `transactionId` duy nhất, `type=transfer`, có
`start <= createdAt < end` và `createdAt <= asOf`, trong snapshot đọc.

| Field trong `summary.created` | Định nghĩa |
|---|---|
| `transactionCount` | Số phần tử C(D) |
| `successCount` | Trạng thái hiệu lực là success, bất kể hoàn tất ngày nào |
| `failedCount` | Trạng thái hiệu lực là failed; không gồm cancelled/expired |
| `cancelledCount` | Trạng thái hiệu lực là cancelled |
| `expiredCount` | Trạng thái hiệu lực là expired |
| `pendingCount` | Trạng thái hiệu lực là pending |
| `unknownCount` | Trạng thái không được hỗ trợ; không tự gộp vào pending |

Tỷ trọng trạng thái = count / `summary.created.transactionCount`. Khi tập rỗng,
count bằng 0 và UI hiển thị không có dữ liệu, không vẽ tỷ lệ giả. Có thể hiển thị
tỷ lệ success của cohort này, nhưng không dùng `summary.completed.count` làm tử số.

### 3.2. Tập hoàn tất S(D)

Mỗi `transactionId` duy nhất, `type=transfer`, trạng thái canonical `success`,
có `start <= effectiveCompletedAt < end` và `effectiveCompletedAt <= asOf`.
Ngày tạo giao dịch không giới hạn S(D).

Quy tắc `effectiveCompletedAt`:

1. Dùng `completed_at` hợp lệ nếu có.
2. Với dữ liệu lịch sử thiếu `completed_at`, dùng `posted_at` hợp lệ đã được lưu
   trên giao dịch success như mốc ghi sổ; đánh dấu `completionTimeSource=posted_at`.
3. Nếu không có cả hai thì không suy ra từ `created_at`, thời gian assessment hay
   thời gian import. Không đưa vào S(D); báo vấn đề chất lượng dữ liệu. Vì không
   biết ngày hoàn tất, vấn đề thiếu mốc này phải được kiểm tra trên toàn bộ
   transfer success, không chỉ C(D), và không gán nhầm là lỗi riêng ngày D.

`completed_at` có mặt nhưng không hợp lệ hoặc mâu thuẫn không được âm thầm dùng
fallback để che lỗi. Các giá trị tiền không phải VND/sai miền dữ liệu cũng phải
được báo lỗi chất lượng dữ liệu, không cộng lẫn hoặc đổi tiền ngầm.

| Field trong `summary.completed` | Định nghĩa |
|---|---|
| `count` | Số phần tử S(D) |
| `totalAmount` | Tổng amount của S(D), không bao gồm phí |
| `totalFee` | Tổng fee của S(D) |
| `averageAmount` | totalAmount / count; `null` nếu count = 0 |

Amount/fee là số nguyên VND; giữ chính xác khi tính tổng, chỉ làm tròn khi hiển
thị average. Không cộng ledger debit + credit hoặc phí thêm lần nữa. Không được
âm thầm làm tròn số vượt giới hạn số nguyên an toàn của client.

**Không có invariant `completed.count <= created.transactionCount`.** Hai tập có
thể khác nhau, kể cả một ngày không có intent mới nhưng vẫn có giao dịch hoàn tất.

## 4. Trạng thái canonical và trạng thái hiệu lực

Mỗi item có `canonicalStatus`, `effectiveStatus`, `statusAt`, `failureReason`,
`createdAt`, `completedAt`, `otpExpiresAt`. Field không có bằng chứng là `null`.

Áp dụng theo thứ tự:

| Canonical snapshot | effectiveStatus | statusAt |
|---|---|---|
| success | success | effectiveCompletedAt nếu có |
| failed | failed | failedAt nếu có |
| cancelled | cancelled | cancelledAt nếu có |
| awaiting_otp, OTP expiry hợp lệ và `otpExpiresAt <= asOf` | expired | otpExpiresAt |
| awaiting_otp, chưa hết hạn | pending | createdAt |
| awaiting_otp, thiếu OTP expiry | pending + cảnh báo chất lượng dữ liệu | createdAt |
| Giá trị khác | unknown + cảnh báo chất lượng dữ liệu | null |

Terminal status luôn ưu tiên hơn OTP expiry. `expired` là trạng thái hiển thị suy
ra, không phải event terminal mới. Nếu canonical đã cancelled thì vẫn cancelled,
không đổi thành expired chỉ vì OTP từng hết hạn. Không suy ra risk đang chạy từ
việc transaction vẫn pending. OTP resend có thể làm thay đổi trạng thái hiệu lực
trong lần đọc tiếp theo.

## 5. Assessment và quyết định đã áp dụng

### Hai khái niệm không được trộn

- `appliedRisk`: snapshot `transactions.risk_assessment` được chiếu từ event
  canonical; phản ánh assessment/quyết định đã được ghi cùng transaction.
- `latestAssessment`: assessment quan sát được trong `risk_decisions`, chọn theo
  transaction ID và source event ID phù hợp với event khởi tạo của transaction,
  `received_at <= asOf`, `scored_at <= asOf`; sắp giảm dần theo
  `(scored_at, received_at, _id)`.

Không giới hạn assessment vào ngày nhận D; không dùng danh sách 50 recent
assessments hoặc trạng thái handled/finalized để lọc dữ liệu báo cáo. Assessment
không xác minh được liên kết với transaction không được ghép tùy ý. Một
transaction được chọn tối đa một assessment cho mỗi vai trò trên.

`selectedAssessment` dùng cho bảng, risk trung bình, bộ lọc risk và top accounts:

1. Ưu tiên appliedRisk hợp lệ nếu có; `source=applied`.
2. Không có appliedRisk thì dùng latestAssessment hợp lệ; `source=observed`.
3. Không có cả hai thì `null`; `source=none`.

Applied risk không được thay thế chỉ vì có assessment mới hơn. Có thể hiển thị
latestAssessment riêng trong drawer, nhưng không ghép score/model từ applied
với threshold/feature version của assessment khác.

Score hợp lệ là số hữu hạn trong [0, 1], cùng transaction và có thời điểm đánh
giá không sau asOf; observed assessment còn phải qua validation nguồn hiện hành.
Nếu có appliedRisk nhưng dữ liệu đó không hợp lệ, trả cảnh báo và để
selectedAssessment=null, không âm thầm fallback sang observed để che lỗi bằng
chứng canonical. Chỉ fallback observed khi thực sự không có appliedRisk.

Canonical risk snapshot hiện chỉ có decision, isAnomalous, riskScore,
modelVersion, evaluatedAt. Thiếu assessment ID, threshold, featureVersion hoặc
receivedAt thì trả `null` cho các field đó. Chỉ bổ sung metadata khi xác minh
được **chính xác cùng assessment**; trùng transaction ID là chưa đủ. Legacy
snapshot thiếu decision được giữ score nếu hợp lệ nhưng decision là `null`;
không suy ra ALLOW/BLOCK từ score, predictedLabel hoặc isAnomalous.

Report không đổi policy risk hiện hành và không tự tính lại decision bằng model
threshold. `predictedLabel` là nhãn model, không phải business decision. Score 0
hợp lệ phải được giữ nguyên; chưa có score/threshold không được chuyển thành 0.
Không gọi score là xác suất gian lận đã hiệu chỉnh.

### Chỉ số risk trên C(D)

| Field trong `summary.risk` | Định nghĩa |
|---|---|
| `assessedCount` | Có selectedAssessment với score hợp lệ |
| `unassessedCount` | transactionCount - assessedCount |
| `averageScore` | Tổng score đã chọn / assessedCount; null nếu không có |
| `coverage` | assessedCount / transactionCount; null nếu C(D) rỗng |
| `allowCount` | Selected decision rõ ràng là ALLOW |
| `modelBlockCount` | Selected decision rõ ràng là BLOCK |
| `unknownDecisionCount` | Có score đã đánh giá nhưng decision chưa xác định |
| `actualBlockedCount` | canonical failed và failureReason=anomalous_transfer |

`allowCount + modelBlockCount + unknownDecisionCount = assessedCount`.
`assessedCount + unassessedCount = created.transactionCount`.
Assessment không hợp lệ được báo lỗi chất lượng dữ liệu, không đưa score sai vào
trung bình hoặc quyết định ALLOW/BLOCK. Trong contract này ALLOW/BLOCK trong
selectedAssessment phải đi cùng score hợp lệ; bằng chứng từ failureReason vẫn
được giữ độc lập trong actualBlockedCount.

Model BLOCK khi transaction vẫn pending **không** làm transaction thành failed.
`actualBlockedCount <= created.failedCount`; đây là nhóm con, không cộng thêm
vào các lát trạng thái. Nếu legacy thiếu appliedRisk nhưng failureReason chứng
minh đã chặn, vẫn đếm actualBlockedCount; không tạo score giả để đủ coverage.

### Top risk accounts

Top nguồn/đích dùng các transaction trong C(D) có selected decision BLOCK, không
gọi mọi failed/cancelled là risk. Mỗi transaction đóng góp một lần cho mỗi phía:
`count` là số giao dịch BLOCK; `amount` là tổng giá trị yêu cầu, **không phải số
tiền đã mất hoặc đã chuyển**. Nhãn UI: “Giao dịch được đánh giá BLOCK”.

Sắp theo count giảm dần, amount giảm dần, accountNo tăng dần; lấy tối đa 5. Thiếu
accountNo không được nhóm thành một tài khoản giả; trả cảnh báo chất lượng dữ
liệu. WARNING không nằm trong top này, và handled không làm mất dữ liệu lịch sử.

## 6. Contract response duy nhất

Endpoint vẫn là `GET /api/admin/reports/transactions`, auth admin hiện hành.
Server trả envelope thành công `{status,message,data}` theo contract mới cho
mọi request hợp lệ; client chỉ gửi `date`. Không chọn version bằng query. Bỏ
implementation cũ và các field summary phẳng. Đây là thay đổi không tương thích:
deploy server và client mới cùng đợt; client cũ phải cập nhật, không có fallback
về cách tính cũ.

Các nhóm field sau là bắt buộc. `reportVersion=2` chỉ nhận diện cấu trúc response
để client phát hiện payload không phù hợp; không phải tham số lựa chọn API:

| Đường dẫn trong data | Nội dung |
|---|---|
| `reportVersion`, `date`, `previousDate`, `timezone`, `currency` | 2, ngày đã validate, D-1, Asia/Ho_Chi_Minh, VND |
| `from`, `to`, `asOf`, `generatedAt`, `projection` | Mốc thời gian và metadata ở mục 2 |
| `summary.created`, `summary.completed`, `summary.risk` | Các field định nghĩa ở mục 3–5 |
| `previous.created`, `previous.completed`, `previous.risk` | Cùng định nghĩa cho D-1, cùng snapshot/asOf |
| `hourly.created` | 24 buckets: hour, count, successCount, failedCount, cancelledCount, expiredCount, pendingCount, unknownCount của C(D) |
| `hourly.completed` | 24 buckets: hour, count, totalAmount, totalFee của S(D) |
| `transactions` | Danh sách C(D), một item/transaction, không cắt theo recent assessments |
| `topRiskSources`, `topRiskReceivers` | Quy tắc mục 5 |
| `dataQuality` | Danh sách code, scope và count; [] nếu không phát hiện lỗi |

Hour có dạng `00:00` đến `23:00` theo giờ Việt Nam; giờ rỗng count/amount/fee bằng
0. Transactions sắp `(createdAt DESC, transactionId ASC)` để ổn định khi đồng giờ.
Item gồm transactionId, tài khoản nguồn/đích và tên, amount, fee, currency; các
field trạng thái ở mục 4; appliedRisk, latestAssessment, selectedAssessment và
assessmentSource. Các object assessment có score/decision cùng metadata có
bằng chứng; field chưa biết phải hiện diện dưới dạng null, không dữ liệu giả.

Data quality codes tối thiểu: `UNKNOWN_STATUS`, `MISSING_OTP_EXPIRY`,
`MISSING_COMPLETION_TIME`, `INVALID_COMPLETION_TIME`, `INVALID_ASSESSMENT`,
`MISSING_RISK_ACCOUNT`, `INVALID_MONEY`. Scope là `created`, `completed` hoặc
`global`; lỗi không thể quy về ngày cụ thể phải ghi global. Không trả số liệu
trông như đầy đủ mà giấu các record bị loại. Không thay dữ liệu sai bằng 0;
UI/export phải đánh dấu có cảnh báo, không coi response có cảnh báo là báo cáo
đã đối soát sạch. Lỗi truy vấn/hạ tầng trả non-2xx, không đổi thành report rỗng.

Mỗi dataQuality item có thêm `date` (YYYY-MM-DD hoặc null cho scope global), để
phân biệt lỗi ngày D và D-1. Khi amount/fee/currency không hợp lệ, giữ dòng khởi
tạo với amount/fee=null và cảnh báo; không cộng vào completed totals. Tổng tiền
vượt giới hạn số nguyên an toàn của client trả lỗi non-2xx, không trả tổng đã
làm tròn. Count top risk vẫn giữ bằng chứng BLOCK nhưng amount loại phần tiền
không hợp lệ, có cảnh báo INVALID_MONEY tương ứng.

Báo cáo mới không đổi contract `/api/admin/monitor` hoặc history/summary dành cho consumer.
Admin report không lấy quy tắc gộp cancelled thành failed từ consumer history.
Phân trang server tương lai phải giữ summary toàn tập và export đầy đủ phạm vi,
không được coi một trang là toàn ngày.

## 7. Hiển thị, so sánh và export

- Cards ghi rõ “Khởi tạo trong ngày”, “Hoàn tất trong ngày”, “Doanh số thực hiện”,
  “Phí thực thu”, “Giá trị trung bình giao dịch hoàn tất”.
- Donut chỉ biểu diễn C(D), đủ success/failed/cancelled/expired/pending/unknown.
  Actual blocked là chỉ số phụ của failed, không phải một lát chồng lấn.
- Biểu đồ số lượng khởi tạo theo createdAt; biểu đồ doanh số theo
  effectiveCompletedAt và chỉ lấy S(D). Không chồng failed/pending vào doanh số.
- Risk trung bình ghi độ phủ assessedCount/transactionCount và nguồn applied hay
  observed ở chi tiết. Không có assessment: “Chưa có đánh giá”, score “—”. Không
  gọi là “Đang đánh giá” nếu không có bằng chứng tác vụ đang xử lý.
- Cột thời gian chính luôn là createdAt; completedAt và thời gian assessment có
  nhãn riêng trong chi tiết. Mọi thời gian hiển thị dùng timezone Việt Nam.
- So sánh với **ngày liền trước của ngày được chọn**, dùng cùng chỉ số/cùng tập.
  Công thức `(current - previous) / previous * 100`; previous=0 và current>0 là
  “Mới phát sinh”, cả hai bằng 0 là “Không đổi”. Average thiếu mẫu số là “—”.
  Ngày đang diễn ra được so với toàn ngày trước nhưng phải ghi rõ “Ngày chưa kết
  thúc; so với toàn ngày liền trước”, không trình bày như hai kỳ đầy đủ tương đương.
- Filter status dùng effectiveStatus; filter risk dùng selected decision, có
  lựa chọn chưa đánh giá/quyết định chưa xác định. Hai trường hợp này khác nhau.
  Search áp dụng ID, số tài khoản và tên theo cùng quy tắc trên bảng/export.
- Export dùng đúng report đã tải, ghi date/asOf/timezone/cảnh báo/phạm vi lọc.
  Summary toàn ngày và dòng đã lọc đặt ở phần riêng có nhãn, không giả vờ khớp
  nhau. Xuất toàn bộ dòng khớp filter, không chỉ trang hiện tại.
- Đổi ngày phải ngăn response cũ ghi đè response mới. Chưa có response khớp ngày
  được chọn hoặc tải lỗi thì không export dưới tên ngày mới. Refresh cùng ngày
  có thể giữ dữ liệu cũ nếu đánh dấu stale và giữ nguyên asOf.
- Không xuất HTML giả `.xls`; dùng CSV an toàn hoặc `.xlsx` thật. Giữ số tài khoản
  dạng text và xử lý spreadsheet formula injection đối với dữ liệu không tin cậy.

## 8. Fixture đối soát bắt buộc

Mọi thời gian dưới đây theo Việt Nam. D = 2026-11-08,
asOf = 2026-11-09 00:02; giả sử projection đầy đủ và không lỗi dữ liệu.

| ID | Tạo | Trạng thái ở asOf | Hoàn tất | Amount / fee (VND) | Selected risk |
|---|---|---|---|---|---|
| T1 | D 23:59 | success | D+1 00:01 | 100000 / 1000 | applied ALLOW, 0.2; assessment nhận D+1 00:00:30 |
| T2 | D 12:00 | success | D 12:01 | 200000 / 2000 | applied ALLOW, 0.1 |
| T3 | D 13:00 | failed, anomalous_transfer | — | 300000 / 3000 | applied BLOCK, 0.9 |
| T4 | D 14:00 | cancelled | — | 400000 / 4000 | Không có |
| T5 | D 23:59:30 | awaiting_otp, expiry D+1 00:04:30 | — | 500000 / 5000 | observed ALLOW, **0** |
| T6 | D 20:00 | awaiting_otp, expiry D 20:05 | — | 600000 / 6000 | Không có |
| T7 | D-1 23:59 | success | D 00:01 | 700000 / 7000 | applied ALLOW, 0.1 |

Kết quả báo cáo D:

- C(D) = T1–T6, transactionCount=6.
- success=2, failed=1, cancelled=1, expired=1, pending=1, unknown=0.
- S(D) = T2 + T7, completed.count=2, totalAmount=900000, totalFee=9000,
  averageAmount=450000. T1 chỉ góp doanh số cho D+1.
- assessedCount=4, unassessedCount=2, averageScore=0.3, coverage=4/6.
- allowCount=3, modelBlockCount=1, unknownDecisionCount=0,
  actualBlockedCount=1. Score 0 của T5 nằm trong mẫu số.
- Top risk chỉ có các phía tài khoản của T3, mỗi phía count=1, amount=300000.
- Bucket khởi tạo 23:00 có T1 + T5; bucket doanh số 00:00 có T7, 12:00 có T2.
- Lọc bảng failed chỉ có T3; summary và charts vẫn giữ số liệu toàn ngày như trên.

Tình huống bổ sung phải kiểm thử trong các phase code:

1. Exactly start thuộc D; exactly end thuộc D+1. Máy client dùng timezone khác
   không đổi ngày/giờ báo cáo Việt Nam.
2. C(D) rỗng nhưng S(D) có dữ liệu: count khởi tạo 0, doanh số vẫn có.
3. Không có S(D): amount/fee/count=0, averageAmount=null. Không assessment:
   averageScore=null; coverage=0 nếu C(D) không rỗng, null nếu C(D) rỗng.
4. Thêm assessment mới trái với applied risk của T3: không thay score/decision đã
   chọn, actualBlockedCount hoặc nguồn lý do giao dịch thất bại.
5. Observed BLOCK cho T5: modelBlockCount tăng, actualBlockedCount không tăng;
   T5 vẫn pending. Nhiều model/feature versions vẫn chỉ đếm một selected score.
6. Trên 50 assessments, redelivery và cùng timestamp: kết quả không phụ thuộc
   recent monitor, không đếm đôi, thứ tự tie-break ổn định.
7. OTP expiry đúng asOf là expired; success/cancelled không bị expiry ghi đè.
8. Thiếu completed_at nhưng có posted_at: fallback có dấu vết. Thiếu cả hai hoặc
   metadata risk thiếu: cảnh báo/null đúng quy tắc, không tự dựng timestamp/score.
9. Unknown status đi vào unknownCount; failed không chứa cancelled/expired.
10. Projector lag/không đo được lag, request chồng nhau, đổi ngày khi API lỗi,
    export sau khi lọc: không hiển thị hoặc xuất nhầm dữ liệu là đầy đủ/ngày mới.

### Invariants nghiệm thu

```text
created.transactionCount = success + failed + cancelled + expired + pending + unknown
sum(hourly.created.count) = created.transactionCount
sum(hourly.completed.count) = completed.count
sum(hourly.completed.totalAmount) = completed.totalAmount
sum(hourly.completed.totalFee) = completed.totalFee
risk.assessedCount + risk.unassessedCount = created.transactionCount
risk.allowCount + risk.modelBlockCount + risk.unknownDecisionCount = risk.assessedCount
risk.actualBlockedCount <= created.failedCount
```

Các invariant này áp dụng cùng snapshot, cùng currency và cùng phạm vi dữ liệu.
Phase 2–6 chỉ hoàn tất khi có test chứng minh, không chỉ khi UI trông hợp lý.

### Kiểm chứng server Phase 2

Từ thư mục `server/`:

```bash
go test ./...
go test -race ./internal/application/reporting ./internal/presentation/http/handler/admin ./internal/presentation/http/openapi
go test -tags=e2e ./internal/infrastructure/mongo -run '^TestTransactionReportMongoE2E$' -count=1 -v
```

E2E chạy MongoDB replica set riêng bằng Testcontainers (cần Docker), không dùng
database ứng dụng. Bao phủ hai cohort, 501 giao dịch qua hai batch
assessment, finalized/handled, score 0, timestamp fallback/global quality và
writer thay đổi dữ liệu giữa các query để chứng minh snapshot nhất quán.

Ví dụ gọi API sau khi deploy server mới:

```http
GET /api/admin/reports/transactions?date=2026-11-08
Authorization: Bearer <admin-access-token>
```

Phase 2 trả projection.status=unknown cùng checkpoint đọc được (hoặc null), vì
chưa đo canonical head trong request. Không suy ra độ trễ bằng thời gian tạo
response. Query completion có kiểm tra lỗi ngày trên lịch sử success để không
giấu record không thể quy về ngày; không giữ toàn bộ lịch sử đó trong RAM.
Chi phí kiểm tra lịch sử và phân trang/aggregation cần được đo khi tối ưu hiệu
năng, không được bỏ kiểm tra âm thầm để response nhanh hơn.
