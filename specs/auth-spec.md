# Design Plan — Login / Register / Verify với JWT trên `UserAccount`

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Tính năng** | Đăng ký – đăng nhập – định danh (KYC) cho `UserAccount` bằng JWT, kết hợp event-sourcing (KurrentDB) + projection worker (MongoDB) |
| **Mục tiêu** | Cung cấp auth + KYC flow cho `UserAccount` mà vẫn giữ event log bất biến làm audit log và không phá vỡ Clean Architecture hiện có |
| **Kiến trúc** | Client App → API Gateway (Go HTTP) → Application Layer (Use Case) → EventStore (KurrentDB) + MongoDB (read model) + Worker (projection) |
| **Dữ liệu đầu vào** | `username`, `email`, `password` cho register; `login` (email/username) + `password` cho login; `idCardFrontUrl`, `idCardBackUrl`, `liveVideoUrl` cho verify |
| **Retry Policy** | External KYC service retry riêng (out of scope); backend chỉ ghi event `AccountVerified` khi nhận kết quả thành công |
| **Trạng thái** | Draft |

---

## 2. Mục tiêu & Phạm vi

### 2.1 Mục tiêu
- Cho phép đăng ký tài khoản mới với `username`, `email`, `password`.
- Cho phép đăng nhập bằng email hoặc username + password, trả về JWT.
- Buộc người dùng phải hoàn tất định danh (KYC) trước khi sử dụng đầy đủ chức năng; trạng thái lưu qua cờ `IsVerify`.
- Cung cấp endpoint `verify` để external KYC service báo kết quả định danh.
- Duy trì event log bất biến cho mọi thay đổi nghiệp vụ (register, verify, withdraw).
- Tận dụng chính projection worker để giải quyết eventual consistency giữa write (KurrentDB) và read (Mongo) mà không cần fallback phức tạp.

### 2.2 Phạm vi
- Endpoint `POST /api/auth/register`.
- Endpoint `POST /api/auth/login`.
- Endpoint `GET /api/auth/me`.
- Endpoint `POST /api/accounts/{id}/verify` (ghi nhận kết quả từ external KYC).
- Password hashing bằng bcrypt.
- JWT HS256 với claim `userID`, `username`, `isVerify`, `expiresAt`.
- Auth middleware `RequireAuth` cho route cần bảo vệ.
- Worker projection xử lý thêm `AccountVerified` event.
- DTO response tách biệt domain entity (không lộ `passwordHash`).

### 2.3 Ngoài phạm vi
- Service KYC thực sự (face match, liveness, OCR CCCD) — chỉ tiêu thụ kết quả.
- Refresh token, logout, revoke token.
- Quên mật khẩu / đổi mật khẩu qua email.
- Phân quyền RBAC, multi-factor auth.
- Rate limiting login / lockout account sau nhiều lần sai.
- Unique index Mongo cho email (chỉ check duplicate qua read model).

---

## 3. Giả định nghiệp vụ

| Giả định | Chi tiết |
|----------|----------|
| `UserAccount` đã được gộp | Identity (auth) + wallet (Amount/Withdraw) + KYC (3 field identity) cùng tồn tại trong một entity |
| Event-sourcing là source of truth | Mọi thay đổi đều ghi event lên KurrentDB; Mongo là read model do worker project |
| Worker set `IsVerify=false` | Khi project `AccountCreated`, worker **tự set** `IsVerify=false` (không có trong event payload) — đảm bảo mọi account mới đều bắt buộc verify |
| User phải qua KYC mới login đầu | Quy trình tự nhiên: register → KYC (external) → verify → login. Khoảng thời gian này đủ dài để projection catch-up |
| External KYC service đã có | Service khác làm face match, liveness, OCR CCCD; chỉ gọi callback về endpoint `verify` |
| Identity URLs do external service cung cấp | External service upload media lên storage riêng → trả URL cho client → client gọi `verify` với URL |
| Login không reject `IsVerify=false` | JWT luôn trả kèm claim `isVerify=false`; FE tự xử lý UX redirect |

---

## 4. User Flow

### 4.1 Flow chính

