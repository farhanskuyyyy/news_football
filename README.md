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

## Ports

Non-default host ports because the dev machine already uses the defaults (nginx on 8080, local Postgres on 5432, other Redis containers on 6379/6380):

| Service            | Host port | Notes                                  |
|--------------------|-----------|----------------------------------------|
| App (Docker)       | 8082      | maps to 8080 in container              |
| App (`make run`)   | 8090      | from `PORT` in `.env`                  |
| Postgres (Docker)  | 5434      | `.env` `DB_PORT=5434` points here      |
| Redis (Docker)     | not exposed | internal network only; local run uses host Redis on 6379 |

## Test

```bash
make test    # go test -race
make check   # vet + test + build (same as CI)
```

## CI/CD

GitHub Actions (`.github/workflows/ci.yml`): vet + test + build on every push/PR to `main`, then Docker image build, then SSH deploy to VPS (push to `main` only).

Full server setup steps: see [DEPLOY.md](DEPLOY.md).

## Structure

```
main.go            # entrypoint, routes
config/            # env config
models/            # GORM models
database/          # Postgres + Redis connections
services/          # NewsAPI client + mapping
handlers/          # Echo handlers
```
