package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SportmonksClient handles HTTP requests to the Sportmonks Football API v3.
type SportmonksClient struct {
	BaseURL    string
	APIToken   string
	HTTPClient *http.Client
}

// SportmonksResponse represents the standard JSON response envelope returned by Sportmonks v3 API.
type SportmonksResponse struct {
	Data         json.RawMessage `json:"data,omitempty"`
	Pagination   json.RawMessage `json:"pagination,omitempty"`
	Subscription json.RawMessage `json:"subscription,omitempty"`
	RateLimit    json.RawMessage `json:"rate_limit,omitempty"`
	Timezone     string          `json:"timezone,omitempty"`
	Message      string          `json:"message,omitempty"`
}

// NewSportmonksClient initializes a new SportmonksClient instance.
func NewSportmonksClient(baseURL, apiToken string) *SportmonksClient {
	if baseURL == "" {
		baseURL = "https://api.sportmonks.com/v3"
	}
	return &SportmonksClient{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIToken:   apiToken,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Get sends a GET request to any Sportmonks API v3 endpoint with optional query parameters.
func (c *SportmonksClient) Get(endpoint string, queryParams map[string]string) ([]byte, error) {
	endpoint = strings.TrimPrefix(endpoint, "/")

	// Replace placeholder variables like :version or :sport if passed in endpoint string
	endpoint = strings.ReplaceAll(endpoint, ":version", "v3")
	endpoint = strings.ReplaceAll(endpoint, ":sport", "football")

	reqURL, err := url.Parse(fmt.Sprintf("%s/%s", c.BaseURL, endpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	q := reqURL.Query()
	if c.APIToken != "" {
		q.Set("api_token", c.APIToken)
	}
	for k, v := range queryParams {
		if v != "" {
			q.Set(k, v)
		}
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, fmt.Errorf("sportmonks api error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// helper for include param
func buildParams(includes []string, extra ...map[string]string) map[string]string {
	params := map[string]string{}
	if len(includes) > 0 {
		params["include"] = strings.Join(includes, ";")
	}
	for _, m := range extra {
		for k, v := range m {
			params[k] = v
		}
	}
	return params
}

// ==========================================
// 1. CORE ENDPOINTS
// ==========================================

// Continents
func (c *SportmonksClient) GetContinents(includes ...string) ([]byte, error) {
	return c.Get("core/continents", buildParams(includes))
}

func (c *SportmonksClient) GetContinentByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/continents/%s", id), buildParams(includes))
}

// Countries
func (c *SportmonksClient) GetCountries(includes ...string) ([]byte, error) {
	return c.Get("core/countries", buildParams(includes))
}

func (c *SportmonksClient) GetCountryByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/countries/%s", id), buildParams(includes))
}

func (c *SportmonksClient) SearchCountries(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/countries/search/%s", url.PathEscape(name)), buildParams(includes))
}

// Regions
func (c *SportmonksClient) GetRegions(includes ...string) ([]byte, error) {
	return c.Get("core/regions", buildParams(includes))
}

func (c *SportmonksClient) GetRegionByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/regions/%s", id), buildParams(includes))
}

func (c *SportmonksClient) SearchRegions(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/regions/search/%s", url.PathEscape(name)), buildParams(includes))
}

// Cities
func (c *SportmonksClient) GetCities(includes ...string) ([]byte, error) {
	return c.Get("core/cities", buildParams(includes))
}

func (c *SportmonksClient) GetCityByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/cities/%s", id), buildParams(includes))
}

func (c *SportmonksClient) SearchCities(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/cities/search/%s", url.PathEscape(name)), buildParams(includes))
}

// Types & Filters & Timezones
func (c *SportmonksClient) GetTypes(includes ...string) ([]byte, error) {
	return c.Get("core/types", buildParams(includes))
}

func (c *SportmonksClient) GetTypeByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("core/types/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetEntityFilters(includes ...string) ([]byte, error) {
	return c.Get("my/filters/entity", buildParams(includes))
}

func (c *SportmonksClient) GetTimezones(includes ...string) ([]byte, error) {
	return c.Get("core/timezones", buildParams(includes))
}

// ==========================================
// 2. FOOTBALL ENDPOINTS
// ==========================================

// Livescores
func (c *SportmonksClient) GetLivescores(includes ...string) ([]byte, error) {
	return c.Get("football/livescores", buildParams(includes))
}

func (c *SportmonksClient) GetInplayLivescores(includes ...string) ([]byte, error) {
	return c.Get("football/livescores/inplay", buildParams(includes))
}

func (c *SportmonksClient) GetLatestLivescores(includes ...string) ([]byte, error) {
	return c.Get("football/livescores/latest", buildParams(includes))
}

// Fixtures
func (c *SportmonksClient) GetFixtures(includes ...string) ([]byte, error) {
	return c.Get("football/fixtures", buildParams(includes))
}

func (c *SportmonksClient) GetFixtureByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetFixturesByMultipleIDs(ids string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/multi/%s", ids), buildParams(includes))
}

func (c *SportmonksClient) GetFixturesByDate(date string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/date/%s", date), buildParams(includes))
}

func (c *SportmonksClient) GetFixturesBetweenDates(startDate, endDate string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/between/%s/%s", startDate, endDate), buildParams(includes))
}

func (c *SportmonksClient) GetFixturesHeadToHead(team1, team2 string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/head-to-head/%s/%s", team1, team2), buildParams(includes))
}

func (c *SportmonksClient) SearchFixtures(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/fixtures/search/%s", url.PathEscape(name)), buildParams(includes))
}

// Leagues
func (c *SportmonksClient) GetLeagues(includes ...string) ([]byte, error) {
	return c.Get("football/leagues", buildParams(includes))
}

func (c *SportmonksClient) GetLeagueByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/leagues/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetLiveLeagues(includes ...string) ([]byte, error) {
	return c.Get("football/leagues/live", buildParams(includes))
}

func (c *SportmonksClient) SearchLeagues(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/leagues/search/%s", url.PathEscape(name)), buildParams(includes))
}

// Seasons & Rounds & Stages
func (c *SportmonksClient) GetSeasons(includes ...string) ([]byte, error) {
	return c.Get("football/seasons", buildParams(includes))
}

func (c *SportmonksClient) GetSeasonByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/seasons/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetRounds(includes ...string) ([]byte, error) {
	return c.Get("football/rounds", buildParams(includes))
}

func (c *SportmonksClient) GetRoundByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/rounds/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetStages(includes ...string) ([]byte, error) {
	return c.Get("football/stages", buildParams(includes))
}

func (c *SportmonksClient) GetStageByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/stages/%s", id), buildParams(includes))
}

// Teams & Squads
func (c *SportmonksClient) GetTeams(includes ...string) ([]byte, error) {
	return c.Get("football/teams", buildParams(includes))
}

func (c *SportmonksClient) GetTeamByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/teams/%s", id), buildParams(includes))
}

func (c *SportmonksClient) SearchTeams(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/teams/search/%s", url.PathEscape(name)), buildParams(includes))
}

func (c *SportmonksClient) GetTeamSquad(teamID string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/squads/teams/%s", teamID), buildParams(includes))
}

// Players, Coaches & Referees
func (c *SportmonksClient) GetPlayers(includes ...string) ([]byte, error) {
	return c.Get("football/players", buildParams(includes))
}

func (c *SportmonksClient) GetPlayerByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/players/%s", id), buildParams(includes))
}

