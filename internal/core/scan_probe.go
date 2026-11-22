package core

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScanResult is one discovered SSH endpoint.
type ScanResult struct {
	Host      string `json:"host"`
	Address   string `json:"address"`
	Family    string `json:"family"`
	Zone      string `json:"zone,omitempty"`
	Interface string `json:"interface,omitempty"`
	Port      int    `json:"port"`
	Banner    string `json:"banner,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Source    string `json:"source,omitempty"`
}

// ScanDialFunc mirrors the TCP dialer used by the existing checks so tests can
// replace it without touching the network.
type ScanDialFunc func(network, address string, timeout time.Duration) (net.Conn, error)

// ProbeSSH dials every target and keeps the ones answering with an SSH banner
// ("SSH-" first protocol line, RFC 4253 4.2). Targets that refuse, time out or
// answer with something else are skipped; a cancelled context returns what was
// collected so far.
func ProbeSSH(
	ctx context.Context,
	targets []ScanTarget,
	port int,
	timeout time.Duration,
	parallel int,
	dial ScanDialFunc,
) ([]ScanResult, error) {
	if port <= 0 {
		port = DefaultSSHPort
	}
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	if parallel <= 0 {
		parallel = 1
	}
	if parallel > len(targets) && len(targets) > 0 {
		parallel = len(targets)
	}
	if dial == nil {
		dial = checkTCPDial
	}

	var (
		mu      sync.Mutex
		results []ScanResult
	)
	jobs := make(chan ScanTarget)
	var wg sync.WaitGroup

	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				if result, ok := probeScanTarget(ctx, target, port, timeout, dial); ok {
					mu.Lock()
					results = append(results, result)
					mu.Unlock()
				}
			}
		}()
	}

feed:
	for _, target := range targets {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- target:
		}
	}
	close(jobs)
	wg.Wait()

	return results, ctx.Err()
}

func probeScanTarget(
	ctx context.Context,
	target ScanTarget,
	port int,
	timeout time.Duration,
	dial ScanDialFunc,
) (ScanResult, bool) {
	if ctx.Err() != nil {
		return ScanResult{}, false
	}

	host := ScanDialHost(target)
	address := net.JoinHostPort(host, strconv.Itoa(port))

	started := time.Now()
	conn, err := dial("tcp", address, timeout)
	if err != nil {
		return ScanResult{}, false
	}
	defer conn.Close()

	banner, ok := readSSHBanner(conn, timeout)
	if !ok {
		return ScanResult{}, false
	}

	return ScanResult{
		Host:      host,
		Address:   address,
		Family:    target.Family,
		Zone:      target.Zone,
		Interface: target.Interface,
		Port:      port,
		Banner:    banner,
		LatencyMS: SinceMS(started),
		Source:    target.Source,
	}, true
}

// readSSHBanner reads the server identification line within timeout.
func readSSHBanner(conn net.Conn, timeout time.Duration) (string, bool) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", false
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "SSH-") {
		return "", false
	}
	if err != nil && line == "" {
		return "", false
	}
	return line, true
}
