package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
)

type API struct {
	DB            *pgxpool.Pool
	Auth          *identity.Auth
	RecordingsDir string
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func problem(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		problem(w, 415, "Send application/json.")
		return false
	}
	limit := int64(64 * 1024)
	if strings.HasSuffix(r.URL.Path, "-order") {
		limit = 512 * 1024
	}
	if r.URL.Path == "/api/import" || r.URL.Path == "/api/import/preview" {
		limit = 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		problem(w, 400, "Invalid JSON or unsupported fields.")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		problem(w, 400, "Send exactly one JSON object.")
		return false
	}
	return true
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validID(s string) bool { return uuid.MatchString(s) }
func validText(s string, limit int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= limit
}

func (a *API) Register(mux *http.ServeMux) {
	a.registerRecording(mux)
	a.registerTools(mux)
	mux.HandleFunc("GET /api/workspace", a.workspace)
	mux.HandleFunc("POST /api/projects", a.createProject)
	mux.HandleFunc("POST /api/actors", a.createActor)
	mux.HandleFunc("POST /api/scenes", a.createScene)
	mux.HandleFunc("POST /api/characters", a.createCharacter)
	mux.HandleFunc("PUT /api/characters/{characterID}/target-voice", a.targetVoice)
	mux.HandleFunc("PUT /api/assignments/{characterID}", a.assign)
	mux.HandleFunc("POST /api/events", a.createEvent)
	mux.HandleFunc("PUT /api/events/{eventID}", a.updateEvent)
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.DB.Ping(ctx); err != nil {
			problem(w, 503, "Database unavailable.")
			return
		}
		JSON(w, 200, map[string]string{"status": "ok", "database": "connected"})
	})
	// Protect JSON endpoints against accidentally returning the frontend for unknown URLs.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "API endpoint not found.") })
}

func (a *API) failure(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Item not found.")
	case errors.As(err, &pgErr):
		switch pgErr.Code {
		case "23505":
			problem(w, 409, "That name or position is already in use. Choose another.")
		case "23503":
			problem(w, 400, "Choose existing items from the same project.")
		case "23514", "22P02", "22003":
			problem(w, 400, "One or more values are invalid.")
		default:
			slog.Error("database request failed", "error", err)
			problem(w, 503, "Database unavailable. Please try again.")
		}
	default:
		slog.Error("database request failed", "error", err)
		problem(w, 503, "Database unavailable. Please try again.")
	}
}

func (a *API) row(w http.ResponseWriter, r *http.Request, status int, query string, args ...any) {
	var data json.RawMessage
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var err error
	if r.Method == "GET" {
		err = a.DB.QueryRow(ctx, query, args...).Scan(&data)
	} else {
		err = a.write(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, query, args...).Scan(&data) })
	}
	if err != nil {
		a.failure(w, err)
		return
	}
	JSON(w, status, data)
}

// One statement gives the admin a consistent snapshot, including empty arrays.
func (a *API) workspace(w http.ResponseWriter, r *http.Request) {
	a.row(w, r, 200, `SELECT jsonb_build_object(
 'projects', COALESCE((SELECT jsonb_agg(p ORDER BY p.created_at,p.id) FROM active_projects p),'[]'::jsonb),
 'actors', COALESCE((SELECT jsonb_agg(a ORDER BY a.name,a.id) FROM active_actors a),'[]'::jsonb),
 'scenes', COALESCE((SELECT jsonb_agg(s ORDER BY s.position,s.id) FROM active_scenes s),'[]'::jsonb),
 'characters', COALESCE((SELECT jsonb_agg(c ORDER BY c.name,c.id) FROM active_characters c),'[]'::jsonb),
 'assignments', COALESCE((SELECT jsonb_agg(a ORDER BY a.character_id) FROM active_assignments a),'[]'::jsonb),
 'events', COALESCE((SELECT jsonb_agg(e ORDER BY e.position,e.id) FROM active_events e),'[]'::jsonb))`)
}

type named struct {
	Name string `json:"name"`
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var in named
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validText(in.Name, 120) {
		problem(w, 400, "Enter a project name of 1–120 characters.")
		return
	}
	a.row(w, r, 201, `INSERT INTO projects(name) VALUES ($1) RETURNING row_to_json(projects)`, in.Name)
}
func (a *API) createActor(w http.ResponseWriter, r *http.Request) {
	var in named
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validText(in.Name, 120) {
		problem(w, 400, "Enter an actor name of 1–120 characters.")
		return
	}
	a.row(w, r, 201, `INSERT INTO actors(name) VALUES ($1) RETURNING row_to_json(actors)`, in.Name)
}

type projectItem struct {
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Position  int    `json:"position"`
}

