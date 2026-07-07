package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gookit/rux/v2"
)

const (
	sessionCookieName = "sshc_session"
	csrfHeaderName    = "X-SSHC-CSRF"
	sessionTTL        = 12 * time.Hour
)

type authSession struct {
	CSRF      string
	CreatedAt time.Time
}

type loginRequest struct {
	Token string `json:"token"`
}

type loginResponse struct {
	CSRF string `json:"csrf"`
}

func GenerateToken() (string, error) {
	return randomToken(24)
}

func (s *Server) authGuard(c *rux.Context) {
	if !s.tokenEnabled {
		c.Next()
		return
	}
	path := c.Req.URL.Path
	if path == "/api/health" || path == "/api/auth/login" || !strings.HasPrefix(path, "/api/") {
		c.Next()
		return
	}
	session, ok := s.requestSession(c.Req)
	if !ok {
		writeError(c, http.StatusUnauthorized, errors.New("login required"))
		c.Abort()
		return
	}
	if isWriteMethod(c.Req.Method) && c.Req.Header.Get(csrfHeaderName) != session.CSRF {
		writeError(c, http.StatusForbidden, errors.New("invalid csrf token"))
		c.Abort()
		return
	}
	c.Next()
}

func (s *Server) handleAuthLogin(c *rux.Context) {
	if !s.tokenEnabled {
		writeOK(c, loginResponse{})
		return
	}
	var req loginRequest
	if err := readJSON(c, &req); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if !s.matchToken(req.Token) {
		writeError(c, http.StatusUnauthorized, errors.New("invalid token"))
		return
	}
	sessionID, err := randomToken(32)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	s.mu.Lock()
	s.sessions[sessionID] = authSession{CSRF: csrf, CreatedAt: time.Now()}
	s.mu.Unlock()
	http.SetCookie(c.Resp, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeOK(c, loginResponse{CSRF: csrf})
}

func (s *Server) matchToken(token string) bool {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(sum[:], s.tokenHash[:]) == 1
}

func (s *Server) requestSession(req *http.Request) (authSession, bool) {
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return authSession{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[cookie.Value]
	if !ok {
		return authSession{}, false
	}
	if time.Since(session.CreatedAt) > sessionTTL {
		delete(s.sessions, cookie.Value)
		return authSession{}, false
	}
	return session, true
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type authState struct {
	tokenEnabled bool
	tokenHash    [32]byte
	sessions     map[string]authSession
	mu           sync.Mutex
}
