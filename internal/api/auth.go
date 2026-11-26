package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"
)

// handleLogin starts the Discord OAuth2 flow. The state value is bound to
// this browser with a short-lived cookie and checked on the callback (CSRF
// protection for the login itself).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := newRandomToken()
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/auth",
		MaxAge: int((10 * time.Minute).Seconds()), HttpOnly: true,
		Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.oauth.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Always clear the one-time state cookie.
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Path: "/auth", MaxAge: -1, HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})

	if q.Get("error") != "" { // the user clicked "Cancel" on Discord
		s.redirectToDashboard(w, r, "login=cancelled")
		return
	}
	c, err := r.Cookie(stateCookie)
	if err != nil || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		s.redirectToDashboard(w, r, "login=failed")
		return
	}

	ctx := r.Context()
	tok, err := s.oauth.Exchange(ctx, q.Get("code"))
	if err != nil {
		s.log.WarnContext(ctx, "oauth code exchange failed", "err", err)
		s.redirectToDashboard(w, r, "login=failed")
		return
	}
	profile, err := s.oauth.CurrentUser(ctx, tok.AccessToken)
	if err != nil {
		s.log.WarnContext(ctx, "fetching user after login failed", "err", err)
		s.redirectToDashboard(w, r, "login=failed")
		return
	}

	expires := time.Now().Add(min(tok.ExpiresIn, maxSessionLength))
	id, err := s.sessions.create(ctx, Session{User: profile, AccessToken: tok.AccessToken, ExpiresAt: expires})
	if err != nil {
		s.log.ErrorContext(ctx, "creating session failed", "err", err)
		s.redirectToDashboard(w, r, "login=failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.sessionCookieName(), Value: id, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	s.log.InfoContext(ctx, "user logged in", "user_id", profile.ID)
	s.redirectToDashboard(w, r, "")
}

// handleLogout ends this browser's session and revokes its Discord token.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := s.sessionID(r)
	if sess, err := s.sessions.get(ctx, id); err == nil && sess != nil {
		s.revoke(ctx, sess)
	}
	if err := s.sessions.delete(ctx, id); err != nil {
		s.log.WarnContext(ctx, "deleting session failed", "err", err)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleLogoutEverywhere ends every session of the signed-in user, on every
// device, for example after a lost laptop or a shared computer.
func (s *Server) handleLogoutEverywhere(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := s.sessionID(r)
	sess, err := s.sessions.get(ctx, id)
	if err != nil {
		s.log.ErrorContext(ctx, "loading session failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not load your session.")
		return
	}
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Please log in with Discord.")
		return
	}
	if err := s.sessions.endAll(ctx, sess.User.ID.String()); err != nil {
		s.log.ErrorContext(ctx, "ending all sessions failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not sign you out everywhere. Please try again.")
		return
	}
	s.revoke(ctx, sess)
	_ = s.sessions.delete(ctx, id)
	s.clearSessionCookie(w)
	s.log.InfoContext(ctx, "user signed out everywhere", "user_id", sess.User.ID)
	w.WriteHeader(http.StatusNoContent)
}

// revoke tells Discord to invalidate the session's access token. It is best
// effort: the session is gone either way, and the token expires on its own.
func (s *Server) revoke(ctx context.Context, sess *Session) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.oauth.Revoke(ctx, sess.AccessToken); err != nil {
		s.log.WarnContext(ctx, "revoking Discord token failed", "err", err)
	}
}

func (s *Server) sessionCookieName() string {
	if s.cfg.SecureCookies {
		return "__Host-" + sessionCookie
	}
	return sessionCookie
}

func (s *Server) sessionID(r *http.Request) string {
	if c, err := r.Cookie(s.sessionCookieName()); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) redirectToDashboard(w http.ResponseWriter, r *http.Request, query string) {
	u := *s.cfg.DashboardURL
	u.Path = "/"
	u.RawQuery = query
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// requireSession loads the session for API routes; /invite is public.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/invite" {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := s.sessions.get(r.Context(), s.sessionID(r))
		if err != nil {
			s.log.ErrorContext(r.Context(), "loading session failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Could not load your session.")
			return
		}
		if sess == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Please log in with Discord.")
			return
		}
		next.ServeHTTP(w, r.WithContext(withSession(r.Context(), sess)))
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": msg})
}
