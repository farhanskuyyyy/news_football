package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/farhanarfianto/apigo-docker/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFootballScraper(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}

	if err := db.AutoMigrate(
		// Football main entities
		&models.League{},
		&models.Season{},
		&models.Stage{},
		&models.Round{},
		&models.Team{},
		&models.Player{},
		&models.Squad{},
		&models.Fixture{},
		&models.Venue{},
		&models.Coach{},
		&models.Referee{},
		&models.Standing{},
		&models.Topscorer{},
		&models.Transfer{},
		// Fixture sub-entities
		&models.FixtureEvent{},
		&models.FixtureLineup{},
		&models.FixtureStatistic{},
		&models.FixtureScore{},
		&models.Commentary{},
		&models.State{},
		// Pivot / Join tables
		&models.SeasonTeam{},
		&models.PlayerSeason{},
		&models.PlayerStatistic{},
		&models.TeamRival{},
		&models.FixtureReferee{},
	); err != nil {
		t.Fatalf("failed to automigrate football models: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/football/leagues":
			fmt.Fprintln(w, `{"data":[{"id":271,"sport_id":1,"name":"Superliga","active":true}],"pagination":{"has_more":false}}`)
		case "/v3/football/seasons":
			fmt.Fprintln(w, `{"data":[{"id":759,"sport_id":1,"league_id":271,"name":"2016/2017"},{"id":999,"sport_id":1,"league_id":8888,"name":"Inactive Season"}],"pagination":{"has_more":false}}`)
		case "/v3/football/stages":
			fmt.Fprintln(w, `{"data":[{"id":1086,"sport_id":1,"league_id":271,"season_id":759,"name":"Regular Season"}],"pagination":{"has_more":false}}`)
		case "/v3/football/rounds":
			fmt.Fprintln(w, `{"data":[{"id":23317,"sport_id":1,"league_id":271,"season_id":759,"stage_id":1086,"name":"1"}],"pagination":{"has_more":false}}`)
		case "/v3/football/teams/seasons/759":
			fmt.Fprintln(w, `{"data":[{"id":53,"sport_id":1,"name":"Celtic"}],"pagination":{"has_more":false}}`)
		case "/v3/football/players":
			fmt.Fprintln(w, `{"data":[{"id":14,"sport_id":1,"name":"Daniel Agger"}],"pagination":{"has_more":false}}`)
		case "/v3/football/fixtures":
			fmt.Fprintln(w, `{"data":[{"id":216268,"sport_id":1,"league_id":271,"season_id":759,"name":"Esbjerg vs OB","scores":[{"id":1,"fixture_id":216268,"type_id":1,"participant_id":53,"score":{"goals":2,"participant":"home"},"description":"CURRENT"}]}],"pagination":{"has_more":false}}`)
		case "/v3/football/venues":
			fmt.Fprintln(w, `{"data":[{"id":73,"name":"The Paisley Stadium"}],"pagination":{"has_more":false}}`)
		case "/v3/football/coaches":
			fmt.Fprintln(w, `{"data":[{"id":50,"sport_id":1,"name":"Steven Gerrard"}],"pagination":{"has_more":false}}`)
		case "/v3/football/referees":
			fmt.Fprintln(w, `{"data":[{"id":37,"sport_id":1,"name":"Craig Thomson"}],"pagination":{"has_more":false}}`)
		case "/v3/football/standings":
			fmt.Fprintln(w, `{"data":[{"id":2621,"participant_id":53,"sport_id":1,"league_id":271,"season_id":759,"points":53}],"pagination":{"has_more":false}}`)
		case "/v3/football/topscorers/seasons/759":
			fmt.Fprintln(w, `{"data":[{"id":1,"stage_id":1086,"player_id":14,"type_id":83,"position":1,"total":10,"participant_type":"team","participant_id":53}],"pagination":{"has_more":false}}`)
		case "/v3/football/squads/seasons/759/teams/53":
			fmt.Fprintln(w, `{"data":[{"id":100,"player_id":14,"team_id":53,"season_id":759,"captain":true,"player":{"id":14,"sport_id":1,"name":"Daniel Agger"}}],"pagination":{"has_more":false}}`)
		case "/v3/football/squads/teams/53/extended":
			fmt.Fprintln(w, `{"data":[{"id":14,"sport_id":1,"name":"Daniel Agger"}],"pagination":{"has_more":false}}`)
		case "/v3/football/squads/teams/53":
			fmt.Fprintln(w, `{"data":[{"id":100,"player_id":14,"team_id":53,"season_id":759,"captain":true}],"pagination":{"has_more":false}}`)
		case "/v3/football/rivals":
			fmt.Fprintln(w, `{"data":[{"id":1,"sport_id":1,"team_id":53,"rival_id":54}],"pagination":{"has_more":false}}`)
		case "/v3/football/transfers":
			fmt.Fprintln(w, `{"data":[{"id":1,"sport_id":1,"player_id":14,"to_team_id":53,"completed":true}],"pagination":{"has_more":false}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	smClient := NewSportmonksClient(server.URL+"/v3", "test-token")
	scraper := NewFootballScraper(db, smClient)

	// Step 1: Scrape Leagues
	leaguesCount, err := scraper.ScrapeLeagues()
	if err != nil {
		t.Fatalf("unexpected error scraping leagues: %v", err)
	}
	if leaguesCount != 1 {
		t.Fatalf("expected 1 league, got %d", leaguesCount)
	}

	// Step 2: Scrape Football for Active Leagues (where status = true AND active = true)
	result, err := scraper.ScrapeAllFootball()
	if err != nil {
		t.Fatalf("unexpected error scraping football: %v", err)
	}

	if result.ActiveLeaguesCount != 1 || result.Seasons != 1 || result.Teams != 1 || result.Players != 1 || result.Fixtures != 1 {
		t.Errorf("unexpected scrape result count: %+v", result)
	}

	var team models.Team
	if err := db.First(&team, 53).Error; err != nil {
		t.Fatalf("failed to find team 53: %v", err)
	}
	if team.Name != "Celtic" {
		t.Errorf("unexpected team name: %s", team.Name)
	}
}
