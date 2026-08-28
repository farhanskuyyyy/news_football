package services

import (
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
func (s *CoreScraper) ScrapeAll() (*CoreScrapeResult, error) {
	result := &CoreScrapeResult{}

	continents, err := s.ScrapeContinents()
	if err != nil {
		log.Printf("[CoreScraper] Error scraping continents: %v", err)
	} else {
		result.Continents = continents
	}

	countries, err := s.ScrapeCountries()
	if err != nil {
		log.Printf("[CoreScraper] Error scraping countries: %v", err)
	} else {
		result.Countries = countries
	}

	regions, err := s.ScrapeRegions()
	if err != nil {
		log.Printf("[CoreScraper] Error scraping regions: %v", err)
	} else {
		result.Regions = regions
	}

	cities, err := s.ScrapeCities()
	if err != nil {
		log.Printf("[CoreScraper] Error scraping cities: %v", err)
	} else {
		result.Cities = cities
	}

	types, err := s.ScrapeTypes()
	if err != nil {
		log.Printf("[CoreScraper] Error scraping types: %v", err)
	} else {
		result.Types = types
	}

	return result, nil
}

// scrapeCoreEntity is a generic helper to paginate any Core entity endpoint with cursor support.
func scrapeCoreEntity[T any](client *SportmonksClient, db *gorm.DB, endpoint string) (int, error) {
	var total int
	page := 1
	var cursor string

	for {
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
func (s *CoreScraper) ScrapeContinents() (int, error) {
	return scrapeCoreEntity[models.Continent](s.Client, s.DB, "core/continents")
}

// ScrapeCountries fetches all pages of countries and upserts into DB.
func (s *CoreScraper) ScrapeCountries() (int, error) {
	return scrapeCoreEntity[models.Country](s.Client, s.DB, "core/countries")
}

// ScrapeRegions fetches all pages of regions and upserts into DB.
func (s *CoreScraper) ScrapeRegions() (int, error) {
	return scrapeCoreEntity[models.Region](s.Client, s.DB, "core/regions")
}

// ScrapeCities fetches all pages of cities and upserts into DB.
func (s *CoreScraper) ScrapeCities() (int, error) {
	return scrapeCoreEntity[models.City](s.Client, s.DB, "core/cities")
}

// ScrapeTypes fetches all pages of types and upserts into DB.
func (s *CoreScraper) ScrapeTypes() (int, error) {
	return scrapeCoreEntity[models.Type](s.Client, s.DB, "core/types")
}
