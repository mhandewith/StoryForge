package api

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"time"
)

func (a *API) createDialogueGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EventIDs []string `json:"event_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	scene := r.PathValue("id")
	if !validID(scene) || len(in.EventIDs) < 2 || len(in.EventIDs) > 100 {
		problem(w, 400, "Choose two or more dialogue lines.")
		return
	}
	for _, id := range in.EventIDs {
		if !validID(id) {
			problem(w, 400, "Choose valid dialogue lines.")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var group string
	err := a.write(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT id::text,position FROM active_events WHERE scene_id=$1 AND id=ANY($2::uuid[]) ORDER BY position", scene, in.EventIDs)
		if err != nil {
			return err
		}
		type eventPosition struct {
			ID       string
			Position int
		}
		got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[eventPosition])
		if err != nil {
			return err
		}
		ids := make([]string, len(got))
		for i, v := range got {
			ids[i] = v.ID
			if i > 0 && v.Position != got[i-1].Position+1 {
				return clientError{400, "Dialogue groups must use adjacent lines."}
			}
		}
		sort.Strings(ids)
		want := append([]string(nil), in.EventIDs...)
		sort.Strings(want)
		if len(ids) != len(want) {
			return clientError{400, "Lines must belong to this scene and cannot already be grouped."}
		}
		if err = tx.QueryRow(ctx, "INSERT INTO dialogue_groups(scene_id) VALUES($1) RETURNING id::text", scene).Scan(&group); err != nil {
			return err
		}
		for _, id := range in.EventIDs {
			if _, err = tx.Exec(ctx, "INSERT INTO dialogue_group_members(group_id,event_id) VALUES($1,$2)", group, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 201, map[string]string{"id": group})
}
func (a *API) updateDialogueGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Offsets map[string]int `json:"offsets"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validID(r.PathValue("id")) {
		problem(w, 400, "Invalid group.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := a.write(ctx, func(tx pgx.Tx) error {
		for event, offset := range in.Offsets {
			if !validID(event) || offset < -300000 || offset > 300000 {
				return clientError{400, "Offsets must be between -300000 and 300000 ms."}
			}
			tag, err := tx.Exec(ctx, "UPDATE dialogue_group_members SET offset_ms=$3 WHERE group_id=$1 AND event_id=$2", r.PathValue("id"), event, offset)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return clientError{400, "That line is not in this group."}
			}
		}
		return nil
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 200, map[string]string{"status": "saved"})
}
func (a *API) dissolveDialogueGroup(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
		problem(w, 400, "Invalid group.")
		return
	}
	a.row(w, r, 200, "DELETE FROM dialogue_groups WHERE id=$1 RETURNING json_build_object('status','dissolved')", r.PathValue("id"))
}
func (a *API) saveTransition(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Predecessor string `json:"predecessor"`
		Successor   string `json:"successor"`
		OffsetMS    int    `json:"offset_ms"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validID(r.PathValue("id")) || in.OffsetMS < -300000 || in.OffsetMS > 300000 {
		problem(w, 400, "Choose a scene and an offset between -300000 and 300000 ms.")
		return
	}
	a.row(w, r, 200, "INSERT INTO dialogue_transitions(scene_id,predecessor,successor,offset_ms) VALUES($1,$2,$3,$4) ON CONFLICT(scene_id,predecessor,successor) DO UPDATE SET offset_ms=EXCLUDED.offset_ms RETURNING row_to_json(dialogue_transitions)", r.PathValue("id"), in.Predecessor, in.Successor, in.OffsetMS)
}