func (c *SportmonksClient) SearchPlayers(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/players/search/%s", url.PathEscape(name)), buildParams(includes))
}

func (c *SportmonksClient) GetCoaches(includes ...string) ([]byte, error) {
	return c.Get("football/coaches", buildParams(includes))
}

func (c *SportmonksClient) GetCoachByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/coaches/%s", id), buildParams(includes))
}

func (c *SportmonksClient) GetReferees(includes ...string) ([]byte, error) {
	return c.Get("football/referees", buildParams(includes))
}

func (c *SportmonksClient) GetRefereeByID(id string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/referees/%s", id), buildParams(includes))
}

// Standings & Topscorers
func (c *SportmonksClient) GetStandings(includes ...string) ([]byte, error) {
	return c.Get("football/standings", buildParams(includes))
}

func (c *SportmonksClient) GetStandingsBySeason(seasonID string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/standings/seasons/%s", seasonID), buildParams(includes))
}

func (c *SportmonksClient) GetTopscorersBySeason(seasonID string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/topscorers/seasons/%s", seasonID), buildParams(includes))
}

// Transfers & Venues & News & Predictions
func (c *SportmonksClient) GetTransfers(includes ...string) ([]byte, error) {
	return c.Get("football/transfers", buildParams(includes))
}

func (c *SportmonksClient) GetVenues(includes ...string) ([]byte, error) {
	return c.Get("football/venues", buildParams(includes))
}

func (c *SportmonksClient) SearchVenues(name string, includes ...string) ([]byte, error) {
	return c.Get(fmt.Sprintf("football/venues/search/%s", url.PathEscape(name)), buildParams(includes))
}

func (c *SportmonksClient) GetNews(includes ...string) ([]byte, error) {
	return c.Get("football/news/pre-match", buildParams(includes))
}

func (c *SportmonksClient) GetPredictions(includes ...string) ([]byte, error) {
	return c.Get("football/predictions/probabilities", buildParams(includes))
}

// ==========================================
// 3. ODDS ENDPOINTS
// ==========================================

func (c *SportmonksClient) GetBookmakers(includes ...string) ([]byte, error) {
	return c.Get("odds/bookmakers", buildParams(includes))
}

func (c *SportmonksClient) GetMarkets(includes ...string) ([]byte, error) {
	return c.Get("odds/markets", buildParams(includes))
}

func (c *SportmonksClient) GetPreMatchOdds(includes ...string) ([]byte, error) {
	return c.Get("football/odds/pre-match", buildParams(includes))
}

func (c *SportmonksClient) GetInPlayOdds(includes ...string) ([]byte, error) {
	return c.Get("football/odds/inplay", buildParams(includes))
}

// ==========================================
// 4. MY SPORTMONKS
// ==========================================

func (c *SportmonksClient) GetMyUsage(includes ...string) ([]byte, error) {
	return c.Get("my/usage", buildParams(includes))
}

func (c *SportmonksClient) GetMyLeagues(includes ...string) ([]byte, error) {
	return c.Get("my/leagues", buildParams(includes))
}
