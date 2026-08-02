# apigo-docker

Simple news API for CI/CD RnD. Go + Echo + GORM + PostgreSQL + Redis.

## Flow

```
GET /news      -> fetch NewsAPI -> upsert to Postgres -> cache to Redis (5 min) -> return list
GET /news/:id  -> query Postgres by id
GET /health    -> health check
```

## Run with Docker

```bash
cp .env.example .env   # set NEWS_API_KEY
docker compose up --build
```

Then (host port is `8082` in compose — `8080` was taken by another container on the dev machine; change the `app` port mapping if you want another port):

```bash
curl http://localhost:8082/news
curl http://localhost:8082/news/1
```

## Run locally

Needs Postgres + Redis running (can use `docker compose up postgres redis`).

```bash
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go run .
```

## Test

```bash
go test ./...
```

## CI

GitHub Actions (`.github/workflows/ci.yml`): vet + test + build on every push/PR to `main`, then Docker image build.

## Structure

```
main.go            # entrypoint, routes
config/            # env config
models/            # GORM models
database/          # Postgres + Redis connections
services/          # NewsAPI client + mapping
handlers/          # Echo handlers
```
