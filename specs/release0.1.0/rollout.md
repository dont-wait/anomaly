# Runbook: từ commit đến bản phát hành 0.1.0

## 1. Trước release đầu tiên

Hoàn thiện các gap REL trong [roadmap.md](roadmap.md), đặc biệt signing Android, desktop bundles, registry, release deployment config và automation. Cấp domain/host/secrets và backup signing key trước khi tạo candidate chính thức.

Chốt target matrix Android ARM64 + admin Linux/Windows + server amd64, hoặc ghi rõ target được bỏ. Tạo GitHub Environments/variables/secrets theo thiết kế pipeline. Chuẩn bị host và private registry pull permissions trước ngày release, không làm bootstrap gấp trong promote job.

## 2. Tạo candidate

1. Chọn commit chứa code/config cần phát hành; chạy version checker để source app version là `0.1.0`.
2. Tạo candidate ID `rc.1`, cấp Android versionCode mới và ghi vào metadata bền. Ref đề xuất `release/0.1.0-rc.1` không được dịch sang commit khác sau build.
3. Dispatch `release-candidate.yml` với resolved SHA, candidate metadata và environment variant.
4. Pipeline checks → build/sign APK + native packages + images → collect/verify theo đúng target matrix.
5. Lưu CI artifact bundle cùng image digests và report, không public GitHub Release ở bước này.
6. Nếu cần sửa, tạo `rc.2` từ commit mới và cấp versionCode mới; không thay binary của rc.1 bằng build khác.

Candidate cho final release phải chứa endpoint cuối. Candidate staging dùng domain khác chỉ phục vụ staging; không promote APK đó rồi đổi endpoint bằng runtime.env trên server vì frontend đã compile endpoint vào bundle.

## 3. Kiểm tra candidate và deploy thử

- Cài APK/AppImage/deb/NSIS từ candidate artifacts, không dùng binary local trên máy developer.
- Deploy server candidate digests vào staging bằng `deploy-server.yml`; kiểm preflight, migration, reader/writer order, TLS và persistence.
- Test update từ candidate trước khi có bản trước đó. Với 0.1.0, cài từ debug APK khác key không được xem là kiểm thử update release.
- Diễn tập rollback/restore và ghi kết quả; lần đầu chưa có version production trước thì phải chuẩn bị kế hoạch dừng traffic/roll-forward/restore cụ thể.
- Hoàn thiện verification report V01–V18. Candidate fail quay lại bước 2 với candidate mới khi input đã thay đổi.

## 4. Promote và triển khai cuối

1. Chọn candidate đã đạt, xác nhận manifest/SHA/version/artifact retention và final endpoint.
2. Dispatch `release-promote.yml` cho candidate run cụ thể. Workflow verify lại hash/digest và tag `v0.1.0` trỏ đúng SHA.
3. Tạo draft Release, upload đủ installers, `release-manifest.json`, `SHA256SUMS`, installation/update instructions và verification report đã sanitize.
4. Promote GHCR tags sang `0.1.0` nếu dùng, giữ đúng image digest candidate. Tags vẫn không được dùng làm deployment source thay digest.
5. Deploy server production bằng manifest/digests; pull trước, backup, migration và reader upgrade trước writer, rồi smoke.
6. Xác nhận server endpoint cuối healthy/ready theo probes đã thiết lập. Nếu fail, giữ draft và lưu trạng thái deploy để xử lý, không public app installers.
7. Publish draft thành Release `v0.1.0` sau khi đủ gates.
8. Download lại từ Release, verify hash/signature và test install/start; ghi publication/deployment report.

Nếu environment protection của repo yêu cầu approval, đặt ở promotion/deploy theo chính policy đó. Không cần biến mỗi bước build/test thành một lần xin xác nhận thủ công.

## 5. Nội dung asset và hướng dẫn tải

Danh sách minh họa; candidate ID có thể bỏ trong tên final nhưng nội dung binary không đổi:

```text
anomaly-mobile-0.1.0-android-arm64.apk
anomaly-admin-0.1.0-linux-x86_64.AppImage
anomaly-admin-0.1.0-linux-x86_64.deb
anomaly-admin-0.1.0-windows-x86_64.exe
release-manifest.json
SHA256SUMS
INSTALL.md
verification-report.md
```

`INSTALL.md` cần ghi OS/arch đã kiểm thử, download đúng file, cách kiểm checksum, cài/cập nhật/gỡ và trạng thái chữ ký Windows. Server hướng dẫn lấy manifest/config, cấp runtime secrets, pull image và deploy; không phát hành file runtime.env thật.

Version notes ghi commit/build provenance, target support và thay đổi vận hành/cách upgrade. Không cần suy diễn danh sách feature mới từ việc tăng version.

## 6. Cập nhật sau 0.1.0

- **Android:** `0.1.1`, versionCode cao hơn, cùng applicationId và keystore. Người dùng tải APK và cài đè; không yêu cầu uninstall trước.
- **Desktop:** bản mới giữ identifier/data directory, NSIS/deb cài đè hoặc thay AppImage sau khi đóng app. Auto-update chỉ dùng khi đã triển khai plugin/manifest/signature riêng.
- **Server:** tạo manifest mới, pull digest, kiểm migration compatibility, rollout rồi giữ deployment directory cũ cho rollback.
- Không move tag `v0.1.0`, không overwrite assets/images `0.1.0` bằng build khác. Bản lỗi được sửa bằng release mới.

## 7. Rollback theo nền tảng

| Thành phần | Khi có lỗi | Hành động |
| --- | --- | --- |
| Android chưa public | Candidate fail | Không promote; tạo candidate mới |
| Android đã public | Binary lỗi | Dừng phân phối tiếp, phát bản sửa versionCode cao hơn; không hạ code mặc định |
| Desktop | Bộ cài lỗi | Dừng phân phối asset lỗi, phát bản sửa; về bản cũ chỉ nếu dữ liệu/config tương thích |
| API/projector/relay | Deployment fail | Quay digest trước nếu đọc được schema/event mới; nếu không, roll forward bản tương thích |
| ML | Image/config/model load fail | Dừng update, chọn digest/model tương thích trước từ manifest; không tự xóa offsets |
| Migration/data | Dirty/schema/event incompatibility | Dừng writer bị ảnh hưởng, dùng recovery runbook/backup đối soát; không auto-down hoặc xóa volumes |

Swarm rollback không phục hồi Mongo/EventStoreDB/RustFS. Khôi phục snapshot có thể mất dữ liệu sau thời điểm backup; phải xác định recovery point và đối soát trước mở traffic.

## 8. Định nghĩa hoàn tất cơ chế release

- [ ] Từ clean checkout, CI tạo được đầy đủ signed APK, installers và images đã chọn.
- [ ] Source/version/toolchain/input/provenance truy ra được từ mọi file phát hành.
- [ ] Candidate được verify, promote giữ nguyên binary hash và image digest.
- [ ] Server deploy không cần source/build tools trên host, giữ nguyên persistent data khi cập nhật.
- [ ] Cài sạch/cài đè, download từ Release, migration/deploy/restore được kiểm chứng.
- [ ] Một lần job fail/rerun không public release thiếu asset hoặc ghi đè release có nội dung khác.
- [ ] Người dùng có link tải/cài, người vận hành có manifest/config/secrets reference và runbook.

Specs chỉ mô tả cơ chế cần triển khai. Chưa tạo tag, workflow, key, registry package, GitHub Release hoặc deployment thật trong lượt viết tài liệu.
