package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
)

func (a *API) registerRecording(m *http.ServeMux) {
	m.HandleFunc("POST /api/actor/scenes/{id}/preview", a.renderScene)
	m.HandleFunc("GET /api/actor/scenes/{id}/preview/{key}", a.sceneAudio)
	m.HandleFunc("GET /api/session", a.session)
	m.HandleFunc("GET /api/actor-logins", a.actorLogins)
	m.HandleFunc("PUT /api/actors/{id}/login", a.setActorLogin)
	m.HandleFunc("GET /api/actor/workspace", a.actorWorkspace)
	m.HandleFunc("GET /api/actor/takes", a.listTakes)
	m.HandleFunc("POST /api/actor/events/{id}/takes", a.uploadTake)
	m.HandleFunc("PUT /api/actor/takes/{id}/preferred", a.preferTake)
	m.HandleFunc("GET /api/actor/takes/{id}/audio", a.takeAudio)
}

func (a *API) actorWorkspace(w http.ResponseWriter, r *http.Request) {
	u := identity.Current(r.Context())
	a.row(w, r, 200, `WITH my_scenes AS (SELECT DISTINCT e.scene_id FROM active_events e JOIN active_assignments a ON a.character_id=e.character_id WHERE a.actor_id=$1)
 SELECT jsonb_build_object(
 'projects',COALESCE((SELECT jsonb_agg(p ORDER BY p.created_at,p.id) FROM active_projects p WHERE p.id IN(SELECT project_id FROM active_scenes WHERE id IN(SELECT scene_id FROM my_scenes))),'[]'::jsonb),
 'scenes',COALESCE((SELECT jsonb_agg(s ORDER BY s.position,s.id) FROM active_scenes s WHERE s.id IN(SELECT scene_id FROM my_scenes)),'[]'::jsonb),
 'characters',COALESCE((SELECT jsonb_agg(c ORDER BY c.name,c.id) FROM active_characters c WHERE c.id IN(SELECT character_id FROM active_events WHERE scene_id IN(SELECT scene_id FROM my_scenes))),'[]'::jsonb),
 'events',COALESCE((SELECT jsonb_agg(e ORDER BY e.position,e.id) FROM active_events e WHERE e.scene_id IN(SELECT scene_id FROM my_scenes)),'[]'::jsonb),
 'assignments',COALESCE((SELECT jsonb_agg(a) FROM active_assignments a WHERE a.actor_id=$1),'[]'::jsonb), 'actors','[]'::jsonb)`, nullableID(u.ActorID))
}
func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

const takeSelect = `SELECT t.id,t.event_id,t.actor_id,t.revision,t.take_number,t.preferred,t.created_at,
 a.duration_ms,a.mime_type,a.size_bytes,a.sha256,r.text,r.direction,r.character_id,
 c.name AS character_name,p.name AS project_name,s.name AS scene_name,
 t.revision<>e.revision AS stale,actor.name AS actor_name
 FROM takes t JOIN audio_assets a ON a.id=t.asset_id
 JOIN script_event_revisions r ON r.event_id=t.event_id AND r.revision=t.revision
 JOIN active_events e ON e.id=t.event_id JOIN characters c ON c.id=r.character_id
 JOIN active_scenes s ON s.id=e.scene_id JOIN active_projects p ON p.id=e.project_id JOIN actors actor ON actor.id=t.actor_id`

