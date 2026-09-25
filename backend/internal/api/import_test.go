package api

import (
	"strings"
	"testing"
)

func TestImportMultilineAndCast(t *testing.T) {
	source := "\uFEFF[script: A night out]\r\n[cast: Scout | Hazel]\r\n[scene: Woods]\r\n[Scout]\r\n[direction: Softly]\r\nFirst line.\r\nSecond line.\r\n\r\n[Owl]\r\nWho?\r\n[scene: Home]\r\n[Scout]\r\n\\[A literal bracket.]"
	p, err := ParseImport(source)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "A night out" || len(p.Scenes) != 2 || len(p.Cast) != 2 || p.LineCount != 3 {
		t.Fatalf("bad plan: %+v", p)
	}
	if p.Cast[0].Actor != "Hazel" || p.Cast[1].Actor != "" || p.Scenes[0].Lines[0].Text != "First line.\nSecond line." || p.Scenes[0].Lines[0].Direction != "Softly" {
		t.Fatalf("lost text/cast: %+v", p)
	}
	if p.Scenes[1].Lines[0].Text != "[A literal bracket.]" {
		t.Fatal("escaped text lost")
	}
}
func TestImportRejectsMistakes(t *testing.T) {
	for _, source := range []string{"", "plain text", "[script: Title]\n[scene: One]\n[Hazel]", "[script: Title]\n[scnee: One]", "[script: Title]\n[scene: One]\n[Hazel]\n[direction: quiet]\n[direction: loud]\nHello", "[script: Title]\n[cast: A | Hazel]\n[cast: A | Hannah]", "[script: Title]\n[scene: One]\n[Hazel] Hello", "[script: Title]\n[scene: One]\n[Hazel]\n" + strings.Repeat("x", 10001)} {
		if _, err := ParseImport(source); err == nil {
			t.Errorf("accepted invalid source %.80q", source)
		}
	}
}
