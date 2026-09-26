package api

import "testing"

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
