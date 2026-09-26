package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
)

type voiceInput struct {
	Event     string `json:"event_id"`
	Take      string `json:"take_id"`
	Voice     string `json:"voice_id"`
	Source    string `json:"source_key"`
	Character string `json:"character"`
	Text      string `json:"text"`
	Revision  int    `json:"revision"`
	Position  int    `json:"position"`
	Duration  int    `json:"duration_ms"`
}
type rowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readVoiceInputs(ctx context.Context, q rowReader, scene string) ([]voiceInput, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT jsonb_agg(x ORDER BY x.position,x.event_id) FROM (
 SELECT e.id AS event_id,e.text,e.revision,e.position,c.name AS character,c.eleven_voice_id AS voice_id,
 COALESCE(t.id::text,'') AS take_id,COALESCE(t.storage_key,'') AS source_key,COALESCE(t.duration_ms,0) AS duration_ms
 FROM active_events e JOIN active_characters c ON c.id=e.character_id
 LEFT JOIN active_assignments ass ON ass.character_id=e.character_id
 LEFT JOIN LATERAL(SELECT tk.id,asset.storage_key,asset.duration_ms FROM takes tk JOIN audio_assets asset ON asset.id=tk.asset_id
 WHERE tk.event_id=e.id AND tk.revision=e.revision AND tk.actor_id=ass.actor_id
 ORDER BY tk.preferred DESC,tk.created_at DESC,tk.id DESC LIMIT 1) t ON true WHERE e.scene_id=s.id
 )x),'[]'::jsonb) FROM active_scenes s WHERE s.id=$1`, scene).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var lines []voiceInput
	err = json.Unmarshal(raw, &lines)
	return lines, err
}
func voiceHash(lines []voiceInput) string {
	// Names are presentation only. Reordering, script revisions, take or voice changes invalidate the scene.
	items := make([]any, 0, len(lines))
	for _, l := range lines {
		items = append(items, []any{l.Event, l.Revision, l.Take, l.Voice, l.Position})
	}
	b, _ := json.Marshal(items)
	h := sha256.Sum256(append([]byte("eleven-sts-v1:"), b...))
	return hex.EncodeToString(h[:])
}
func (a *API) registerVoicing(m *http.ServeMux) {
	m.HandleFunc("GET /api/voices", a.voiceLibrary)
	m.HandleFunc("POST /api/voices/refresh", a.voiceLibrary)
	m.HandleFunc("POST /api/scenes/{id}/voice", a.queueVoiceScene)
	m.HandleFunc("GET /api/actor/scenes/{id}/voicing", a.voiceSceneStatus)
	m.HandleFunc("GET /api/actor/scenes/{id}/converted/{asset}", a.convertedAudio)
}
func (a *API) voiceLibrary(w http.ResponseWriter, r *http.Request) {
	if !a.Eleven.Enabled() {
		JSON(w, 200, map[string]any{"configured": false, "voices": []ElevenVoice{}})
		return
	}
	voices, err := a.Eleven.Voices(r.Context(), r.Method == "POST")
	if err != nil {
		problem(w, 503, err.Error())
		return
	}
	JSON(w, 200, map[string]any{"configured": true, "voices": voices})
}
func (a *API) voiceSceneAccess(w http.ResponseWriter, r *http.Request) bool {
	if !validID(r.PathValue("id")) {
		problem(w, 400, "Choose a scene.")
		return false
	}
	_, err := a.previewLines(r.Context(), r.PathValue("id"))
	if err != nil {
		a.failure(w, err)
		return false
	}
	return true
}
func (a *API) voiceSceneStatus(w http.ResponseWriter, r *http.Request) {
	if !a.voiceSceneAccess(w, r) {
		return
	}
	scene := r.PathValue("id")
	lines, err := readVoiceInputs(r.Context(), a.DB, scene)
	if err != nil {
		a.failure(w, err)
		return
	}
	missingTakes, missingVoices := 0, 0
	for _, l := range lines {
		if l.Take == "" {
			missingTakes++
		}
		if l.Voice == "" {
			missingVoices++
		}
	}
	var run, finished, jobs json.RawMessage
	err = a.DB.QueryRow(r.Context(), `SELECT
 COALESCE((SELECT jsonb_build_object('id',id,'state',state,'error',error,'done',(SELECT count(*) FROM voice_jobs WHERE run_id=r.id AND state='complete'),'total',(SELECT count(*) FROM voice_jobs WHERE run_id=r.id)) FROM voice_runs r WHERE scene_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1),'null'::jsonb),
 COALESCE((SELECT jsonb_build_object('id',id,'snapshot_hash',snapshot_hash) FROM voice_runs WHERE scene_id=$1 AND state='complete' ORDER BY created_at DESC,id DESC LIMIT 1),'null'::jsonb),
 COALESCE((SELECT jsonb_agg(x) FROM (SELECT DISTINCT ON(j.event_id) j.id,j.event_id,j.take_id,j.voice_id FROM voice_jobs j JOIN voice_runs r ON r.id=j.run_id WHERE r.scene_id=$1 AND j.state='complete' ORDER BY j.event_id,j.completed_at DESC,j.created_at DESC,j.id DESC)x),'[]'::jsonb)`, scene).Scan(&run, &finished, &jobs)
	if err != nil {
		a.failure(w, err)
		return
	}
	// Only admin status includes raw source identity. The actor response contains public app IDs, never paths.
	public := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		public = append(public, map[string]any{"event_id": l.Event, "take_id": l.Take, "voice_id": l.Voice, "character": l.Character, "text": l.Text, "position": l.Position})
	}
	JSON(w, 200, map[string]any{"configured": a.Eleven.Enabled(), "ready": len(lines) > 0 && len(lines) <= 120 && missingTakes == 0 && missingVoices == 0, "missing_takes": missingTakes, "missing_voices": missingVoices, "snapshot_hash": voiceHash(lines), "lines": public, "run": run, "finished": finished, "conversions": jobs})
}
func (a *API) queueVoiceScene(w http.ResponseWriter, r *http.Request) {
	if !a.Eleven.Enabled() || a.RecordingsDir == "" {
		problem(w, 503, "Configure ElevenLabs and recording storage in Unraid first.")
		return
	}
	var in struct {
		RequestID string `json:"request_id"`
		Snapshot  string `json:"snapshot_hash"`
		Event     string `json:"event_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	scene := r.PathValue("id")
	if !validID(scene) || !validID(in.RequestID) || (in.Event != "" && !validID(in.Event)) {
		problem(w, 400, "Choose a scene and provide a request ID.")
		return
	}
	voices, err := a.Eleven.Voices(r.Context(), true)
	if err != nil {
		problem(w, 503, err.Error())
		return
	}
	available := map[string]bool{}
	for _, v := range voices {
		available[v.ID] = true
	}
	var run string
	err = a.write(r.Context(), func(tx pgx.Tx) error {
		var oldScene, oldHash, oldEvent string
		err := tx.QueryRow(r.Context(), `SELECT id::text,scene_id::text,snapshot_hash,forced_event FROM voice_runs WHERE request_id=$1`, in.RequestID).Scan(&run, &oldScene, &oldHash, &oldEvent)
		if err == nil {
			if oldScene != scene || oldHash != in.Snapshot || oldEvent != in.Event {
				return clientError{409, "Request ID belongs to a different scene version."}
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		lines, err := readVoiceInputs(r.Context(), tx, scene)
		if err != nil {
			return err
		}
		hash := voiceHash(lines)
		if hash != in.Snapshot {
			return clientError{409, "The scene changed. Review the current takes and try again."}
		}
		if len(lines) == 0 || len(lines) > 120 {
			return clientError{400, "Voice scenes with 1–120 lines."}
		}
		found := in.Event == ""
		duration := (len(lines) - 1) * 300
		for _, l := range lines {
			duration += l.Duration
			if l.Take == "" {
				return clientError{400, "Every line needs a current take before voicing the scene."}
			}
			if !available[l.Voice] {
				return clientError{400, "Assign an available ElevenLabs voice to every character. Refresh voices if needed."}
			}
			if l.Event == in.Event {
				found = true
			}
		}
		if !found {
			return clientError{400, "The selected line is not in this scene."}
		}
		if duration > 20*60*1000 {
			return clientError{400, "Split scenes longer than twenty minutes before converting."}
		}
		err = tx.QueryRow(r.Context(), `SELECT id::text FROM voice_runs WHERE scene_id=$1 AND state IN ('queued','running')`, scene).Scan(&run)
		if err == nil {
			return clientError{409, "This scene is already queued or converting."}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if in.Event == "" {
			err = tx.QueryRow(r.Context(), `SELECT id::text FROM voice_runs WHERE scene_id=$1 AND snapshot_hash=$2 AND state='complete' AND id=(SELECT id FROM voice_runs WHERE scene_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1)`, scene, hash).Scan(&run)
			if err == nil {
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err = tx.QueryRow(r.Context(), `INSERT INTO voice_runs(scene_id,snapshot_hash,request_id,requested_by,forced_event) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, scene, hash, in.RequestID, identity.Current(r.Context()).Email, in.Event).Scan(&run); err != nil {
			return err
		}
		for _, l := range lines {
			key := ""
			var completed *time.Time
			if l.Event != in.Event {
				err = tx.QueryRow(r.Context(), `SELECT audio_key,completed_at FROM voice_jobs WHERE take_id=$1 AND voice_id=$2 AND state='complete' ORDER BY completed_at DESC LIMIT 1`, l.Take, l.Voice).Scan(&key, &completed)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if key != "" {
					if _, e := os.Stat(filepath.Join(a.RecordingsDir, key)); e != nil {
						key = ""
						completed = nil
					}
				}
				if in.Event != "" && key == "" {
					return clientError{409, "Other lines need conversion too. Use Voice scene first."}
				}
			}
			state := "queued"
			if key != "" {
				state = "complete"
			}
			_, err = tx.Exec(r.Context(), `INSERT INTO voice_jobs(run_id,event_id,take_id,voice_id,position,source_key,state,audio_key,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, run, l.Event, l.Take, l.Voice, l.Position, l.Source, state, key, completed)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 202, map[string]string{"id": run})
}
func (a *API) convertedAudio(w http.ResponseWriter, r *http.Request) {
	if !a.voiceSceneAccess(w, r) {
		return
	}
	id := r.PathValue("asset")
	if !validID(id) {
		problem(w, 404, "Converted audio not found.")
		return
	}
	var key string
	err := a.DB.QueryRow(r.Context(), `SELECT audio_key FROM voice_runs WHERE id=$1 AND scene_id=$2 AND state='complete'
 UNION ALL SELECT j.audio_key FROM voice_jobs j JOIN voice_runs r ON r.id=j.run_id WHERE j.id=$1 AND r.scene_id=$2 AND j.state='complete' LIMIT 1`, id, r.PathValue("id")).Scan(&key)
	if err != nil {
		a.failure(w, err)
		return
	}
	if key == "" || filepath.Base(key) != key {
		problem(w, 404, "Converted audio not found.")
		return
	}
	f, err := os.Open(filepath.Join(a.RecordingsDir, key))
	if err != nil {
		problem(w, 404, "Converted file unavailable. Ask an administrator to regenerate it.")
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
	http.ServeContent(w, r, "converted.mp3", info.ModTime(), f)
}
