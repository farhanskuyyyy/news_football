package routes

import (
	"net/http"

	"github.com/farhanarfianto/apigo-docker/handlers"
	"github.com/labstack/echo/v4"
)

// SetupRoutes registers all API endpoints for the Echo server.
func SetupRoutes(e *echo.Echo, h *handlers.NewsHandler, smh *handlers.SportmonksHandler, ph *handlers.PortalHandler) {
	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, echo.Map{"status": "ok bang"})
	})

	// News endpoints group
	news := e.Group("/news")
	news.GET("", h.GetNews)
	news.GET("/:id", h.GetNewsByID)
	news.POST("/refresh", h.RefreshNews)

	// Portal endpoints group (Database-backed Football Portal API for Frontend)
	if ph != nil {
		portal := e.Group("/portal")
		portal.GET("/leagues", ph.GetLeagues)
		portal.GET("/leagues/:league_id/seasons", ph.GetLeagueSeasons)
		portal.GET("/seasons/:season_id/overview", ph.GetSeasonOverview)
		portal.GET("/seasons/:season_id/standings", ph.GetSeasonStandings)
		portal.GET("/seasons/:season_id/rounds", ph.GetSeasonRounds)
		portal.GET("/seasons/:season_id/fixtures", ph.GetSeasonFixtures)
		portal.GET("/seasons/:season_id/teams", ph.GetSeasonTeams)
		portal.GET("/seasons/:season_id/topscorers", ph.GetSeasonTopscorers)
		portal.GET("/seasons/:season_id/transfers", ph.GetSeasonTransfers)
		portal.GET("/fixtures/:id", ph.GetFixtureDetail)
		portal.GET("/teams/:id", ph.GetTeamDetail)
		portal.GET("/players/:id", ph.GetPlayerDetail)
	}

	// Sportmonks endpoints group
	if smh != nil {
		sm := e.Group("/sportmonks")

		// Dedicated helper routes
		sm.GET("/continents", smh.GetContinents)
		sm.GET("/countries", smh.GetCountries)
		sm.GET("/livescores", smh.GetLivescores)
		sm.GET("/livescores/inplay", smh.GetInplayLivescores)
		sm.GET("/fixtures", smh.GetFixtures)
		sm.GET("/fixtures/:id", smh.GetFixtureByID)
		sm.GET("/leagues", smh.GetLeagues)
		sm.GET("/leagues/:id", smh.GetLeagueByID)
		sm.GET("/teams", smh.GetTeams)
		sm.GET("/teams/:id", smh.GetTeamByID)
		sm.GET("/teams/search/:name", smh.SearchTeams)
		sm.GET("/standings", smh.GetStandings)
		sm.GET("/odds/bookmakers", smh.GetBookmakers)
		sm.GET("/odds/markets", smh.GetMarkets)
		sm.GET("/my/usage", smh.GetMyUsage)

		// Scraper endpoints for Core, Leagues & Football entities
		sm.POST("/scrape/core", smh.ScrapeCoreData)
		sm.POST("/scrape/leagues", smh.ScrapeLeaguesData)
		sm.POST("/scrape/football", smh.ScrapeFootballData)
		sm.POST("/scrape/fixture-details", smh.ScrapeFixtureDetailsData)
		sm.POST("/scrape/player-statistics", smh.ScrapePlayerStatisticsData)
		sm.GET("/sync/status", smh.GetSyncStatus)
		sm.POST("/sync/seed", smh.SeedSyncTablesHandler)

		// Dynamic proxy route covering ALL 168+ Sportmonks v3 endpoints
		// e.g. GET /sportmonks/football/transfers/latest
		sm.GET("/*", smh.Proxy)
	}
}
