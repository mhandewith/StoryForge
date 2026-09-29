package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
)

// One render at a time bounds CPU and temporary disk use on the home server.
var sceneRenderLock sync.Mutex
var errPreviewLimit = errors.New("Preview exceeds five minutes per line or twenty minutes per scene. Split it into smaller parts.")

const maxScenePCM int64 = 24000 * 2 * 20 * 60

type previewLine struct {
	ID        string `json:"id"`
	Character string `json:"character"`
	Text      string `json:"text"`
	Revision  int    `json:"revision"`
	Source    string `json:"source"`
	Duration  int    `json:"duration_ms"`
	Converted bool   `json:"converted"`
}

// One SQL statement captures ordering, assignment and chosen takes together.
// Assignments control scene access, not which existing performances are selected.
// Current-revision takes stay eligible when a role is reassigned.
func (a *API) previewLines(ctx context.Context, scene string) ([]previewLine, error) {
	u := identity.Current(ctx)
	var raw []byte
	err := a.DB.QueryRow(ctx, `SELECT COALESCE((SELECT jsonb_agg(x ORDER BY x.position,x.id) FROM (
 SELECT e.id,e.character_id AS character,e.text,e.revision,e.position,COALESCE(converted.audio_key,t.storage_key,'') AS source,COALESCE(t.duration_ms,0) AS duration_ms,converted.audio_key IS NOT NULL AS converted
 FROM active_events e JOIN active_characters c ON c.id=e.character_id
 LEFT JOIN LATERAL (SELECT tk.id,asset.storage_key,asset.duration_ms FROM takes tk JOIN audio_assets asset ON asset.id=tk.asset_id
 WHERE tk.event_id=e.id AND tk.revision=e.revision
	 ORDER BY tk.preferred DESC,tk.created_at DESC,tk.id DESC LIMIT 1) t ON true
 LEFT JOIN LATERAL (SELECT j.audio_key FROM voice_jobs j WHERE j.event_id=e.id AND j.take_id=t.id AND j.voice_id=c.eleven_voice_id AND j.state='complete' AND j.audio_key<>'' ORDER BY j.completed_at DESC,j.created_at DESC,j.id DESC LIMIT 1) converted ON true
 WHERE e.scene_id=s.id) x),'[]'::jsonb)
 FROM active_scenes s WHERE s.id=$1 AND ($2::boolean OR EXISTS(
 SELECT 1 FROM active_events e JOIN active_assignments ass ON ass.character_id=e.character_id
 WHERE e.scene_id=s.id AND $3::uuid IS NOT NULL AND (ass.actor_id=$3 OR ass.actor_id IS NULL)))`, scene, u.Admin, nullableID(u.ActorID)).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var lines []previewLine
	err = json.Unmarshal(raw, &lines)
	return lines, err
}

func previewKey(lines []previewLine) string {
	b, _ := json.Marshal(lines)
	sum := sha256.Sum256(append([]byte("scene-v1-en-us-165-gap300:"), b...))
	return hex.EncodeToString(sum[:])
}

func previewVoice(character string) string {
	voices := []string{"en-us+m1", "en-us+m2", "en-us+m3", "en-us+f1", "en-us+f2", "en-us+f3"}
	h := sha256.Sum256([]byte(character))
	return voices[int(h[0])%len(voices)]
}

// Keep each HTTP render small enough for the tunnel and bound temporary audio.
// Duration estimates leave ample room below the compiler's twenty-minute limit.
func previewParts(lines []previewLine) [][]previewLine {
	var parts [][]previewLine
	start, duration := 0, 0
	for i, line := range lines {
		ms := line.Duration
		if line.Source == "" || ms <= 0 {
			ms = len(strings.Fields(line.Text))*600 + 2000
		}
		if i > start && (i-start >= 40 || duration+ms+300 > 10*60*1000) {
			parts = append(parts, lines[start:i])
			start, duration = i, 0
		}
		duration += ms + 300
	}
	if start < len(lines) {
		parts = append(parts, lines[start:])
	}
	return parts
}

