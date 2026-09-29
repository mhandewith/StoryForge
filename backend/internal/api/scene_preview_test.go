package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompileSceneAudioFormatsAndFocusedTrim(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg integration test runs in the image build")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "take.wav")
	if err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=700:duration=1", "-ar", "24000", source).Run(); err != nil {
		t.Fatal(err)
	}
	a := &API{RecordingsDir: dir}
	for _, ext := range []string{".mp3", ".wav"} {
		t.Run(ext, func(t *testing.T) {
			out := filepath.Join(dir, "mixed"+ext)
			if err := a.compileScene(context.Background(), dir, out, []previewLine{{Source: "take.wav"}, {Source: "take.wav", StartMS: 500}}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(out)
			if err != nil || len(data) < 100 {
				t.Fatalf("missing mixed audio: %v", err)
			}
			if ext == ".wav" && string(data[:4]) != "RIFF" {
				t.Fatal("WAV export is not a WAV file")
			}
			pcm, err := exec.Command("ffmpeg", "-v", "error", "-i", out, "-f", "s16le", "-ar", "24000", "-ac", "1", "pipe:1").Output()
			if err != nil || len(pcm) < 70000 || len(pcm) > 75000 {
				t.Fatalf("expected 1.5 seconds of mixed audio, got %d bytes: %v", len(pcm), err)
			}
		})
	}
	out := filepath.Join(dir, "focused.wav")
	if err := a.compileScene(context.Background(), dir, out, []previewLine{{Source: "take.wav", StartMS: -500}}); err != nil {
		t.Fatal(err)
	}
	pcm, err := exec.Command("ffmpeg", "-v", "error", "-i", out, "-f", "s16le", "pipe:1").Output()
	if err != nil || len(pcm) != 24000 {
		t.Fatalf("expected the final half-second of the take, got %d bytes: %v", len(pcm), err)
	}
}

func TestPreviewParts(t *testing.T) {
	lines := make([]previewLine, 125)
	for i := range lines {
		lines[i] = previewLine{Text: "Hello"}
	}
	parts := previewParts(lines)
	if len(parts) != 4 || len(parts[0]) != 40 || len(parts[3]) != 5 {
		t.Fatalf("unexpected parts: %v", parts)
	}
	count := 0
	for _, part := range parts {
		for i := range part {
			if &part[i] != &lines[count] {
				t.Fatal("lost or reordered a line")
			}
			count++
		}
	}
	long := []previewLine{{Source: "a.wav", Duration: 300000}, {Source: "b.wav", Duration: 300000}, {Text: "Hello"}}
	if len(previewParts(long)) != 2 {
		t.Fatal("long recordings must split by duration")
	}
	if len(previewParts(nil)) != 0 {
		t.Fatal("empty scene has parts")
	}
}

func TestPreviewInvalidation(t *testing.T) {
	lines := []previewLine{{ID: "a", Character: "fox", Text: "Hello", Revision: 1}, {ID: "b", Character: "wolf", Text: "Goodbye", Revision: 1}}
	key := previewKey(lines)
	cases := [][]previewLine{
		{lines[1], lines[0]},
		{{ID: "a", Character: "fox", Text: "Changed", Revision: 2}, lines[1]},
		{{ID: "a", Character: "fox", Text: "Hello", Revision: 1, Source: "take.wav"}, lines[1]},
		{lines[0]},
	}
	for _, changed := range cases {
		if previewKey(changed) == key {
			t.Fatal("changed scene reused old preview")
		}
	}
	if previewKey(lines) != key {
		t.Fatal("unstable cache key")
	}
	if previewVoice("fox") != previewVoice("fox") {
		t.Fatal("unstable character voice")
	}
}

func TestCalculateTimelineGroupsAndOverlap(t *testing.T) {
	elements := []timelineElement{
		{Key: "line:a", Position: 1, Lines: []timelineLine{{ID: "a", Duration: 1000}}},
		{Key: "group:g", Position: 2, Lines: []timelineLine{{ID: "b", Duration: 800}, {ID: "c", Duration: 500, Start: 400}}},
		{Key: "line:d", Position: 3, Lines: []timelineLine{{ID: "d", Duration: 200}}},
	}
	got := calculateTimeline(elements, map[string]int{"line:a\x00group:g": -250})
	if got[0].Start != 0 || got[1].Start != 750 || got[2].Start != 1150 || got[3].Start != 1650 {
		t.Fatalf("unexpected positions: %#v", got)
	}
}
