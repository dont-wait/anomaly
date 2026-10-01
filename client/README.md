# Anomaly Frontend Workspace

The frontend workspace owns the shared toolchain for the consumer application:

- `mobile/`: consumer app and Android Tauri shell

Install dependencies once from this directory:

```bash
corepack enable
yarn install
```

Run a target from `client/`:

```bash
yarn dev:mobile
```

Run checks:

```bash
yarn lint
yarn typecheck
yarn test
yarn build
```

The shared Vite, Vitest, ESLint, TypeScript tooling, Nix flake and lockfile live
at this level. The consumer source, HTML entrypoint and Tauri project stay inside
`mobile/`.
