package api

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (a *API) teams(w http.ResponseWriter, r *http.Request) {
	a.row(w, r, 200, `SELECT COALESCE(jsonb_agg(x ORDER BY x.name,x.id),'[]'::jsonb) FROM (SELECT t.id,t.name,COALESCE((SELECT jsonb_agg(a.id ORDER BY a.name,a.id) FROM recording_team_members m JOIN active_actors a ON a.id=m.actor_id WHERE m.team_id=t.id),'[]'::jsonb) AS actor_ids FROM recording_teams t)x`)
}
func (a *API) saveTeam(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string   `json:"name"`
		Actors []string `json:"actor_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validText(in.Name, 120) || len(in.Actors) < 2 || len(in.Actors) > 50 {
		problem(w, 400, "Name the team and choose 2–50 actors.")
		return
	}
	seen := map[string]bool{}
	for _, id := range in.Actors {
		if !validID(id) || seen[id] {
			problem(w, 400, "Choose each actor once.")
			return
		}
		seen[id] = true
	}
	id := r.PathValue("id")
	if id != "" && !validID(id) {
		problem(w, 400, "Choose a team.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := a.write(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM active_actors WHERE id=ANY($1::uuid[])", in.Actors).Scan(&count); err != nil {
			return err
		}
		if count != len(in.Actors) {
			return clientError{400, "One of these actors is no longer available."}
		}
		if id == "" {
			if err := tx.QueryRow(ctx, "INSERT INTO recording_teams(name) VALUES($1) RETURNING id::text", in.Name).Scan(&id); err != nil {
				return err
			}
		} else {
			result, err := tx.Exec(ctx, "UPDATE recording_teams SET name=$2 WHERE id=$1", id, in.Name)
			if err != nil {
				return err
			}
			if result.RowsAffected() == 0 {
				return pgx.ErrNoRows
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM recording_team_members WHERE team_id=$1", id); err != nil {
			return err
		}
		for _, actor := range in.Actors {
			if _, err := tx.Exec(ctx, "INSERT INTO recording_team_members(team_id,actor_id) VALUES($1,$2)", id, actor); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 200, map[string]any{"id": id, "name": in.Name, "actor_ids": in.Actors})
}
