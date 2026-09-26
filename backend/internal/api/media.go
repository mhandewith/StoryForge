package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const maxAudioBytes = 25 * 1024 * 1024

func CheckStorage(dir string) error {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".storage-check-")
	if err != nil {
		return errors.New("folder is not writable by container user 10001:10001")
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	_, err = exec.LookPath("ffprobe")
	return err
}

type audioInfo struct {
	Duration int
	Rate     int
	Channels int
}

func inspectAudio(ctx context.Context, path string) (audioInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Restrict demuxers/protocols: uploaded files must not fetch external resources.
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "matroska,webm,mov,wav,ogg,mp3", "-show_entries", "stream=codec_type,codec_name,sample_rate,channels:packet=pts_time,duration_time", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return audioInfo{}, errors.New("That file could not be read as audio. Keep the recording and try again.")
	}
	var report struct {
		Streams []struct {
			Type     string `json:"codec_type"`
			Codec    string `json:"codec_name"`
			Rate     string `json:"sample_rate"`
			Channels int    `json:"channels"`
		} `json:"streams"`
		Packets []struct {
			PTS      string `json:"pts_time"`
			Duration string `json:"duration_time"`
		} `json:"packets"`
	}
	if json.Unmarshal(out, &report) != nil || len(report.Streams) != 1 || report.Streams[0].Type != "audio" {
		return audioInfo{}, errors.New("Upload a single audio recording without video.")
	}
	s := report.Streams[0]
	rate, _ := strconv.Atoi(s.Rate)
	if rate < 8000 || rate > 192000 || s.Channels < 1 || s.Channels > 2 {
		return audioInfo{}, errors.New("Use a mono or stereo microphone recording.")
	}
	start, end := math.Inf(1), float64(0)
	for _, p := range report.Packets {
		pts, e := strconv.ParseFloat(p.PTS, 64)
		d, e2 := strconv.ParseFloat(p.Duration, 64)
		if e != nil {
			continue
		}
		if e2 != nil {
			d = 0
		}
		start = math.Min(start, pts)
		end = math.Max(end, pts+d)
	}
	duration := (end - start) * 1000
	if math.IsInf(duration, 0) || math.IsNaN(duration) || duration < 1 || duration > 300000 {
		return audioInfo{}, errors.New("A take must be between a moment and five minutes long.")
	}
	return audioInfo{int(math.Ceil(duration)), rate, s.Channels}, nil
}
