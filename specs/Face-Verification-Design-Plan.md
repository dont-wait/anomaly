# Spec triển khai KYC realtime: so khớp ảnh căn cước với video thực tế

Ngày: 2026-10-19. Trạng thái: đề xuất triển khai, chưa phải chức năng đã chạy.

## 1. Mục tiêu và phạm vi

Thay pipeline giả lập bằng xử lý ảnh/video và inference thực tế: lấy chân dung từ ảnh mặt trước căn cước, lấy khuôn mặt của người quay video, dùng **một mô hình embedding khuôn mặt dùng chung cho hai nguồn** để xác minh 1:1. Kết quả phải phụ thuộc nội dung media; đổi tên file không được làm đổi điểm hoặc quyết định.

**Yêu cầu bắt buộc sau cập nhật:** phân tích và so khớp thật ngay trong lúc quay, trả phản hồi realtime từ server; preview camera phải mượt trên thiết bị nghiệm thu. Chỉ quay xong rồi upload và chờ phân tích không đáp ứng yêu cầu realtime. Điểm/tiến độ không được giả lập bằng timer, tên file hoặc random. Các mục tiêu hiệu năng bên dưới phải đo trên thiết bị thật, chưa phải cam kết hệ thống hiện tại đã đạt.

**Ưu tiên bố cục:** tăng diện tích khung camera để nhìn rõ khuôn mặt và thao tác quay đầu. Thu gọn chi tiết phụ, hướng dẫn và phần trang trí để camera chiếm phần lớn màn hình khi quay; yêu cầu cụ thể tại 8.3.

Chưa chốt model/weights vì người triển khai sẽ cung cấp mô hình. Spec chốt giao diện tích hợp, quy tắc xử lý, hợp đồng API, dữ liệu lưu và tiêu chí nghiệm thu để thay model mà không đổi luồng đăng ký.

Phân biệt các kết luận:

| Kiểm tra | Kết luận được phép |
|---|---|
| Face verification | Khuôn mặt trong video có khớp chân dung trên ảnh căn cước hay không |
| Active liveness | Video có quan sát được chuỗi động tác yêu cầu hay không |
| Xác thực căn cước | Giấy tờ có thật, hợp lệ, còn hiệu lực và thuộc dữ liệu cơ quan cấp hay không |

Phiên bản này triển khai hai dòng đầu. Không có xác thực chip/NFC, cơ sở dữ liệu cơ quan cấp, OCR đối chiếu thông tin đăng ký hoặc kiểm định chống làm giả giấy tờ. Mặt sau vẫn được nhận và lưu theo luồng hiện có nhưng không tham gia face matching. Một ảnh giấy tờ giả có chân dung đúng người vẫn có thể qua face matching; UI và báo cáo không được mô tả kết quả này là đã xác thực tính pháp lý của căn cước.

Một model nhận dạng khuôn mặt không tự đảm nhiệm phát hiện thẻ, phát hiện mặt và chống giả mạo. Các bước tiền xử lý, detector/landmark hoặc thư viện hỗ trợ được phép sử dụng riêng; chỉ dùng cùng một model embedding cho so khớp danh tính. Không huấn luyện model mới trong phạm vi này.

## 2. Hiện trạng đã đối chiếu mã nguồn

| Thành phần | Hiện trạng | Việc cần làm |
|---|---|---|
| `client/mobile/src/features/registration/components/FaceCapture.tsx` | Camera thật, `MediaRecorder`, video 12 giây; hướng dẫn nhìn thẳng → trái → phải → chớp mắt, mỗi bước khoảng 3 giây | Stream trong lúc quay, phản hồi theo bằng chứng server, giữ preview mượt |
| `client/mobile/src/features/registration/api/kyc.ts` | Gửi multipart lên Go bằng KYC token; timeout 120 giây | Bổ sung API realtime; giữ endpoint complete cho bước commit cuối |
| `server/internal/presentation/http/handler/account/kyc_handler.go` | Kiểm tra media; gọi Python; chỉ upload media chính thức khi Python trả `VERIFIED` | Kiểm tra đầy đủ bằng chứng và chuyển kết quả thật xuống command |
| `server/internal/infrastructure/kyc/client.go` | Gọi Python, timeout 45 giây; đọc score nhưng chưa đọc quality checks | Mở rộng DTO nội bộ và kiểm tra tính nhất quán |
| `kyc-service/app/services/pipeline.py` | Chỉ có `StubVisionPipeline`; điểm lấy từ tên file và ngưỡng cấu hình | Thêm `RealVisionPipeline`; bỏ đường stub trong môi trường chạy thật |
| `kyc-service/app/core/config.py` | Backend mặc định `disabled`; threshold 0.78/0.85 đang là giá trị cấu hình | Thêm manifest model và cấu hình đã hiệu chuẩn |
| `server/internal/application/account/commands/verify_account.go` | Tạo session verified nhưng OCR, liveness, face match đều `not_run`; chưa lưu điểm thật | Lưu trạng thái và provenance đúng kết quả inference |
| `server/internal/domain/account/account.go` | Đã có session, media, face score và provider | Bổ sung trường còn thiếu và trạng thái kiểm tra |
| `client/mobile/src/features/registration/useRegistrationFlow.ts` | Trừ lượt thử ở state client | Đưa chính sách giới hạn lượt về Go trước khi dùng để giới hạn truy cập thật |

README Python hiện có đoạn mô tả client gọi trực tiếp Python; khi triển khai phải sửa cho thống nhất với luồng thực tế `mobile → Go → Python`.

## 3. Kiến trúc và trách nhiệm

Luồng chính:

1. Mobile đăng ký tài khoản chưa xác minh, nhận KYC token theo cơ chế hiện có.
2. Mobile gửi hai mặt căn cước để tạo realtime attempt; Go xác thực KYC token, kiểm tra ảnh, hạn mức và idempotency. Python tách chân dung và tính embedding tham chiếu trước khi bắt đầu quay.
3. Mobile mở kênh realtime qua Go; chỉ bắt đầu quay khi model/decoder sẵn sàng. Stream các chunk từ cùng một `MediaRecorder` đang ghi video.
4. Go chuyển luồng sang Python qua mạng nội bộ. Python decode liên tục, phân tích chất lượng/động tác và so khớp embedding thật khi video đang chạy.
5. Go chuyển các sự kiện tiến độ về mobile: điều chỉnh ánh sáng/vị trí, động tác đã đạt và trạng thái đang đối chiếu. Camera không chờ mỗi lần inference mới hiển thị frame tiếp theo.
6. Khi dừng quay, Go/Python kết thúc stream, kiểm tra tính đầy đủ, gắn hash video với attempt; mobile gửi complete để xác minh cuối cùng trên đúng video và commit. Go lưu metadata lần thử nếu không đạt.
7. Khi kết quả cuối đạt, Go lưu media riêng tư, lưu session có kết quả thật rồi đánh dấu tài khoản verified; chỉ sau khi lưu thành công mới trả thành công cho mobile. Kết quả tạm lúc quay không được tự đánh dấu tài khoản verified.

