package main

import (
	"log"

	"github.com/farhanarfianto/apigo-docker/config"
	"github.com/farhanarfianto/apigo-docker/database"
	"github.com/farhanarfianto/apigo-docker/handlers"
	"github.com/farhanarfianto/apigo-docker/routes"
	"github.com/farhanarfianto/apigo-docker/services"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()

	db, err := database.ConnectPostgres(cfg)
	if err != nil {
		log.Fatalf("failed to connect postgres: %v", err)
	}

	rdb := database.ConnectRedis(cfg)
	client := services.NewNewsAPIClient(cfg.NewsAPIURL, cfg.NewsAPIKey)
	smClient := services.NewSportmonksClient(cfg.SportmonksBaseURL, cfg.SportmonksAPIToken)
	coreScraper := services.NewCoreScraper(db, smClient)

	h := handlers.NewNewsHandler(db, rdb, client, cfg.RefreshToken)
	smh := handlers.NewSportmonksHandler(smClient, coreScraper)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Register routes
	routes.SetupRoutes(e, h, smh)

	e.Logger.Fatal(e.Start(":" + cfg.Port))
}
