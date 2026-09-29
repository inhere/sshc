package core

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultForwardKeepaliveEvery = 30 * time.Second
	defaultForwardKeepaliveWait  = 10 * time.Second
	forwardAcceptBackoffMin      = 5 * time.Millisecond
	forwardAcceptBackoffMax      = time.Second
)

// ForwardRule is one runtime local port forwarding rule.
type ForwardRule struct {
	LocalAddr  string
	RemoteAddr string
}

// ForwardOptions tunes the forward session.
type ForwardOptions struct {
	ConnectTimeout time.Duration
	KeepaliveEvery time.Duration
	KeepaliveWait  time.Duration
	Logf           func(format string, args ...any)
}

// ForwardSession is a running local forward: one SSH session plus its listeners.
type ForwardSession interface {
	// Endpoints returns the actual local listener addresses (ports resolved).
	Endpoints() []string
	// Wait blocks until the session ends and returns its error, if any.
	Wait() error
	// Close stops the session. It is safe to call more than once.
	Close() error
}

// forwardDialer is the in-package session seam: dial remote services, drive the
// keepalive and observe session loss, without widening the RemoteClient interface.
type forwardDialer interface {
	Dial(network, addr string) (net.Conn, error)
	SendKeepalive() error
	Wait() error
	Close() error
}

type remoteForwardDialer struct {
	client *remoteClient
}

func (d remoteForwardDialer) Dial(network, addr string) (net.Conn, error) {
	return d.client.Dial(network, addr)
}

func (d remoteForwardDialer) SendKeepalive() error {
	_, _, err := d.client.SendRequest("keepalive@openssh.com", true, nil)
	return err
}

func (d remoteForwardDialer) Wait() error {
	return d.client.Wait()
}

func (d remoteForwardDialer) Close() error {
	return d.client.Close()
}

// newForwardDialerForTest mirrors remoteClientDialForTest for the session seam.
var newForwardDialerForTest func(host Host) (forwardDialer, error)

// newForwardDialerAdapter wraps a concrete remote client as the forward session seam.
func newForwardDialerAdapter(client *remoteClient) forwardDialer {
	return remoteForwardDialer{client: client}
}

func newForwardDialer(host Host) (forwardDialer, error) {
	if newForwardDialerForTest != nil {
		return newForwardDialerForTest(host)
	}
	client, err := newSSHClient(host)
	if err != nil {
		return nil, err
	}
	concrete, ok := client.(*remoteClient)
	if !ok {
		_ = client.Close()
		return nil, fmt.Errorf("unsupported ssh client type %T", client)
	}
	return newForwardDialerAdapter(concrete), nil
}

type forwardSession struct {
	dialer    forwardDialer
	rules     []ForwardRule
	listeners []net.Listener
	options   ForwardOptions

	stopCh     chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup
	activeMu   sync.Mutex
	active     map[net.Conn]struct{}
	stateMu    sync.Mutex
	err        error
	stopped    bool
	userClosed bool
	stopOnce   sync.Once
}

// StartLocalForward opens every local listener and forwards each connection
// through the SSH session dialer. Rules are validated before any resource is taken.
func StartLocalForward(host Host, rules []ForwardRule, opts ForwardOptions) (ForwardSession, error) {
	if len(rules) == 0 {
		return nil, errors.New("at least one forward rule is required")
	}
	prepared, err := prepareForwardRules(rules)
	if err != nil {
		return nil, err
	}
	opts = normalizeForwardOptions(opts)
	if opts.ConnectTimeout > 0 {
		host.ConnectTimeout = opts.ConnectTimeout.String()
	}

	dialer, err := newForwardDialer(host)
	if err != nil {
		return nil, err
	}

	session := &forwardSession{
		dialer:  dialer,
		rules:   prepared,
		options: opts,
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		active:  map[net.Conn]struct{}{},
	}
	for _, rule := range prepared {
		listener, err := net.Listen("tcp", rule.LocalAddr)
		if err != nil {
			session.closeListeners()
			_ = dialer.Close()
			return nil, fmt.Errorf("listen %s: %w", rule.LocalAddr, err)
		}
		session.listeners = append(session.listeners, listener)
	}

	session.wg.Add(len(session.listeners) + 2)
	for i := range session.listeners {
		rule := session.rules[i]
		listener := session.listeners[i]
		go session.acceptLoop(listener, rule)
	}
	go session.keepaliveLoop()
	go session.sessionWatch()
	go func() {
		session.wg.Wait()
		close(session.done)
	}()
	return session, nil
}