```text
Bắt đầu (chưa có tài khoản)
  → Register: POST /api/auth/register (username, email, password)
  → 201 UserResponse { id, username, email, isVerify:false, ... }
  → Login: POST /api/auth/login (login, password)
  → 200 AuthResponse { token, expiresAt, user { isVerify:false } }
  → FE đọc claim isVerify=false → redirect sang màn KYC
  → User hoàn tất KYC ở external service → nhận URL CCCD mặt trước/sau + live video
  → Verify: POST /api/accounts/{id}/verify (urls, JWT)
  → 200 UserResponse { isVerify:true, idCardFrontUrl, idCardBackUrl, liveVideoUrl }
  → FE cho phép user dùng app
  → Login lần sau: claim isVerify=true
```

### 4.2 Flow logout / token hết hạn

```text
JWT hết hạn (exp < now)
  → Request bất kỳ có Authorization → middleware Parse fail
  → 401 Unauthorized
  → FE redirect về login
```

### 4.3 Flow register thất bại

```text
POST /api/auth/register
  ├─ Email sai định dạng         → 400 ErrInvalidEmail
  ├─ Password < 8 ký tự          → 400 ErrWeakPassword
  ├─ Username rỗng               → 400 ErrInvalidUsername
  ├─ Email đã tồn tại            → 409 ErrUserAlreadyExists
  └─ Username đã tồn tại        → 409 ErrUserAlreadyExists
```

### 4.4 Flow login thất bại

```text
POST /api/auth/login
  ├─ Sai email/username hoặc password → 401 ErrInvalidCredentials
  └─ OK (kể cả isVerify=false)        → 200 AuthResponse
```

---

## 5. Trải nghiệm người dùng

### 5.1 UX register
- Client validate sơ bộ: email có `@`, password ≥ 8 ký tự, username không rỗng.
- Hiển thị lỗi server (400/409) bằng thông điệp dễ hiểu, không lộ stack trace.

### 5.2 UX sau register
- Sau 201, FE chuyển sang màn login (không tự động login vì user chọn "không auto-login").
- User nhập lại email + password → login.

### 5.3 UX login
- FE nhận `AuthResponse` → lưu `token` (localStorage / cookie httpOnly).
- Decode JWT client-side để biết `isVerify`:
  - `isVerify=true` → vào app.
  - `isVerify=false` → vào màn "Hoàn tất định danh" (điều hướng sang external KYC service).

### 5.4 UX verify
- External KYC service upload ảnh/video → trả URLs cho FE.
- FE gọi `POST /api/accounts/{id}/verify` với JWT đang có + body chứa URLs.
- Response trả về `UserResponse` với `isVerify=true` đầy đủ → FE chuyển sang app chính.

### 5.5 UX rule

| Rule | Chi tiết |
|------|----------|
| Clear feedback | Error message ngắn gọn, mapping từ `ErrInvalid*` / `ErrUserAlreadyExists` / `ErrInvalidCredentials` |
| No password echo | Không hiển thị password hash / token rò rỉ ra DOM log |
| Token storage | FE tự quyết (httpOnly cookie là tốt nhất cho web) — backend không áp đặt |
| No manual refresh | Backend không cấp refresh token trong MVP; hết hạn → login lại |

---

## 6. Kiến trúc hệ thống

### 6.1 Thành phần

1. **Client App (Web / Tauri)**
   - Form register / login.
   - Decode JWT để biết `isVerify`.
   - Điều phối external KYC service.
   - Gọi `verify` sau khi nhận URL từ KYC.

2. **API Gateway — Go HTTP server (`cmd/api/main.go`)**
   - CORS middleware (`middleware/cors.go`).
   - Auth middleware `RequireAuth` (`middleware/auth.go`) — Bearer JWT validation.
   - Route registration (`presentation/http/router.go`).

3. **Presentation Layer (`internal/presentation/http/handler/account/`)**
   - Handlers: `Register`, `Login`, `Me`, `Verify`, `GetByID`, `GetByEmail`, `GetAll`.
   - DTOs: `RegisterRequest`, `LoginRequest`, `VerifyRequest`, `UserResponse`, `AuthResponse`.
   - Mapper `toUserResponse` / `toAuthResponse` — không lộ `PasswordHash`.

4. **Application Layer (`internal/application/account/`)**
   - Commands: `RegisterAccountCommandHandler`, `VerifyAccountCommandHandler`.
   - Queries: `LoginQueryHandler`, `GetAccountByIDQueryHandler`, `GetAccountByEmailQueryHandler`, `GetAllAccountsQueryHandler`.
   - Port: `AccountQueryRepository`, `TokenService` interface, `Claims`.

