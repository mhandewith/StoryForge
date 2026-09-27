package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestShortIsolationPaddingAndTrim(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg integration test runs in the image build")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "raw.wav")
	if err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=700:duration=2.22", "-ar", "16000", source).Run(); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(source)
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		f, _, err := r.FormFile("audio")
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		padded := filepath.Join(dir, "received.wav")
		_ = os.WriteFile(padded, data, 0600)
		info, err := inspectAudio(context.Background(), padded)
		if err != nil || info.Duration < 4990 || info.Duration > 5010 {
			t.Errorf("unexpected padded audio: %+v %v", info, err)
		}
		if changed.Load() {
			_, _ = w.Write(original)
		} else {
			_, _ = w.Write(data)
		}
	}))
	defer server.Close()
	a := &API{RecordingsDir: dir, Eleven: &ElevenClient{key: "test", base: server.URL, http: server.Client()}}
	if err := a.convertVoiceLine(context.Background(), isolationVoiceID, "raw.wav", "out.mp3"); err != nil {
		t.Fatal(err)
	}
	info, err := inspectAudio(context.Background(), filepath.Join(dir, "out.mp3"))
	if err != nil || info.Duration < 2200 || info.Duration > 2370 {
		t.Fatalf("unexpected trimmed audio: %+v %v", info, err)
	}
	// Confirm the middle of the result is speech/tone, not leftover silence.
	pcm, err := exec.Command("ffmpeg", "-v", "error", "-i", filepath.Join(dir, "out.mp3"), "-ss", "0.5", "-t", "1", "-f", "s16le", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	nonzero := 0
	for _, b := range pcm {
		if b != 0 {
			nonzero++
		}
	}
	if nonzero < len(pcm)/2 {
		t.Fatal("trim lost the original signal")
	}
	after, _ := os.ReadFile(source)
	if string(after) != string(original) {
		t.Fatal("raw recording changed")
	}
	changed.Store(true)
	if err := a.convertVoiceLine(context.Background(), isolationVoiceID, "raw.wav", "bad.mp3"); err == nil {
		t.Fatal("accepted provider timing change")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.mp3")); !os.IsNotExist(err) {
		t.Fatal("published unsafe trim")
	}
}
