package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gookit/rux/v2"
)

const (
	DefaultAddr = "127.0.0.1:8822"
	AppName     = "sshc"
)

type Config struct {
	Addr     string
	Open     bool
	Readonly bool
	WebDir   string
	Token    string
}

type Server struct {
	config Config
	router http.Handler
	http   *http.Server
	authState
}

func New(config Config) (*Server, error) {
	normalized, err := ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	s := &Server{config: normalized}
	if normalized.Token != "" {
		s.tokenEnabled = true
		s.tokenHash = sha256Token(normalized.Token)
		s.sessions = make(map[string]authSession)
		s.config.Token = ""
	}
	s.router = s.routes()
	s.http = &http.Server{
		Handler:           s.router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

func (s *Server) Config() Config {
	return s.config
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) Start(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return "", err
	}
	url := listenerURL(ln.Addr())
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdownCtx)
	}()
	go func() {
		err := s.http.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// P1 keeps serve logging minimal; command-level lifecycle reporting follows in later phases.
		}
	}()
	return url, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func ValidateConfig(config Config) (Config, error) {
	config.Addr = normalizeListenAddr(config.Addr)
	host, _, err := net.SplitHostPort(config.Addr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid serve addr %q: %w", config.Addr, err)
	}
	if !isLoopbackHost(host) && strings.TrimSpace(config.Token) == "" {
		return Config{}, fmt.Errorf("serve --token is required when binding non-loopback addr %q", config.Addr)
	}
	config.WebDir = strings.TrimSpace(config.WebDir)
	config.Token = strings.TrimSpace(config.Token)
	return config, nil
}

func normalizeListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DefaultAddr
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func listenerURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://" + net.JoinHostPort(host, port)
}

func (s *Server) routes() http.Handler {
	r := rux.New()
	r.Use(s.authGuard)
	r.GET("/api/health", s.handleHealth)
	r.POST("/api/auth/login", s.handleAuthLogin)
	r.GET("/api/config/summary", s.handleConfigSummary)
	r.GET("/api/hosts", s.handleHostsList)
	r.POST("/api/hosts", s.handleHostsCreate)
	r.GET("/api/hosts/{name}", s.handleHostsShow)
	r.PUT("/api/hosts/{name}", s.handleHostsUpdate)
	r.DELETE("/api/hosts/{name}", s.handleHostsDelete)
	r.POST("/api/hosts/{name}/trust", s.handleHostsTrust)
	r.GET("/api/auth-profiles", s.handleAuthList)
	r.POST("/api/auth-profiles", s.handleAuthCreate)
	r.GET("/api/auth-profiles/{name}", s.handleAuthShow)
	r.PUT("/api/auth-profiles/{name}", s.handleAuthUpdate)
	r.DELETE("/api/auth-profiles/{name}", s.handleAuthDelete)
	r.GET("/api/logs", s.handleLogsList)
	r.GET("/api/logs/{task_id}", s.handleLogsShow)
	r.GET("/api/logs/{task_id}/output", s.handleLogOutput)
	r.GET("/", s.handleAssets)
	r.GET("/*path", s.handleAssets)
	return r
}

func sha256Token(token string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(token)))
}