5. **Domain Layer (`internal/domain/account/`)**
   - `UserAccount` entity + business rules (`Withdraw`).
   - Domain errors: `ErrInvalidEmail`, `ErrWeakPassword`, `ErrInvalidUsername`, `ErrUserAlreadyExists`, `ErrInvalidCredentials`, `ErrAccountNotFound`, `ErrInvalidAmount`, `ErrInsufficientFunds`.

6. **Infrastructure — JWT (`internal/infrastructure/auth/`)**
   - `token_service.go`: HS256 với `golang-jwt/jwt/v5`. Issue + Parse.

7. **Infrastructure — Event Store (`internal/infrastructure/eventstore/`)**
   - Stream `account-{id}` trong KurrentDB.
   - Events: `AccountCreated`, `AccountVerified`, `AccountWithdraw`.
   - `applyEvent` replay state.

8. **Infrastructure — MongoDB (`internal/infrastructure/mongo/`)**
   - Collection `accounts` làm read model.
   - `accountRecord` chứa đầy đủ field kể cả `IsVerify` + 3 field identity.
   - `FindByID`, `FindByEmail`, `FindByUsername`, `FindAll`, `Save` (ReplaceOne upsert).

9. **Projection Worker (`cmd/worker/main.go`)**
   - Subscribe `$all` của KurrentDB.
   - Replay account state qua `esRepo.FindByID` → upsert Mongo.
   - Lưu checkpoint để resume.

### 6.2 Dependency direction (Clean Architecture)

```
Presentation ──> Application ──> Domain
       │                │             ▲
       ▼                ▼             │
   Infrastr. JWT   Infrastr. Event/Mongo (depend on Domain via ports)
```

Outer layers depend inward. Domain không biết JSON / HTTP / KurrentDB / Mongo.

---

## 7. Pipeline xử lý

### 7.1 Register pipeline

1. `httpx.DecodeJSON` parse `RegisterRequest` (body ≤ 1 MB, không cho field thừa).
2. Handler gọi `RegisterAccountCommandHandler.Handle`.
3. Validate format: email regex, password len ≥ 8, username non-empty.
4. Check duplicate qua read repo (Mongo): `FindByEmail`, `FindByUsername` → `ErrUserAlreadyExists` nếu có.
5. `bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)`.
6. Build `UserAccount{Id: newID(), Username, Email, PasswordHash, IsVerify:false, Amount:0}`.
7. `writeRepo.Save` (eventstore) → append `AccountCreated` event lên stream `account-{id}`.
8. Trả về `UserAccount` → Handler map sang `UserResponse` (không passwordHash) → 201.

### 7.2 Worker projection — AccountCreated

1. Worker nhận event `AccountCreated` qua subscribe `$all`.
2. `esRepo.FindByID(ctx, id)` → replay stream từ đầu → state có `Id, Username, Email, PasswordHash, Amount=0, IsVerify=false` (default zero value).
3. `mongoRepo.Save(ctx, acc)` → `ReplaceOne` upsert theo `_id`. Record có `IsVerify=false` do worker đặt (mặc định từ replay).
4. Lưu checkpoint.

### 7.3 Verify pipeline

1. `httpx.DecodeJSON` parse `VerifyRequest{idCardFrontUrl, idCardBackUrl, liveVideoUrl}`.
2. Middleware `RequireAuth` validate JWT → inject `Claims` vào context.
3. Handler lấy `userID` từ context, kiểm tra khớp `{id}` trong URL (chống user verify hộ).
4. `VerifyAccountCommandHandler.Handle` → build entity với 3 URL identity, gọi `repo.Save`.
5. `Save` (eventstore) phát hiện stream đã tồn tại → append `AccountVerified` event (không phải `AccountCreated`).
6. Trả về UserAccount → map `UserResponse` → 200.

### 7.4 Worker projection — AccountVerified

1. Worker nhận event `AccountVerified`.
2. `esRepo.FindByID(ctx, id)` → replay full stream:
   - `AccountCreated` set base state (Username, Email, PasswordHash, Amount=0).
   - `AccountVerified` set 3 URL + `IsVerify=true`.
3. `mongoRepo.Save(ctx, acc)` → upsert với `IsVerify=true` + identity URLs.
4. Lưu checkpoint.

### 7.5 Login pipeline

