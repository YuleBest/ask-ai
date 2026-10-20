package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

func TestErrorsUseConfiguredLanguage(t *testing.T) {
	client := ExaClient{APIKey: "test", Language: i18n.English}
	_, err := client.Fetch(context.Background(), "not-a-url")
	if err == nil {
		t.Fatal("Fetch() accepted invalid URL")
	}
	if got, want := err.Error(), "Invalid URL: not-a-url"; got != want {
		t.Fatalf("Fetch() error = %q, want %q", got, want)
	}
}

func TestExaSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("path = %q, want /search", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var payload struct {
			Query      string `json:"query"`
			NumResults int    `json:"numResults"`
			Type       string `json:"type"`
			Contents   struct {
				Highlights any `json:"highlights"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Query != "Go latest version" || payload.NumResults != 3 || payload.Type != "auto" {
			t.Errorf("payload = %+v", payload)
		}
		if payload.Contents.Highlights != true {
			t.Errorf("contents.highlights = %#v, want true", payload.Contents.Highlights)
		}
		io.WriteString(w, `{"results":[{"title":"Go","url":"https://go.dev","publishedDate":"2026-01-01","highlights":["Go 1.26 released"]}]}`)
	}))
	defer server.Close()

	client := ExaClient{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	results, err := client.Search(context.Background(), "Go latest version", 3)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://go.dev" ||
		results[0].Title != "Go" || len(results[0].Highlights) != 1 {
		t.Fatalf("results = %+v", results)
	}
}

func TestExaFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contents" {
			t.Errorf("path = %q, want /contents", r.URL.Path)
		}
		var payload struct {
			URLs []string `json:"urls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.URLs) != 1 || payload.URLs[0] != "https://go.dev/doc" {
			t.Errorf("urls = %+v", payload.URLs)
		}
		io.WriteString(w, `{"results":[{"title":"Go docs","url":"https://go.dev/doc","text":"document text"}]}`)
	}))
	defer server.Close()

	client := ExaClient{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	result, err := client.Fetch(context.Background(), "https://go.dev/doc")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Text != "document text" || result.URL != "https://go.dev/doc" {
		t.Fatalf("result = %+v", result)
	}
}

func TestExaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid key"}`)
	}))
	defer server.Close()

	client := ExaClient{APIKey: "bad-key", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Search(context.Background(), "test", 1)
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestExaFetchRejectsInvalidURL(t *testing.T) {
	client := ExaClient{APIKey: "test-key"}
	if _, err := client.Fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("Fetch() error = nil for file URL")
	}
}

func TestExaFetchErrorObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"statuses":[{"id":"https://example.com","status":"error","error":{"tag":"CRAWL_UNKNOWN_ERROR","httpStatusCode":500}}]}`)
	}))
	defer server.Close()

	client := ExaClient{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "CRAWL_UNKNOWN_ERROR") {
		t.Fatalf("Fetch() error = %v, want crawl error object", err)
	}
}
