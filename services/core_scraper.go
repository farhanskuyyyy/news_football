package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaginationInfo struct {
	Count       int     `json:"count"`
	PerPage     int     `json:"per_page"`
	CurrentPage int     `json:"current_page"`
	NextPage    *string `json:"next_page"`
	NextCursor  *string `json:"next_cursor"`
	HasMore     bool    `json:"has_more"`
}

type CoreResponseEnvelope[T any] struct {
	Data       []T             `json:"data"`
	Pagination *PaginationInfo `json:"pagination"`
}

type CoreScrapeResult struct {
	Continents int `json:"continents"`
	Countries  int `json:"countries"`
	Regions    int `json:"regions"`
	Cities     int `json:"cities"`
	Types      int `json:"types"`
}

// TTLCore is the sync interval for core reference data (rarely changes).
const TTLCore = 30 * 24 * time.Hour

// shouldSyncDB reports whether a table needs syncing based on its sync_tables
// record: skip only if it was synced successfully within the TTL.
func shouldSyncDB(db *gorm.DB, tableName string, defaultTTL time.Duration, force bool) bool {
	if force {
		return true
	}
	var rec models.SyncTable
	if err := db.First(&rec, "table_name = ?", tableName).Error; err != nil {
		return true // never synced
	}
	ttl := defaultTTL
	if rec.IntervalSeconds > 0 {
		ttl = time.Duration(rec.IntervalSeconds) * time.Second
	}
	if time.Since(rec.LatestSyncedAt) < ttl && rec.Status == "success" {
		log.Printf("[SyncTracker] SKIPPING '%s': last synced %v ago (TTL %v)", tableName, time.Since(rec.LatestSyncedAt).Round(time.Second), ttl)
		return false
	}
	return true
}

// markSyncedDB records the outcome of a sync step. On error, status is "failed"
// so the next run retries this table while leaving already-succeeded tables
// (marked "success") to be skipped within their TTL.
func markSyncedDB(db *gorm.DB, tableName string, count int, defaultTTL time.Duration, syncErr error) {
	status := "success"
	errMsg := ""
	if syncErr != nil {
		status = "failed"
		errMsg = syncErr.Error()
	}
	rec := models.SyncTable{
		TableName:       tableName,
		LatestSyncedAt:  time.Now(),
		IntervalSeconds: int(defaultTTL.Seconds()),
		RecordsSynced:   count,
		Status:          status,
		ErrorMessage:    errMsg,
		UpdatedAt:       time.Now(),
	}
	_ = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "table_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"latest_synced_at", "interval_seconds", "records_synced", "status", "error_message", "updated_at"}),
	}).Create(&rec)
}

type CoreScraper struct {
	DB     *gorm.DB
	Client *SportmonksClient
}

func NewCoreScraper(db *gorm.DB, client *SportmonksClient) *CoreScraper {
	return &CoreScraper{
		DB:     db,
		Client: client,
	}
}

// ScrapeAll fetches and saves all data for Continents, Countries, Regions, Cities, and Types from Sportmonks v3 API.
// ScrapeAll scrapes core reference entities, each tracked in sync_tables on its
// OWN completion. A successful entity is marked "success" (skipped within TTL on
// the next run); a failing entity is marked "failed" and retried next run,
// without forcing the already-succeeded entities to re-run. Pass force=true to
// bypass the TTL for every entity.
func (s *CoreScraper) ScrapeAll(ctx context.Context, force ...bool) (*CoreScrapeResult, error) {
	isForce := len(force) > 0 && force[0]
	result := &CoreScrapeResult{}

	steps := []struct {
		name string
		fn   func(context.Context) (int, error)
		set  func(int)
	}{
		{"continents", s.ScrapeContinents, func(n int) { result.Continents = n }},
		{"countries", s.ScrapeCountries, func(n int) { result.Countries = n }},
		{"regions", s.ScrapeRegions, func(n int) { result.Regions = n }},
		{"cities", s.ScrapeCities, func(n int) { result.Cities = n }},
		{"types", s.ScrapeTypes, func(n int) { result.Types = n }},
	}

	for _, st := range steps {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if !shouldSyncDB(s.DB, st.name, TTLCore, isForce) {
			continue
		}
		n, err := st.fn(ctx)
		markSyncedDB(s.DB, st.name, n, TTLCore, err)
		if err != nil {
			log.Printf("[CoreScraper] Error scraping %s: %v", st.name, err)
		} else {
			st.set(n)
		}
	}

	return result, nil
}