1. `httpx.DecodeJSON` parse `LoginRequest{login, password}`.
2. Handler gọi `LoginQueryHandler.Handle`.
3. `readRepo.FindByEmail(login)`; nếu miss → `FindByUsername(login)`.
4. Nếu cả 2 miss → `ErrInvalidCredentials` (không phân biệt "không tồn tại" / "sai password" để chống enumeration).
5. `bcrypt.CompareHashAndPassword(acc.PasswordHash, password)` → mismatch → `ErrInvalidCredentials`.
6. `tokenService.Issue(acc.Id, acc.Username, acc.IsVerify)` → JWT HS256 với claims.
7. Trả `LoginResult{User, Token, ExpiresAt}` → Handler map `AuthResponse` → 200.

### 7.6 JWT verification (middleware)

1. Đọc header `Authorization: Bearer <token>`.
2. Nếu thiếu / sai format → 401.
3. `tokenService.Parse(token)` → validate signature với `JWT_SECRET`, kiểm tra `ExpiresAt`.
4. Inject `Claims{UserID, Username, IsVerify, ExpiresAt}` vào `context.Context`.
5. Cho request đi tiếp tới handler.

---

## 8. Stack kỹ thuật đề xuất

| Thành phần | Công nghệ đề xuất | Vai trò |
|-----------|-------------------|---------|
| Password hashing | `golang.org/x/crypto/bcrypt` | Hash + verify password (DefaultCost) |
| JWT | `github.com/golang-jwt/jwt/v5` | Issue + Parse JWT HS256 |
| Event store | `github.com/kurrent-io/KurrentDB-Client-Go` | Append + read stream, optimistic concurrency |
| Read model | `go.mongodb.org/mongo-driver/v2` | Collection `accounts`, ReplaceOne upsert |
| Logging | `github.com/rs/zerolog` | Structured log, không log raw password / token |
| HTTP | `net/http` (stdlib, Go 1.25 mux pattern) | Routing + middleware |
| Config | `github.com/joho/godotenv` + custom `Loader` | Env loader, singleton |

### 8.1 Lý do chọn stack
- Tất cả dependency đã có trong `go.mod` trừ `golang-jwt/jwt/v5` (thêm mới).
- `bcrypt` đã có ở `golang.org/x/crypto` (indirect), chỉ cần đưa thành direct.
- Tận dụng Go 1.25 `mux.HandleFunc("POST /path", h)` cho routing gọn.
- Không thêm web framework ngoài stdlib — phù hợp style hiện tại.

---

## 9. API đề xuất

### 9.1 Register
`POST /api/auth/register`