func (a *API) listTakes(w http.ResponseWriter, r *http.Request) {
	u := identity.Current(r.Context())
	a.row(w, r, 200, `SELECT COALESCE(jsonb_agg(x ORDER BY x.created_at DESC,x.id),'[]'::jsonb) FROM (`+takeSelect+` WHERE ($1::boolean OR t.actor_id=$2)) x`, u.Admin, nullableID(u.ActorID))
}
func (a *API) uploadTake(w http.ResponseWriter, r *http.Request) {
	u := identity.Current(r.Context())
	if u.ActorID == "" {
		problem(w, 403, "Link your account to an actor before recording.")
		return
	}
	if a.RecordingsDir == "" {
		problem(w, 503, "Recording storage needs to be configured in Unraid.")
		return
	}
	event := r.PathValue("id")
	revision, err := strconv.Atoi(r.URL.Query().Get("revision"))
	requestID := r.Header.Get("X-Upload-ID")
	if !validID(event) || err != nil || revision < 1 || len(requestID) != 32 || strings.Trim(requestID, "0123456789abcdef") != "" {
		problem(w, 400, "Provide a line, its revision, and a valid upload ID.")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	extensions := map[string]string{"audio/webm": ".webm", "audio/mp4": ".m4a", "audio/ogg": ".ogg", "audio/wav": ".wav", "audio/x-wav": ".wav"}
	ext, ok := extensions[contentType]
	if err != nil || !ok {
		problem(w, 415, "Record audio in WebM, MP4, Ogg, or WAV format.")
		return
	}
	// Authorize before accepting a potentially large upload; recheck inside the commit.
	var permitted bool
	err = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM active_events e JOIN active_assignments a ON a.character_id=e.character_id WHERE e.id=$1 AND a.actor_id=$2)`, event, u.ActorID).Scan(&permitted)
	if err != nil {
		a.failure(w, err)
		return
	}
	if !permitted {
		problem(w, 403, "This line is not assigned to you.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes)
	f, err := os.CreateTemp(a.RecordingsDir, ".upload-")
	if err != nil {
		problem(w, 503, "Recording storage is unavailable. Your local take is still here.")
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), r.Body)
	if err != nil {
		problem(w, 413, "The upload did not finish or exceeded 25 MB. Your local take is still here.")
		return
	}
	if size == 0 {
		problem(w, 400, "The recording is empty.")
		return
	}
	if err = f.Sync(); err != nil {
		problem(w, 503, "Unable to save the audio file.")
		return
	}
	if err = f.Close(); err != nil {
		problem(w, 503, "Unable to close the audio file.")
		return
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	info, err := inspectAudio(r.Context(), tmp)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		problem(w, 503, "Unable to create recording ID.")
		return
	}
	key := hex.EncodeToString(token) + ext
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var result json.RawMessage
	err = a.write(ctx, func(tx pgx.Tx) error {
		var oldEvent, oldDigest string
		var oldRevision int
		e := tx.QueryRow(ctx, `SELECT t.event_id::text,t.revision,a.sha256,row_to_json(t) FROM takes t JOIN audio_assets a ON a.id=t.asset_id WHERE t.actor_id=$1 AND t.request_id=$2`, u.ActorID, requestID).Scan(&oldEvent, &oldRevision, &oldDigest, &result)
		if e == nil {
			if oldEvent != event || oldRevision != revision || oldDigest != digest {
				return clientError{409, "That upload ID was already used for another take."}
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var current int
		if e = tx.QueryRow(ctx, `SELECT e.revision FROM active_events e JOIN active_assignments a ON a.character_id=e.character_id WHERE e.id=$1 AND a.actor_id=$2`, event, u.ActorID).Scan(&current); e != nil {
			return clientError{409, "The line or assignment changed. Download your take before reloading."}
		}
		if current != revision {
			return clientError{409, "The script changed while you recorded. Download this take, then reload the current line."}
		}
		var asset string
		if e = tx.QueryRow(ctx, `INSERT INTO audio_assets(storage_key,sha256,mime_type,size_bytes,duration_ms,sample_rate,channels) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, key, digest, contentType, size, info.Duration, info.Rate, info.Channels).Scan(&asset); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `INSERT INTO takes(event_id,revision,actor_id,asset_id,take_number,preferred,request_id)
   SELECT $1,$2,$3,$4,COALESCE(max(take_number),0)+1,NOT COALESCE(bool_or(preferred),false),$5 FROM takes WHERE event_id=$1 AND actor_id=$3 RETURNING row_to_json(takes)`, event, revision, u.ActorID, asset, requestID).Scan(&result); e != nil {
			return e
		}
		// Install the durable source before committing metadata. Never delete an installed
		// source on an ambiguous commit error: a retry uses the upload ID, not a new take.
		if e = os.Rename(tmp, filepath.Join(a.RecordingsDir, key)); e != nil {
			return e
		}
		if dir, e := os.Open(a.RecordingsDir); e == nil {
			_ = dir.Sync()
			_ = dir.Close()
		}
		return nil
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 201, result)
}
func (a *API) preferTake(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Preferred bool `json:"preferred"`
	}
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 400, "Invalid take.")
		return
	}
	u := identity.Current(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := a.write(ctx, func(tx pgx.Tx) error {
		var event, actor string
		if err := tx.QueryRow(ctx, `SELECT t.event_id::text,t.actor_id::text FROM takes t JOIN active_events e ON e.id=t.event_id WHERE t.id=$1 AND ($2::boolean OR t.actor_id=$3)`, id, u.Admin, nullableID(u.ActorID)).Scan(&event, &actor); err != nil {
			return err
		}
		if in.Preferred {
			if _, err := tx.Exec(ctx, `UPDATE takes SET preferred=false WHERE event_id=$1 AND actor_id=$2 AND preferred`, event, actor); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE takes SET preferred=$2 WHERE id=$1`, id, in.Preferred)
		return err
	})
	if err != nil {
		a.failure(w, err)
		return
	}
	JSON(w, 200, map[string]string{"status": "saved"})
}
func (a *API) takeAudio(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
		problem(w, 404, "Take not found.")
		return
	}
	u := identity.Current(r.Context())
	var key, contentType string
	err := a.DB.QueryRow(r.Context(), `SELECT a.storage_key,a.mime_type FROM takes t JOIN audio_assets a ON a.id=t.asset_id JOIN active_events e ON e.id=t.event_id WHERE t.id=$1 AND ($2::boolean OR t.actor_id=$3)`, r.PathValue("id"), u.Admin, nullableID(u.ActorID)).Scan(&key, &contentType)
	if err != nil {
		a.failure(w, err)
		return
	}
	if a.RecordingsDir == "" || filepath.Base(key) != key {
		problem(w, 503, "Recording storage is unavailable.")
		return
	}
	f, err := os.Open(filepath.Join(a.RecordingsDir, key))
	if err != nil {
		problem(w, 503, "The audio file is unavailable. Check the recording volume.")
		return
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		problem(w, 503, "Cannot read audio file.")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", `inline; filename="take`+filepath.Ext(key)+`"`)
	http.ServeContent(w, r, key, s.ModTime(), f)
}
