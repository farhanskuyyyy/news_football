package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
	"github.com/farhanarfianto/apigo-docker/services"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	newsCacheKey = "news:list"
	newsCacheTTL = 5 * time.Minute
)

type NewsHandler struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Client *services.NewsAPIClient
}

func NewNewsHandler(db *gorm.DB, rdb *redis.Client, client *services.NewsAPIClient) *NewsHandler {
	return &NewsHandler{DB: db, Redis: rdb, Client: client}
}

// GetNews fetches articles from NewsAPI, upserts them into Postgres,
// and returns the stored list. Result is cached in Redis for 5 minutes.
func (h *NewsHandler) GetNews(c echo.Context) error {
	ctx := c.Request().Context()

	if cached, err := h.Redis.Get(ctx, newsCacheKey).Result(); err == nil {
		var news []models.News
		if json.Unmarshal([]byte(cached), &news) == nil {
			return c.JSON(http.StatusOK, echo.Map{"source": "cache", "data": news})
		}
	}

	articles, err := h.Client.FetchArticles()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to fetch from newsapi: "+err.Error())
	}

	news := services.ArticlesToNews(articles)
	if len(news) > 0 {
		if err := h.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "url"}},
			DoNothing: true,
		}).Create(&news).Error; err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to save news: "+err.Error())
		}
	}

	var stored []models.News
	if err := h.DB.Order("published_at DESC").Find(&stored).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query news: "+err.Error())
	}

	h.cacheNews(ctx, stored)
	return c.JSON(http.StatusOK, echo.Map{"source": "db", "data": stored})
}

// GetNewsByID returns a single stored news row by its primary key.
func (h *NewsHandler) GetNewsByID(c echo.Context) error {
	id := c.Param("id")

	var news models.News
	if err := h.DB.First(&news, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusNotFound, "news not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, echo.Map{"data": news})
}

func (h *NewsHandler) cacheNews(ctx context.Context, news []models.News) {
	payload, err := json.Marshal(news)
	if err != nil {
		return
	}
	h.Redis.Set(ctx, newsCacheKey, payload, newsCacheTTL)
}
