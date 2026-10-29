# CI/CD và gap cần triển khai để release

> Cập nhật triển khai: runtime image pipeline đã có `make release`,
> `.github/workflows/release-images.yml`, `deploy/release/images.json`,
> image-only Compose/Swarm và digest manifest. Xem
> [hướng dẫn chạy thật](../../deploy/release/README.md).
> Native APK/desktop installers, Ubuntu client builders và candidate/promotion/deploy
> workflows dưới đây vẫn là thiết kế cần triển khai, không thuộc `make release` images.


## 1. Hiện trạng

`.github/workflows/ci-fe.yml` và `ci-be.yml` chạy kiểm tra PR/push, chưa có workflow build signed APK, tạo native desktop installers, publish GHCR và promote/deploy release. Frontend CI hiện dùng Node24, các Nix shell dùng Node22/24 tùy shell; package root chưa khai báo `packageManager`. Cần chọn/pin toolchain để release, không đổi ngầm theo runner.

Tài liệu này mô tả pipeline cần tạo; không phải kế hoạch phát triển tính năng.

## 2. Workflow release-candidate.yml

Trigger: `workflow_dispatch` với ref/commit, candidate ID, environment build và Android versionCode đã cấp. Resolve ref thành SHA duy nhất tại job đầu; mọi job checkout SHA đó. Reject ref mutable nếu metadata không ghi resolved SHA.

| Job | Runner | Input | Output / điều kiện |
| --- | --- | --- | --- |
| `prepare` | Linux | ref, version, candidate, environment | Resolved SHA, version consistency, target matrix, endpoint, versionCode |
| `checks` | Linux | SHA + locked dependencies | Client lint → typecheck → test; BE/Python checks theo repo |
| `android` | Ubuntu Android builder container theo digest | SHA, endpoint, signing secrets | Signed ARM64 APK + verify report |
| `desktop-linux` | Ubuntu22.04 builder container theo digest | SHA, endpoint | AppImage/deb + metadata report |
| `desktop-windows` | Windows MSVC | SHA, endpoint, signing policy | NSIS installer + signing status |
| `images` | Linux Buildx | SHA, pinned external source/model | GHCR candidate refs/digests + build report |
| `collect` | Linux | Tất cả artifact refs | Manifest, SHA256SUMS, artifact inventory |
| `candidate-verify` | Runner theo target | Collected binaries/images | Check identity/version/signature/digest, completeness |
| `staging-smoke` | Runner/deploy host đã chỉ định | Candidate manifest | Report deploy/install/smoke; có manual evidence cho máy thật |

`collect` chỉ chạy khi tất cả target được chọn thành công. Dùng artifact name có candidate + target + arch; reject trùng tên, thiếu output hoặc output commit khác. Candidate build không tạo public release.

Permissions mặc định `contents:read`; chỉ job image có `packages:write`. Không cấp release/signing/production secrets cho pull request không tin cậy. Pin action revisions theo policy repo và ghi version toolchain/build log.

## 3. Workflow release-promote.yml

Trigger thủ công, input `candidate_manifest`, `candidate_run_id`, final tag. Thứ tự bắt buộc:

1. Download candidate bundle theo run ID cụ thể; verify manifest/checksum và commit/version.
2. Kiểm evidence artifact/install/deploy đã đạt; reject candidate đang build hoặc report của run khác.
3. Xác nhận final tag chưa trỏ commit khác; lần promote lại phải cùng commit và cùng artifact hashes.
4. Tạo tag `v0.1.0` đúng candidate SHA hoặc xác minh tag tồn tại đúng SHA.
5. Tạo GitHub **draft Release**, upload đủ assets + manifest + checksums. Một job điều phối release ID, tránh mỗi matrix job tạo release khác.
6. Retag/publish GHCR final names từ candidate digest nếu cần, kiểm digest trước/sau.
7. Deploy/verify server theo workflow deploy ở dưới trước khi public app installers.
8. Publish draft thành final sau khi tất cả gate đạt. Nếu deploy fail, giữ draft và report lỗi; không public release thiếu một nền tảng.

Promotion dùng candidate binaries nguyên byte. Nếu sửa code, endpoint, signing hoặc dependency, phải tạo candidate mới. Workflow phải idempotent: asset cùng hash có thể giữ; khác hash thì fail, không overwrite release đã publish.

## 4. Workflow deploy-server.yml

Trigger thủ công hoặc reusable workflow từ promotion; input environment và manifest ref. Dùng GitHub Environment để ràng buộc secrets/host và concurrency group riêng môi trường, `cancel-in-progress=false` cho mutation đang triển khai.

Deployment có thể qua SSH với host key pinned hoặc runner nội bộ được giới hạn quyền. Người triển khai phải chọn phương thức; spec không giả định đã có credential. Không cần đưa signing keys Android lên deployment host.

Job gọi `deploy-server` và `smoke` theo [server.md](server.md). Production environment gates theo cấu hình repo nếu có; không mặc định thêm manual approval vào mọi build. Lưu report khi success/failure; không để output đầy secret từ `docker compose config` đi vào CI logs.

## 5. Biến và secrets cần cấu hình

Tên dưới đây là interface **đề xuất**, không khẳng định repo đã có các secrets đó.

