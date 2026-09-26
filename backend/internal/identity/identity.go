package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type User struct {
	Email   string `json:"email"`
	Admin   bool   `json:"admin"`
	ActorID string `json:"actor_id"`
	Name    string `json:"name"`
}
type contextKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}
func Current(ctx context.Context) User { u, _ := ctx.Value(contextKey{}).(User); return u }

type Auth struct {
	verifier *oidc.IDTokenVerifier
	admins   map[string]bool
	origin   string
	devToken string
}

func Email(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && len(s) <= 254
}

func FromEnv() (*Auth, error) {
	origin := strings.TrimRight(os.Getenv("STORYFORGE_PUBLIC_ORIGIN"), "/")
	team := strings.TrimRight(os.Getenv("CF_ACCESS_TEAM_DOMAIN"), "/")
	aud := os.Getenv("CF_ACCESS_AUD")
	if origin == "" && team == "" && aud == "" && os.Getenv("STORYFORGE_AUTH_MODE") != "development" {
		return nil, nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("STORYFORGE_PUBLIC_ORIGIN must be the site's origin, without a path")
	}
	a := &Auth{origin: origin, admins: map[string]bool{}}
	for _, email := range strings.Split(os.Getenv("STORYFORGE_ADMIN_EMAILS"), ",") {
		email = strings.ToLower(strings.TrimSpace(email))
		if !Email(email) {
			return nil, errors.New("set STORYFORGE_ADMIN_EMAILS to your administrator email")
		}
		a.admins[email] = true
	}
	if os.Getenv("STORYFORGE_AUTH_MODE") == "development" {
		if u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
			return nil, errors.New("development authentication requires a localhost public origin")
		}
		a.devToken = os.Getenv("STORYFORGE_DEV_AUTH_TOKEN")
		if len(a.devToken) < 32 {
			return nil, errors.New("development authentication requires a random token of at least 32 characters")
		}
		return a, nil
	}
	if u.Scheme != "https" {
		return nil, errors.New("public origin must use HTTPS")
	}
	t, err := url.Parse(team)
	if err != nil || t.Scheme != "https" || !strings.HasSuffix(t.Hostname(), ".cloudflareaccess.com") || t.Host != t.Hostname() || t.Path != "" || t.User != nil || t.RawQuery != "" || t.Fragment != "" || aud == "" {
		return nil, errors.New("configure CF_ACCESS_TEAM_DOMAIN as https://TEAM.cloudflareaccess.com and CF_ACCESS_AUD")
	}
	ctx := oidc.ClientContext(context.Background(), &http.Client{Timeout: 5 * time.Second})
	a.verifier = oidc.NewVerifier(team, oidc.NewRemoteKeySet(ctx, team+"/cdn-cgi/access/certs"), &oidc.Config{ClientID: aud, SupportedSigningAlgs: []string{"RS256"}})
	return a, nil
}
func (a *Auth) Identify(r *http.Request) (User, error) {
	email := ""
	if a.devToken != "" {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.devToken)) != 1 {
			return User{}, errors.New("sign in required")
		}
		email = r.Header.Get("X-StoryForge-Dev-Email")
	} else {
		token := r.Header.Get("Cf-Access-Jwt-Assertion")
		if token == "" || len(token) > 16384 {
			return User{}, errors.New("sign in through the secure StoryForge address")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()
		verified, err := a.verifier.Verify(ctx, token)
		if err != nil {
			return User{}, errors.New("your login expired; sign in again")
		}
		var claims struct {
			Email string `json:"email"`
		}
		if verified.Claims(&claims) != nil {
			return User{}, errors.New("invalid identity")
		}
		email = claims.Email
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !Email(email) {
		return User{}, errors.New("login must include an email address")
	}
	return User{Email: email, Admin: a.admins[email]}, nil
}
func (a *Auth) SameOrigin(r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return true
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == a.origin
}