Python giữ tính stateless theo nghiệp vụ: không tạo tài khoản, không đếm lượt, không truy vấn Mongo và không tự quyết định hết lượt. Được giữ context xử lý tạm theo realtime attempt (decoder, embedding tham chiếu, timeline, trạng thái động tác) có TTL; đóng kết nối hoặc hết hạn phải dọn. File tạm phục vụ decode phải được xóa trong cả đường thành công, timeout và lỗi.

Go sở hữu auth, retry policy, rate limit, lưu trữ và trạng thái tài khoản. Client chỉ hướng dẫn quay/chụp và hiển thị kết quả; không gửi score, embedding hoặc cờ đã xác minh.

## 4. Hợp đồng mô hình

### 4.1 Manifest bắt buộc

Model được cung cấp dưới dạng artifact bất biến cùng manifest:

- `model_id`, `model_version`, SHA-256 của weights, nguồn weights và quyền sử dụng cho môi trường dự kiến.
- Runtime/phiên bản tương thích; ONNX Runtime là phương án tích hợp đề xuất nếu có ONNX export. Model định dạng khác phải có adapter tương đương.
- Input shape, tensor layout, RGB/BGR, resize, căn chỉnh landmark, normalization và dtype đúng theo model.
- Output name, chiều embedding, quy tắc L2 normalize và metric dùng khi so khớp.
- Detector/aligner đi kèm, phiên bản của chúng và các giới hạn đầu vào.
- `calibration_version`, cấu hình threshold và báo cáo đánh giá cùng phiên bản.

Không mặc định mọi model đều dùng ảnh 112×112, embedding 512 chiều hoặc cùng ngưỡng. Model không đáp ứng giao diện embedding phải được đánh giá và sửa adapter trước khi tích hợp.

### 4.2 Giao diện nội bộ đề xuất

```python
class FaceEmbeddingModel(Protocol):
    def embed(self, aligned_faces: Sequence[NDArray]) -> NDArray:
        """Trả N embedding hữu hạn, cùng chiều, đã chuẩn hóa L2."""
```

Adapter phải phát hiện sai shape, NaN/Inf, vector zero và batch lỗi. Các lỗi này là lỗi hệ thống, không phải bằng chứng khác người.

Model load một lần mỗi worker, warm-up khi khởi động; không tải weights mỗi request hoặc tự tải từ Internet trong request. Readiness chỉ đạt sau khi artifact, checksum, preprocessing, runtime và calibration đều hợp lệ. Thiếu model hoặc load thất bại → không phục vụ xác minh, không tự chuyển sang stub.

## 5. Pipeline xử lý thật

### 5.1 Kiểm tra và giải mã media

- Giữ giới hạn Go: 10 MiB mỗi ảnh, 25 MiB video, tổng request 48 MiB; Python giới hạn tương ứng cho mặt trước + video và multipart overhead.
- Hợp đồng chung chỉ nhận JPEG/PNG và MP4/WebM. Python hiện cho phép một số loại rộng hơn trong stub; pipeline thật cần đồng bộ với Go.
- Kiểm tra nội dung thực, giải mã ảnh/video hoàn chỉnh; header container hợp lệ không đủ chứng minh video giải mã được.
- Chuẩn hóa EXIF orientation và rotation metadata; preview phản chiếu không được làm sai trái/phải theo góc nhìn người quay.
- Decoder có giới hạn thời gian, số pixel, số frame và bộ nhớ; không gọi shell bằng filename người dùng. Khi dùng decoder bên ngoài, truyền argument có cấu trúc và hạn chế tài nguyên.
- Mốc ban đầu đề xuất: video 10–15 giây để tương thích clip 12 giây, tối đa 1920×1080, tối đa 900 frame giải mã; tất cả đặt trong config. Phải kiểm chứng lại trên video do thiết bị thật tạo ra.
- Duration và timestamp lấy từ media đã decode; không tin duration, extension hoặc MIME do client khai báo.

### 5.2 Tách chân dung căn cước

1. Phát hiện vùng thẻ và chuẩn hóa phối cảnh; hỗ trợ danh sách layout căn cước được cấu hình và kiểm thử rõ ràng.
2. Xác định vùng chân dung theo layout đã nhận diện. Không chọn khuôn mặt lớn nhất ở bất kỳ đâu trong ảnh làm chân dung mặc định.
3. Phát hiện đúng một mặt phù hợp trong vùng chân dung; bỏ qua hình bảo an nhỏ nếu layout có quy định rõ. Không xác định chắc vùng chân dung → yêu cầu chụp lại.
4. Kiểm tra mờ, sáng, chói, kích thước khuôn mặt và khả năng căn chỉnh. Ngưỡng nằm trong cấu hình có phiên bản.
5. Align mặt theo preprocessing của model và tạo embedding tham chiếu.

Không nhận diện được layout → trả `CCCD_PORTRAIT_NOT_FOUND` trong MVP. Không tự đoán crop từ một layout khác. Việc hỗ trợ thẻ mới cần fixture và kiểm chứng riêng.

### 5.3 Phân tích video và theo dõi người quay

1. Đọc timestamp theo thứ tự, xử lý rotation; phát hiện stream rỗng/hỏng và timeline bất thường.
2. Dùng detector/landmark theo thời gian để theo dõi mặt. Bắt đầu với 15 frame/giây cho động tác và blink; benchmark để tăng nếu bỏ sót blink. Không chỉ lấy vài frame thưa cho liveness.
3. Đánh giá chất lượng, số mặt và khoảng mất dấu trên toàn timeline đã lấy mẫu. Không che việc xuất hiện mặt thứ hai bằng cách chỉ chọn các frame đẹp.
4. Kiểm tra tính liên tục danh tính ở các đoạn trước/trong/sau động tác bằng cùng model embedding. Đổi người giữa clip phải làm hỏng xác minh, dù frame cuối khớp căn cước.
5. Chọn tối thiểu 5 frame có mặt rõ, ưu tiên gần chính diện và phân bố trên ít nhất 3 khoảng thời gian. Không dùng 5 frame sát nhau từ một khoảnh khắc duy nhất.
6. Nếu không đủ frame đạt chất lượng, trả lỗi quay lại; không hạ ngưỡng match để bù chất lượng kém.

Số frame, khoảng cách thời gian tối thiểu, tỉ lệ mặt hợp lệ, khoảng mất dấu và ngưỡng liên tục danh tính đều cần cấu hình và hiệu chuẩn. Không tự bỏ qua mặt thứ hai chỉ vì tồn tại ít frame; chính sách bỏ detection nhiễu phải có dữ liệu kiểm chứng.

### 5.4 Active liveness

Giữ `TURN_HEAD_LEFT_RIGHT_BLINK` cho MVP: nhìn thẳng → quay trái → quay phải → chớp mắt. Server chỉ chấp nhận challenge nằm trong allowlist.

