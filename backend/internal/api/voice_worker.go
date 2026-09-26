package api

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Work is persisted before dispatch. Interrupted paid requests are deliberately
// not retried automatically: the provider may have charged for a lost response.
func (a *API) StartVoiceWorker(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !a.Eleven.Enabled() || a.RecordingsDir == "" {
			return
		}
		for ctx.Err() == nil {
			a.voiceWorkerSession(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
	return func() { cancel(); <-done }
}
func (a *API) voiceWorkerSession(ctx context.Context) {
	conn, err := a.DB.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(738192043)`).Scan(&locked); err != nil || !locked {
		return
	}
	// Always close the session to release a session-level advisory lock, including on cancellation.
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Conn().Close(c)
	}()
	_, err = conn.Exec(ctx, `UPDATE voice_jobs SET state='failed',error='Conversion interrupted. It may have used credits; check ElevenLabs before retrying.' WHERE state='running'`)
	if err != nil {
		return
	}
	_, err = conn.Exec(ctx, `UPDATE voice_runs SET state='failed',error='A conversion was interrupted. Completed lines were kept. Check ElevenLabs before retrying.' WHERE state IN ('queued','running') AND id IN(SELECT run_id FROM voice_jobs WHERE state='failed')`)
	if err != nil {
		return
	}
	for ctx.Err() == nil {
		var run, scene string
		err = conn.QueryRow(ctx, `SELECT id::text,scene_id::text FROM voice_runs WHERE state IN ('queued','running') ORDER BY created_at,id LIMIT 1`).Scan(&run, &scene)
		if errors.Is(err, pgx.ErrNoRows) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if err != nil {
			return
		}
		if err = a.workVoiceRun(ctx, conn, run, scene); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("voice queue failed", "run_id", run, "error", err)
			// Leave durable state for conservative recovery after a DB/session failure.
			return
		}
	}
}
func (a *API) failVoiceRun(ctx context.Context, c *pgxpool.Conn, run, job, message string) error {
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if job != "" {
		if _, err = tx.Exec(ctx, `UPDATE voice_jobs SET state='failed',error=$2 WHERE id=$1`, job, message); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE voice_runs SET state='failed',error=$2 WHERE id=$1`, run, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (a *API) workVoiceRun(ctx context.Context, c *pgxpool.Conn, run, scene string) error {
	var active bool
	if err := c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM active_scenes WHERE id=$1)`, scene).Scan(&active); err != nil {
		return err
	}
	if !active {
		return a.failVoiceRun(ctx, c, run, "", "Scene was removed. No further lines were sent.")
	}
	if _, err := c.Exec(ctx, `UPDATE voice_runs SET state='running' WHERE id=$1`, run); err != nil {
		return err
	}
	var job, voice, source string
	err := c.QueryRow(ctx, `SELECT id::text,voice_id,source_key FROM voice_jobs WHERE run_id=$1 AND state='queued' ORDER BY position,id LIMIT 1`, run).Scan(&job, &voice, &source)
	if err == nil {
		// Mark dispatch durably before contacting ElevenLabs. No automatic paid retry.
		if _, err = c.Exec(ctx, `UPDATE voice_jobs SET state='running' WHERE id=$1 AND state='queued'`, job); err != nil {
			return err
		}
		key := "converted-" + job + ".mp3"
		convertCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		conversionErr := a.convertVoiceLine(convertCtx, voice, source, key)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if conversionErr != nil {
			return a.failVoiceRun(ctx, c, run, job, conversionErr.Error())
		}
		_, err = c.Exec(ctx, `UPDATE voice_jobs SET state='complete',audio_key=$2,completed_at=now() WHERE id=$1`, job, key)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	rows, err := c.Query(ctx, `SELECT event_id::text,audio_key FROM voice_jobs WHERE run_id=$1 AND state='complete' ORDER BY position,id`, run)
	if err != nil {
		return err
	}
	lines := []previewLine{}
	for rows.Next() {
		var l previewLine
		if err = rows.Scan(&l.ID, &l.Source); err != nil {
			break
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(lines) == 0 {
		return a.failVoiceRun(ctx, c, run, "", "No converted lines were available to compile.")
	}
	key := "voiced-scene-" + run + ".mp3"
	renderCtx, cancel := context.WithTimeout(ctx, 80*time.Second)
	err = a.compileScene(renderCtx, a.RecordingsDir, filepath.Join(a.RecordingsDir, key), lines)
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return a.failVoiceRun(ctx, c, run, "", "Converted lines are saved, but scene assembly failed. Check storage or scene length, then use Voice scene to retry assembly.")
	}
	_, err = c.Exec(ctx, `UPDATE voice_runs SET state='complete',audio_key=$2,error='' WHERE id=$1`, run, key)
	return err
}
func (a *API) convertVoiceLine(ctx context.Context, voice, source, key string) error {
	if source == "" || filepath.Base(source) != source {
		return errors.New("Original recording is unavailable.")
	}
	dir, err := os.MkdirTemp(a.RecordingsDir, ".convert-")
	if err != nil {
		return errors.New("Recording storage is not writable.")
	}
	defer os.RemoveAll(dir)
	// Normalize a copy for provider compatibility. The original source is untouched.
	wav := filepath.Join(dir, "input.wav")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-protocol_whitelist", "file,pipe", "-format_whitelist", "matroska,webm,mov,wav,ogg", "-i", filepath.Join(a.RecordingsDir, source), "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
	if err = cmd.Run(); err != nil {
		return errors.New("Original recording could not be prepared. Check the recording volume.")
	}
	output := filepath.Join(dir, "converted.mp3")
	if err = a.Eleven.Convert(ctx, voice, wav, output); err != nil {
		return err
	}
	if _, err = inspectAudio(ctx, output); err != nil {
		return errors.New("ElevenLabs returned invalid or oversized audio. The request may have used credits; check before retrying.")
	}
	if err = os.Rename(output, filepath.Join(a.RecordingsDir, key)); err != nil {
		return errors.New("Converted audio could not be installed. Check storage; the request may have used credits.")
	}
	if f, e := os.Open(a.RecordingsDir); e == nil {
		_ = f.Sync()
		_ = f.Close()
	}
	return nil
}
