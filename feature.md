# Apigo Docker - Football & News Aggregator

## Project Overview
Apigo Docker is a Go-based backend service that aggregates football (sport) data from the Sportmonks API and news from NewsAPI, providing a comprehensive REST API for frontend consumption. The application includes background scraping jobs to keep data synchronized, caching mechanisms, and a modular architecture separating concerns into handlers, services, models, and database layers.

## Tech Stack
- **Language**: Go (Golang)
- **Web Framework**: Echo v4
- **ORM**: GORM with PostgreSQL driver
- **Database**: PostgreSQL (primary data store)
- **Caching**: Redis (for news caching)
- **HTTP Clients**: Custom wrappers for Sportmonks v3 API and NewsAPI
- **Dependency Management**: Go modules
- **Containerization**: Docker (multi-stage build implied)

## Features
- **News Aggregation**:
  - Fetch latest football news from NewsAPI
  - Cache results in Redis for 5 minutes
  - Manual refresh endpoint with optional token protection
  - CRUD operations for stored news articles
- **Football Data Portal**:
  - League & season management (listing, enabling/disabling)
  - Season overview (team/fixture/round/stage counts)
  - Standings with detailed metrics (P, W, D, L, GF, GA, GD, PTS, form)
  - Cup bracket visualization (knock-out/qualifying stages as ties)
  - Rounds and fixtures filtering (by round, stage, status)
  - Detailed match center (events, lineups, statistics, scores, referees)
  - Team profiles (squad, players, venue, rivals, coaches)
  - Player details (bio, position, current teams, transfers, statistics, topscorer records)
  - Topscorers per season (configurable metric type)
  - Transfer listings per season/team
- **Sportmonks API Proxy**:
  - Direct proxy to all Sportmonks v3 endpoints under `/sportmonks/*`
  - Helper endpoints for core data (continents, countries, regions, cities, types)
  - Football-specific helper endpoints (livescores, fixtures, leagues, teams, players, standings, topscorers, transfers, venues, coaches, referees, bookmakers, markets, usage)
- **Background Synchronization**:
  - Configurable auto-scrape scheduler (TTL-gated) for football data
  - Per-table TTL synchronization strategy (dynamic intervals)
  - Manual trigger endpoints for scraping core, leagues, football data, fixture details, player statistics
  - Job control (start, stop, list running scrape jobs)
  - Sync status monitoring table
- **Resilience & Rate Limiting**:
  - Automatic retries with exponential backoff for API calls
  - Rate limit handling (429) with retry-after logic
  - Pagination cursors and safety valves against infinite loops
  - Database upserts with conflict resolution (preserve admin flags)
  - Structured logging for scrape jobs and errors
- **Extensible Data Model**:
  - Comprehensive football data model (leagues, seasons, stages, rounds, teams, players, fixtures, events, lineups, details, statistics, scores, venues, coaches, referees, standings, topscorers, transfers, rivals, states)
  - Pivot tables for many-to-many relationships (season_team, player_season, team_rival, team_coach, fixture_referee)
  - Sync tracking table for TTL-based updates
  - News article model with source, author, content, URLs

## API Endpoints

### Health Check
- `GET /health` - Returns service status

### News Endpoints (`/news`)
- `GET /news` - Get cached news list (source indicates cache/db)
- `GET /news/{id}` - Get single news article by ID
- `POST /news/refresh` - Force refresh from NewsAPI (requires X-Refresh-Token if set)

### Portal Endpoints (`/portal`) - Football Presentation API
- `GET /portal/leagues` - List all leagues with season counts
- `POST /portal/leagues/{id}/status` - Enable/disable league for scraping
- `GET /portal/leagues/{league_id}/seasons` - List seasons for a league
- `GET /portal/seasons/{season_id}/overview` - Season high-level metrics
- `GET /portal/seasons/{season_id}/standings` - League table standings
- `GET /portal/seasons/{season_id}/bracket` - Cup bracket (knock-out/qualifying stages)
- `GET /portal/seasons/{season_id}/rounds` - List rounds with fixture counts
- `GET /portal/seasons/{season_id}/fixtures` - Filter fixtures (by round, stage, status, team)
- `GET /portal/seasons/{season_id}/topscorers` - Topscorers for a season (by type)
- `GET /portal/seasons/{season_id}/transfers` - Transfers relevant to season teams
- `GET /portal/fixtures/{id}` - Complete match center for a fixture
- `GET /portal/teams/{id}` - Team profile with squad, players, venue, rivals, coach
- `GET /portal/players/{id}` - Detailed player information
- `GET /portal/seasons/{season_id}/teams` - Teams participating in a season