- Landmark/head pose phải chứng minh động tác đúng thứ tự trên timeline; không đánh dấu đạt theo việc client đã hiển thị hướng dẫn hoặc đã quay đủ 12 giây. UI chuyển trạng thái động tác theo sự kiện server, timer chỉ hiển thị thời gian/cửa sổ hướng dẫn. Nếu hết thời gian mà chưa đủ bằng chứng thì yêu cầu quay lại.
- Trái/phải theo người quay, có test preview mirror và rotation. Dùng khoảng giữ tư thế, hysteresis và cửa sổ thời gian có dung sai; không yêu cầu đúng tuyệt đối từng mốc 3 giây của UI.
- Blink cần chuỗi mở → nhắm → mở qua nhiều frame hợp lệ; một frame nhắm mắt không đủ.
- `liveness_score` là chỉ số tổng hợp bằng chứng động tác, không phải xác suất người thật. Công thức và ngưỡng phải được lưu theo `pipeline_version`.
- Muốn `VERIFIED` phải đủ tất cả động tác và qua ngưỡng tin cậy; không lấy match score cao bù liveness thất bại.

Chuỗi cố định và video upload có thể bị phát lại hoặc thay bằng video dựng; kiểm tra động tác không chứng minh chống replay/deepfake. MVP phải ghi rõ giới hạn này. Trước khi tuyên bố chống phát lại, cần giai đoạn riêng: Go phát hành challenge ngẫu nhiên có nonce, TTL và ràng buộc attempt; Python xác minh hành động tương ứng và có đánh giá presentation attack/injection. Nonce đơn lẻ không chứng minh video quay mới. Nếu yêu cầu chống giả mạo chuyên biệt, tích hợp PAD như một kiểm tra riêng, không suy diễn từ embedding.

### 5.5 So khớp danh tính

Với embedding L2-normalized `u` của chân dung và `v_i` của các frame hợp lệ:

```text
cos_i = dot(u, v_i)                      # [-1, 1]
score_i = clip((cos_i + 1) / 2, 0, 1)    # [0, 1], giữ hợp đồng score
match_score = median(score_i)
```

Đây là quy ước cho adapter dùng cosine; model dùng metric khác phải có mapping được định nghĩa trong manifest và hiệu chuẩn lại. `match_score` không phải phần trăm chắc chắn cùng người.

Chỉ đạt khi đủ frame hợp lệ, tính liên tục danh tính đạt, `match_score >= MATCH_THRESHOLD` và tỉ lệ frame đạt ngưỡng ≥ `MIN_MATCH_FRAME_RATIO` đã hiệu chuẩn. Không lấy `max(score_i)` làm quyết định vì một frame khớp không đại diện cả video.

Giá trị 0.78 và 0.85 hiện có không được coi là ngưỡng đã kiểm chứng cho model thật. Không tự đặt score bằng threshold khi không inference được.

### 5.6 Phân tích realtime và xác minh cuối

- Tính embedding căn cước một lần khi tạo attempt, dùng cùng model/version cho toàn attempt. Không tách chân dung lại cho từng chunk.
- Liveness dùng timeline decode liên tục, mục tiêu ban đầu 15 FPS phân tích; matching chạy trên frame chất lượng tốt khoảng 3–5 lần/giây. Hai nhịp độc lập với preview mục tiêu 30 FPS.
- Kết quả từng frame chỉ là bằng chứng tạm. Trước đủ số frame, độ phủ thời gian và động tác, trạng thái là `collecting`; UI không hiển thị thành công chỉ vì một frame khớp.
- Khi đã có đủ bằng chứng tạm, trạng thái là `candidate_match`; thông báo cho người dùng tiếp tục giữ camera đến khi hoàn tất. Không phát `VERIFIED` qua sự kiện tiến độ.
- Bước complete vẫn phân tích video hoàn chỉnh bằng pipeline thật để kiểm tra đầy đủ timeline và thống nhất bằng chứng; realtime giúp phản hồi sớm trong lúc quay. Không dùng kết quả tạm thay cho kết quả tổng hợp cuối.
- Nếu hai kết quả khác nhau, dùng kết quả xác minh cuối; ghi diagnostic theo attempt, không ghi raw embedding. Không bỏ qua lỗi cuối chỉ vì trước đó từng báo khớp.
- Model đang lỗi/quá tải hoặc frame mới chưa được xử lý thì hiển thị trạng thái chờ/kết nối phù hợp; không tăng tiến độ giả. Không giữ kết quả cũ như thể đang phân tích camera hiện tại.

## 6. API và quyết định

### 6.1 Mobile → Go: giữ hợp đồng đang dùng

`POST /api/kyc/complete`, `Authorization: Bearer <kycToken>`, multipart:

| Field | Nội dung |
|---|---|
| `idCardFront` | Ảnh mặt trước |
| `idCardBack` | Ảnh mặt sau |
| `liveVideo` | File video quay từ camera |
| `challengeType` | `TURN_HEAD_LEFT_RIGHT_BLINK` |

Giữ envelope `httpx.SuccessResponse` và `data.decision`, `data.reasonCode`, `data.reasonMessage`, `data.user` đang dùng. Chỉ `VERIFIED` mới có user đã xác minh. Không trả embedding, crop sinh trắc học hoặc ngưỡng ra client. Token KYC không được đổi thành access token trong bước này.

Luồng realtime bổ sung multipart `realtimeAttemptId` khi complete. Go kiểm tra attempt thuộc account trong token, còn hạn, stream đã kết thúc và hash các bytes video nhận qua stream trùng `liveVideo`. Hash ảnh cũng phải trùng ảnh khi mở attempt. Hash do Go tính, không tin hash do client khai báo. Sai ràng buộc → từ chối; không sử dụng bằng chứng của attempt khác. Client cũ thiếu field có thể giữ luồng upload cũ trong giai đoạn migration, nhưng luồng đó không được tính là đã hoàn thành tính năng realtime.

### 6.2 Go → Python: giữ endpoint và bổ sung metadata

`POST /v1/kyc/verify-face`, multipart: `cccd_front_image`, `live_video`, `challenge_type`. Go sinh và truyền `X-Request-ID` cho mỗi attempt để đối chiếu kết quả và log. Python echo ID trong metadata.

Giữ field response hiện tại; bổ sung object `analysis` cho Go lưu bằng chứng. Ví dụ minh họa cấu trúc, không phải điểm benchmark:

```json
{
  "success": true,
  "decision": "VERIFIED",
  "reason_code": null,
  "reason_message": null,
  "match_score": 0.91,
  "liveness_score": 0.94,
  "quality_checks": {
    "cccd_portrait_extracted": true,
    "single_face_in_video": true,
    "image_quality_passed": true
  },
  "analysis": {
    "request_id": "server-generated-attempt-id",
    "model_id": "configured-face-embedding",
    "model_version": "weights-v1",
    "model_sha256": "<sha256>",
    "pipeline_version": "real-v1",
    "calibration_version": "calibration-v1",
    "matching_status": "passed",
    "liveness_status": "passed",
    "subject_consistency_passed": true,
    "challenge_passed": true,
    "sampled_frames": 180,
    "matched_frames": 8,
    "video_duration_seconds": 12.0,
    "processing_ms": 4200
  }
}
```

