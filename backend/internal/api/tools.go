package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Serialize workspace writes so removal, import and reorder cannot interleave.
// A transaction-scoped lock also coordinates multiple app instances.
func (a *API) write(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(738192042)"); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type clientError struct {
	status  int
	message string
}

func (e clientError) Error() string { return e.message }
func (a *API) toolError(w http.ResponseWriter, err error) {
	var ce clientError
	if errors.As(err, &ce) {
		problem(w, ce.status, ce.message)
	} else {
		a.failure(w, err)
	}
}
func (a *API) registerTools(mux *http.ServeMux) {
	mux.HandleFunc("DELETE /api/{kind}/{id}", a.remove)
	mux.HandleFunc("PUT /api/projects/{id}/scene-order", a.reorder)
	mux.HandleFunc("PUT /api/scenes/{id}/line-order", a.reorder)
	mux.HandleFunc("POST /api/import/preview", a.previewImport)
	mux.HandleFunc("POST /api/import", a.importScript)
}
func (a *API) remove(w http.ResponseWriter, r *http.Request) {
	id, kind := r.PathValue("id"), r.PathValue("kind")
	views := map[string]string{"projects": "active_projects", "scenes": "active_scenes", "events": "active_events", "actors": "active_actors", "characters": "active_characters"}
	view, ok := views[kind]
	if !ok {
		problem(w, 404, "Unknown item type.")
		return
	}
	if !validID(id) {
		problem(w, 400, "Invalid item ID.")
		return
	}
	var in struct {
		Confirm  bool `json:"confirm"`
		Revision int  `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		problem(w, 400, "Confirm removal first.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := a.write(ctx, func(tx pgx.Tx) error {
		var found bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+view+" WHERE id=$1)", id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return pgx.ErrNoRows
		}
		switch kind {
		case "characters":
			var used bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM active_events WHERE character_id=$1)", id).Scan(&used); err != nil {
				return err
			}
			if used {
				return clientError{409, "This character has dialogue. Reassign or remove those lines first."}
			}
			if _, err := tx.Exec(ctx, "DELETE FROM assignments WHERE character_id=$1", id); err != nil {
				return err
			}
		case "actors":
			if _, err := tx.Exec(ctx, "DELETE FROM assignments WHERE actor_id=$1", id); err != nil {
				return err
			}
		case "scenes":
			if _, err := tx.Exec(ctx, "UPDATE script_events SET deleted_at=now(),updated_at=now() WHERE scene_id=$1 AND deleted_at IS NULL", id); err != nil {
				return err
			}
		case "events":
			var revision int
			if err := tx.QueryRow(ctx, "SELECT revision FROM script_events WHERE id=$1", id).Scan(&revision); err != nil {
				return err
			}
			if in.Revision != revision {
				return clientError{409, "This line changed. Reload before deleting it."}
			}
		}
		_, err := tx.Exec(ctx, "UPDATE "+kindTable(kind)+" SET deleted_at=now() WHERE id=$1", id)
		return err
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 200, map[string]string{"status": "removed"})
}
func kindTable(kind string) string {
	if kind == "events" {
		return "script_events"
	}
	return kind
}

func (a *API) reorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs      []string `json:"ids"`
		Expected []string `json:"expected"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validID(r.PathValue("id")) || len(in.IDs) > 5000 || len(in.IDs) != len(in.Expected) {
		problem(w, 400, "Send the complete current and requested order.")
		return
	}
	seen := map[string]bool{}
	for _, id := range in.IDs {
		if !validID(id) || seen[id] {
			problem(w, 400, "Order must contain each item exactly once.")
			return
		}
		seen[id] = true
	}
	for _, id := range in.Expected {
		if !validID(id) {
			problem(w, 400, "Invalid current order.")
			return
		}
	}
	table, view, parent, parentView := "scenes", "active_scenes", "project_id", "active_projects"
	if r.Pattern == "PUT /api/scenes/{id}/line-order" {
		table, view, parent, parentView = "script_events", "active_events", "scene_id", "active_scenes"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err := a.write(ctx, func(tx pgx.Tx) error {
		var found bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+parentView+" WHERE id=$1)", r.PathValue("id")).Scan(&found); err != nil {
			return err
		}
		if !found {
			return pgx.ErrNoRows
		}
		rows, err := tx.Query(ctx, "SELECT id::text FROM "+view+" WHERE "+parent+"=$1 ORDER BY position,id", r.PathValue("id"))
		if err != nil {
			return err
		}
		current, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if !slices.Equal(current, in.Expected) {
			return clientError{409, "The order changed. Reload and try again."}
		}
		for _, id := range current {
			if !seen[id] {
				return clientError{400, "Order must contain exactly the items in this list."}
			}
		}
		if len(current) == 0 || slices.Equal(current, in.IDs) {
			return nil
		}
		// Move to distinct temporary positive positions, avoiding uniqueness collisions.
		_, err = tx.Exec(ctx, fmt.Sprintf(`WITH positions AS (SELECT id,row_number() OVER (ORDER BY position,id) AS n FROM %s WHERE %s=$1 AND deleted_at IS NULL),
   maximum AS (SELECT COALESCE(max(position),0) AS n FROM %s WHERE %s=$1 AND deleted_at IS NULL)
   UPDATE %s t SET position=maximum.n+positions.n FROM positions,maximum WHERE t.id=positions.id`, table, parent, table, parent, table), r.PathValue("id"))
		if err != nil {
			return err
		}
		extra := ""
		if table == "script_events" {
			extra = ",revision=revision+1,updated_at=now()"
		}
		_, err = tx.Exec(ctx, "UPDATE "+table+" t SET position=o.n"+extra+" FROM unnest($1::uuid[]) WITH ORDINALITY o(id,n) WHERE t.id=o.id", in.IDs)
		return err
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 200, map[string]string{"status": "reordered"})
}
