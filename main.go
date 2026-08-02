package main

import (
	"log"
	"net/http"

	"github.com/farhanarfianto/apigo-docker/config"
	"github.com/farhanarfianto/apigo-docker/database"
	"github.com/farhanarfianto/apigo-docker/handlers"
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
	h := handlers.NewNewsHandler(db, rdb, client)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
	})
	e.GET("/news", h.GetNews)
	e.GET("/news/:id", h.GetNewsByID)

	e.Logger.Fatal(e.Start(":" + cfg.Port))
}
