package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEndpoints(t *testing.T) {
	for _, path := range []string{"/", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type = %q", got)
			}
			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["service"] != "StoryForge" {
				t.Fatalf("unexpected service: %v", body)
			}
			if path == "/healthz" && body["status"] != "ok" {
				t.Fatalf("unexpected health: %v", body)
			}
		})
	}
}

func TestRouting(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/missing", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	} {
		recorder := httptest.NewRecorder()
		handler().ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != tc.status {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, recorder.Code, tc.status)
		}
	}
}
