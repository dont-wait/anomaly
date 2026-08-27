# Anomaly Server

Backend API for the Anomaly project.

## Overview

This service currently exposes a small HTTP API for account creation and account queries.

- API server: Go
- Database: MongoDB, KurrentDB
- Dev Ops: Docker 
- Caching: Redis
- Streaming: Kafka

## Prerequisites

Choose one of these workflows:

### Run with Docker Compose

- Docker
- Docker Compose

### Run the API locally

- Go `1.26`
- A running MongoDB instance

## Quick Start

### 1. Create the environment file

Create `server/.env` with these values:

```env
MONGO_ROOT_USERNAME=username
MONGO_ROOT_PASSWORD=password
RUSTFS_ACCESS_KEY=rustfsadmin
RUSTFS_SECRET_KEY=rustfsadmin
CORS_ALLOWED_ORIGINS=http://localhost:1420,http://localhost:3000,http://localhost:5173,tauri://localhost,http://tauri.localhost
```

`docker-compose.yml` uses `MONGO_ROOT_USERNAME` and `MONGO_ROOT_PASSWORD` for MongoDB.

## Run With Docker

From `server/`:

```bash
docker compose up --build
```

Main services:

- API: `http://localhost:8080`
- Health check: `http://localhost:8080/health`
- MongoDB: `localhost:27017`
- Redis: `localhost:6379`
- Kafka: `localhost:9092`
- Kafka UI: `http://localhost:8082`
- KurrentDB UI/API: `http://localhost:2113`
- RustFS API: `http://localhost:9000`
- RustFS Console: `http://localhost:9001`
- RustFS Admin Client: `docker compose exec rustfs-admin mc ls rustfs`

Stop the stack with:

```bash
docker compose down
```

If you also want to remove volumes:

```bash
docker compose down -v
```

## Run Locally

If you only want to run the API without the full stack, start MongoDB first, then run:

```bash
go mod download
go run ./cmd/api
```

Optional environment variables:

```bash
export MONGO_URI="mongodb://localhost:27017"
export MONGO_DB="anomaly"
export CORS_ALLOWED_ORIGINS="http://localhost:1420,http://localhost:3000,http://localhost:5173,tauri://localhost,http://tauri.localhost"
```

If your MongoDB instance requires authentication, use a URI like this instead:

```bash
export MONGO_URI="mongodb://username:password@localhost:27017/?authSource=admin"
```

## API Endpoints

### Health

```http
GET /health
```

## Development Notes

- Default API port: `8080`
- Default Mongo database: `anomaly`
- The current implementation stores accounts in MongoDB
- CORS should include `http://localhost:1420` if you are calling the API from the local client app

## Verification

Run the test suite from `server/`:

```bash
go test ./...
```
