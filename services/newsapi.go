package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
)

type NewsAPIResponse struct {
	Status       string    `json:"status"`
	TotalResults int       `json:"totalResults"`
	Articles     []Article `json:"articles"`
}

type Article struct {
	Source struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"source"`
	Author      string    `json:"author"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	URLToImage  string    `json:"urlToImage"`
	PublishedAt time.Time `json:"publishedAt"`
	Content     string    `json:"content"`
}

type NewsAPIClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewNewsAPIClient(baseURL, apiKey string) *NewsAPIClient {
	return &NewsAPIClient{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *NewsAPIClient) FetchArticles() ([]Article, error) {
	url := fmt.Sprintf("%s&apiKey=%s", c.BaseURL, c.APIKey)
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newsapi returned status %d", resp.StatusCode)
	}

	var result NewsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Articles, nil
}

func ArticlesToNews(articles []Article) []models.News {
	news := make([]models.News, 0, len(articles))
	for _, a := range articles {
		if a.URL == "" {
			continue
		}
		news = append(news, models.News{
			Source:      a.Source.Name,
			Author:      a.Author,
			Title:       a.Title,
			Description: a.Description,
			URL:         a.URL,
			URLToImage:  a.URLToImage,
			PublishedAt: a.PublishedAt,
			Content:     a.Content,
		})
	}
	return news
}
