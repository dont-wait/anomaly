# Anomaly Frontend Workspace

The frontend workspace owns the shared toolchain for both applications:

- `mobile/`: consumer app and Android Tauri shell
- `desktop/`: admin desktop app and Tauri shell

Admin report implementation target: [transaction & risk report v2 contract](../server/docs/admin-transaction-report-contract.md).
This Phase 1 contract defines shared server/client semantics for issue #223;
it does not describe the currently deployed API response.

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
