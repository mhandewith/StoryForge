package api

import "testing"

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
