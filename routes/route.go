package routes

import (
	"net/http"

	"github.com/farhanarfianto/apigo-docker/handlers"
	"github.com/labstack/echo/v4"
)

// SetupRoutes registers all API endpoints for the Echo server.
func SetupRoutes(e *echo.Echo, h *handlers.NewsHandler, smh *handlers.SportmonksHandler) {
	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, echo.Map{"status": "ok bang"})
	})

	// News endpoints group
	news := e.Group("/news")
	news.GET("", h.GetNews)
	news.GET("/:id", h.GetNewsByID)
	news.POST("/refresh", h.RefreshNews)

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

		// Scraper endpoint for Core entities
		sm.POST("/scrape/core", smh.ScrapeCoreData)

		// Dynamic proxy route covering ALL 168+ Sportmonks v3 endpoints
		// e.g. GET /sportmonks/football/transfers/latest
		sm.GET("/*", smh.Proxy)
	}
}
