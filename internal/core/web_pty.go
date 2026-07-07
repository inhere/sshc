package core

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

type WebPTY interface {
	io.Reader
	io.Writer
	Resize(cols, rows int) error
	Close() error
	Wait() error
}

type WebPTYOptions struct {
	Cols int
	Rows int
	Term string
}

type sshWebPTY struct {
	client  RemoteClient
	session *ssh.Session
	stdin   io.WriteCloser
	output  io.Reader
}

func OpenWebPTY(host Host, opts WebPTYOptions) (WebPTY, error) {
	if IsCommandProxyHost(host) {
		return nil, fmt.Errorf("host %s uses command_proxy backend; web terminal is not supported yet", HostLogName(host))
	}
	client, err := newSSHClientWithOptions(host, sshClientOptions{NoHostKeyPrompt: true})
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	cols, rows := normalizeWebPTYSize(opts.Cols, opts.Rows)
	if err := session.RequestPty(loginTermName(opts.Term), rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	output := mergeReaders(stdout, stderr)
	return &sshWebPTY{
		client:  client,
		session: session,
		stdin:   stdin,
		output:  output,
	}, nil
}

func (p *sshWebPTY) Read(buf []byte) (int, error) {
	return p.output.Read(buf)
}

func (p *sshWebPTY) Write(buf []byte) (int, error) {
	return p.stdin.Write(buf)
}

func (p *sshWebPTY) Resize(cols, rows int) error {
	cols, rows = normalizeWebPTYSize(cols, rows)
	return p.session.WindowChange(rows, cols)
}

func (p *sshWebPTY) Close() error {
	var errs []string
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if p.session != nil {
		if err := p.session.Close(); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, err.Error())
		}
	}
	if p.client != nil {
		if err := p.client.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (p *sshWebPTY) Wait() error {
	if p.session == nil {
		return nil
	}
	return p.session.Wait()
}

func normalizeWebPTYSize(cols, rows int) (int, int) {
	if cols < 1 {
		cols = defaultPTYWidth
	}
	if rows < 1 {
		rows = defaultPTYHeight
	}
	return cols, rows
}

func mergeReaders(readers ...io.Reader) io.Reader {
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(len(readers))
	for _, reader := range readers {
		reader := reader
		go func() {
			defer wg.Done()
			_, _ = io.Copy(pw, reader)
		}()
	}
	go func() {
		wg.Wait()
		_ = pw.Close()
	}()
	return pr
}
