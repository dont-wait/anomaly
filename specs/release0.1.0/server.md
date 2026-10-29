# Server: build image, publish registry và deploy 0.1.0

## 1. Đơn vị phát hành

Không phát hành source tree để SSH vào host rồi `git pull && build`. CI build các image từ commit đã chốt; host chỉ nhận release config, secret references và digest.

| Image đề xuất | Build context hiện hữu | Dockerfile | Command trên host |
| --- | --- | --- | --- |
| `ghcr.io/<owner>/anomaly-server` | `server/` | `server/Dockerfile` | `/app/server`, `/app/projector`, `/app/relay`, `/app/migrate`, `/app/loan-projector` theo service |
| `ghcr.io/<owner>/anomaly-kyc` | `kyc-service/` | `kyc-service/Dockerfile` | Image default command |
| `ghcr.io/<owner>/anomaly-ml` | Parent chứa `anomaly/` và `GATv2/` | `anomaly/anomaly-service/Dockerfile` | Image default command |
| `ghcr.io/<owner>/anomaly-ml-autoscaler` | `deploy/swarm/autoscaler/` | Dockerfile trong context | Image default command |

Baseline image platform `linux/amd64`; ARM64 chỉ thêm sau khi build/verify dependencies CV/torch và native runtime trên target đó.

## 2. Build/push pipeline

1. Checkout repo tại candidate SHA, không build từ branch có thể đổi trong lúc pipeline chạy.
2. Checkout GATv2 vào sibling directory với revision được pin trong metadata release; không lấy HEAD hiện tại của repo ngoài.
3. Kiểm model bundle/config được cấp đủ, checksum đúng và permissions cho phép build/distribute. KYC weights không bắt buộc đưa vào image: mount riêng read-only trên host.
4. Build bằng Docker Buildx cho platform đã chốt; label OCI source/revision/version, ghi base image digests và build report.
5. Test startup/binaries trong image, tạo SBOM/provenance nếu tooling được triển khai; không đưa `.env`, private keys hoặc training files vào context/image.
6. Login GHCR bằng token job có `packages:write`; push candidate tags bất biến và lấy registry digest từ build output.
7. Ghi digest vào manifest. Promotion cuối retag cùng manifest digest nếu cần `:0.1.0`, không rebuild để đổi tên tag.

Lệnh build minh họa từ root repo, cần thay owner/tag và login trước:

```bash
docker buildx build --platform linux/amd64 --push \
  -t ghcr.io/<owner>/anomaly-server:0.1.0-rc.1 \
  -f server/Dockerfile server
docker buildx build --platform linux/amd64 --push \
  -t ghcr.io/<owner>/anomaly-kyc:0.1.0-rc.1 \
  -f kyc-service/Dockerfile kyc-service
```

ML Dockerfile hiện `COPY anomaly/...` và `COPY GATv2/src`, có `RUN --network=host`. Vì vậy release job cần dựng đúng parent context và builder entitlement, hoặc sửa Dockerfile để dùng context/dependency đóng gói phù hợp CI. Không giả định build với context `anomaly-service/` là đủ. Dependencies `torch>=2.0`/Python hiện chưa đảm bảo tái lập chính xác; phải lock/pin cho release image.

Nguồn quy trình registry: [GitHub: publish Docker images](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images). Quyền push image và quyền host pull image là hai cấu hình riêng; host private GHCR cần credential chỉ đọc.

## 3. Release deployment config cần tạo

Tạo `deploy/release/compose.yml` **image-only**, dựa trên cấu hình service/env hiện có trong `server/docker-compose.yml`. Không chỉ đổi README trong khi deploy file vẫn có `build:`.

Các service Go API, banking projector, relay, loan projector và migration dùng cùng `SERVER_IMAGE` digest. KYC dùng `KYC_IMAGE`. Hạ tầng dùng digest đã pin riêng. Không đổi volume/project name theo mỗi version vì sẽ tạo database rỗng mới.

Ví dụ phần image binding, chưa phải Compose đầy đủ:

```yaml
services:
  anomaly-server:
    image: ${SERVER_IMAGE:?SERVER_IMAGE is required}
  anomaly-worker:
    image: ${SERVER_IMAGE:?SERVER_IMAGE is required}
    command: ["/app/projector"]
  anomaly-relay:
    image: ${SERVER_IMAGE:?SERVER_IMAGE is required}
    command: ["/app/relay"]
  anomaly-migrate:
    image: ${SERVER_IMAGE:?SERVER_IMAGE is required}
    command: ["/app/migrate", "up"]
```

Config đầy đủ phải mang theo env/network/dependencies/healthchecks từ source, volume names cố định, restart policy và port exposure phù hợp. Không dùng fragment trên để chạy server thật.

ML vẫn deploy riêng bằng Swarm theo `deploy/swarm/ml-stack.yml`; release config phải thay local tags `:swarm` bằng registry digests. Một host có thể chạy cả Compose lẫn Swarm, nhưng phải khai báo rõ broker advertised listener và địa chỉ EventStoreDB mà worker truy cập được. Docker DNS trong Compose không tự resolve xuyên Swarm overlay.