func prepareForwardRules(rules []ForwardRule) ([]ForwardRule, error) {
	prepared := make([]ForwardRule, 0, len(rules))
	seen := map[string]bool{}
	for _, rule := range rules {
		local, err := normalizeForwardLocalAddr(rule.LocalAddr)
		if err != nil {
			return nil, err
		}
		remote, err := normalizeForwardRemoteAddr(rule.RemoteAddr)
		if err != nil {
			return nil, err
		}
		// A local port of 0 lets the OS pick a free port, so several :0 rules are
		// independent listeners and must not be treated as duplicates.
		if _, portText, splitErr := net.SplitHostPort(local); splitErr == nil && portText != "0" {
			if seen[local] {
				return nil, fmt.Errorf("duplicate local endpoint %q", local)
			}
			seen[local] = true
		}
		prepared = append(prepared, ForwardRule{LocalAddr: local, RemoteAddr: remote})
	}
	return prepared, nil
}

func normalizeForwardOptions(opts ForwardOptions) ForwardOptions {
	if opts.KeepaliveEvery <= 0 {
		opts.KeepaliveEvery = defaultForwardKeepaliveEvery
	}
	if opts.KeepaliveWait <= 0 {
		opts.KeepaliveWait = defaultForwardKeepaliveWait
	}
	return opts
}

func (s *forwardSession) Endpoints() []string {
	endpoints := make([]string, 0, len(s.listeners))
	for _, listener := range s.listeners {
		if addr := listener.Addr(); addr != nil {
			endpoints = append(endpoints, addr.String())
		}
	}
	return endpoints
}

func (s *forwardSession) Wait() error {
	<-s.done
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.err
}

func (s *forwardSession) Close() error {
	s.stop(nil, true)
	<-s.done
	return nil
}

func (s *forwardSession) acceptLoop(listener net.Listener, rule ForwardRule) {
	defer s.wg.Done()
	backoff := forwardAcceptBackoffMin
	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.isStopped() || errors.Is(err, net.ErrClosed) {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				s.logf("accept %s failed temporarily: %v (retry in %s)", rule.LocalAddr, err, backoff)
				time.Sleep(backoff)
				if backoff *= 2; backoff > forwardAcceptBackoffMax {
					backoff = forwardAcceptBackoffMax
				}
				continue
			}
			s.stop(fmt.Errorf("accept %s: %w", rule.LocalAddr, err), false)
			return
		}
		backoff = forwardAcceptBackoffMin
		s.wg.Add(1)
		go s.handleConn(conn, rule)
	}
}

func (s *forwardSession) handleConn(local net.Conn, rule ForwardRule) {
	defer s.wg.Done()
	remote, err := s.dialer.Dial("tcp", rule.RemoteAddr)
	if err != nil {
		s.logf("forward %s -> %s failed: %v", rule.LocalAddr, rule.RemoteAddr, err)
		_ = local.Close()
		return
	}
	s.trackActive(local, remote)
	defer s.untrackActive(local, remote)
	if s.isStopped() {
		_ = local.Close()
		_ = remote.Close()
		return
	}

	started := time.Now()
	s.logf("forward connected %s -> %s", rule.LocalAddr, rule.RemoteAddr)
	var wg sync.WaitGroup
	var sent, received int64
	wg.Add(2)
	go func() {
		defer wg.Done()
		sent = copyAndCloseWrite(remote, local)
	}()
	go func() {
		defer wg.Done()
		received = copyAndCloseWrite(local, remote)
	}()
	wg.Wait()
	_ = local.Close()
	_ = remote.Close()
	s.logf("forward closed %s -> %s in %s (sent %d bytes, received %d bytes)",
		rule.LocalAddr, rule.RemoteAddr, time.Since(started).Round(time.Millisecond), sent, received)
}

// copyAndCloseWrite copies src into dst and half-closes dst when it supports it.
func copyAndCloseWrite(dst net.Conn, src net.Conn) int64 {
	written, _ := io.Copy(dst, src)
	if closer, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
	return written
}

func (s *forwardSession) keepaliveLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.options.KeepaliveEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
		result := make(chan error, 1)
		go func() { result <- s.dialer.SendKeepalive() }()
		select {
		case err := <-result:
			if err != nil {
				s.stop(fmt.Errorf("ssh session keepalive failed: %w", err), false)
				return
			}
		case <-time.After(s.options.KeepaliveWait):
			s.stop(fmt.Errorf("ssh session keepalive timed out after %s", s.options.KeepaliveWait), false)
			return
		case <-s.stopCh:
			return
		}
	}
}

func (s *forwardSession) sessionWatch() {
	defer s.wg.Done()
	err := s.dialer.Wait()
	if s.isStopped() {
		return
	}
	if err != nil {
		s.stop(fmt.Errorf("ssh session closed: %w", err), false)
		return
	}
	s.stop(errors.New("ssh session closed"), false)
}

