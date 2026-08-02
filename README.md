# apigo-docker

Simple news API for CI/CD RnD. Go + Echo + GORM + PostgreSQL + Redis.

## Flow

```
GET /news      -> fetch NewsAPI -> upsert to Postgres -> cache to Redis (5 min) -> return list
GET /news/:id  -> query Postgres by id
GET /health    -> health check
```

## Quick start (Makefile)

```bash
make help    # list all commands
make up      # build + start full stack (app + postgres + redis)
make smoke   # test endpoints
make logs    # tail app logs
make down    # stop (data kept)
```

## Run with Docker (manual)

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

```bash
make run     # starts postgres + redis containers, then go run .
```

Note: local run uses `REDIS_ADDR=localhost:6379` from `.env` — needs a Redis reachable on the host (the compose Redis is not exposed to the host; adjust `.env` or add a port mapping if needed).

## Test

```bash
make test    # go test -race
make check   # vet + test + build (same as CI)
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