### Sportmonks Endpoints (`/sportmonks`) - API Wrapper & Scraper Triggers
- `GET /sportmonks/continents` - List continents
- `GET /sportmonks/countries` - List countries
- `GET /sportmonks/regions` - List regions
- `GET /sportmonks/cities` - List cities
- `GET /sportmonks/types` - List entity types
- `GET /sportmonks/livescores` - Live match scores
- `GET /sportmonks/livescores/inplay` - In-play live scores
- `GET /sportmonks/fixtures` - List fixtures (with includes/filter)
- `GET /sportmonks/fixtures/{id}` - Single fixture details
- `GET /sportmonks/leagues` - List leagues
- `GET /sportmonks/leagues/{id}` - Single league details
- `GET /sportmonks/teams` - List teams
- `GET /sportmonks/teams/{id}` - Single team details
- `GET /sportmonks/teams/search/{name}` - Search teams by name
- `GET /sportmonks/standings` - List standings
- `GET /sportmonks/topscorers` - List topscorers
- `GET /sportmonks/transfers` - List transfers
- `GET /sportmonks/venues` - List venues
- `GET /sportmonks/coaches` - List coaches
- `GET /sportmonks/referees` - List referees
- `GET /sportmonks/bookmakers` - List betting bookmakers
- `GET /sportmonks/markets` - List betting markets
- `GET /sportmonks/my/usage` - API usage statistics
- **Scraper Triggers** (POST):
  - `/sportmonks/scrape/core` - Scrape core reference data
  - `/sportmonks/scrape/leagues` - Scrape leagues
  - `/sportmonks/scrape/football` - Scrape all football data (options: league_id, season_id, force)
  - `/sportmonks/scrape/fixture-details` - Scrape detailed fixture data (options: limit, season_id, force)
  - `/sportmonks/scrape/fixture/{id}` - Scrape single fixture on demand
  - `/sportmonks/scrape/player-statistics` - Scrape player season statistics (option: season_id)
  - `/sportmonks/scrape/stop/{job}` - Stop a running scrape job
  - `/sportmonks/scrape/jobs` - List currently running scrape jobs
  - `/sportmonks/sync/status` - Get synchronization tracker status
  - `/sportmonks/sync/seed` - Re-seed sync_tables configuration
- **Dynamic Proxy**:
  - `GET /sportmonks/*` - Proxy any Sportmonks v3 endpoint (e.g., `/sportmonks/football/transfers/latest`)

### Dynamic Proxy Examples
- `GET /sportmonks/football/fixtures/between/2026-01-01/2026-01-31?include=participants`
- `GET /sportmonks/football/leagues/live`
- `GET /sportmonks/football/players/search/Lionel%20Messi`

## Architecture
```
┌─────────────────┐    ┌──────────────────┐    ┌────────────────────┐
│   HTTP Server   │    │   Background     │    │   External APIs    │
│   (Echo)        │◄───│   Scheduler      │───►│   Sportmonks v3    │
└─────────────────┘    │   (Scraper Jobs) │    │   NewsAPI          │
        │              └──────────────────┘    └────────────────────┘
        ▼                         │
┌─────────────────┐              ▼
│   Handlers      │    ┌────────────────────┐
│   (News,        │    │   Services Layer   │
│    Portal,      │    │   (CoreScraper,    │
│    Sportmonks)  │    │    FootballScraper,│
└─────────────────┘    │    NewsAPIClient)  │
        │              └────────────────────┘
        ▼                         │
┌─────────────────┐              ▼
│   Models        │    ┌────────────────────┐
│   (GORM structs)│    │   Database Layer   │
└─────────────────┘    │   (Postgres + Redis)│
        │              └────────────────────┘
        ▼
┌─────────────────┐
│   Config        │
└─────────────────┘
```
- **Handlers**: Define HTTP endpoints, validate input, orchestrate service calls
- **Services**: Contain business logic and API client wrappers (Sportmonks, NewsAPI)
- **Models**: GORM structs representing database tables
- **Database**: PostgreSQL for persistent storage, Redis for news caching
- **Background Scheduler**: Triggered at intervals to run football scrapers based on TTL configuration
- **Sync Tracking**: `sync_tables` table records last sync time, status, and interval per table to enable intelligent incremental updates

## Setup Instructions
### Prerequisites
- Go 1.20+ (for local development)
- PostgreSQL server
- Redis server
- Docker & Docker Compose (for containerized deployment)
- API keys:
  - Sportmonks API Token
  - NewsAPI Key (optional for news features)
  - Optional refresh token for protected news refresh endpoint

