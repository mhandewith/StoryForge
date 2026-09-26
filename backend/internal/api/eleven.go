package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type ElevenVoice struct {
	ID   string `json:"voice_id"`
	Name string `json:"name"`
}
type ElevenClient struct {
	key, base string
	http      *http.Client
	mu        sync.Mutex
	voices    []ElevenVoice
	fetched   time.Time
}

func NewElevenClient() (*ElevenClient, error) {
	base := "https://api.elevenlabs.io"
	if test := os.Getenv("ELEVENLABS_TEST_URL"); test != "" {
		if os.Getenv("STORYFORGE_AUTH_MODE") != "development" {
			return nil, errors.New("ELEVENLABS_TEST_URL is only allowed with local development authentication")
		}
		base = strings.TrimRight(test, "/")
	}
	return &ElevenClient{key: strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")), base: base, http: &http.Client{Timeout: 4 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *ElevenClient) Enabled() bool { return c != nil && c.key != "" }
func (c *ElevenClient) Voices(ctx context.Context, refresh bool) ([]ElevenVoice, error) {
	if !c.Enabled() {
		return nil, errors.New("Set ELEVENLABS_API_KEY in Unraid to connect your voice library.")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !refresh && !c.fetched.IsZero() && time.Since(c.fetched) < 10*time.Minute {
		return append([]ElevenVoice{}, c.voices...), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	voices := []ElevenVoice{}
	token := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		endpoint := c.base + "/v2/voices?page_size=100"
		if token != "" {
			endpoint += "&next_page_token=" + url.QueryEscape(token)
		}
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, errors.New("Invalid voice service configuration.")
		}
		req.Header.Set("xi-api-key", c.key)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, errors.New("Could not refresh ElevenLabs voices. Try again shortly.")
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return nil, elevenStatus(res.StatusCode)
		}
		var result struct {
			Voices  []ElevenVoice `json:"voices"`
			HasMore bool          `json:"has_more"`
			Next    string        `json:"next_page_token"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&result)
		res.Body.Close()
		if err != nil {
			return nil, errors.New("ElevenLabs returned an unreadable voice list.")
		}
		for _, v := range result.Voices {
			if v.ID != "" && v.Name != "" {
				voices = append(voices, v)
			}
		}
		if !result.HasMore {
			sort.Slice(voices, func(i, j int) bool { return voices[i].Name < voices[j].Name })
			c.voices = voices
			c.fetched = time.Now()
			return append([]ElevenVoice{}, voices...), nil
		}
		if result.Next == "" || seen[result.Next] {
			return nil, errors.New("ElevenLabs voice pagination was incomplete. Try refresh again.")
		}
		token = result.Next
		seen[token] = true
	}
	return nil, errors.New("Voice library exceeds the supported size.")
}
func elevenStatus(code int) error {
	switch code {
	case 401, 403:
		return errors.New("ElevenLabs rejected access. Check your API key and its Voice Changer/Voices permissions.")
	case 429:
		return errors.New("ElevenLabs is rate limited or out of credits. Check your account before retrying.")
	case 404, 422:
		return errors.New("ElevenLabs could not use the selected voice or recording. Refresh voices and check the assignment.")
	}
	return fmt.Errorf("ElevenLabs returned HTTP %d. Check your account before retrying; the request may have used credits.", code)
}
func (c *ElevenClient) Convert(ctx context.Context, voice, source, destination string) error {
	if !c.Enabled() {
		return errors.New("ElevenLabs is not configured.")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("audio", "performance.wav")
	if err != nil {
		return err
	}
	f, err := os.Open(source)
	if err != nil {
		return errors.New("Original recording is unavailable.")
	}
	n, err := io.Copy(part, io.LimitReader(f, 50<<20))
	f.Close()
	if err != nil || n >= 50<<20 {
		return errors.New("Recording could not be prepared for conversion.")
	}
	_ = form.WriteField("model_id", "eleven_multilingual_sts_v2")
	// Explicit settings make the cache independent of external saved voice settings.
	_ = form.WriteField("voice_settings", `{"stability":0.5,"similarity_boost":0.75,"style":0,"use_speaker_boost":true}`)
	_ = form.WriteField("remove_background_noise", "false")
	_ = form.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/v1/speech-to-speech/"+url.PathEscape(voice)+"?output_format=mp3_44100_128", &body)
	if err != nil {
		return errors.New("Invalid conversion configuration.")
	}
	req.Header.Set("xi-api-key", c.key)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("Conversion was interrupted. It may have used credits; check ElevenLabs before retrying.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return elevenStatus(res.StatusCode)
	}
	out, err := os.Create(destination)
	if err != nil {
		return errors.New("Cannot save converted audio. Check recording storage.")
	}
	n, err = io.Copy(out, io.LimitReader(res.Body, maxAudioBytes+1))
	syncErr := out.Sync()
	closeErr := out.Close()
	if err != nil || syncErr != nil || closeErr != nil || n == 0 || n > maxAudioBytes {
		return errors.New("Converted audio did not save completely. The request may have used credits.")
	}
	return nil
}
