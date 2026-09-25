package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type importCast struct {
	Name  string `json:"name"`
	Actor string `json:"actor"`
}
type importLine struct {
	Character  string `json:"character"`
	Text       string `json:"text"`
	Direction  string `json:"direction"`
	SourceLine int    `json:"source_line"`
}
type importScene struct {
	Name  string       `json:"name"`
	Lines []importLine `json:"lines"`
}
type importPlan struct {
	Name      string        `json:"name"`
	Cast      []importCast  `json:"cast"`
	Scenes    []importScene `json:"scenes"`
	LineCount int           `json:"line_count"`
}

// ParseImport is deliberately small: metadata tags occupy their own lines;
// dialogue keeps its line breaks until the next tag. Unknown tags fail loudly.
func ParseImport(source string) (importPlan, error) {
	plan := importPlan{Cast: []importCast{}, Scenes: []importScene{}}
	if len(source) > 512*1024 || !utf8.ValidString(source) || strings.ContainsRune(source, 0) {
		return plan, errors.New("Use a UTF-8 script no larger than 512 KB.")
	}
	names := map[string]bool{}
	declared := map[string]bool{}
	var current *importLine
	fail := func(n int, message string) error { return fmt.Errorf("Line %d: %s", n, message) }
	flush := func() error {
		if current == nil {
			return nil
		}
		current.Text = strings.TrimSpace(current.Text)
		if !validText(current.Text, 10000) {
			return fail(current.SourceLine, "dialogue must contain 1–10000 characters.")
		}
		i := len(plan.Scenes) - 1
		plan.Scenes[i].Lines = append(plan.Scenes[i].Lines, *current)
		plan.LineCount++
		current = nil
		if plan.LineCount > 2000 {
			return errors.New("Import at most 2000 dialogue lines at once.")
		}
		return nil
	}
	for i, raw := range strings.Split(strings.ReplaceAll(strings.TrimPrefix(source, "\uFEFF"), "\r\n", "\n"), "\n") {
		n := i + 1
		s := strings.TrimSpace(raw)
		if strings.HasPrefix(s, "[") {
			if !strings.HasSuffix(s, "]") {
				return plan, fail(n, "put each tag on its own line, inside [brackets].")
			}
			tag := strings.TrimSpace(s[1 : len(s)-1])
			key, value, hasValue := strings.Cut(tag, ":")
			if hasValue {
				key = strings.ToLower(strings.TrimSpace(key))
				value = strings.TrimSpace(value)
				switch key {
				case "script":
					if plan.Name != "" || len(plan.Scenes) > 0 || len(plan.Cast) > 0 || !validText(value, 120) {
						return plan, fail(n, "start with one [script: Title] tag (1–120 characters).")
					}
					plan.Name = value
				case "cast":
					if plan.Name == "" || len(plan.Scenes) > 0 {
						return plan, fail(n, "put cast tags after the script title and before scenes.")
					}
					parts := strings.Split(value, "|")
					if len(parts) > 2 {
						return plan, fail(n, "use [cast: Character | Actor] or [cast: Character].")
					}
					name := strings.TrimSpace(parts[0])
					actor := ""
					if len(parts) == 2 {
						actor = strings.TrimSpace(parts[1])
						if !validText(actor, 120) {
							return plan, fail(n, "actor name must contain 1–120 characters.")
						}
					}
					if !validText(name, 120) || strings.ContainsAny(name, "[]:") {
						return plan, fail(n, "character names must be 1–120 characters without brackets or colons.")
					}
					if declared[name] {
						return plan, fail(n, "this character already has a cast tag.")
					}
					declared[name] = true
					names[name] = true
					plan.Cast = append(plan.Cast, importCast{name, actor})
				case "scene":
					if plan.Name == "" || !validText(value, 120) {
						return plan, fail(n, "use a script title, then [scene: Name] (1–120 characters).")
					}
					if err := flush(); err != nil {
						return plan, err
					}
					plan.Scenes = append(plan.Scenes, importScene{Name: value, Lines: []importLine{}})
					if len(plan.Scenes) > 200 {
						return plan, fail(n, "import at most 200 scenes.")
					}
				case "direction":
					if current == nil || current.Direction != "" || strings.TrimSpace(current.Text) != "" || !validText(value, 2000) {
						return plan, fail(n, "put one [direction: Note] after a character tag and before its dialogue.")
					}
					current.Direction = value
				default:
					return plan, fail(n, "unknown tag. Use script, cast, scene, direction, or [Character].")
				}
			} else {
				if plan.Name == "" || len(plan.Scenes) == 0 || !validText(tag, 120) || strings.ContainsAny(tag, "[]|") {
					return plan, fail(n, "put a [Character] tag after a scene, using a name of 1–120 characters.")
				}
				if err := flush(); err != nil {
					return plan, err
				}
				if !names[tag] {
					names[tag] = true
					plan.Cast = append(plan.Cast, importCast{Name: tag})
				}
				current = &importLine{Character: tag, SourceLine: n}
			}
		} else if current != nil {
			if strings.HasPrefix(s, `\[`) {
				raw = strings.Replace(raw, `\[`, `[`, 1)
			}
			current.Text += raw + "\n"
		} else if s != "" {
			return plan, fail(n, "dialogue needs a [Character] tag inside a scene.")
		}
		if len(plan.Cast) > 100 {
			return plan, fail(n, "import at most 100 characters.")
		}
	}
	if err := flush(); err != nil {
		return plan, err
	}
	if plan.Name == "" || len(plan.Scenes) == 0 || plan.LineCount == 0 {
		return plan, errors.New("Include a [script: Title], a [scene: Name], and at least one [Character] with dialogue.")
	}
	return plan, nil
}