func (a *API) renderScene(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Part     int    `json:"part"`
		Snapshot string `json:"snapshot"`
	}
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 400, "Choose a scene.")
		return
	}
	if a.RecordingsDir == "" {
		problem(w, 503, "Recording storage needs to be configured.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 80*time.Second)
	defer cancel()
	lines, err := a.previewLines(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Scene not found or not assigned to you.")
		return
	}
	if err != nil {
		a.failure(w, err)
		return
	}
	if len(lines) == 0 {
		problem(w, 400, "Add dialogue before generating a preview.")
		return
	}
	snapshot := previewKey(lines)
	if in.Snapshot != "" && in.Snapshot != snapshot {
		problem(w, 409, "The scene changed while building its preview. Generate again to include the latest changes.")
		return
	}
	parts := previewParts(lines)
	if in.Part < 0 || in.Part >= len(parts) {
		problem(w, 400, "Choose an available preview part.")
		return
	}
	start := 1
	for _, part := range parts[:in.Part] {
		start += len(part)
	}
	lines = parts[in.Part]
	key := previewKey(lines)
	dir := filepath.Join(a.RecordingsDir, "scene-previews")
	file := filepath.Join(dir, id+"-"+key+".mp3")
	if !sceneRenderLock.TryLock() {
		problem(w, 409, "Another scene is being generated. Try again shortly.")
		return
	}
	defer sceneRenderLock.Unlock()
	if err = os.MkdirAll(dir, 0750); err != nil {
		a.failure(w, err)
		return
	}
	if _, err = os.Stat(file); os.IsNotExist(err) {
		if err = a.compileScene(ctx, dir, file, lines); err != nil {
			slog.Error("scene preview failed", "scene_id", id, "error", err)
			if ctx.Err() != nil {
				problem(w, 503, "This preview took too long. Try a shorter scene.")
			} else if errors.Is(err, errPreviewLimit) {
				problem(w, 400, errPreviewLimit.Error())
			} else {
				problem(w, 503, "Could not generate this scene. Check server audio tools and storage, then try again.")
			}
			return
		}
	} else if err != nil {
		a.failure(w, err)
		return
	}
	recorded, converted := 0, 0
	for _, l := range lines {
		if l.Converted {
			converted++
		}
		if l.Source != "" {
			recorded++
		}
	}
	JSON(w, 200, map[string]any{"url": "/api/actor/scenes/" + id + "/preview/" + key, "recorded_lines": recorded, "converted_lines": converted, "synthetic_lines": len(lines) - recorded, "part": in.Part, "parts": len(parts), "start_line": start, "end_line": start + len(lines) - 1, "snapshot": snapshot})
}

func (a *API) sceneAudio(w http.ResponseWriter, r *http.Request) {
	id, key := r.PathValue("id"), r.PathValue("key")
	if !validID(id) || len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" || a.RecordingsDir == "" {
		problem(w, 404, "Preview not found.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, err := a.previewLines(ctx, id) // Reauthorize every playback, including range requests.
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Scene not found or not assigned to you.")
		return
	}
	if err != nil {
		a.failure(w, err)
		return
	}
	f, err := os.Open(filepath.Join(a.RecordingsDir, "scene-previews", id+"-"+key+".mp3"))
	if err != nil {
		problem(w, 404, "Generate this preview again.")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		a.failure(w, err)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", `inline; filename="storyforge-scene.mp3"`)
	http.ServeContent(w, r, "storyforge-scene.mp3", info.ModTime(), f)
}

func (a *API) compileScene(ctx context.Context, dir, destination string, lines []previewLine) error {
	tmp, err := os.MkdirTemp(dir, ".render-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	pcm, err := os.Create(filepath.Join(tmp, "scene.pcm"))
	if err != nil {
		return err
	}
	defer pcm.Close()
	var total int64
	for i, line := range lines {
		source := filepath.Join(tmp, "speech.wav")
		if line.Source != "" {
			if filepath.Base(line.Source) != line.Source {
				return errors.New("invalid source key")
			}
			source = filepath.Join(a.RecordingsDir, line.Source)
		} else {
			cmd := exec.CommandContext(ctx, "espeak-ng", "-v", previewVoice(line.Character), "-s", "165", "-w", source, "--stdin")
			cmd.Stdin = strings.NewReader(line.Text)
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("speech synthesis: %w", err)
			}
		}
		segment := filepath.Join(tmp, "line.pcm")
		cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-protocol_whitelist", "file,pipe", "-i", source, "-vn", "-fs", "14448000", "-ac", "1", "-ar", "24000", "-f", "s16le", segment)
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("normalize audio: %w", err)
		}
		info, err := os.Stat(segment)
		if err != nil {
			return err
		}
		if info.Size() >= 14448000 {
			return errPreviewLimit
		}
		f, err := os.Open(segment)
		if err != nil {
			return err
		}
		n, err := io.Copy(pcm, io.LimitReader(f, maxScenePCM-total+1))
		f.Close()
		total += n
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("empty audio")
		}
		if total > maxScenePCM {
			return errPreviewLimit
		}
		if i < len(lines)-1 {
			silence := make([]byte, 14400)
			if _, err = pcm.Write(silence); err != nil {
				return err
			}
			total += int64(len(silence))
		}
	}
	if total > maxScenePCM {
		return errPreviewLimit
	}
	if err = pcm.Close(); err != nil {
		return err
	}
	output := filepath.Join(tmp, "scene.mp3")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", filepath.Join(tmp, "scene.pcm"), "-c:a", "libmp3lame", "-b:a", "96k", output)
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("encode scene: %w", err)
	}
	return os.Rename(output, destination)
}