Các field `analysis` là bổ sung **đề xuất**, phải triển khai đồng thời schema Python, DTO Go và persistence. Trạng thái kiểm tra dùng `not_run | passed | failed`; score trả 0 khi chưa chạy để tương thích field number hiện có, nhưng `not_run` mới là thông tin phân biệt chưa chạy với kết quả thấp.

| Tình huống | Python | Go → mobile |
|---|---|---|
| Mọi kiểm tra bắt buộc đạt | HTTP 200, `VERIFIED`, `success=true` | HTTP 200, `VERIFIED` sau khi lưu thành công |
| Media giải mã được nhưng chất lượng/động tác/match chưa đạt | HTTP 200, `RETRY_ALLOWED`, `success=false` | `RETRY_ALLOWED` hoặc `FAILED_FINAL` theo lượt còn lại |
| Request sai, file hỏng, challenge không hỗ trợ | HTTP 400/413/422 | Lỗi request theo envelope lỗi hiện có; không tính là face mismatch |
| Model chưa sẵn sàng, timeout, hết tài nguyên, inference lỗi | HTTP 503/500 hoặc lỗi transport | HTTP 502, `data.decision=SYSTEM_ERROR`; không trừ lượt sinh trắc học |

Python không phát `FAILED_FINAL` theo số lượt. Các endpoint riêng `/extract-id-face`, `/run-liveness`, `/match-face` tiếp tục dùng chung component xử lý thật; `VERIFIED` của `/run-liveness` chỉ diễn tả kiểm tra liveness đạt, không đủ quyền xác minh tài khoản. Go chỉ dùng kết quả tổng hợp từ `/verify-face`.

Go bắt buộc kiểm tra enum, `success`, mọi score hữu hạn trong [0,1], đủ metadata và tất cả cờ kiểm tra khi `VERIFIED`. Thiếu `analysis`, quality checks false, `not_run`, request ID sai hoặc response mâu thuẫn → `SYSTEM_ERROR`; không lưu verified. Python là dịch vụ nội bộ có xác thực giữa service, không nhận bearer token của người dùng làm thông tin xác thực service.

### 6.3 Lỗi nghiệp vụ và hướng dẫn mobile

| Reason code hiện có | Hành động |
|---|---|
| `CCCD_IMAGE_QUALITY_LOW`, `CCCD_PORTRAIT_NOT_FOUND` | Quay về bước chọn/chụp lại căn cước |
| `LIVE_FACE_NOT_FOUND`, `MULTIPLE_FACES_DETECTED` | Quay lại một mình, giữ mặt trong khung |
| `LIVE_VIDEO_QUALITY_LOW`, `FACE_OCCLUDED` | Quay lại ở nơi đủ sáng, bỏ vật che mặt |
| `LIVENESS_CHALLENGE_FAILED`, `LIVENESS_NOT_CONFIDENT` | Quay lại và làm đủ động tác |
| `FACE_MISMATCH` | Thông báo khuôn mặt chưa khớp, cho thử lại theo hạn mức |
| `INTERNAL_ERROR` | Giữ phiên, cho thử lại, không trừ lượt |

Không đạt tính liên tục người quay ánh xạ `LIVENESS_NOT_CONFIDENT` trong MVP. Mã lỗi mới phải cập nhật enum, message, client và contract test cùng lúc.

### 6.4 API realtime đề xuất

Các API dưới đây chưa tồn tại; phải triển khai Go, Python và mobile cùng phiên bản.

| API | Hợp đồng |
|---|---|
| `POST /api/kyc/realtime/attempts` | KYC bearer + `Idempotency-Key`; multipart `idCardFront`, `idCardBack`, `challengeType`. Trả `attemptId`, `expiresAt`, `streamPath`, `streamTicket`, `streamTicketExpiresAt`, MIME/codec cho phép và cấu hình capture |
| `GET /api/kyc/realtime/attempts/{attemptId}/stream` | Upgrade WebSocket qua WSS ngoài localhost; chỉ truyền media sau khi xác thực ticket một lần |
| `DELETE /api/kyc/realtime/attempts/{attemptId}` | KYC bearer; hủy attempt thuộc tài khoản, dọn decoder/file tạm, không tự tính là mismatch |

Ảnh không đạt phải trả mã lỗi trước khi mở camera ghi hình. `streamTicket` là credential ngắn hạn riêng cho attempt, TTL ban đầu 30 giây; attempt TTL ban đầu 2 phút và không vượt hạn KYC token. Trình duyệt gửi ticket trong message `authenticate` đầu tiên, không đưa token KYC hoặc ticket lên query URL/log. Server chỉ cho phép message auth nhỏ trước xác thực, deadline 5 giây; kiểm tra Origin allowlist và giới hạn số socket chưa xác thực. Khi nối lại, xin ticket mới qua API KYC có xác thực; không phát lại video cũ thành một lượt quay mới.

Protocol version `1`:

1. Client `authenticate` → server `ready` sau khi kiểm tra ownership, TTL và cấp worker. `ready` chứa `attemptId`, `eventSeq`, giới hạn media và codec đã thương lượng.
2. Client `start` với MIME/codec được hỗ trợ → server `started`; sau đó client gửi binary message: header version + chunk sequence tăng dần + các bytes chunk video. Timestamp client chỉ dùng diagnostic; timeline liveness lấy từ decoder.
3. Server phát `progress` có `attemptId`, `eventSeq`, `processedMediaTimeMs`, `faceState`, `qualityState`, `challengeStep`, `challengePassed`, `matchingState`, `feedbackCode`. Các trạng thái đều lấy từ inference đã hoàn thành; không gửi score/threshold hoặc embedding ra mobile.
4. Khi `MediaRecorder.stop()` phát chunk cuối, client gửi chunk đó trước rồi mới gửi `end` với `lastChunkSeq`. Server chỉ phát `stream_finished` sau khi nhận đủ, flush decoder và chốt hash. `stream_finished` không đồng nghĩa tài khoản đã verified.
5. Client gọi complete với cùng file ghép từ chính chuỗi chunk đã gửi, `realtimeAttemptId` và key của attempt. Go xác minh cuối và lưu kết quả theo luồng 6.1–6.2.

Go nối Python bằng WebSocket nội bộ `/v1/kyc/realtime` có service auth: gửi cấu hình/context tham chiếu đã được kiểm tra, chuyển media và nhận các sự kiện có request/attempt ID. Python không quyết định hạn mức hoặc ghi tài khoản. Go sở hữu file tạm/hash để ràng buộc stream với complete; Python giữ context trong worker đã cấp cho socket. Khi nhiều replica, socket phải gắn đúng worker suốt attempt; mất worker → lỗi hệ thống và quay lại, không ghép state từ worker khác.

