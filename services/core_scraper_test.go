package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/farhanarfianto/apigo-docker/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCoreScraper(t *testing.T) {
	// Setup in-memory SQLite DB for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Continent{},
		&models.Country{},
		&models.Region{},
		&models.City{},
		&models.Type{},
	); err != nil {
		t.Fatalf("failed to automigrate models: %v", err)
	}

	// Mock Sportmonks server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/core/continents":
			fmt.Fprintln(w, `{"data":[{"id":1,"name":"Europe","code":"EU"}],"pagination":{"has_more":false}}`)
		case "/v3/core/countries":
			fmt.Fprintln(w, `{"data":[{"id":2,"continent_id":1,"name":"Poland","official_name":"Republic of Poland","borders":["CZE","DEU"]}],"pagination":{"has_more":false}}`)
		case "/v3/core/regions":
			fmt.Fprintln(w, `{"data":[{"id":10,"country_id":2,"name":"Region A"}],"pagination":{"has_more":false}}`)
		case "/v3/core/cities":
			fmt.Fprintln(w, `{"data":[{"id":100,"country_id":2,"region_id":10,"name":"City A"}],"pagination":{"has_more":false}}`)
		case "/v3/core/types":
			fmt.Fprintln(w, `{"data":[{"id":1,"name":"1st Half","code":"1st-half","developer_name":"1ST_HALF","model_type":"period"}],"pagination":{"has_more":false}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	smClient := NewSportmonksClient(server.URL+"/v3", "test-token")
	scraper := NewCoreScraper(db, smClient)

	result, err := scraper.ScrapeAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error scraping all: %v", err)
	}

	if result.Continents != 1 || result.Countries != 1 || result.Regions != 1 || result.Cities != 1 || result.Types != 1 {
		t.Errorf("unexpected scrape result count: %+v", result)
	}

	var count int64
	db.Model(&models.Continent{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 continent in DB, got %d", count)
	}

	var country models.Country
	if err := db.First(&country, 2).Error; err != nil {
		t.Fatalf("failed to find country 2: %v", err)
	}
	if country.Name != "Poland" || len(country.Borders) != 2 {
		t.Errorf("unexpected country data: %+v", country)
	}
}
