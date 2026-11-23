# Plan: License source-available và CLA

## 1. Mục tiêu

Công khai source để mọi người đọc, học, chạy và nghiên cứu, nhưng không cho phép lấy source hoặc dùng danh nghĩa "anomaly" để bán. Chủ sở hữu vẫn giữ quyền cấp license thương mại riêng.

## 2. Quyết định

| Hạng mục | Chọn | Lý do |
|---|---|---|
| License | PolyForm Noncommercial 1.0.0 (`PolyForm-Noncommercial-1.0.0`) | Cấm mục đích thương mại, văn bản ngắn, có SPDX id |
| Tên và logo | Không nằm trong license, ghi ở `NOTICE` | License không bảo vệ tên; nhãn hiệu là bước riêng |
| Đóng góp | CLA, đồng ý bằng checkbox trong PR template | Để chủ dự án còn cấp license thương mại riêng được |
| Cách gọi | "source-available", không gọi "open source" | Không đạt định nghĩa OSI |

Phương án thay thế đã cân nhắc: BUSL 1.1 (tự chuyển sang Apache-2.0 sau tối đa 4 năm, cần điền *Additional Use Grant*), Elastic License 2.0 (chỉ cấm bán dạng managed service, yếu hơn). AGPL, MIT, Apache không ngăn việc bán lại nên loại.

## 3. Các bước

### 3.1 Kiểm tra trước khi công bố (chặn các bước sau)

- [x] Liệt kê license của dependency: Go (`go-licenses report ./...` trong `server/`, `consumer-mock/`), npm (`yarn licenses list` trong `client/`), Python (`pip-licenses` trong `kyc-service/`, `anomaly-service/`). Báo cáo lưu ở `docs/THIRD_PARTY_LICENSES.md`.
- [x] Loại hoặc thay dependency có license copyleft mạnh (GPL/AGPL) nếu xung đột với PolyForm.
- [x] Kiểm tra license của model, trọng số, dataset và ảnh/font/icon. Artifact không có trong repo thì ghi rõ trong `NOTICE`.
- [x] Quét secret và dữ liệu cá nhân trong git history (`gitleaks detect`). Các file `*.docx` chưa track ở root không được commit.
- [ ] Hỏi trường/giảng viên xem đồ án có ràng buộc về quyền sở hữu hoặc công bố không.

### 3.2 File license và thông báo

- [x] `LICENSE`: nguyên văn PolyForm Noncommercial 1.0.0, kèm dòng `Required Notice: Copyright <năm> <chủ sở hữu>`.
- [x] `NOTICE`: tên "anomaly", logo và nhận diện không được cấp theo license; nêu cách xin giấy phép thương mại (email liên hệ).
- [x] `README.md`: thêm mục License, nói rõ "source-available, phi thương mại" và dẫn tới `LICENSE`, `NOTICE`, `CLA.md`.
- [x] Gán SPDX vào manifest: `package.json` (`client/`, `client/mobile`, `client/desktop`) và `pyproject.toml` (`kyc-service`, `anomaly-service`). Dùng `"license": "SEE LICENSE IN LICENSE"` cho npm nếu SPDX không được chấp nhận. `go.mod` không có trường license.
- [ ] Không thêm header vào từng file ở đợt này; chỉ thêm nếu cần sau.

### 3.3 CLA

- [x] `CLA.md`: Individual CLA, ngắn, gồm các điều khoản:
  1. Người đóng góp giữ bản quyền phần mình đóng góp.
  2. Cấp cho chủ dự án license vĩnh viễn, toàn cầu, không độc quyền, miễn phí, có thể cấp lại (sublicense) và không thể hủy ngang, để dùng, sửa, phân phối đóng góp, **kể cả theo license khác, bao gồm thương mại**.
  3. Cấp license bằng sáng chế cho phần đóng góp.
  4. Người đóng góp cam đoan là tác giả, có quyền cấp license, không vi phạm quyền của bên thứ ba; nếu làm cho công ty thì đã có phép của bên đó.
  5. Đóng góp được cung cấp nguyên trạng, không bảo hành.
- [ ] Cân nhắc Corporate CLA nếu có tổ chức đóng góp (làm khi có nhu cầu).
- [ ] Nên nhờ người có chuyên môn pháp lý xem lại `CLA.md` trước khi áp dụng; mẫu trên chỉ là điểm xuất phát, không phải tư vấn pháp lý.
- [x] Ký CLA bằng cách tick checkbox trong PR template (không dùng bot). Bản ghi là mô tả PR; chấp nhận mức bằng chứng yếu hơn chữ ký có lưu trữ.
- [x] `.github/pull_request_template.md`: thêm checkbox "Tôi đồng ý CLA".
- [x] Thêm mục "Đóng góp" vào `README.md` (hoặc `CONTRIBUTING.md`) giải thích CLA.

### 3.4 Sau khi công bố

- [ ] Đóng góp từ trước khi có CLA (nếu có người khác commit): liên hệ để xin xác nhận, hoặc viết lại phần đó. Kiểm tra bằng `git shortlog -sne`.
- [ ] Nếu cần bảo vệ tên chắc hơn: đăng ký nhãn hiệu "anomaly" tại Cục Sở hữu trí tuệ.
- [ ] Nếu bán bản thương mại: cấp license riêng cho khách; chủ dự án giữ quyền nhờ CLA.

## 4. Thứ tự và phạm vi PR

1. PR 1: mục 3.1 (báo cáo dependency) và 3.2 (`LICENSE`, `NOTICE`, README, manifest).
2. PR 2: mục 3.3 (`CLA.md`, workflow, PR template, README đóng góp).
3. Chỉ chuyển repo sang public sau khi cả hai PR được merge và mục 3.1 đã sạch.

## 5. Rủi ro

- Người dùng có thể coi PolyForm là "không phải open source" và ngại dùng. Chấp nhận vì đúng mục tiêu.
- "Phi thương mại" có vùng xám (tổ chức phi lợi nhuận, giáo dục). PolyForm Noncommercial đã định nghĩa các trường hợp được phép; xem lại văn bản trước khi trả lời thắc mắc.
- Không có CLA thì sau này khó cấp license thương mại cho phần code của người khác.
- Dependency copyleft hoặc model có license hạn chế có thể buộc đổi license hoặc đổi thành phần.