type importRequest struct {
	Text      string `json:"text"`
	RequestID string `json:"request_id"`
}

func readImport(w http.ResponseWriter, r *http.Request) (importRequest, importPlan, bool) {
	var in importRequest
	if !decode(w, r, &in) {
		return in, importPlan{}, false
	}
	plan, err := ParseImport(in.Text)
	if err != nil {
		problem(w, 400, err.Error())
		return in, plan, false
	}
	return in, plan, true
}
func (a *API) previewImport(w http.ResponseWriter, r *http.Request) {
	_, plan, ok := readImport(w, r)
	if ok {
		JSON(w, 200, plan)
	}
}
func (a *API) importScript(w http.ResponseWriter, r *http.Request) {
	in, plan, ok := readImport(w, r)
	if !ok {
		return
	}
	if len(in.RequestID) != 32 || strings.Trim(in.RequestID, "0123456789abcdef") != "" {
		problem(w, 400, "Provide a valid import request ID.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var id string
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(in.Text)))
	err := a.write(ctx, func(tx pgx.Tx) error {
		var oldHash string
		var archived bool
		err := tx.QueryRow(ctx, `SELECT p.id::text,i.source_hash,p.deleted_at IS NOT NULL FROM script_imports i JOIN projects p ON p.id=i.project_id WHERE request_id=$1`, in.RequestID).Scan(&id, &oldHash, &archived)
		if err == nil {
			if archived || oldHash != hash {
				return clientError{409, "This import request was already used. Preview the script again."}
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = tx.QueryRow(ctx, "INSERT INTO projects(name) VALUES ($1) RETURNING id::text", plan.Name).Scan(&id); err != nil {
			return err
		}
		characters := map[string]string{}
		for _, cast := range plan.Cast {
			var characterID string
			if err = tx.QueryRow(ctx, "INSERT INTO characters(project_id,name) VALUES ($1,$2) RETURNING id::text", id, cast.Name).Scan(&characterID); err != nil {
				return err
			}
			characters[cast.Name] = characterID
			if cast.Actor == "" {
				continue
			}
			rows, err := tx.Query(ctx, "SELECT id::text FROM active_actors WHERE name=$1", cast.Actor)
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			if len(ids) > 1 {
				return clientError{409, "More than one actor is named " + cast.Actor + ". Resolve that duplicate or omit the actor from the cast tag."}
			}
			var actorID string
			if len(ids) == 1 {
				actorID = ids[0]
			} else if err = tx.QueryRow(ctx, "INSERT INTO actors(name) VALUES ($1) RETURNING id::text", cast.Actor).Scan(&actorID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO assignments(character_id,actor_id) VALUES ($1,$2)", characterID, actorID); err != nil {
				return err
			}
		}
		for i, scene := range plan.Scenes {
			var sceneID string
			if err = tx.QueryRow(ctx, "INSERT INTO scenes(project_id,name,position) VALUES ($1,$2,$3) RETURNING id::text", id, scene.Name, i+1).Scan(&sceneID); err != nil {
				return err
			}
			batch := &pgx.Batch{}
			for j, line := range scene.Lines {
				batch.Queue("INSERT INTO script_events(project_id,scene_id,character_id,text,direction,position) VALUES ($1,$2,$3,$4,$5,$6)", id, sceneID, characters[line.Character], line.Text, line.Direction, j+1)
			}
			if err = tx.SendBatch(ctx, batch).Close(); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "INSERT INTO script_imports(request_id,project_id,source_hash) VALUES ($1,$2,$3)", in.RequestID, id, hash)
		return err
	})
	if err != nil {
		a.toolError(w, err)
		return
	}
	JSON(w, 201, map[string]string{"id": id, "name": plan.Name})
}
