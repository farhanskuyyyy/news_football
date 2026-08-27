package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSportmonksClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Unauthenticated"}`))
			return
		}

		if r.URL.Path == "/v3/football/leagues" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"id":8,"name":"Premier League"}]}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not found"}`))
	}))
	defer server.Close()

	client := NewSportmonksClient(server.URL+"/v3", "test-token")

	// Test successful Get
	data, err := client.GetLeagues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"data":[{"id":8,"name":"Premier League"}]}` {
		t.Errorf("unexpected response: %s", string(data))
	}

	// Test unauthenticated
	badClient := NewSportmonksClient(server.URL+"/v3", "wrong-token")
	_, err = badClient.GetLeagues()
	if err == nil {
		t.Fatal("expected error for unauthenticated call, got nil")
	}
}

func TestSportmonksClientHelperMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"id":1,"name":"Europe"}}`))
	}))
	defer server.Close()

	client := NewSportmonksClient(server.URL+"/v3", "dummy-token")

	data, err := client.GetContinents("countries")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data response")
	}

	data, err = client.GetContinentByID("1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data response")
	}
}