**Request**
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "s3cret-pwd"
}
```

**Response — 201**
```json
{
  "id": "abcd1234...",
  "username": "alice",
  "email": "alice@example.com",
  "idCardFrontUrl": "",
  "idCardBackUrl": "",
  "liveVideoUrl": "",
  "isVerify": false,
  "amount": 0
}
```

**Error responses**
- `400 ErrInvalidEmail` / `ErrWeakPassword` / `ErrInvalidUsername`
- `409 ErrUserAlreadyExists`

---

### 9.2 Login
`POST /api/auth/login`

**Request**
```json
{
  "login": "alice@example.com",
  "password": "s3cret-pwd"
}
```

`login` chấp nhận cả email lẫn username.

**Response — 200**
```json
{
  "token": "<jwt>",
  "expiresAt": "2026-08-28T10:00:00Z",
  "user": {
    "id": "...",
    "username": "alice",
    "email": "alice@example.com",
    "idCardFrontUrl": "",
    "idCardBackUrl": "",
    "liveVideoUrl": "",
    "isVerify": false,
    "amount": 0
  }
}
```

**Error responses**
- `401 ErrInvalidCredentials`

---

### 9.3 Me
`GET /api/auth/me` — yêu cầu `Authorization: Bearer <jwt>`

**Response — 200**
```json
{
  "id": "...",
  "username": "alice",
  "email": "alice@example.com",
  "idCardFrontUrl": "...",
  "idCardBackUrl": "...",
  "liveVideoUrl": "...",
  "isVerify": true,
  "amount": 0
}
```

**Error responses**
- `401` thiếu / sai token

---

### 9.4 Verify
`POST /api/accounts/{id}/verify` — yêu cầu `Authorization: Bearer <jwt>`

**Request**
```json
{
  "idCardFrontUrl": "https://storage.example.com/cccd-front.jpg",
  "idCardBackUrl": "https://storage.example.com/cccd-back.jpg",
  "liveVideoUrl": "https://storage.example.com/live.mp4"
}
```

**Response — 200**
```json
{
  "id": "...",
  "username": "alice",
  "email": "alice@example.com",
  "idCardFrontUrl": "https://...",
  "idCardBackUrl": "https://...",
  "liveVideoUrl": "https://...",
  "isVerify": true,
  "amount": 0
}
```

**Error responses**
- `401` thiếu / sai token
- `403` user trong JWT không khớp `{id}` trong URL

---

### 9.5 Endpoints giữ nguyên (không đổi)

| Method | Path | Mô tả |
|---|---|---|
| GET | `/api/accounts` | List accounts |
| GET | `/api/accounts/{id}` | Get by ID |
| GET | `/api/accounts/by-email/{email}` | Get by email |

---

## 10. Decision Model / Error Codes

### 10.1 Error mapping (HTTP)

| Domain Error | HTTP | Ý nghĩa FE |
|---|---|---|
| `ErrInvalidEmail` | 400 | "Email không hợp lệ" |
| `ErrWeakPassword` | 400 | "Mật khẩu phải ≥ 8 ký tự" |
| `ErrInvalidUsername` | 400 | "Username không được rỗng" |
| `ErrUserAlreadyExists` | 409 | "Email hoặc username đã được sử dụng" |
| `ErrInvalidCredentials` | 401 | "Sai email/username hoặc mật khẩu" |
| `ErrAccountNotFound` | 404 | "Không tìm thấy tài khoản" |
| `ErrInvalidAmount` | 400 | "Số tiền không hợp lệ" |
| `ErrInsufficientFunds` | 400 | "Số dư không đủ" |

### 10.2 Response shape cho error

```json
{ "error": "<message từ err.Error() hoặc http.StatusText(code)>" }
```

5xx luôn trả `"internal server error"` (không lộ chi tiết).

---

## 11. Authentication Policy

| Rule | Chi tiết |
|------|----------|
| Password length | ≥ 8 ký tự (chỉ check length, không enforce complexity trong MVP) |
| Password storage | bcrypt cost 10 (DefaultCost) |
| JWT algorithm | HS256 |
| JWT TTL | 24h (cấu hình qua `JWT_EXPIRY`) |
| JWT secret | Từ env `JWT_SECRET` (bắt buộc, fail nếu thiếu) |
| Login identifier | email **hoặc** username |
| Verify auth | JWT bắt buộc, userID trong claim phải khớp path param `{id}` |
| Refresh token | Không có trong MVP — hết hạn login lại |
| Logout | Không có — JWT stateless; client tự xoá token |

### 11.1 IsVerify policy
- Worker **luôn** set `IsVerify=false` khi project `AccountCreated` (giá trị default zero-value).
- Không thể set `IsVerify=true` từ client trực tiếp — chỉ qua event `AccountVerified`.
- Không có endpoint un-verify.

---

## 12. Bảo mật & Quyền riêng tư

| Hạng mục | Yêu cầu |
|----------|---------|
| In transit | TLS cho mọi HTTP traffic (terminate ở gateway/proxy) |
| Password storage | bcrypt hash, không bao giờ lưu plaintext |
| Password exposure | Không log, không trả qua API — DTO `UserResponse` loại `PasswordHash` |
| Token exposure | Không log raw JWT ở application logs |
| CORS | `middleware.NewCORS(allowedOrigins)` — chỉ origin trong allowlist được gắn `Access-Control-Allow-Origin` |
| JWT secret | Không commit vào repo; lấy từ env |
| Identity URLs | Backend chỉ lưu URL (string); raw ảnh/video do external service quản lý storage |
| Logging | Structured log qua `zerolog` — không log body request chứa password |
| Event log | Chứa `passwordHash` (bcrypt) — không reversible nhưng cần rotate bằng event mới nếu đổi policy hash |
| Replay protection | JWT TTL giới hạn; không có cơ chế revoke (chấp nhận rủi ro trong MVP) |

---

## 13. Yêu cầu phi chức năng

- 95% request `/register` và `/login` trả response trong vòng 200ms (không tính network).
- `/verify` trả trong vòng 100ms (chỉ ghi event).
- Worker projection catch-up trong vòng 1 giây sau khi event xuất hiện.
- Threshold JWT expiry cấu hình server-side qua env.
- Request có trace ID để debug khi cần (mở rộng sau nếu cần).
- Health endpoint `GET /health` (đã có) cho load balancer / container probe.

---

## 14. Acceptance Criteria

1. User có thể đăng ký qua `POST /api/auth/register` với username/email/password hợp lệ.
2. Register thất bại với mã 400 nếu email sai định dạng, password < 8 ký tự, username rỗng.
3. Register thất bại với mã 409 nếu email hoặc username đã tồn tại.
4. Sau register, `UserAccount` được ghi vào KurrentDB (`AccountCreated` event).
5. Worker project account mới vào Mongo với `IsVerify=false` (trong vòng 1s).
6. User có thể đăng nhập qua `POST /api/auth/login` bằng email **hoặc** username + password.
7. Login trả JWT hợp lệ với claims `userID`, `username`, `isVerify`, `expiresAt`.
8. Login **không** reject user có `IsVerify=false`; FE tự xử lý claim.
9. `GET /api/auth/me` với JWT hợp lệ trả `UserResponse` không chứa `passwordHash`.
10. `POST /api/accounts/{id}/verify` với JWT hợp lệ ghi `AccountVerified` event lên KurrentDB.
11. Sau verify, worker project Mongo với `IsVerify=true` và 3 field identity.
12. JWT hết hạn (`expiresAt < now`) → middleware trả 401, không cho request đi tiếp.
13. JWT sai signature → middleware trả 401.
14. `UserResponse` **không bao giờ** chứa `passwordHash` dù ở bất kỳ endpoint nào.
15. Event log chứa đủ 3 loại event: `AccountCreated`, `AccountVerified`, `AccountWithdraw` (event store bất biến).
16. `POST /api/accounts` (tạo account cũ) đã bị xoá — chỉ còn `POST /api/auth/register`.

---

## 15. Open Questions

| # | Câu hỏi | Ảnh hưởng |
|---|---------|-----------|
| OQ-01 | External KYC service có gọi trực tiếp endpoint `verify` của backend, hay chỉ trả URL cho FE rồi FE gọi? | API contract |
| OQ-02 | Có cần endpoint đổi password không? (Hiện không có trong scope.) | Security policy |
| OQ-03 | Refresh token có nên thêm vào MVP không? | UX khi JWT hết hạn |
| OQ-04 | Email có cần verify (gửi link confirm) trước khi cho dùng app không? | Onboarding flow |
| OQ-05 | Account lockout sau N lần login sai? | Brute force protection |
| OQ-06 | Media retention cho CCCD / live video do ai quản? Backend chỉ giữ URL? | Privacy/compliance |

---

## 16. Next Steps

1. Cập nhật `domain/account/account.go` — thêm `IsVerify` + 3 field identity, giữ `Withdraw`.
2. Thêm `errors.go` — sentinel mới: `ErrInvalidEmail`, `ErrWeakPassword`, `ErrInvalidUsername`, `ErrUserAlreadyExists`, `ErrInvalidCredentials`.
3. Cập nhật event schema — thu gọn `AccountCreated`, thêm `AccountVerified` + `applyEvent`.
4. Mở rộng `mongo/account_record.go` + `account_repository.go`.
5. Cập nhật worker `cmd/worker/main.go` xử lý `EventAccountVerified`.
6. Viết `application/account/commands/register_account.go` + `verify_account.go`.
7. Viết `application/account/queries/login.go` + `port.go` (TokenService, Claims).
8. Viết `infrastructure/auth/token_service.go` (HS256 JWT).
9. Cập nhật handler — xoá `Create`, thêm `Register`, `Login`, `Me`, `Verify` + DTOs.
10. Viết `middleware/auth.go` (`RequireAuth`).
11. Cập nhật `composition/account.go` + `router.go` + `cmd/api/main.go`.
12. Thêm `JWT_SECRET`, `JWT_EXPIRY` vào `.env.example` / `.env` + `AuthConfig` trong `domain/config.go`.
13. `go get github.com/golang-jwt/jwt/v5`; `go get golang.org/x/crypto`.
14. Chạy `go build ./...`, `go vet ./...`, `go test ./...`.
15. Smoke test end-to-end: register → đợi worker project → login → verify → login lại kiểm tra claim `isVerify=true`.

---

*Phiên bản: 0.1*
*Ngày tạo: 2026-08-27*
*Trạng thái: Draft*