Chunk phải giữ nguyên thứ tự, không trùng, không thiếu; server kiểm tra sequence, tổng bytes, TTL, giới hạn message và tốc độ. Không decode từng Blob như file MP4/WebM độc lập: cần decoder incremental theo container/codec, giữ header và state giữa các chunk. Dùng `timeslice` khoảng 200–300 ms làm cấu hình ban đầu; đây không phải bảo đảm chunk đến đúng chu kỳ. Chỉ bật MIME/codec đã chứng minh decode incremental trên thiết bị mục tiêu. Nếu codec chỉ cho kết quả sau khi stop, thiết bị đó chưa đạt realtime; thông báo rõ thay vì giả tiến độ.

Mất socket, gap media, ứng dụng xuống nền hoặc camera bị ngắt → dừng ghi, hủy tiến độ đang hiển thị và yêu cầu quay lại; không trừ lượt sinh trắc học do lỗi hệ thống. Mở lại attempt chịu rate limit để tránh dùng hủy kết nối vượt hạn mức. Socket không hoạt động hoặc vượt TTL phải giải phóng tài nguyên ở cả Go và Python.

## 7. Lưu kết quả, retry và tính nhất quán

### 7.1 Hồ sơ xác minh

Mở rộng `VerifyAccountCommand` để nhận evidence có kiểu từ orchestration của Go; không nhận evidence trực tiếp từ request client. Không đánh dấu verified chỉ nhờ ba storage key hợp lệ.

- `OCRStatus = not_run` vì chưa chạy OCR.
- `FaceMatchStatus = passed`, `FaceMatchScore` lấy từ inference thật.
- `LivenessStatus = passed`, thêm `LivenessScore`.
- Giữ `Provider`, `ProviderReferenceId`; thêm model ID/version/checksum, pipeline/calibration version, quality checks, challenge, timestamp và duration.
- Điền `MediaObject` với content type, size, SHA-256 và thông tin object tương ứng; không thay storage key bằng URL public.
- Lưu evidence hash của mặt trước/video để gắn kết quả với đúng bytes đã phân tích và lưu.
- Cập nhật domain, Mongo record mapper, migration tương thích và test round-trip. Session cũ vẫn giữ `not_run`, không tự backfill thành `passed`.

### 7.2 Lượt thử và idempotency

Đề xuất Go giới hạn 3 lần phân tích không đạt trong một cửa sổ 24 giờ mỗi tài khoản; đây là policy mới, cấu hình được. Hết lượt trả `FAILED_FINAL` cho cửa sổ hiện tại; sau `retryAfter` mới tiếp tục. Đổi KYC token hoặc mở lại app không reset bộ đếm. Auth/rate limit có bộ đếm riêng cho request sai và lưu lượng bất thường.

Thêm header `Idempotency-Key` trên request complete. Go ràng buộc key với account + hash media + challenge, giữ tối thiểu 24 giờ:

Với realtime, dùng key tạo attempt xuyên suốt đến complete. Lúc create bind key với account + hash hai ảnh + challenge; lúc complete bổ sung hash video một lần. Không coi create và complete là hai lượt. Tạo attempt để giữ chỗ trong hạn mức; chỉ chốt trừ lượt một lần khi có kết quả nghiệp vụ cuối. Cleanup lease phải giải phóng chỗ của attempt lỗi/hết hạn, tránh khóa tài khoản.

- Cùng key, cùng nội dung, đã xong → trả lại kết quả, không inference/trừ lượt/upload lại.
- Cùng key, khác nội dung → HTTP 409 theo error envelope; cập nhật OpenAPI.
- Đang xử lý → không tạo job thứ hai; trả trạng thái retry có kiểm soát, không trừ thêm lượt.
- `SYSTEM_ERROR` → cho retry cùng key; không cache lỗi thành kết quả vĩnh viễn.
- Client giữ key khi retry do mất mạng, sinh key mới khi chụp/quay lại.

Go lưu attempt metadata với trạng thái `processing | completed | system_error`, decision, reason và evidence; không bắt buộc lưu media thô của attempt thất bại. Nếu dùng chung `kyc_sessions`, phải mở rộng status và mapper vốn đang yêu cầu storage key; hoặc tạo collection attempt riêng. Không ghi session thất bại với storage key giả.

Trả thêm `attemptsRemaining` và `retryAfter` (nullable) trong `data` để client dùng số lượt authoritative. Việc ghi kết quả và trừ lượt phải nguyên tử/idempotent; hai request đồng thời không vượt hạn mức. Có lease cho attempt đang chạy để request mất kết nối không khóa tài khoản vô thời hạn.

### 7.3 Media và commit

Giữ nguyên tắc chỉ lưu media chính thức khi analysis đạt. Object storage và Mongo không có transaction chung: dùng key theo attempt, conditional update/version khi commit verified và tiến trình dọn orphan.

Nếu upload hoặc lưu DB thất bại, tài khoản chưa verified và trả lỗi hệ thống. Chỉ xóa object được xác nhận chưa có session đã commit tham chiếu; khi DB timeout không rõ kết quả, đọc lại bằng attempt ID trước khi bù trừ. Request thua trong cuộc đua xác minh phải dọn object riêng của nó, giữ nguyên session verified đầu tiên.

Media riêng tư, không public qua route tải file chung. Log không chứa ảnh, video, embedding, token hoặc số căn cước đầy đủ. File tạm xóa ngay; embedding không lưu mặc định. Retention media chính thức phải được cấu hình rõ trước triển khai, có cơ chế xóa và kiểm tra orphan; không dùng dữ liệu người dùng làm dữ liệu huấn luyện mặc định.

## 8. Cấu hình và vận hành

Giữ settings/env làm đầu vào triển khai; nhóm tham số xử lý và manifest/calibration trong file TOML có phiên bản. `pipeline.py` chỉ orchestration, không nhét toàn bộ decode, model và threshold vào một file.

```text
VISION_PIPELINE_BACKEND=real
ALLOW_STUB_VISION_PIPELINE=false
VISION_CONFIG_PATH=/etc/anomaly/vision.toml
FACE_MODEL_MANIFEST_PATH=/models/face/manifest.json
```

TOML chứa giới hạn decode, layout thẻ, quality gates, sampling, động tác, model provider, frame aggregation và calibration thresholds. Không có nhiều nguồn threshold ghi đè mơ hồ; startup báo cấu hình hiệu lực không chứa dữ liệu nhạy cảm. `MATCH_THRESHOLD`/`LIVENESS_THRESHOLD` hiện tại cần được chuyển đổi hoặc quy định precedence rõ khi migration.

Tách module đề xuất: `document_processor.py`, `video_processor.py`, `liveness.py`, `embedding_model.py`, `face_matcher.py`, `real_pipeline.py`; inject qua `VisionPipeline` hiện có.

