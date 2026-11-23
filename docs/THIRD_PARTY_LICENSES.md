# Third-party licenses

Kiểm tra ngày 2026-11-23 để chuẩn bị công bố dưới PolyForm Noncommercial 1.0.0.
Đây là kiểm tra bằng script trên license file/metadata đã cài, không phải tư vấn
pháp lý. Chạy lại khi thay đổi dependency.

## Kết quả

| Thành phần | Cách kiểm tra | Kết quả |
|---|---|---|
| Go `server/` (44 module) | quét LICENSE trong module cache | Apache-2.0 21, MIT 13, BSD 10. Không có GPL/AGPL |
| Go `consumer-mock/` (3 module) | như trên | Apache-2.0, BSD, MIT |
| npm `client/` (~260 package) | trường `license` trong `node_modules` | MIT, Apache-2.0, ISC, BSD, MPL-2.0 (3), BlueOak, CC0, CC-BY-4.0, 0BSD. Không có GPL/AGPL/UNLICENSED |
| Python `anomaly-service/` | metadata trong `.venv` | MIT, BSD, Apache-2.0, PSF, MPL-2.0 (`certifi`, `tqdm`). Gói NVIDIA CUDA là proprietary, xem bên dưới |
| Python `kyc-service/` | chỉ đọc `pyproject.toml` (chưa cài) | Các gói khai báo (fastapi, httpx, boto3, uvicorn, numpy, opencv, mediapipe, onnxruntime, ...) thường là MIT/BSD/Apache. Cần chạy lại sau khi cài |
| Rust `client/*/src-tauri` | `cargo metadata --offline` thất bại | Chưa kiểm tra. Tauri 2 là MIT/Apache-2.0; cần `cargo license` sau khi tải crate |

## Điểm cần lưu ý

- **MPL-2.0** (3 package npm, `certifi`, `tqdm`): copyleft ở mức file, chỉ áp dụng cho
  file của chính package đó. Dùng nguyên trạng thì không ảnh hưởng license của Anomaly.
- **Gói NVIDIA CUDA** (`nvidia-*`, kéo theo từ `torch`): license proprietary của NVIDIA.
  Repo không chứa chúng, chỉ cài lúc runtime, nên không phân phối lại. Không đóng gói
  chúng vào image hay bản phát hành được công bố mà chưa đọc điều khoản NVIDIA.
- **`opencv-contrib-python`, `mediapipe`**: wheel có thể bundle thư viện native (FFmpeg
  LGPL, v.v.). Chỉ lo khi đóng gói phân phối lại.
- **Model và dữ liệu**: `anomaly-service/models/gatv2-1m-baseline-v1/` (gồm `model.pt`)
  đang được commit trong repo, trái với ghi chú "weights không nằm trong repo" ở
  `CLAUDE.md`. Cần xác nhận đây là model bạn tự train trên dữ liệu bạn có quyền dùng
  (dữ liệu synthetic từ generator A-F). Nếu dùng dữ liệu hay trọng số bên ngoài thì
  phải ghi nguồn và license vào `NOTICE`.
- **Ảnh, icon, font**: 34 PNG, 4 SVG, 2 ICO được commit; chưa xác minh nguồn. Icon do
  Tauri sinh ra là mẫu mặc định (MIT/Apache); các ảnh khác cần bạn xác nhận là tự tạo
  hoặc có license cho phép.
- **File sinh tự động**: `client/src-tauri/gen/android/.gradle/**` (cache Gradle `*.bin`)
  đang bị track trong git. Nên `git rm --cached` và thêm vào `.gitignore`.
