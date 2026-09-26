package api

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
	"net/http"
	"strings"
	"time"
)

func (a *API) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if a.Auth == nil {
			problem(w, 503, "Set the Cloudflare login configuration in Unraid to open StoryForge.")
			return
		}
		u, err := a.Auth.Identify(r)
		if err != nil {
			problem(w, 401, err.Error())
			return
		}
		if !a.Auth.SameOrigin(r) {
			problem(w, 403, "Open StoryForge directly to make changes.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		err = a.DB.QueryRow(ctx, `SELECT a.id::text,a.name FROM actor_logins l JOIN active_actors a ON a.id=l.actor_id WHERE l.email=$1`, u.Email).Scan(&u.ActorID, &u.Name)
		cancel()
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			a.failure(w, err)
			return
		}
		if r.URL.Path != "/api/session" && !strings.HasPrefix(r.URL.Path, "/api/actor/") && !u.Admin {
			problem(w, 403, "Administrator access is required.")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/actor/") && !u.Admin && u.ActorID == "" {
			problem(w, 403, "Your account needs to be linked to an actor. Ask Dad to set your login email.")
			return
		}
		next.ServeHTTP(w, r.WithContext(identity.WithUser(r.Context(), u)))
	})
}
func (a *API) session(w http.ResponseWriter, r *http.Request) {
	JSON(w, 200, map[string]any{"user": identity.Current(r.Context()), "recording_enabled": a.RecordingsDir != ""})
}
func (a *API) actorLogins(w http.ResponseWriter, r *http.Request) {
	a.row(w, r, 200, `SELECT COALESCE(jsonb_agg(l),'[]'::jsonb) FROM actor_logins l JOIN active_actors a ON a.id=l.actor_id`)
}
func (a *API) setActorLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	id := r.PathValue("id")
	if !validID(id) || (in.Email != "" && !identity.Email(in.Email)) {
		problem(w, 400, "Enter a valid Google login email, or leave it blank to unlink.")
		return
	}
	if in.Email == "" {
		a.row(w, r, 200, `WITH removed AS (DELETE FROM actor_logins WHERE actor_id=$1) SELECT '{}'::json`, id)
		return
	}
	a.row(w, r, 200, `INSERT INTO actor_logins(actor_id,email) SELECT id,$2 FROM active_actors WHERE id=$1 ON CONFLICT(actor_id) DO UPDATE SET email=EXCLUDED.email RETURNING row_to_json(actor_logins)`, id, in.Email)
}