## 4. Config/secret trên host

Layout đề xuất:

```text
/opt/anomaly/
  releases/0.1.0/             # manifest + immutable deployment config
  current -> releases/0.1.0   # chỉ chuyển sau smoke thành công
  shared/runtime.env         # secret, mode 0600; không nằm trong release download
  shared/models/kyc/         # mount read-only
  shared/backups/            # hoặc kho backup ngoài host
```

Biến image refs/endpoint không bí mật nằm trong release metadata; passwords/tokens chỉ đặt trên host hoặc secret store. Template cần liệt kê Mongo/Redis/JWT/KYC/HMAC/RustFS/SMTP và ML connection settings theo code hiện hành; không thay secret đang dùng khi update trừ khi có kế hoạch rotation.

Release compose/stack phải xử lý:

- Domain/TLS và reverse proxy; API health, WebSocket upgrade/timeout và request upload limits.
- Không publish database/cache/KYC/metrics/admin UI ra Internet chỉ vì Compose dev đang map port.
- Persistent volumes và UID/GID: Go image chạy UID10001; KYC non-root; kiểm quyền đọc model/ghi volume, không giải quyết bằng chmod toàn bộ 777.
- Registry authentication trên host và các Swarm nodes. Deploy private registry cần truyền auth đúng cách, không ghi token vào log.
- Backup retention, disk limit và log rotation. ML training volume local mỗi node cần chính sách riêng khi scale nhiều node.

## 5. Contract deploy script cần tạo

`deploy-server` nhận `--manifest <file> --environment staging|production`; dùng manifest được verify, không nhận `latest` tùy ý. Phải validate digest/platform/config/secret references trước mutation, khóa concurrency theo environment và lưu deployment report.

Thứ tự:

1. Preflight host: Docker/Compose/Swarm, disk, network/domain, registry pull permission, volumes và secret files.
2. Pull toàn bộ digest trước maintenance; lỗi pull phải dừng khi bản cũ còn hoạt động.
3. Snapshot/backup theo runbook, lưu migration status và versions đang chạy.
4. Stop writer nếu event/schema upgrade cần maintenance; không tự dừng database.
5. Chạy `/app/migrate status`, `/app/migrate up`, kiểm `dirty=false` bằng image mới.
6. Nâng compatible readers/projectors trước API writer. Chỉ một instance migration chạy; không để mỗi replica tự migrate đồng thời.
7. Deploy ML/consumer/policy theo cấu hình topic/schema tương thích; kiểm connectivity/readiness.
8. Start API, smoke qua public endpoint, verify actual image digests. Chỉ sau đó đánh dấu deployment successful/chuyển `current`.

Sau khi đã tạo release Compose đầy đủ, lệnh tương đương từ root repo có dạng:

```bash
docker compose --project-name anomaly \
  --env-file /opt/anomaly/shared/runtime.env \
  -f deploy/release/compose.yml pull
docker compose --project-name anomaly \
  --env-file /opt/anomaly/shared/runtime.env \
  -f deploy/release/compose.yml run --rm --no-deps anomaly-migrate /app/migrate up
```

Đây là bước pull/migrate của script đề xuất; runtime.env hoặc cơ chế render phải cung cấp image refs đã lấy từ manifest. Bootstrap infra và khởi động các dependency phải có bước riêng trước migration. Không chạy fragment này khi chưa tạo file cấu hình.

## 6. Upgrade và rollback

- Migration set đi cùng server image; lưu version/checksum và kiểm schema thực, không chỉ số migration.
- Disbursement/repayment events yêu cầu reader mới; theo [runbook giải ngân](../LOAN_DISBURSEMENT_ROLLOUT.md) và [runbook repayment](../../server/docs/loan-repayment-rollout.md).
- Không tự `migrate-down`, xóa volumes, reset banking journal hoặc chạy legacy import trong deploy script chung.
- Rollback image chỉ khi bản trước đọc được schema/event đã phát sinh; nếu không phải roll forward bản tương thích. Swarm `failure_action: rollback` chỉ xử lý service update, không đảo migration/canonical events.
- Đối với lần release đầu chưa có version trước: rollback là dừng traffic mới và phục hồi deployment/data theo runbook đã kiểm thử, không giả định có image 0.0.x để quay lại.
- Lưu deployment report đủ manifest/digests/migration/smoke/time, giữ release directory trước và backup độc lập.

## 7. Gap release tooling

- [ ] GHCR namespace, token permissions và host/node read credentials.
- [ ] Image tags/digest collection và promotion không rebuild.
- [ ] ML external checkout/context và dependency pinning.
- [ ] Release Compose image-only, Swarm digest config, bootstrap domain/network/volume.
- [ ] Secret templates, model distribution/mount permissions, exclude files khỏi build context.
- [ ] Deploy script có preflight, concurrency lock, migration/reader ordering và report.
- [ ] Backup/restore/rollback drill cho chính deployment config và images release.
