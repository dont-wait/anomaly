# Anomaly Frontend Workspace

The frontend workspace owns the shared toolchain for both applications:

- `mobile/`: consumer app and Android Tauri shell
- `desktop/`: admin desktop app and Tauri shell

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