func (s *forwardSession) stop(reason error, userClosed bool) {
	s.stopOnce.Do(func() {
		s.stateMu.Lock()
		if userClosed {
			s.userClosed = true
		}
		if reason != nil {
			s.err = reason
		}
		s.stopped = true
		s.stateMu.Unlock()

		close(s.stopCh)
		s.closeListeners()
		s.closeActive()
		if err := s.dialer.Close(); err != nil && reason == nil {
			s.stateMu.Lock()
			if s.err == nil {
				s.err = err
			}
			s.stateMu.Unlock()
		}
	})
	if reason != nil {
		s.logf("%v", reason)
	}
}

func (s *forwardSession) isStopped() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.stopped
}

func (s *forwardSession) closeListeners() {
	for _, listener := range s.listeners {
		_ = listener.Close()
	}
}

func (s *forwardSession) trackActive(conns ...net.Conn) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for _, conn := range conns {
		s.active[conn] = struct{}{}
	}
}

func (s *forwardSession) untrackActive(conns ...net.Conn) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for _, conn := range conns {
		delete(s.active, conn)
	}
}

func (s *forwardSession) closeActive() {
	s.activeMu.Lock()
	conns := make([]net.Conn, 0, len(s.active))
	for conn := range s.active {
		conns = append(conns, conn)
	}
	s.active = map[net.Conn]struct{}{}
	s.activeMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (s *forwardSession) logf(format string, args ...any) {
	if s.options.Logf == nil {
		return
	}
	s.options.Logf(format, args...)
}

// normalizeForwardEndpoint canonicalizes one side of a forward rule.
func normalizeForwardEndpoint(value string, allowZero bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("endpoint is empty")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("endpoint %q contains whitespace", value)
	}
	if strings.Contains(strings.ToLower(value), "udp") {
		return "", fmt.Errorf("endpoint %q: only TCP forwarding is supported", value)
	}
	if strings.Contains(value, "/") {
		return "", fmt.Errorf("endpoint %q: unix sockets are not supported", value)
	}
	if !strings.Contains(value, ":") && strings.Contains(value, "-") {
		return "", fmt.Errorf("port ranges are not supported: %q", value)
	}
	if port, err := strconv.Atoi(value); err == nil {
		if err := validateForwardPortNumber(port, allowZero); err != nil {
			return "", err
		}
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("endpoint %q, want [host:]port", value)
	}
	port, err := parseForwardPort(portText)
	if err != nil {
		return "", err
	}
	if err := validateForwardPortNumber(port, allowZero); err != nil {
		return "", err
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if err := validateForwardEndpointHost(host); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parseForwardPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("port is required")
	}
	if strings.ContainsAny(value, "-:") {
		return 0, fmt.Errorf("port ranges are not supported: %q", value)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return port, nil
}

func validateForwardPortNumber(port int, allowZero bool) error {
	if port == 0 && allowZero {
		return nil
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d, want 1-65535", port)
	}
	return nil
}

func validateForwardEndpointHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" || host == "." {
		return fmt.Errorf("invalid endpoint host %q", host)
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("invalid endpoint host %q", host)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if !isValidRawHostname(host) {
		return fmt.Errorf("invalid endpoint host %q", host)
	}
	return nil
}

func isLoopbackEndpointHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func normalizeForwardLocalAddr(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("local endpoint is required")
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid local endpoint %q: %w", value, err)
	}
	port, err := parseForwardPort(portText)
	if err != nil {
		return "", fmt.Errorf("invalid local endpoint %q: %w", value, err)
	}
	if err := validateForwardPortNumber(port, true); err != nil {
		return "", fmt.Errorf("invalid local endpoint %q: %w", value, err)
	}
	host = strings.TrimSpace(host)
	if !isLoopbackEndpointHost(host) {
		return "", fmt.Errorf("local endpoint %q must bind a loopback address (127.0.0.1 or [::1])", value)
	}
	if !strings.EqualFold(host, "localhost") && net.ParseIP(host) == nil {
		return "", fmt.Errorf("invalid local endpoint host %q", host)
	}
	return net.JoinHostPort(host, portText), nil
}

func normalizeForwardRemoteAddr(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("remote endpoint is required")
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid remote endpoint %q: %w", value, err)
	}
	port, err := parseForwardPort(portText)
	if err != nil {
		return "", fmt.Errorf("invalid remote endpoint %q: %w", value, err)
	}
	if err := validateForwardPortNumber(port, false); err != nil {
		return "", fmt.Errorf("invalid remote endpoint %q: %w", value, err)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if err := validateForwardEndpointHost(host); err != nil {
		return "", fmt.Errorf("invalid remote endpoint %q: %w", value, err)
	}
	return net.JoinHostPort(host, portText), nil
}
