package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSpeechBounds(t *testing.T) {
	pcm := make([]byte, 4*16000*3)
	if start, end := speechBounds(pcm); start != 0 || end != 48000 {
		t.Fatalf("silence-only audio must be preserved: %d %d", start, end)
	}
	// Two quiet bursts with a long internal pause, including opposite-phase stereo.
	for _, frame := range []int{8000, 32000} {
		binary.LittleEndian.PutUint16(pcm[frame*4:], 120)
		binary.LittleEndian.PutUint16(pcm[frame*4+2:], 65536-120)
	}
	if start, end := speechBounds(pcm); start != 6400 || end != 33601 {
		t.Fatalf("speech handles/internal pause lost: %d %d", start, end)
	}
}

func TestConvertedEdgesTrimmedRawPreserved(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg integration test runs in the image build")
	}
	for _, voice := range []string{"test-voice", isolationVoiceID} {
		t.Run(voice, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "raw.wav")
			// Quiet speech-like bursts separated by 800 ms. Trim only the outer silence.
			expression := "aevalsrc=0.006*sin(2*PI*700*t)*(between(t\\,0.7\\,1.2)+between(t\\,2\\,2.5)):s=16000:d=4"
			if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", expression, "-c:a", "pcm_s16le", source).CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, out)
			}
			original, _ := os.ReadFile(source)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				f, _, err := r.FormFile("audio")
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				_, _ = io.Copy(w, f)
			}))
			defer server.Close()
			a := &API{RecordingsDir: dir, Eleven: &ElevenClient{key: "test", base: server.URL, http: server.Client()}}
			if err := a.convertVoiceLine(context.Background(), voice, "raw.wav", "out.mp3"); err != nil {
				t.Fatal(err)
			}
			info, err := inspectAudio(context.Background(), filepath.Join(dir, "out.mp3"))
			if err != nil || info.Duration < 1950 || info.Duration > 2200 {
				t.Fatalf("expected two seconds including internal pause: %+v %v", info, err)
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(original, after) {
				t.Fatal("raw recording changed")
			}
		})
	}
}
