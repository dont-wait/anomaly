# Specification: cơ chế phát hành Anomaly 0.1.0

> Cập nhật triển khai: runtime image pipeline đã có `make release`,
> `.github/workflows/release-images.yml`, `deploy/release/images.json`,
> image-only Compose/Swarm và digest manifest. Xem
> [hướng dẫn chạy thật](../../deploy/release/README.md).
> Native APK/desktop installers, Ubuntu client builders và candidate/promotion/deploy
> workflows dưới đây vẫn là thiết kế cần triển khai, không thuộc `make release` images.


> Draft ngày 29/10/2026, đối chiếu checkout `e2d0840`.
> Đây là thiết kế release engineering cần triển khai: build → ký → lưu artifact → kiểm tra → deploy → phân phối. Các workflow/script/config ghi là **cần tạo** chưa tồn tại và chưa được thực thi.

## 1. Kết quả cần đạt

Từ một commit đã chốt, tạo được bộ phát hành `0.1.0` gồm APK Android, bộ cài admin desktop và container images server. Có quy trình lặp lại được để build, kiểm tra, phát hành, cập nhật và rollback; người dùng tải bộ cài, người vận hành triển khai bằng image digest mà không phải build lại trên server.

Spec không quyết định release có tính năng nghiệp vụ nào. Việc còn thiếu dashboard, loan UI hay model accuracy thuộc backlog sản phẩm riêng; ở đây chỉ ghi gap liên quan trực tiếp đến đóng gói và triển khai.

## 2. Phương án phân phối đề xuất

| Thành phần | Target cho pipeline 0.1.0 | Đầu ra | Nơi nhận |
| --- | --- | --- | --- |
| Mobile consumer | Android ARM64 | APK release đã ký | GitHub Release, tải và cài trực tiếp |
| Admin desktop | Linux x86_64 | AppImage + deb | GitHub Release |
| Admin desktop | Windows x86_64 | NSIS installer `.exe` | GitHub Release |
| Go server | Linux amd64 | OCI image chứa API/projector/relay/migrate | GHCR |
| KYC | Linux amd64 | OCI image; model/config mount riêng | GHCR + kho artifact model riêng |
| ML worker | Linux amd64 CPU | OCI image chứa source/model đã pin | GHCR |
| ML autoscaler | Linux amd64 | OCI image | GHCR |

Đây là target pipeline đề xuất, không phải tuyên bố các target đã build được. Nếu target nào không được phân phối, phải bỏ rõ khỏi release matrix trước build; không tạo release trông đầy đủ nhưng thiếu asset. iOS/macOS, Google Play và auto-update desktop là extension có quy trình riêng, chưa bắt buộc cho cơ chế phát hành đầu tiên.

## 3. Thiết kế chung

- Source version `0.1.0`; tag cuối `v0.1.0`; candidate refs `release/0.1.0-rc.N` trỏ tới commit cụ thể.
- Candidate build dùng application version `0.1.0`, candidate ID nằm trong tên file/manifest; Android `versionCode` tăng cho mỗi candidate.
- Khi candidate được duyệt, tag cuối trỏ đúng commit candidate. Promote **chính artifact đã kiểm tra**, không build lại APK/bộ cài/image khi publish cuối.
- Artifact staging có endpoint staging không được promote thành production. Candidate cần build ngay với endpoint phân phối cuối và smoke qua endpoint đó trước publish; build riêng staging phải ghi variant riêng.
- GitHub Release lưu installer, checksum và manifest; GHCR lưu image. Server pull digest từ manifest; tag thuận tiện `0.1.0` không thay thế digest.
- Server triển khai trước; phân phối app sau khi endpoint public sẵn sàng. Tải bản mới và cài đè là cơ chế update đầu tiên.
- Các thao tác build, publish package, publish release và deploy là các bước tách biệt. Tạo draft/CI artifact không có nghĩa đã public hoặc deploy.

## 4. Tài liệu chi tiết

| File | Nội dung |
| --- | --- |
| [build-environments.md](build-environments.md) | NixOS host, Ubuntu builders, cache/linker isolation và native Windows/Apple runners |
| [mobile.md](mobile.md) | Android signing, versionCode, toolchain, lệnh build, kiểm chữ ký và phân phối APK |
| [desktop.md](desktop.md) | Linux/Windows runners, Tauri bundles, signing, cài đặt và cập nhật |
| [server.md](server.md) | Build/push image, release Compose/Swarm, config/secret, migration và deploy |
| [roadmap.md](roadmap.md) | Đặc tả CI/CD jobs, files/scripts cần tạo, secrets và gap release tooling |
| [testing.md](testing.md) | Kiểm tra chính artifact được phát hành, manifest và điều kiện promote |
| [rollout.md](rollout.md) | Runbook từ candidate tới release cuối, update, retry và rollback |

## 5. Cấu trúc release cần triển khai

```text
.github/workflows/
  release-candidate.yml       # cần tạo: kiểm tra và build, chưa publish public
  release-promote.yml         # cần tạo: promote artifact/image đã kiểm tra
  deploy-server.yml           # cần tạo: triển khai digest vào môi trường chọn
scripts/release/
  check-version.*             # cần tạo
  build-mobile.*              # cần tạo
  build-desktop.*             # cần tạo; hỗ trợ runner Linux/Windows
  build-images.*              # cần tạo
  collect-artifacts.*          # cần tạo
  verify-artifacts.*           # cần tạo
  promote.*                   # cần tạo
  deploy-server.*              # cần tạo
  smoke.*                     # cần tạo
deploy/release/
  compose.yml                 # cần tạo: image-only, không build trên host
  env.example                 # cần tạo: tên biến, không chứa credentials
  ml-stack.yml                # cần tạo hoặc render từ stack hiện hữu
  README.md                   # cần tạo: bootstrap host/domain/volumes
```

Đuôi script tùy runtime được chọn khi triển khai; contract input/output của từng script được chốt trong các file còn lại. Không mặc định shell Bash chạy được trên runner Windows.

## 6. Manifest và tính bất biến

Mỗi candidate có `release-manifest.json`, tối thiểu chứa:

```json
{
  "version": "0.1.0",
  "candidate": "rc.1",
  "commit": "<full-git-sha>",
  "runId": "<ci-run-id>",
  "environment": "production",
  "apiEndpoint": "https://<api-domain>",
  "artifacts": [],
  "images": {},
  "migrationVersion": "<from-source-and-target-db>",
  "modelArtifacts": [],
  "verificationReport": "<report-reference>"
}
```

Mỗi artifact ghi tên, platform/arch, byte size, SHA-256, app identifier/version và signing status/fingerprint khi có. Mỗi image ghi registry reference + digest, platform, source commit; ML ghi thêm external source revision và model checksum. Toolchain versions và lockfile checksums nằm trong build report.

Manifest không chứa secret. Tạo `SHA256SUMS` sau khi hoàn tất ký và đổi tên file; không ký/sửa binary sau bước checksum. Manifest cuối giữ candidate provenance và bổ sung tag/publication time.

## 7. Quyết định cấu hình còn cần điền

Chủ repo cần chọn GitHub owner/package namespace, API domain cuối, host deploy, chứng thư Windows và nơi giữ keystore Android. Spec dùng placeholder cho các giá trị này, không tự mua domain, tạo account, tạo key hoặc deploy.

Các gap release được liệt kê ở [roadmap.md](roadmap.md), trạng thái Open để người phát triển triển khai. Không sửa source/workflow trong lượt viết specs.
