package core

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

// startBannerServer accepts one connection per call and writes reply.
func startBannerServer(t *testing.T, reply string) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if reply != "" {
				_, _ = conn.Write([]byte(reply))
			}
			_ = conn.Close()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func TestProbeSSH(t *testing.T) {
	t.Run("ssh banner is reported", func(t *testing.T) {
		is := assert.New(t)

		addr, stop := startBannerServer(t, "SSH-2.0-Test\r\n")
		defer stop()
		host, portText, err := net.SplitHostPort(addr)
		is.NoErr(err)
		port := 0
		_, err = fmt.Sscan(portText, &port)
		is.NoErr(err)

		targets := []ScanTarget{{Address: host, Family: ScanFamilyIPv4, Source: ScanSourceCIDR}}
		results, err := ProbeSSH(context.Background(), targets, port, time.Second, 2, nil)
		is.NoErr(err)
		is.Len(results, 1)
		is.Eq("SSH-2.0-Test", results[0].Banner)
		is.Eq(port, results[0].Port)
		is.Eq(addr, results[0].Address)
		is.Eq(ScanSourceCIDR, results[0].Source)
	})

	t.Run("non ssh service is skipped", func(t *testing.T) {
		is := assert.New(t)

		addr, stop := startBannerServer(t, "HTTP/1.1 400 Bad Request\r\n")
		defer stop()
		host, portText, err := net.SplitHostPort(addr)
		is.NoErr(err)
		port := 0
		_, err = fmt.Sscan(portText, &port)
		is.NoErr(err)

		targets := []ScanTarget{{Address: host}}
		results, err := ProbeSSH(context.Background(), targets, port, 300*time.Millisecond, 1, nil)
		is.NoErr(err)
		is.Len(results, 0)
	})

	t.Run("closed port is skipped", func(t *testing.T) {
		is := assert.New(t)

		targets := []ScanTarget{
			{Address: "127.0.0.1"}, {Address: "127.0.0.2"},
		}
		results, err := ProbeSSH(context.Background(), targets, 1, 100*time.Millisecond, 2, func(_, _ string, _ time.Duration) (net.Conn, error) {
			return nil, fmt.Errorf("connection refused")
		})
		is.NoErr(err)
		is.Len(results, 0)
	})

	t.Run("cancelled context returns promptly", func(t *testing.T) {
		is := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		targets := []ScanTarget{{Address: "127.0.0.1"}, {Address: "127.0.0.2"}, {Address: "127.0.0.3"}}
		started := time.Now()
		results, err := ProbeSSH(ctx, targets, 22, time.Second, 2, func(_, _ string, _ time.Duration) (net.Conn, error) {
			return nil, fmt.Errorf("connection refused")
		})
		is.Len(results, 0)
		is.True(time.Since(started) < time.Second)
		is.True(err != nil)
	})

	t.Run("ipv6 zone is kept in the dial address", func(t *testing.T) {
		is := assert.New(t)

		var dialed string
		targets := []ScanTarget{{Address: "fe80::1", Zone: "12", Family: ScanFamilyIPv6}}
		_, err := ProbeSSH(context.Background(), targets, 22, time.Second, 1, func(_, address string, _ time.Duration) (net.Conn, error) {
			dialed = address
			return nil, fmt.Errorf("connection refused")
		})
		is.NoErr(err)
		is.Eq("[fe80::1%12]:22", dialed)
	})
}
