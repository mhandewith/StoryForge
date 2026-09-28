package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var exportLock sync.Mutex

type exportLine struct {
	Scene         string `json:"scene"`
	ScenePosition int    `json:"scene_position"`
	Position      int    `json:"line_position"`
	Event         string `json:"event_id"`
	Character     string `json:"character"`
	Actor         string `json:"actor"`
	Text          string `json:"text"`
	Take          string `json:"take_id"`
	Source        string `json:"-"`
	Kind          string `json:"kind"`
	File          string `json:"file"`
}

func exportName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
		if b.Len() >= 60 {
			break
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "untitled"
	}
	return result
}
func (a *API) exportProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 400, "Choose a project.")
		return
	}
	if a.RecordingsDir == "" {
		problem(w, 503, "Recording storage is not configured.")
		return
	}
	if !exportLock.TryLock() {
		problem(w, 409, "Another export is being prepared. Try again shortly.")
		return
	}
	defer exportLock.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var name string
	if err := a.DB.QueryRow(ctx, "SELECT name FROM active_projects WHERE id=$1", id).Scan(&name); err != nil {
		a.failure(w, err)
		return
	}
	rows, err := a.DB.Query(ctx, `SELECT s.name,s.position,e.position,e.id::text,c.name,COALESCE(t.actor_name,''),e.text,COALESCE(t.id::text,''),COALESCE(j.audio_key,t.storage_key,''),CASE WHEN j.audio_key IS NOT NULL THEN 'converted' WHEN t.id IS NOT NULL THEN 'raw' ELSE 'missing' END
 FROM active_events e JOIN active_scenes s ON s.id=e.scene_id JOIN active_characters c ON c.id=e.character_id
 LEFT JOIN active_assignments ass ON ass.character_id=c.id
 LEFT JOIN LATERAL (SELECT tk.id,asset.storage_key,actor.name AS actor_name FROM takes tk JOIN audio_assets asset ON asset.id=tk.asset_id JOIN actors actor ON actor.id=tk.actor_id WHERE tk.event_id=e.id AND tk.revision=e.revision AND ass.character_id IS NOT NULL AND (ass.actor_id IS NULL OR tk.actor_id=ass.actor_id) ORDER BY tk.preferred DESC,tk.created_at DESC,tk.id DESC LIMIT 1)t ON true
 LEFT JOIN LATERAL (SELECT audio_key FROM voice_jobs WHERE event_id=e.id AND take_id=t.id AND voice_id=c.eleven_voice_id AND state='complete' AND audio_key<>'' ORDER BY completed_at DESC,created_at DESC,id DESC LIMIT 1)j ON true
 WHERE e.project_id=$1 ORDER BY s.position,s.id,e.position,e.id`, id)
	if err != nil {
		a.failure(w, err)
		return
	}
	lines := []exportLine{}
	for rows.Next() {
		var l exportLine
		if err = rows.Scan(&l.Scene, &l.ScenePosition, &l.Position, &l.Event, &l.Character, &l.Actor, &l.Text, &l.Take, &l.Source, &l.Kind); err != nil {
			break
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		a.failure(w, err)
		return
	}
	file, err := os.CreateTemp(a.RecordingsDir, ".export-*.zip")
	if err != nil {
		problem(w, 503, "Could not create export. Check recording storage.")
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	z := zip.NewWriter(file)
	defer z.Close()
	var size int64
	count := 0
	for i := range lines {
		l := &lines[i]
		if l.Source == "" {
			continue
		}
		if ctx.Err() != nil {
			problem(w, 503, "Export took too long. Try again.")
			return
		}
		if filepath.Base(l.Source) != l.Source {
			problem(w, 503, "Invalid recording path.")
			return
		}
		input, e := os.Open(filepath.Join(a.RecordingsDir, l.Source))
		if e != nil {
			problem(w, 503, "A selected recording is missing from storage. Restore it or select another take before exporting.")
			return
		}
		info, e := input.Stat()
		if e != nil || !info.Mode().IsRegular() {
			input.Close()
			problem(w, 503, "A selected recording is unavailable.")
			return
		}
		size += info.Size()
		if size > 512*1024*1024 {
			input.Close()
			problem(w, 400, "This export exceeds 512 MB. Export a smaller project.")
			return
		}
		l.File = fmt.Sprintf("%010d_%s/%010d_%s_%s_%s%s", l.ScenePosition, exportName(l.Scene), l.Position, exportName(l.Character), exportName(l.Actor), l.Kind, filepath.Ext(l.Source))
		out, e := z.CreateHeader(&zip.FileHeader{Name: l.File, Method: zip.Store})
		if e == nil {
			_, e = io.Copy(out, input)
		}
		input.Close()
		if e != nil {
			problem(w, 503, "Could not write export. Check storage.")
			return
		}
		count++
	}
	if count == 0 {
		problem(w, 400, "This project has no recorded audio to export.")
		return
	}
	manifest, _ := z.Create("manifest.json")
	if manifest == nil {
		problem(w, 503, "Could not write export manifest.")
		return
	}
	err = json.NewEncoder(manifest).Encode(map[string]any{"project": name, "exported_at": time.Now().UTC(), "lines": lines})
	readme, e := z.Create("README.txt")
	if e == nil {
		_, e = io.WriteString(readme, "Files sort by scene and line position. Current converted audio is preferred over the selected raw take. Original formats are preserved. No computer-generated placeholder voices are exported. See manifest.json for missing lines, actor names, dialogue, and take IDs. Export does not make paid requests.\n")
	}
	if err != nil || e != nil || z.Close() != nil {
		problem(w, 503, "Could not finish export.")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		problem(w, 503, "Could not read export.")
		return
	}
	info, err := file.Stat()
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		problem(w, 503, "Could not finish export in time.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-audio.zip"`, exportName(name)))
	http.ServeContent(w, r, exportName(name)+"-audio.zip", info.ModTime(), file)
}
