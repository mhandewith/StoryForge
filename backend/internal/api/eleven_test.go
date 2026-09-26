package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVoiceLibraryPaginationCacheRefresh(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "test-secret" {
			t.Error("missing authentication")
		}
		calls.Add(1)
		if r.URL.Query().Get("next_page_token") == "next" {
			_ = json.NewEncoder(w).Encode(map[string]any{"voices": []ElevenVoice{{"b", "Owl"}}, "has_more": false})
			return
		}
		if r.URL.Query().Get("page_size") != "100" {
			t.Error("missing page size")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"voices": []ElevenVoice{{"a", "Wolf"}}, "has_more": true, "next_page_token": "next"})
	}))
	defer server.Close()
	c := &ElevenClient{key: "test-secret", base: server.URL, http: server.Client()}
	voices, err := c.Voices(context.Background(), false)
	if err != nil || len(voices) != 2 || voices[0].Name != "Owl" {
		t.Fatalf("unexpected voices %v %v", voices, err)
	}
	_, _ = c.Voices(context.Background(), false)
	if calls.Load() != 2 {
		t.Fatal("cache missed")
	}
	_, _ = c.Voices(context.Background(), true)
	if calls.Load() != 4 {
		t.Fatal("refresh did not bypass cache")
	}
	c.fetched = time.Now().Add(-11 * time.Minute)
	_, _ = c.Voices(context.Background(), false)
	if calls.Load() != 6 {
		t.Fatal("expired cache was reused")
	}
}

func TestConversionMultipartAndErrors(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "raw.wav")
	_ = os.WriteFile(source, []byte("original audio"), 0600)
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(429)
			_, _ = w.Write([]byte("sensitive vendor detail test-secret"))
			return
		}
		if r.URL.Path != "/v1/speech-to-speech/voice-123" || r.URL.Query().Get("output_format") != "mp3_44100_128" {
			t.Error("incorrect endpoint")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		if r.FormValue("model_id") != "eleven_multilingual_sts_v2" || r.FormValue("voice_settings") == "" {
			t.Error("missing model or pinned settings")
		}
		f, _, err := r.FormFile("audio")
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if string(b) != "original audio" {
			t.Error("wrong source")
		}
		_, _ = w.Write([]byte("converted audio"))
	}))
	defer server.Close()
	c := &ElevenClient{key: "test-secret", base: server.URL, http: server.Client()}
	output := filepath.Join(dir, "output.mp3")
	if err := c.Convert(context.Background(), "voice-123", source, output); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(output)
	if string(b) != "converted audio" {
		t.Fatal("wrong output")
	}
	b, _ = os.ReadFile(source)
	if string(b) != "original audio" {
		t.Fatal("source modified")
	}
	fail.Store(true)
	err := c.Convert(context.Background(), "voice-123", source, output)
	if err == nil || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("unsafe provider error")
	}
}

func TestVoiceHashTracksSelection(t *testing.T) {
	lines := []voiceInput{{Event: "one", Take: "a", Voice: "wolf", Revision: 1, Position: 1}, {Event: "two", Take: "b", Voice: "owl", Revision: 1, Position: 2}}
	initial := voiceHash(lines)
	for _, change := range []func([]voiceInput){func(l []voiceInput) { l[0].Take = "new" }, func(l []voiceInput) { l[0].Voice = "fox" }, func(l []voiceInput) { l[0].Revision++ }, func(l []voiceInput) { l[0], l[1] = l[1], l[0] }} {
		copyLines := append([]voiceInput{}, lines...)
		change(copyLines)
		if voiceHash(copyLines) == initial {
			t.Fatal("outdated scene hash unchanged")
		}
	}
}