### Local Development
1. Clone the repository
2. Copy `.env.example` to `.env` and fill in required values
3. Ensure PostgreSQL and Redis are running on default ports (or configure via env)
4. Run database migrations (auto-migrated on startup)
5. Start the application:
   ```bash
   go run main.go
   ```
6. Server will start on port defined by `PORT` env (default 8080)

### Using Docker Compose
1. Create a `.env` file with required variables (see Environment Variables section)
2. Build and start services:
   ```bash
   docker compose up --build
   ```
3. The application will be available at `http://localhost:8080`

## Environment Variables
| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP server port | `8080` | No |
| `DB_HOST` | PostgreSQL host | `localhost` | No |
| `DB_PORT` | PostgreSQL port | `5432` | No |
| `DB_USER` | PostgreSQL username | `postgres` | No |
| `DB_PASS` | PostgreSQL password | `postgres` | No |
| `DB_NAME` | PostgreSQL database name | `newsdb` | No |
| `REDIS_ADDR` | Redis address (host:port) | `localhost:6379` | No |
| `NEWS_API_URL` | NewsAPI endpoint URL | `https://newsapi.org/v2/everything?q=European%20football&language=en&sortBy=publishedAt` | No |
| `NEWS_API_KEY` | NewsAPI API key | *(empty)* | Yes for news features |
| `SPORTMONKS_BASE_URL` | Sportmonks API base URL | `https://api.sportmonks.com/v3` | No |
| `SPORTMONKS_API_TOKEN` | Sportmonks API token | *(empty)* | Yes for football features |
| `REFRESH_TOKEN` | Token for protecting `/news/refresh` endpoint | *(empty)* | No (if empty, endpoint is open) |
| `SCRAPE_CRON_ENABLED` | Enable background scraper scheduler | `true` | No |
| `SCRAPE_CRON_INTERVAL` | Interval between scrape runs (e.g., `30m`, `1h`) | `30m` | No |

All variables are loaded via `os.Getenv` with fallbacks defined in `config.Load()`.

## Docker Setup
### Container Architecture
The project uses a multi-stage Docker build (implicit in Dockerfile):
1. **Build Stage**: Uses Golang image to compile the application
2. **Runtime Stage**: Uses a minimal base image (Alpine or similar) to run the compiled binary

### Docker Compose Services
- **app**: Main Go application
  - Depends on: db, redis
  - Exposes: Port 8080 → host
  - Environment: Loads from `.env` file
  - Volumes: None (binary baked into image)
- **db**: PostgreSQL 15
  - Ports: 5432 → host
  - Environment: POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
  - Volumes: Persistent data storage
- **redis**: Redis 7-alpine
  - Ports: 6379 → host
  - Volumes: Persistent cache storage

### Example docker-compose.yml
```yaml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=db
      - DB_PORT=5432
      - DB_USER=postgres
      - DB_PASS=postgres
      - DB_NAME=apigo
      - REDIS_ADDR=redis:6379
      - NEWS_API_URL=https://newsapi.org/v2/everything?q=European%20football&language=en&sortBy=publishedAt
      - NEWS_API_KEY=${NEWS_API_KEY}
      - SPORTMONKS_BASE_URL=https://api.sportmonks.com/v3
      - SPORTMONKS_API_TOKEN=${SPORTMONKS_API_TOKEN}
      - REFRESH_TOKEN=${REFRESH_TOKEN}
      - SCRAPE_CRON_ENABLED=true
      - SCRAPE_CRON_INTERVAL=30m
    depends_on:
      - db
      - redis
  db:
    image: postgres:15
    restart: unless-stopped
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=apigo
    volumes:
      - pgdata:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    restart: unless-stopped
    volumes:
      - redisdata:/data
volumes:
  pgdata:
  redisdata:
```

### Dockerfile (if not present, implied structure)
```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main .

# Runtime stage
FROM alpine:3.18
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
```

## Notes
- The auto-scrape scheduler respects TTL settings per table to minimize API calls while keeping data fresh.
- Manual scrape endpoints return `202 Accepted` when a job starts, `409 Conflict` if already running.
- The `/sportmonks/*` proxy enables access to any Sportmonks endpoint not explicitly wrapped.
- Admin control over league scraping status is preserved via the `status` flag on the leagues table (not overwritten during re-scrapes).
- Comprehensive logging is provided for scrape jobs, including panic recovery and retry attempts.