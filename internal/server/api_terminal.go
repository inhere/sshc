package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/gookit/rux/v2"
)

func (s *Server) handleTerminalCreate(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	var req terminalCreateRequest
	if err := readJSON(c, &req); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	session, err := s.terminals.create(c.Req.Context(), req, c.Req.RemoteAddr)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	writeJSON(c, http.StatusCreated, response{OK: true, Data: session.info()})
}

func (s *Server) handleTerminalList(c *rux.Context) {
	writeOK(c, s.terminals.list())
}

func (s *Server) handleTerminalResize(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	var req terminalResizeRequest
	if err := readJSON(c, &req); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if err := s.terminals.resize(c.Req.Context(), c.Param("id"), req.Cols, req.Rows); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeOK(c, map[string]string{"resized": c.Param("id")})
}

func (s *Server) handleTerminalDelete(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	if err := s.terminals.close(c.Req.Context(), c.Param("id"), "deleted by user"); err != nil {
		writeError(c, http.StatusNotFound, err)
		return
	}
	writeOK(c, map[string]string{"closed": c.Param("id")})
}

func (s *Server) handleTerminalWS(c *rux.Context) {
	session, ok := s.terminals.get(c.Param("id"))
	if !ok {
		writeError(c, http.StatusNotFound, errors.New("terminal session not found"))
		return
	}
	conn, err := websocket.Accept(c.Resp, c.Req, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(c.Req.Context())
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := session.pty.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					errCh <- writeErr
					return
				}
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		errCh <- copyWebSocketToPTY(ctx, session.pty, func(ctx context.Context) ([]byte, error) {
			messageType, data, err := conn.Read(ctx)
			if err != nil {
				return nil, err
			}
			if messageType != websocket.MessageBinary && messageType != websocket.MessageText {
				return nil, nil
			}
			return data, nil
		})
	}()

	err = <-errCh
	cancel()
	reason := "websocket disconnected"
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "context canceled") {
		reason = err.Error()
	}
	_ = s.terminals.close(context.Background(), session.ID, reason)
}
