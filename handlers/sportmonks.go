package handlers

import (
	"net/http"

	"github.com/farhanarfianto/apigo-docker/services"
	"github.com/labstack/echo/v4"
)

type SportmonksHandler struct {
	Client  *services.SportmonksClient
	Scraper *services.CoreScraper
}

func NewSportmonksHandler(client *services.SportmonksClient, scraper *services.CoreScraper) *SportmonksHandler {
	return &SportmonksHandler{
		Client:  client,
		Scraper: scraper,
	}
}

// ScrapeCoreData triggers fetching and saving all Core entities (Continents, Countries, Regions, Cities, Types) into DB.
func (h *SportmonksHandler) ScrapeCoreData(c echo.Context) error {
	if h.Scraper == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "scraper is not initialized")
	}

	result, err := h.Scraper.ScrapeAll()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status":  "success",
		"scraped": result,
	})
}

// Proxy accepts any route under /sportmonks/* and proxies it directly to Sportmonks v3 API.
// e.g. GET /sportmonks/football/fixtures/between/2026-01-01/2026-01-31?include=participants
func (h *SportmonksHandler) Proxy(c echo.Context) error {
	endpoint := c.Param("*")
	if endpoint == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "endpoint path is required")
	}

	queryParams := make(map[string]string)
	for k, v := range c.QueryParams() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	body, err := h.Client.Get(endpoint, queryParams)
	if err != nil && len(body) == 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.Blob(http.StatusOK, "application/json", body)
}

func (h *SportmonksHandler) GetContinents(c echo.Context) error {
	body, err := h.Client.GetContinents(extractInclude(c)...)
	return respond(c, body, err)
}

func (c *SportmonksHandler) GetCountries(ctx echo.Context) error {
	body, err := c.Client.GetCountries(extractInclude(ctx)...)
	return respond(ctx, body, err)
}

func (h *SportmonksHandler) GetLivescores(c echo.Context) error {
	body, err := h.Client.GetLivescores(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetInplayLivescores(c echo.Context) error {
	body, err := h.Client.GetInplayLivescores(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetFixtures(c echo.Context) error {
	body, err := h.Client.GetFixtures(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetFixtureByID(c echo.Context) error {
	id := c.Param("id")
	body, err := h.Client.GetFixtureByID(id, extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetLeagues(c echo.Context) error {
	body, err := h.Client.GetLeagues(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetLeagueByID(c echo.Context) error {
	id := c.Param("id")
	body, err := h.Client.GetLeagueByID(id, extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetTeams(c echo.Context) error {
	body, err := h.Client.GetTeams(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetTeamByID(c echo.Context) error {
	id := c.Param("id")
	body, err := h.Client.GetTeamByID(id, extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) SearchTeams(c echo.Context) error {
	name := c.Param("name")
	body, err := h.Client.SearchTeams(name, extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetStandings(c echo.Context) error {
	body, err := h.Client.GetStandings(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetBookmakers(c echo.Context) error {
	body, err := h.Client.GetBookmakers(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetMarkets(c echo.Context) error {
	body, err := h.Client.GetMarkets(extractInclude(c)...)
	return respond(c, body, err)
}

func (h *SportmonksHandler) GetMyUsage(c echo.Context) error {
	body, err := h.Client.GetMyUsage(extractInclude(c)...)
	return respond(c, body, err)
}

func extractInclude(c echo.Context) []string {
	inc := c.QueryParam("include")
	if inc == "" {
		return nil
	}
	return []string{inc}
}

func respond(c echo.Context, body []byte, err error) error {
	if err != nil && len(body) == 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.Blob(http.StatusOK, "application/json", body)
}