| Tên | Loại | Job dùng |
| --- | --- | --- |
| `RELEASE_API_ENDPOINT` | Environment variable | Mobile/desktop build |
| `ANDROID_KEYSTORE_BASE64` | Secret | Android signing setup |
| `ANDROID_KEY_ALIAS` | Secret/config | Android signing |
| `ANDROID_STORE_PASSWORD`, `ANDROID_KEY_PASSWORD` | Secret | Android signing |
| `WINDOWS_SIGNING_*` | Secret/provider config, tùy cách ký | Windows signing |
| `GATV2_SOURCE_REF` | Pinned revision variable | Image build |
| `GATV2_READ_TOKEN` | Secret nếu source private | External checkout, read-only |
| `GITHUB_TOKEN` | Job token | GHCR publish hoặc GitHub Release theo job permissions |
| `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_ROOT` | Environment variables | Deploy |
| `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS` | Secret/config | SSH deploy nếu chọn phương thức này |
| Runtime secret references | Host/secret store | Server runtime, không compile vào client |

Base64 keystore là mã hóa biểu diễn, không phải bảo mật; lưu dưới dạng secret. Mask log, cleanup temp files và không cache credentials. SSH key chỉ cấp deployment role cần thiết; host pull private images dùng registry read credential riêng.

Builder local/CI theo [build-environments.md](build-environments.md): hai image client riêng, source snapshot sạch, không dùng Rust target/node_modules/SDK paths của Nix host.

## 6. Cache và retention

- Yarn cache theo OS + Yarn version + lockfile; không cache `.env`, toàn bộ home hoặc signing temp directory.
- Rust cache tách mobile/desktop, target/profile/toolchain/Cargo.lock; không reuse bundle output làm artifact mới.
- Gradle cache có thể dùng nhưng project signing properties/keystore không đi cùng.
- Container cache không chứa build secret trong layer; model cache phải đúng checksum/revision.
- CI artifact retention phải đủ cho cửa sổ review/promotion. Nếu artifact hết hạn, tạo candidate mới; không build lại rồi giả danh cùng hash đã duyệt.
- GitHub final Release giữ installers/manifest/checksums lâu dài; registry giữ candidate/final digests theo retention không xóa digest đang deploy hoặc còn cần rollback.

## 7. Backlog release engineering

Tất cả mục dưới đây **Open**, người dùng sẽ triển khai.

| ID | Gap hiện tại | File/phần cần thay đổi hoặc tạo | Hoàn tất khi |
| --- | --- | --- | --- |
| REL-01 | Chưa chốt release metadata/toolchain | Version checker, packageManager, toolchain pin | Một SHA + version dùng thống nhất mọi job |
| REL-02 | Android chưa có release signing | Gradle signing + secrets/ignore/backup | APK cuối verify đúng certificate, update được |
| REL-03 | Chưa có versionCode allocation | Candidate metadata/counter script | Candidate mới tăng code, rerun không cấp lại tùy tiện |
| REL-04 | Desktop bundle tắt, metadata/icons cần đủ | Tauri bundle config | AppImage/deb/NSIS tạo đủ và chạy được |
| REL-05 | Desktop Cargo.lock chưa track | Rust lockfile/check pipeline | Build locked từ clean checkout |
| REL-06 | Endpoint release chưa được validate thống nhất | Build scripts cho hai workspace | Thiếu/localhost/non-HTTPS fail trước build |
| REL-07 | Chưa có native build matrix | release-candidate.yml | Signed APK và packages từ native runners |
| REL-08 | Chưa có GHCR release publishing | Image scripts/job, permissions | Digest/platform/commit ghi vào manifest |
| REL-09 | ML build cần sibling GATv2, deps chưa pin chặt | Checkout/context/Dockerfile/dependency lock | CI clean runner build được đúng source/model |
| REL-10 | Deployment hiện còn build local, tags mutable | Release Compose/Swarm templates | Host chỉ pull digest, volumes giữ ổn định |
| REL-11 | Chưa có deploy automation | Deploy/preflight/migrate/smoke scripts | Report và thứ tự rollout/rollback tái hiện được |
| REL-12 | Chưa có artifact inventory/checksum/promotion | Collect/verify/promote scripts | Publish dùng đúng bytes candidate, fail nếu thiếu |
| REL-13 | Signing/update/install chưa có report release | Verification jobs + manual report | Cài sạch/update/download cuối đạt |
| REL-14 | Secret/host/domain/registry namespace cần cấp | Environment config/runbook | Inputs đủ, secret không nằm trong binary/log |
| REL-15 | Backup/restore/image rollback cần diễn tập | Deployment runbook/scripts | Phục hồi chính digest/schema/volume được chứng minh |
| REL-16 | Chưa có Ubuntu client builders dùng chung local/CI | deploy/builders Dockerfiles + wrapper/cache isolation | Compile/bundle trong container, không phụ thuộc Nix host; artifact chạy trên target sạch |

Thứ tự triển khai: REL-01/14 → signing/packaging và images chạy song song → collect/verify → image-only deploy → smoke/restore → promotion. Không cần mở feature roadmap mới để hoàn thành cơ chế release này.
