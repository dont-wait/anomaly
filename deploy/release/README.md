# Release images từ máy NixOS hoặc GitHub CI

`make release` chạy pipeline local trong Docker: checks → build **cả bốn** runtime
images → smoke imports/model → push GHCR → xuất digest manifest. Không deploy,
không tạo GitHub Release và không tạo APK/desktop installers trong target này.

## Chạy local

Cần Python 3.12+, Docker Engine/Buildx (trên NixOS chỉ cần Docker host, không cần
host linker/Go/Python ML dependencies), Git và registry credentials.

1. Commit source/release files. Pipeline từ chối working tree dirty/untracked để
   không publish artifact khác commit được ghi trong manifest.
2. Commit các thay đổi GATv2 cần đóng gói ở repo đó trước; pipeline chỉ lấy commit
   được chỉ định bằng `git archive`, không lấy uncommitted working files.
3. Đăng nhập GHCR bằng `docker login ghcr.io`; token cần quyền push packages trong
   namespace chọn. Không đưa token vào Makefile hoặc command-line arguments.
4. Từ root repo **hoặc client/**:

```bash
make release-plan GATV2_REF=<full-commit-sha>
make release GATV2_REF=<full-commit-sha>
```

Tùy chọn:

```bash
make release-build GATV2_REF=<sha>             # checks/build/smoke, không push
make release-check GATV2_REF=<sha>             # checks targets, không build runtime/push
make release GATV2_REF=<sha> BUILD_NETWORK=host # khi Docker bridge DNS local lỗi
make release GATV2_REF=<sha> REGISTRY=ghcr.io/<owner> VERSION=0.1.0
```

`GATV2_DIR` mặc định sibling `../GATv2` tính từ root repo. Tag chứa version,
commit và build ID mới cho mỗi run, không ghi đè `latest`/`0.1.0`. Override
`RELEASE_BUILD_ID` chỉ khi biết mình đang chọn lại namespace tag; registry phải
có policy bất biến nếu cần bảo vệ tuyệt đối khỏi người dùng overwrite.

Output ở `dist/release/<tag>/`: `build-manifest.json` chứa local image IDs;
`release-manifest.json` và `images.env` chỉ xuất khi **tất cả push đã thành công**.
Push registry không có transaction: nếu push image thứ ba lỗi, hai image đầu có
thể đã tồn tại; không deploy dựa trên build manifest hoặc tag rời. Chạy lại tạo
build ID mới, chỉ dùng bộ manifest hoàn chỉnh.

Images: `anomaly-server`, `anomaly-kyc`, `anomaly-ml`, `anomaly-ml-autoscaler`.
Go API/projectors/relay/migrate dùng cùng server image. ML dùng named build
context GATv2, không gửi toàn bộ thư mục cha chứa các repo khác vào Docker.
Model bundle được verify checksum trước build và load bằng runtime image trước
push; điều này không chứng minh model accuracy hoặc protocol integration đạt.

## Checks chạy trong Docker

- Go: `go vet` trong checks build; `go test` chạy trong checks container với
  Unix Docker socket và host networking để Testcontainers tạo Redis độc lập.
  Chỉ dùng Docker daemon local; không mount socket vào runtime images.
- KYC: Ruff + test kỹ thuật vision/CORS. Suite RustFS/Docker đầy đủ được CI BE
  hiện hữu chạy riêng, không giả vờ test này đã chạy trong BuildKit.
- ML: offline pytest; runtime smoke load bundle từ source GATv2 được pin.
- Autoscaler: parse Python syntax; chưa có suite nghiệp vụ cho autoscaler.
- Frontend lint/typecheck/test và full BE CI chạy khi PR vào `main` hoặc push
  lên `main`, theo paths filter hiện hữu. Khi release, hai CI được gọi lại trên
  source release; job publish chỉ chạy sau khi cả hai pass. Local `make release`
  chỉ gate các runtime image checks nêu trên.

## CI tự động

`.github/workflows/release-images.yml` chạy khi push tag `v*` hoặc dispatch từ
Actions. Nó gọi lại `ci-fe.yml` và `ci-be.yml` qua `workflow_call`; chỉ khi cả hai
pass mới chạy job publish (`make release-test`, sau đó `make release`). PR hoặc
push/merge lên `main` không tự kích hoạt workflow release. `workflow_call` chỉ
cho phép release gọi lại CI, không thay thế các trigger PR/push hiện tại.

Cấu hình repository variables:

- `GATV2_REF`: commit GATv2 tương thích (bắt buộc nếu dispatch không điền input).
- `GATV2_REPOSITORY`: mặc định `pdmh18/GATv2`, đổi nếu dùng fork khác.
- Secret `GATV2_READ_TOKEN`: cần nếu GATv2 private và GITHUB_TOKEN không đọc được.

Workflow dùng job token `packages:write` để push GHCR của repository owner; các
job PR thông thường không được cấp credentials publish. Source GitHub Actions
là commit đã push, không phải image trong Docker daemon của laptop. Nếu muốn
chạy trên laptop, dùng `make release` trực tiếp; không cần self-hosted runner.

## Manifest triển khai

`compose.yml` là bản image-only giữ env/dependency/volume contracts của Compose
server. Runtime secrets ở file ngoài repo, còn `images.env` chứa digest từ run
hoàn chỉnh. Từ root repo, sau điền env, chọn **cùng project name** với deployment
đang dùng để không tạo volumes/database mới:

```bash
docker compose --project-name anomaly \
  --env-file /opt/anomaly/shared/runtime.env --env-file dist/release/<tag>/images.env \
  -f deploy/release/compose.yml --profile app config --quiet
docker compose --project-name anomaly \
  --env-file /opt/anomaly/shared/runtime.env --env-file dist/release/<tag>/images.env \
  -f deploy/release/compose.yml --profile app pull
```

Khởi động/upgrade phải theo runbook migration/compatible reader trước writer;
`make release` không tự restart live services. Các infrastructure ports của
release Compose bind localhost. ML Swarm cần listener/address reachable được
operator cấu hình có chủ ý; hai network Compose/Swarm không tự dùng chung DNS.

Để deploy ML: export runtime variables và các digest refs trong `images.env`
rồi `docker stack deploy --with-registry-auth -c deploy/release/ml-stack.yml ml`.
Đừng đưa file secrets vào log/artifacts. Model directory KYC mount read-only;
`RUSTFS_IMAGE` cần phiên bản/digest operator đã thử, không tự chọn latest.

Python runtime dependencies/base tags chưa được lock toàn bộ: digest trong
manifest xác định artifact được deploy, không phải cam kết hai lần rebuild sẽ
ra cùng digest. Ubuntu client builders, APK/desktop signing và install/promotion
pipeline trong specs vẫn là phần riêng chưa được target image release này triển khai.
