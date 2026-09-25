package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectInvalidRequestsBeforeDatabase(t *testing.T) {
	mux := http.NewServeMux()
	(&API{}).Register(mux)
	for _, tc := range []struct {
		name, path, body string
		status           int
	}{
		{"blank project", "/api/projects", `{"name":"  "}`, 400},
		{"unknown field", "/api/actors", `{"name":"Hazel","admin":true}`, 400},
		{"multiple objects", "/api/projects", `{"name":"Moon"} {"name":"Sun"}`, 400},
		{"oversized body", "/api/projects", `{"name":"` + strings.Repeat("a", 70000) + `"}`, 400},
		{"invalid relation", "/api/characters", `{"name":"Guide","project_id":"invalid"}`, 400},
		{"missing scene position", "/api/scenes", `{"name":"Forest","project_id":"00000000-0000-0000-0000-000000000000"}`, 400},
		{"empty line", "/api/events", `{}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRejectCrossSiteForm(t *testing.T) {
	mux := http.NewServeMux()
	(&API{}).Register(mux)
	r := httptest.NewRequest("POST", "/api/projects", strings.NewReader("name=unwanted"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("status %d", w.Code)
	}
}