- Stub chỉ dùng unit/contract test. Staging/production từ chối khởi động với backend stub dù bật override.
- Thêm `/ready` kiểm tra model/calibration; `/health` chỉ phản ánh tiến trình còn sống.
- Không chạy decode/inference nặng trực tiếp trên async event loop; dùng worker/executor có giới hạn, queue bounded và backpressure.
- Với timeout Go → Python hiện tại 45 giây: ngân sách Python đề xuất ≤30 giây gồm chờ queue; không nhận việc nếu không còn ngân sách. Kiểm tra timeout toàn Go request vẫn đủ cho upload và commit; client hiện có 120 giây.
- CPU là cấu hình baseline để benchmark; GPU là tùy chọn. Provider runtime phải khai báo rõ; không đổi provider âm thầm khi yêu cầu GPU bắt buộc.
- Đo latency từng stage, p50/p95, queue, bộ nhớ, số frame, tỉ lệ từng reason và tỷ lệ lỗi hệ thống. Không log vector sinh trắc học.
- Model/calibration mới phải phát hành theo cặp; rollback quay lại cặp artifact thật đã kiểm chứng. Nếu chưa có cặp đạt, tắt KYC và báo lỗi hệ thống.

### 8.1 Giữ camera mượt, giới hạn tải realtime

- Preview dùng trực tiếp `<video srcObject>`; một camera stream và một recorder. Không render từng frame bằng React, không gán lại `srcObject` theo tiến độ và không vẽ toàn bộ preview qua canvas chỉ để hiển thị camera.
- Capture baseline 640×480, mục tiêu 30 FPS, bitrate khoảng 1–1.2 Mbps; đề xuất ràng buộc FPS tối đa 30 để tránh camera tự chạy 60 FPS làm tăng tải. Dùng chất lượng cao hơn chỉ khi dữ liệu nghiệm thu chứng minh cần và thiết bị chịu được. Không hạ độ rõ chân dung đến mức sai matching.
- Main thread chỉ điều phối UI, recorder và chuyển binary chunk; không encode JPEG/base64 liên tục, không infer model, hash cả video hoặc copy các buffer lớn đồng bộ trên main thread. Hash ở Go; preprocessing nặng phía client nếu thực sự cần thì đặt ở Worker có feature detection.
- Ref giữ chunk/timestamp/trạng thái tần suất cao; React chỉ cập nhật hướng dẫn khi đổi trạng thái và timer khoảng 4 lần/giây. Không giữ mảng frame hoặc Blob trong state. Overlay ít lớp, tránh blur/shadow/filter động phủ video; đo bằng profiler trước khi quyết định hiệu ứng.
- Client giữ tối đa tổng video 25 MiB và buffer gửi có giới hạn. WebSocket không tự quản lý backpressure; theo dõi `bufferedAmount`, ACK sequence và tuổi chunk chưa xử lý. Mốc ban đầu: cảnh báo buffer 512 KiB hoặc tuổi backlog >500 ms, dừng attempt nếu >1 MiB hoặc backlog >1 giây kéo dài; cấu hình và benchmark lại theo codec/thiết bị/mạng.
- Không bỏ chunk video nén khi nghẽn vì có thể làm hỏng decoder/timeline. Khi không đủ băng thông, báo kết nối chậm và kết thúc attempt có kiểm soát; cho quay lại với cấu hình capture đã được kiểm chứng. Không pause recorder rồi ghép hai đoạn thành liveness liên tục.
- Decoder Go/Python có queue bounded. Sau decode, matcher có thể bỏ frame cũ chưa xử lý và ưu tiên frame mới; nhánh liveness phải theo dõi coverage và gap riêng. Thiếu coverage cần thiết cho blink/động tác → không kết luận đạt. Giảm nhịp embedding trước khi ảnh hưởng preview hoặc liveness; không giảm ngưỡng để bù quá tải.
- Mỗi attempt có giới hạn CPU/RAM, một context model dùng lại, không tạo process/inference session cho từng frame. Hạn chế số phiên realtime theo benchmark; hết slot thì trả lỗi hệ thống trước khi bắt đầu quay.
- Khi hủy/đổi màn hình: hủy callback/timer, đóng socket, dừng recorder và camera tracks, xóa handler/buffer. Không để lần mở camera tiếp theo chạy nhiều vòng xử lý chồng nhau.

### 8.2 Mục tiêu nghiệm thu realtime

Các mốc sau là mục tiêu cần đo trên danh sách điện thoại/WebView/Tauri và cấu hình server được ghi rõ, không suy ra từ máy phát triển:

| Chỉ số | Mục tiêu ban đầu |
|---|---|
| Preview lúc stream + inference | Trung bình ≥24 FPS với camera mục tiêu 30 FPS; không có freeze ≥250 ms do tác vụ ứng dụng |
| Tác vụ trên main thread do mã KYC | p95 <16 ms, không có tác vụ >50 ms do xử lý media của ứng dụng |
| Độ trễ frame đến feedback hiển thị | p95 ≤500 ms trên mạng nghiệm thu RTT ≤100 ms, uplink đủ bitrate; đo cả thời gian chờ chunk, mạng, queue, decode và inference |
| Sự kiện tiến độ | 3–5 lần/giây khi có bằng chứng mới; sự kiện đổi bước gửi ngay, không spam render theo FPS camera |
| Hoàn tất sau stop | p95 ≤5 giây trên video 12 giây ở cấu hình nghiệm thu, gồm upload complete, kiểm tra cuối và commit; nếu chưa đạt thì chưa hoàn thành mục tiêu trải nghiệm |
| Buffer và tài nguyên | Bounded ở cả client/Go/Python; không tăng dần qua 10 lượt mở–quay–hủy liên tiếp |

Đo bằng profiling thực tế: preview frame timing, dropped frames nếu WebView hỗ trợ, main-thread traces, timestamp capture/chunk, ACK, processed media time, latency từng stage và thời gian commit. Độ trễ một chiều cần phương pháp đồng bộ clock hoặc trace có ước lượng sai số; không trừ timestamp từ hai máy chưa đồng bộ. Ghi baseline preview-only để xác định phần giật do ứng dụng thêm vào. Kiểm chứng khi bật tiết kiệm pin, máy nóng và mạng yếu; quá ngân sách thì xử lý rõ trạng thái lỗi, không báo đang realtime bằng kết quả cũ.

### 8.3 Khung camera lớn, chi tiết phụ gọn

Hiện `.camera-frame` trong `registration.css` cao cố định 260 px; vùng oval chỉ 155×206 px. `FaceStep` và layout đăng ký còn có tiêu đề/mô tả, thương hiệu, tiến trình, hướng dẫn, chấm bước, số lượt, notice và footer xếp dọc. Khi triển khai cần chỉnh cả bố cục bước quay và phần bao ngoài để camera có thêm không gian thực tế.

