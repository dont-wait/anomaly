# Anomaly

Monorepo cho dự án Anomaly.

## Repo Layout

- `client/`: shared frontend workspace and toolchain
- `client/mobile/`: consumer app với Tauri 2, React 19, TypeScript, Android và Linux desktop
- `server/`: backend API Go, worker projection, hạ tầng local bằng Docker Compose
- `specs/`: tài liệu thiết kế nội bộ

## Docs

- `server/README.md`: điểm vào chính cho backend docs
- `client/README.md`: shared frontend workspace commands
- `client/mobile/README.md`: hướng dẫn cho consumer app

## Quick Start

Backend:

```bash
cd server
go test ./...
```

Client:

```bash
cd client
corepack enable
yarn install
yarn dev:mobile
```
