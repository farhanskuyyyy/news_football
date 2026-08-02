package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArticlesToNews(t *testing.T) {
	articles := []Article{
		{Title: "Match Report", URL: "https://example.com/1", Author: "John"},
		{Title: "No URL", URL: ""}, // should be skipped
		{Title: "Transfer News", URL: "https://example.com/2"},
	}

	news := ArticlesToNews(articles)

	if len(news) != 2 {
		t.Fatalf("expected 2 news items, got %d", len(news))
	}
	if news[0].Title != "Match Report" || news[0].URL != "https://example.com/1" {
		t.Errorf("unexpected first item: %+v", news[0])
	}
}

func TestFetchArticles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apiKey") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(NewsAPIResponse{
			Status:       "ok",
			TotalResults: 1,
			Articles:     []Article{{Title: "Test", URL: "https://example.com/a"}},
		})
	}))
	defer server.Close()

	client := NewNewsAPIClient(server.URL+"/v2/everything?q=test", "test-key")
	articles, err := client.FetchArticles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 1 || articles[0].Title != "Test" {
		t.Errorf("unexpected articles: %+v", articles)
	}
}

func TestFetchArticlesBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewNewsAPIClient(server.URL+"/v2/everything?q=test", "bad-key")
	if _, err := client.FetchArticles(); err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}