- Bước khuôn mặt chuyển sang bố cục ưu tiên camera. Trên mobile, preview trải gần hết chiều ngang, gutter tối đa 12 px mỗi bên; chiều cao co giãn theo viewport khả dụng thay vì giữ 260 px. Mục tiêu preview chiếm ít nhất 60% chiều cao viewport sau khi trừ safe area ở cỡ chữ mặc định, trên màn hình dọc từ 320×568 trở lên.
- Khi đang quay, thu gọn thương hiệu, mô tả dài, tiến trình đăng ký và footer trang trí. Header chỉ giữ tên bước ngắn cùng hành động thoát/hỗ trợ hợp lệ; không thêm một hàng riêng cho từng nhãn trạng thái. Trước khi quay có thể hiển thị hướng dẫn đầy đủ trong sheet trợ giúp hoặc phần thu gọn.
- Trong preview chỉ có oval mảnh, hướng dẫn động tác hiện tại ngắn, thời gian và trạng thái kết nối cần thiết. Đặt nhãn ở mép trên/dưới ngoài vùng khuôn mặt; không che mắt, miệng hoặc khoảng trống hai bên khi quay đầu. Hướng dẫn tối đa hai dòng; thông tin chi tiết mở trong trợ giúp.
- Số lượt còn lại và tiến độ động tác dùng một hàng gọn; không giữ notice dài và chấm bước ở các khối có margin lớn bên dưới camera. Nút mở camera/bắt đầu quay/đang quay dùng cùng một vùng điều khiển phía dưới, không xếp đồng thời thành nhiều nút hoặc đẩy camera ra khỏi viewport.
- Oval tính theo kích thước preview và vùng mặt detector cần thấy, có giới hạn để chừa khoảng quay trái/phải; không giữ kích thước cố định 155×206 px. Oval và preview chỉ là hướng dẫn UI, không thay đổi crop/alignment hoặc threshold của model.
- Tăng khung hiển thị không tự tăng độ phân giải capture hoặc bitrate. Giữ baseline ở 8.1 để tránh tăng tải. Preview crop bằng `object-fit` phải được kiểm tra với tỉ lệ camera thực; overlay ánh xạ đúng vùng video nhìn thấy. Nếu `cover` cắt mất trán/cằm hoặc khoảng quay đầu thì chỉnh tỉ lệ khung hoặc dùng `contain`, không phóng lớn đến mức người dùng không thấy vùng đang được kiểm tra.
- Bố cục dựa trên dynamic viewport (`100dvh` nếu hỗ trợ), tính safe-area inset và có fallback theo WebView. Không cần trình duyệt Fullscreen API để có khung lớn. Ở màn hình thấp/ngang hoặc khi tăng cỡ chữ, cho phép cuộn có kiểm soát và ưu tiên đọc/nút điều khiển, không ép chữ nhỏ hay che nút để đạt tỷ lệ 60%.
- Chỉ áp dụng thay đổi layout chuyên biệt cho bước khuôn mặt; giữ khả năng sử dụng của các bước đăng ký khác. Khi lỗi cần quay/chụp lại, hiển thị thông báo gọn và hành động rõ; không chèn banner dài làm khung camera thay đổi kích thước liên tục trong lúc quay.

Nghiệm thu trực quan trên viewport 320×568, 360×640, 390×844, màn hình ngang và desktop, có safe area và cỡ chữ lớn: khung lớn hơn bản cũ, đạt tỷ lệ mục tiêu ở màn hình dọc/cỡ chữ mặc định, thấy mặt và khoảng quay đầu, nút chính không bị khuất, không phải cuộn để vừa xem mặt vừa làm động tác. Khi ngoại lệ accessibility cần cuộn phải ghi rõ trong kết quả kiểm tra. Đo lại FPS/latency ở 8.2 với layout mới; ảnh chụp màn hình chỉ chứng minh bố cục, không chứng minh quay mượt.

## 9. Hiệu chuẩn và nghiệm thu

### 9.1 Bộ dữ liệu

Dùng ảnh thẻ và video thật có quyền sử dụng: cùng người và khác người; nhiều thiết bị, ánh sáng, góc, tuổi ảnh thẻ, kính và layout được hỗ trợ. Chia train/calibration/test theo **danh tính**, không để cùng người lọt qua các tập. Không đánh giá bằng các frame gần nhau trong cùng một video ở hai tập.

Hiệu chuẩn trên đúng cách tổng hợp đa frame và quality gates dùng lúc chạy. Báo cáo số người, số cặp genuine/impostor, false match rate, false non-match rate, tỉ lệ không thu được mẫu và latency theo phần cứng. Không dùng một con số accuracy tổng hợp để chọn threshold.

Mục tiêu kỹ thuật ban đầu đề xuất: FMR ≤0.1%, FNMR ≤5%, p95 xử lý Python ≤30 giây cho API phân tích file độc lập trên cấu hình phần cứng được ghi rõ. Luồng realtime bắt buộc đạt ngân sách chặt hơn ở 8.2, gồm cả xác minh cuối; đạt mốc 30 giây không đủ nghiệm thu realtime. Đây là điều kiện cần đo để quyết định phát hành, chưa phải kết quả đạt được. Nếu không đủ số mẫu để hỗ trợ mục tiêu FMR thì ghi thiếu bằng chứng; zero false match trên tập nhỏ không chứng minh đạt mục tiêu. Báo cáo thêm khoảng tin cậy và nhóm điều kiện khó.

Đánh giá liveness riêng bằng ảnh in, ảnh trên màn hình, video phát lại, video thiếu/sai thứ tự động tác và đổi người. Báo cáo giới hạn replay/deepfake; không tuyên bố PAD đạt khi chỉ có kiểm tra động tác.

### 9.2 Ma trận kiểm chứng bắt buộc

| Trường hợp | Kỳ vọng |
|---|---|
| Căn cước + video cùng người, đủ động tác/chất lượng | Inference thật; verified sau commit; score và version lưu đúng |
| Hai người khác nhau | Không verified trong bộ test nghiệm thu |
| Cùng media đổi tên thành `pass`, `high`, `mismatch` | Kết quả/score không đổi |
| Tên file giống nhau nhưng nội dung khác | Kết quả phụ thuộc nội dung |
| Video tĩnh, thiếu blink, quay sai thứ tự | Không verified |
| Hai mặt hoặc đổi người trong clip | Không verified |
| Layout thẻ chưa hỗ trợ, không thấy chân dung, ảnh quá mờ | Mã lỗi đúng và yêu cầu chụp lại |
| MP4/WebM từ camera thật; ảnh xoay/preview mirror | Decode, trái/phải, timestamp đúng |
| MIME giả, file hỏng, duration/pixel/frame vượt giới hạn | Từ chối có kiểm soát, không tăng bộ nhớ vô hạn |
| Thiếu weights, sai checksum, output NaN, runtime timeout | Không verified; không fallback mock; không trừ lượt sinh trắc học |
| Python trả verified nhưng thiếu evidence/cờ false | Go trả lỗi hệ thống |
| Retry mất mạng, cùng key, hai request đồng thời | Một kết quả chính thức, một lần trừ lượt, không orphan tồn đọng |
| Storage lỗi hoặc DB timeout khi commit | Không báo thành công giả, reconciliation giữ media của session đã commit |
| Reload ứng dụng hoặc xin lại KYC token | Số lượt không reset |
| Session trước khi chuyển model thật | Giữ lịch sử `not_run`, không giả bằng chứng |
| Người quay chưa dừng video | Nhận feedback/đối chiếu từ inference thật khi camera vẫn chạy; không chờ toàn bộ file |
| Timer UI chạy nhưng server không phát progress | Không tự đánh dấu động tác đạt hoặc khuôn mặt khớp |
| Stream và `liveVideo` complete khác bytes hoặc khác ảnh tham chiếu | Từ chối attempt, không xác minh bằng bằng chứng khác |
| Chunk thiếu/trùng/sai thứ tự, nghẽn mạng, xuống nền | Dừng có kiểm soát, dọn tài nguyên, không giữ kết quả tạm thành verified |
| Mạng chậm, inference chậm, server hết slot | Preview không bị block bởi queue; bộ nhớ bounded, hiển thị lỗi/chờ đúng |
| MP4/WebM chia chunk từ thiết bị thật | Decoder trả frame và progress trước stop; không coi từng chunk là file độc lập |
| 10 lượt mở–quay–hủy và profile khi quay | Không tăng track/socket/timer/buffer; đạt tiêu chí FPS/latency/main-thread ở 8.2 |
| Layout camera mới trên các viewport ở 8.3 | Preview chiếm phần lớn màn hình; mặt/động tác và nút chính nhìn rõ; chi tiết phụ không làm khung quay nhỏ lại |

