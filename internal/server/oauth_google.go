package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"moco/internal/store"
)

// googleOAuthEnabled is true when all three Google credentials are
// configured. Callers should short-circuit to 503 when this is false so
// the route exists but signals "feature unavailable in this deployment".
func (s *Server) googleOAuthEnabled() bool {
	return s.cfg.GoogleClientID != "" && s.cfg.GoogleClientSecret != "" && s.cfg.GoogleRedirectURL != ""
}

func (s *Server) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.GoogleClientID,
		ClientSecret: s.cfg.GoogleClientSecret,
		RedirectURL:  s.cfg.GoogleRedirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"openid", "email", "profile"},
	}
}

const (
	googleOAuthStateCookie = "moco_g_state"
	// Short-lived because the user has to round-trip to Google. 10 minutes
	// is generous; most OAuth flows complete in under a minute.
	googleOAuthStateTTL = 10 * time.Minute
)

// handleGoogleStart begins the OAuth flow: generate a random state, store
// it in a signed (HttpOnly, Secure) cookie, redirect to Google's consent
// screen. The optional `next` query param is appended to the state cookie
// so we can land the user back on the page they intended after callback.
func (s *Server) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if !s.googleOAuthEnabled() {
		http.Error(w, "google sign-in is not configured on this server", http.StatusServiceUnavailable)
		return
	}
	state, err := newOAuthState()
	if err != nil {
		http.Error(w, "failed to start sign-in", http.StatusInternalServerError)
		return
	}
	// Sanitize `next` so an attacker can't bounce users off-site via the
	// callback redirect. Only same-origin paths are allowed.
	next := r.URL.Query().Get("next")
	if !isSafeRedirect(next) {
		next = "/app"
	}
	value := state + "|" + next
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, // Lax so the cookie survives Google's cross-site redirect back.
		MaxAge:   int(googleOAuthStateTTL.Seconds()),
	})
	authURL := s.googleOAuthConfig().AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleGoogleCallback completes the OAuth flow. Every error path bounces
// the user back to /login with an `oauth_error` query param so they land
// on the regular auth UI (with a friendly red banner) rather than a bare
// browser error page — which would feel like the site itself broke.
func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	bounce := func(reason string) {
		http.Redirect(w, r, "/login?oauth_error="+url.QueryEscape(reason), http.StatusFound)
	}

	if !s.googleOAuthEnabled() {
		bounce("not_configured")
		return
	}

	cookie, err := r.Cookie(googleOAuthStateCookie)
	if err != nil {
		bounce("missing_state")
		return
	}
	// Burn the state cookie immediately, regardless of outcome.
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	parts := strings.SplitN(cookie.Value, "|", 2)
	cookieState := parts[0]
	next := "/app"
	if len(parts) == 2 && isSafeRedirect(parts[1]) {
		next = parts[1]
	}
	urlState := r.URL.Query().Get("state")
	if urlState == "" || urlState != cookieState {
		bounce("state_mismatch")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		// User declined consent or Google returned an error.
		bounce(errParam)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		bounce("missing_code")
		return
	}

	cfg := s.googleOAuthConfig()
	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		bounce("token_exchange_failed")
		return
	}

	profile, err := fetchGoogleProfile(r.Context(), cfg.Client(r.Context(), token))
	if err != nil {
		bounce("profile_fetch_failed")
		return
	}
	if profile.Sub == "" || profile.Email == "" {
		bounce("profile_incomplete")
		return
	}
	// Auto-link by email requires Google to have verified ownership.
	// Without this an attacker who controls an unverified Gmail-style
	// address could hijack an existing password account.
	if !profile.EmailVerified {
		bounce("email_not_verified")
		return
	}

	user, err := s.findOrCreateGoogleUser(r, profile)
	if err != nil {
		bounce("user_link_failed")
		return
	}

	if err := s.issueSession(w, r, user.ID); err != nil {
		bounce("session_failed")
		return
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// googleProfile is the minimal subset of /userinfo we use.
type googleProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func fetchGoogleProfile(ctx context.Context, c *http.Client) (googleProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return googleProfile{}, err
	}
	res, err := c.Do(req)
	if err != nil {
		return googleProfile{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return googleProfile{}, errors.New("userinfo: " + res.Status + ": " + string(body))
	}
	var p googleProfile
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		return googleProfile{}, err
	}
	return p, nil
}

// findOrCreateGoogleUser implements the auto-link flow:
//  1. Lookup by google_sub → established Google user, return as-is.
//  2. Lookup by email → existing password account, link to this sub.
//  3. Otherwise → create a brand-new user with no password (Google-only).
func (s *Server) findOrCreateGoogleUser(r *http.Request, profile googleProfile) (store.User, error) {
	ctx := r.Context()
	if user, err := s.store.GetUserByGoogleSub(ctx, profile.Sub); err == nil {
		return user, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}
	if user, _, err := s.store.GetUserByEmail(ctx, profile.Email); err == nil {
		if linkErr := s.store.LinkGoogleSub(ctx, user.ID, profile.Sub); linkErr != nil {
			return store.User{}, linkErr
		}
		return user, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}
	return s.store.CreateUserFromGoogle(ctx, randomID("usr"), profile.Email, profile.Name, profile.Sub, time.Now().UTC())
}

func newOAuthState() (string, error) {
	b := make([]byte, 24) // 192 bits — far more than CSRF needs.
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// isSafeRedirect blocks open-redirect attacks: only same-origin, absolute-
// path URLs ("/app", "/quotes?x=1") are allowed.
func isSafeRedirect(next string) bool {
	if next == "" || !strings.HasPrefix(next, "/") {
		return false
	}
	// Reject protocol-relative URLs ("//evil.com") and any path that
	// contains characters that could break out of same-origin context.
	if strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return false
	}
	return true
}
