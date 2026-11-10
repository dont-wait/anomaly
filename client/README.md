# Anomaly Frontend Workspace

The frontend workspace owns the shared toolchain for both applications:

- `mobile/`: consumer app and Android Tauri shell
- `desktop/`: admin desktop app and Tauri shell

Admin reports use the shared [transaction & risk report contract](../server/docs/admin-transaction-report-contract.md)
for issue #223. Server and desktop client use one response shape without a
version query parameter. Deploy the new server and client together; the legacy
report response is no longer supported.

Install dependencies once from this directory:

```bash
corepack enable
yarn install
```

Run a target from `client/`:

```bash
yarn dev:mobile
yarn dev:desktop
```

Desktop dev server dùng `http://localhost:1422`. Khi gọi API local, thêm origin
này vào `CORS_ALLOWED_ORIGINS` của server; `.env.example` đã bao gồm cấu hình đó.

Run checks for both targets:

```bash
yarn lint
yarn typecheck
yarn test
yarn build
```

The shared Vite, Vitest, ESLint, TypeScript tooling, Nix flake and lockfile live
at this level. Target-specific source, HTML entrypoints and Tauri projects stay
inside `mobile/` or `desktop/`.