Unit test được mock detector/model để test orchestration. Nhóm nghiệm thu bắt buộc chạy backend `real` với weights thật và media thật; không coi fixture bytes giả hoặc test stub hiện tại là bằng chứng face matching hoạt động.

## 10. Thứ tự triển khai

1. Chốt artifact model, manifest, các layout thẻ hỗ trợ và bộ media nghiệm thu; kiểm chứng inference offline hai nguồn dùng cùng preprocessing.
2. Xây adapter model, decode/portrait, theo dõi video, liveness và matcher thành module riêng; có output chẩn đoán không chứa media thô.
3. Lắp `RealVisionPipeline` vào factory; đồng bộ các endpoint riêng và tổng hợp, readiness, config và loại bỏ fallback stub. Thêm context realtime, decoder incremental và các API/WebSocket ở 6.4.
4. Mở rộng DTO/schema evidence; Go kiểm tra kết quả, truyền xuống command và lưu Mongo đúng trạng thái/score/version.
5. Hoàn thiện retry/idempotency trên Go, race/compensation và số lượt authoritative; mobile stream cùng lúc ghi, nhận tiến độ từ server, tối ưu preview/buffer theo 8.1 và mở rộng khung camera/thu gọn chi tiết phụ theo 8.3.
6. Chạy bộ dữ liệu thật để hiệu chuẩn; benchmark, test luồng camera → Go → Python → object storage → Mongo trên thiết bị thật, gồm ma trận realtime và mục tiêu mượt ở 8.2.
7. Cập nhật README, env example, Docker/Nix dependencies, OpenAPI, hướng dẫn cấp weights và báo cáo nghiệm thu trước chuyển môi trường chạy thật.

Chỉ hoàn tất thay mock khi bước 1–6 chạy được bằng backend thật và có bằng chứng DB/media tương ứng. Chạy build hoặc API contract test đơn thuần chưa đủ nghiệm thu.

Chỉ hoàn tất yêu cầu cập nhật khi có feedback dựa trên model thật **trước lúc dừng quay**, xác minh cuối gắn đúng video và số đo FPS/latency đạt trên thiết bị mục tiêu. Một animation tiến độ mượt hoặc upload nhanh sau quay không thay thế các tiêu chí này.

## 11. Tài liệu kỹ thuật tham chiếu

- [ONNX Runtime Python API](https://onnxruntime.ai/docs/api/python/api_summary.html): `InferenceSession` và cấu hình execution provider cho adapter ONNX.
- [MediaPipe FaceLandmarkerOptions](https://ai.google.dev/edge/api/mediapipe/python/mp/tasks/vision/FaceLandmarkerOptions) và [FaceLandmarkerResult](https://ai.google.dev/edge/api/mediapipe/python/mp/tasks/vision/FaceLandmarkerResult): landmarks, blendshapes và transformation matrices là dữ liệu đầu vào hỗ trợ phân tích động tác; thuật toán liveness trong spec là phần ứng dụng phải xây dựng và kiểm chứng.
- [MediaStream Recording API](https://developer.mozilla.org/en-US/docs/Web/API/MediaStream_Recording_API) và [MediaRecorder dataavailable](https://developer.mozilla.org/en-US/docs/Web/API/MediaRecorder/dataavailable_event): Blob từng phần không nhất thiết phát độc lập và `timeslice` không có nhịp chính xác; cần kiểm chứng decoder streaming theo codec.
- [WebSocket](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket) và [bufferedAmount](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/bufferedAmount): theo dõi số bytes đang đợi và tự giới hạn queue trong ứng dụng.

Các thư viện trên là phương án tích hợp, không ấn định model embedding hay thay thế báo cáo hiệu chuẩn trên dữ liệu của Anomaly.

## 12. Trạng thái triển khai trong checkout

Đã có code cho adapter ONNX/MediaPipe, pipeline thật, decoder streaming WebM/VP8, API realtime qua Go, ticket một lần, kiểm tra hash khi complete, retry/idempotency/giới hạn lượt qua Redis, lưu evidence và khung camera lớn. Bước xác minh file hoàn chỉnh dùng file tạm seekable để đọc cả MP4 có chỉ mục cuối file; file tạm được xóa sau xử lý. Go có worker reconciliation cho media chưa được hồ sơ canonical tham chiếu.

Cấu hình và hướng dẫn chạy: `kyc-service/README.md`, `kyc-service/config/manifest.example.json`, `kyc-service/config/vision.example.toml`, `server/.env.example`. Migration tương ứng là `000009_real_kyc_evidence`; chưa tự chạy migration trên cơ sở dữ liệu người dùng.

Các phần cần nghiệm thu trước khi coi mục tiêu hoàn tất:

- Chưa có weights/model embedding, MediaPipe task và cấu hình hiệu chuẩn của người triển khai. Mẫu cấu hình cố ý chưa cho bật real mode; thiếu artifact phải báo chưa sẵn sàng.
- Chưa kiểm chứng nhận dạng trên căn cước/video người thật, chưa có báo cáo FMR/FNMR hoặc số đo FPS/latency trên thiết bị mục tiêu. Các mục tiêu tại 8.2 vẫn là tiêu chí cần đạt.
- Realtime hiện chỉ thương lượng WebM/VP8. MP4 hỗ trợ ở bước phân tích file; chưa tuyên bố MP4 realtime trên Safari/WebView khác.
- Tách chân dung dùng một ROI/layout được cấu hình sau căn phối cảnh, chưa có bộ phân loại nhiều mẫu thẻ. Giới hạn active liveness/replay và xác thực giấy tờ ở mục 1, 5.4 vẫn giữ nguyên.
- Test dùng model thay thế chỉ để kiểm chứng protocol, state và lưu trữ; không được dùng kết quả đó làm bằng chứng đã khớp người thật.