func (a *API) createScene(w http.ResponseWriter, r *http.Request) {
	var in projectItem
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validText(in.Name, 120) || !validID(in.ProjectID) || in.Position < 1 || in.Position > 2147483647 {
		problem(w, 400, "Enter a name, project, and positive scene position.")
		return
	}
	a.row(w, r, 201, `INSERT INTO scenes(project_id,name,position) SELECT id,$2,$3 FROM active_projects WHERE id=$1 RETURNING row_to_json(scenes)`, in.ProjectID, in.Name, in.Position)
}
func (a *API) createCharacter(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validText(in.Name, 120) || !validID(in.ProjectID) {
		problem(w, 400, "Enter a character name and project.")
		return
	}
	a.row(w, r, 201, `INSERT INTO characters(project_id,name) SELECT id,$2 FROM active_projects WHERE id=$1 RETURNING row_to_json(characters)`, in.ProjectID, in.Name)
}
func (a *API) targetVoice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetVoice string `json:"target_voice"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.TargetVoice = strings.TrimSpace(in.TargetVoice)
	if !validID(r.PathValue("characterID")) || utf8.RuneCountInString(in.TargetVoice) > 120 || strings.ContainsRune(in.TargetVoice, 0) {
		problem(w, 400, "Enter a target voice of up to 120 characters.")
		return
	}
	a.row(w, r, 200, `UPDATE characters SET target_voice=$2 WHERE id=$1 AND id IN (SELECT id FROM active_characters) RETURNING row_to_json(characters)`, r.PathValue("characterID"), in.TargetVoice)
}

func (a *API) assign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ActorID string `json:"actor_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validID(in.ActorID) || !validID(r.PathValue("characterID")) {
		problem(w, 400, "Choose a character and actor.")
		return
	}
	a.row(w, r, 200, `INSERT INTO assignments(character_id,actor_id) SELECT c.id,a.id FROM active_characters c CROSS JOIN active_actors a WHERE c.id=$1 AND a.id=$2
 ON CONFLICT (character_id) DO UPDATE SET actor_id=EXCLUDED.actor_id RETURNING row_to_json(assignments)`, r.PathValue("characterID"), in.ActorID)
}

type eventInput struct {
	ProjectID   string `json:"project_id"`
	SceneID     string `json:"scene_id"`
	CharacterID string `json:"character_id"`
	Text        string `json:"text"`
	Direction   string `json:"direction"`
	Position    int    `json:"position"`
	StartMS     int64  `json:"start_ms"`
	Revision    int    `json:"revision"`
}

func (in *eventInput) valid() bool {
	in.Text = strings.TrimSpace(in.Text)
	in.Direction = strings.TrimSpace(in.Direction)
	return validID(in.ProjectID) && validID(in.SceneID) && validID(in.CharacterID) && validText(in.Text, 10000) && utf8.RuneCountInString(in.Direction) <= 2000 && in.Position > 0 && in.Position <= 2147483647 && in.StartMS >= 0 && in.StartMS <= 86400000
}
func (a *API) createEvent(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if !decode(w, r, &in) {
		return
	}
	if !in.valid() || in.Revision != 0 {
		problem(w, 400, "Enter a character, dialogue, positive position, and timing from 0 to 86400000 ms.")
		return
	}
	a.row(w, r, 201, `INSERT INTO script_events(project_id,scene_id,character_id,text,direction,position,start_ms)
 SELECT $1,s.id,c.id,$4,$5,$6,$7 FROM active_scenes s CROSS JOIN active_characters c WHERE s.id=$2 AND c.id=$3
 RETURNING row_to_json(script_events)`, in.ProjectID, in.SceneID, in.CharacterID, in.Text, in.Direction, in.Position, in.StartMS)
}
func (a *API) updateEvent(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if !decode(w, r, &in) {
		return
	}
	if !in.valid() || in.Revision < 1 || !validID(r.PathValue("eventID")) {
		problem(w, 400, "Enter valid dialogue values and the current revision.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var data json.RawMessage
	err := a.write(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE script_events SET character_id=$1,text=$2,direction=$3,position=$4,start_ms=$5,revision=revision+1,updated_at=now()
 WHERE id=$6 AND project_id=$7 AND scene_id=$8 AND revision=$9 AND id IN (SELECT id FROM active_events)
 AND $1 IN (SELECT id FROM active_characters) RETURNING row_to_json(script_events)`, in.CharacterID, in.Text, in.Direction, in.Position, in.StartMS, r.PathValue("eventID"), in.ProjectID, in.SceneID, in.Revision).Scan(&data)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "This line changed or no longer exists. Reload before editing again.")
		return
	}
	if err != nil {
		a.failure(w, err)
		return
	}
	JSON(w, 200, data)
}