// scrapeCoreEntity is a generic helper to paginate any Core entity endpoint with cursor support.
func scrapeCoreEntity[T any](ctx context.Context, client *SportmonksClient, db *gorm.DB, endpoint string) (int, error) {
	var total int
	page := 1
	var cursor string
	iterations := 0

	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		iterations++
		if iterations > 20000 {
			log.Printf("[CoreScraper] %s: aborting after %d iterations (possible pagination loop)", endpoint, iterations)
			return total, fmt.Errorf("%s: pagination exceeded %d iterations", endpoint, iterations)
		}

		params := make(map[string]string)

		if cursor != "" {
			// When cursor is used, Sportmonks strictly forbids "per_page" and "page"
			params["cursor"] = cursor
		} else {
			params["page"] = strconv.Itoa(page)
			params["per_page"] = strconv.Itoa(perPage)
		}

		raw, err := client.Get(endpoint, params)
		if err != nil {
			return total, fmt.Errorf("failed to fetch %s (page %d, cursor %s): %w", endpoint, page, cursor, err)
		}

		var envelope CoreResponseEnvelope[T]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse %s json: %w", endpoint, err)
		}

		if len(envelope.Data) > 0 {
			if err := db.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&envelope.Data, dbBatchSize).Error; err != nil {
				return total, fmt.Errorf("failed to save %s (page %d): %w", endpoint, page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}

		nextCursor := ""
		if envelope.Pagination.NextCursor != nil && *envelope.Pagination.NextCursor != "" {
			nextCursor = extractCursor(*envelope.Pagination.NextCursor)
		} else if envelope.Pagination.NextPage != nil && *envelope.Pagination.NextPage != "" {
			nextCursor = extractCursor(*envelope.Pagination.NextPage)
		}

		if nextCursor != "" {
			cursor = nextCursor
		} else {
			page++
		}

		// Rate limit: ~3 req/sec
		time.Sleep(350 * time.Millisecond)
	}

	log.Printf("[CoreScraper] %s: fetched %d records", endpoint, total)
	return total, nil
}

// ScrapeContinents fetches all pages of continents and upserts into DB.
func (s *CoreScraper) ScrapeContinents(ctx context.Context) (int, error) {
	return scrapeCoreEntity[models.Continent](ctx, s.Client, s.DB, "core/continents")
}

// ScrapeCountries fetches all pages of countries and upserts into DB.
func (s *CoreScraper) ScrapeCountries(ctx context.Context) (int, error) {
	return scrapeCoreEntity[models.Country](ctx, s.Client, s.DB, "core/countries")
}

// ScrapeRegions fetches all pages of regions and upserts into DB.
func (s *CoreScraper) ScrapeRegions(ctx context.Context) (int, error) {
	return scrapeCoreEntity[models.Region](ctx, s.Client, s.DB, "core/regions")
}

// ScrapeCities fetches all pages of cities and upserts into DB.
func (s *CoreScraper) ScrapeCities(ctx context.Context) (int, error) {
	return scrapeCoreEntity[models.City](ctx, s.Client, s.DB, "core/cities")
}

// ScrapeTypes fetches all pages of types and upserts into DB.
func (s *CoreScraper) ScrapeTypes(ctx context.Context) (int, error) {
	return scrapeCoreEntity[models.Type](ctx, s.Client, s.DB, "core/types")
}
