package api

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
)

// Scan stereo PCM so opposite-phase channels cannot cancel out quiet speech.
// A conservative -50 dBFS peak threshold and 100 ms handles retain breaths and
// soft consonants. Only edges are removed; internal pauses are never shortened.
func speechBounds(pcm []byte) (start, end int) {
	const frameBytes = 4 // two 16-bit channels at 16 kHz
	frames := len(pcm) / frameBytes
	first, last := frames, -1
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < 2; channel++ {
			i := frame*frameBytes + channel*2
			v := int(int16(binary.LittleEndian.Uint16(pcm[i : i+2])))
			if v > 103 || v < -103 {
				if frame < first {
					first = frame
				}
				last = frame
			}
		}
	}
	// Keep silence-only/very quiet results intact rather than creating empty audio.
	if last < 0 {
		return 0, frames
	}
	return max(0, first-1600), min(frames, last+1+1600)
}

func trimProcessedAudio(ctx context.Context, source, destination string) error {
	pcm, err := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "wav,mp3,ogg,matroska,webm,mov", "-i", source, "-t", "300", "-vn", "-ar", "16000", "-ac", "2", "-f", "s16le", "pipe:1").Output()
	if err != nil {
		return fmt.Errorf("decode processed audio: %w", err)
	}
	start, end := speechBounds(pcm)
	if end <= start {
		return fmt.Errorf("processed audio has no samples")
	}
	filter := fmt.Sprintf("atrim=start=%.6f:end=%.6f,asetpts=PTS-STARTPTS", float64(start)/16000, float64(end)/16000)
	return exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-i", source, "-af", filter, "-c:a", "libmp3lame", "-b:a", "128k", destination).Run()
}
