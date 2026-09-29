package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func srtTime(ms int) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}

// exportSceneProduction is intentionally scene-scoped: each archive is portable
// and its timeline JSON is enough to reconstruct the arrangement elsewhere.
func (a *API) exportSceneProduction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) || a.RecordingsDir == "" {
		problem(w, 400, "Choose a scene with recording storage configured.")
		return
	}
	ctx, c := context.WithTimeout(r.Context(), 90*time.Second)
	defer c()
	var name string
	if err := a.DB.QueryRow(ctx, "SELECT name FROM active_scenes WHERE id=$1", id).Scan(&name); err != nil {
		a.failure(w, err)
		return
	}
	lines, err := a.previewLines(ctx, id)
	if err != nil {
		a.failure(w, err)
		return
	}
	if len(lines) == 0 {
		problem(w, 400, "Add dialogue before exporting.")
		return
	}
	tmp, err := os.MkdirTemp(a.RecordingsDir, ".production-")
	if err != nil {
		a.failure(w, err)
		return
	}
	defer os.RemoveAll(tmp)
	mix := filepath.Join(tmp, "mixed.wav")
	if err = a.compileScene(ctx, tmp, mix, lines); err != nil {
		problem(w, 503, "Could not mix this scene for export.")
		return
	}
	file, err := os.CreateTemp(a.RecordingsDir, ".scene-export-*.zip")
	if err != nil {
		a.failure(w, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	z := zip.NewWriter(file)
	prefix := exportName(name)
	copyFile := func(zipName, path string) error {
		out, e := z.Create(zipName)
		if e != nil {
			return e
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		_, e = io.Copy(out, in)
		return e
	}
	if err = copyFile(prefix+"_Mixed.wav", mix); err != nil {
		a.failure(w, err)
		return
	}
	script, _ := z.Create(prefix + "_Script.txt")
	subs, _ := z.Create(prefix + "_Subtitles.srt")
	for i, l := range lines {
		fmt.Fprintf(script, "[%s – %s] %s: %s\n", srtTime(l.StartMS), srtTime(l.EndMS), l.Character, l.Text)
		fmt.Fprintf(subs, "%d\n%s --> %s\n%s: %s\n\n", i+1, srtTime(l.StartMS), srtTime(l.EndMS), l.Character, l.Text)
	}
	timeline, _ := z.Create(prefix + "_Timeline.json")
	transitions := []map[string]any{}
	rows, _ := a.DB.Query(ctx, "SELECT predecessor,successor,offset_ms FROM dialogue_transitions WHERE scene_id=$1", id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var predecessor, successor string
			var offset int
			if rows.Scan(&predecessor, &successor, &offset) == nil {
				transitions = append(transitions, map[string]any{"predecessor": predecessor, "successor": successor, "offset_ms": offset})
			}
		}
	}
	json.NewEncoder(timeline).Encode(map[string]any{"scene_id": id, "scene": name, "lines": lines, "transitions": transitions, "subtitle_note": "Overlapping subtitle timestamps are preserved; rendering depends on the target player or editor."})
	for i, l := range lines {
		if l.Source == "" || filepath.Base(l.Source) != l.Source {
			continue
		}
		ext := filepath.Ext(l.Source)
		if ext == "" {
			ext = ".audio"
		}
		if err = copyFile(fmt.Sprintf("Dialogue/%03d_%s%s", i+1, exportName(l.Character), ext), filepath.Join(a.RecordingsDir, l.Source)); err != nil {
			problem(w, 503, "A selected recording is unavailable.")
			return
		}
	}
	if err = z.Close(); err != nil {
		a.failure(w, err)
		return
	}
	file.Seek(0, 0)
	info, _ := file.Stat()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, prefix))
	http.ServeContent(w, r, prefix+".zip", info.ModTime(), file)
}
