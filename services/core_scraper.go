package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/farhanarfianto/apigo-docker/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaginationInfo struct {
	Count       int     `json:"count"`
	PerPage     int     `json:"per_page"`
	CurrentPage int     `json:"current_page"`
	NextPage    *string `json:"next_page"`
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

// ScrapeContinents fetches all pages of continents and upserts into Postgres DB.
func (s *CoreScraper) ScrapeContinents() (int, error) {
	var total int
	page := 1

	for {
		raw, err := s.Client.Get("core/continents", map[string]string{"page": strconv.Itoa(page)})
		if err != nil {
			return total, fmt.Errorf("failed to fetch continents page %d: %w", page, err)
		}

		var envelope CoreResponseEnvelope[models.Continent]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse continents json: %w", err)
		}

		if len(envelope.Data) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&envelope.Data).Error; err != nil {
				return total, fmt.Errorf("failed to save continents page %d: %w", page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}
		page++
	}

	return total, nil
}

// ScrapeCountries fetches all pages of countries and upserts into Postgres DB.
func (s *CoreScraper) ScrapeCountries() (int, error) {
	var total int
	page := 1

	for {
		raw, err := s.Client.Get("core/countries", map[string]string{"page": strconv.Itoa(page)})
		if err != nil {
			return total, fmt.Errorf("failed to fetch countries page %d: %w", page, err)
		}

		var envelope CoreResponseEnvelope[models.Country]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse countries json: %w", err)
		}

		if len(envelope.Data) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&envelope.Data).Error; err != nil {
				return total, fmt.Errorf("failed to save countries page %d: %w", page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}
		page++
	}

	return total, nil
}

// ScrapeRegions fetches all pages of regions and upserts into Postgres DB.
func (s *CoreScraper) ScrapeRegions() (int, error) {
	var total int
	page := 1

	for {
		raw, err := s.Client.Get("core/regions", map[string]string{"page": strconv.Itoa(page)})
		if err != nil {
			return total, fmt.Errorf("failed to fetch regions page %d: %w", page, err)
		}

		var envelope CoreResponseEnvelope[models.Region]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse regions json: %w", err)
		}

		if len(envelope.Data) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&envelope.Data).Error; err != nil {
				return total, fmt.Errorf("failed to save regions page %d: %w", page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}
		page++
	}

	return total, nil
}

// ScrapeCities fetches all pages of cities and upserts into Postgres DB.
func (s *CoreScraper) ScrapeCities() (int, error) {
	var total int
	page := 1

	for {
		raw, err := s.Client.Get("core/cities", map[string]string{"page": strconv.Itoa(page)})
		if err != nil {
			return total, fmt.Errorf("failed to fetch cities page %d: %w", page, err)
		}

		var envelope CoreResponseEnvelope[models.City]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse cities json: %w", err)
		}

		if len(envelope.Data) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&envelope.Data).Error; err != nil {
				return total, fmt.Errorf("failed to save cities page %d: %w", page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}
		page++
	}

	return total, nil
}

// ScrapeTypes fetches all pages of types and upserts into Postgres DB.
func (s *CoreScraper) ScrapeTypes() (int, error) {
	var total int
	page := 1

	for {
		raw, err := s.Client.Get("core/types", map[string]string{"page": strconv.Itoa(page)})
		if err != nil {
			return total, fmt.Errorf("failed to fetch types page %d: %w", page, err)
		}

		var envelope CoreResponseEnvelope[models.Type]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse types json: %w", err)
		}

		if len(envelope.Data) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&envelope.Data).Error; err != nil {
				return total, fmt.Errorf("failed to save types page %d: %w", page, err)
			}
			total += len(envelope.Data)
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}
		page++
	}

	return total, nil
}
