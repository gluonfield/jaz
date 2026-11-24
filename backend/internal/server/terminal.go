package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/terminal"
)

const terminalWriteTimeout = 5 * time.Second

type terminalClientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func (s *Server) terminalManager() *terminal.Manager {
	s.terminalOnce.Do(func() {
		if s.Terminal == nil {
			s.Terminal = terminal.New()
		}
	})
	return s.Terminal
}

func (s *Server) handleSessionTerminal(w http.ResponseWriter, r *http.Request, session storage.Session) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	cwd, ok := sessionCwd(w, session)
	if !ok {
		return
	}
	conn, err := httpapi.UpgradeWebSocket(w, r)
	if err != nil {
		return
	}
	stop := context.AfterFunc(r.Context(), func() {
		_ = conn.Close()
	})
	defer stop()
	defer conn.Close()
	size := terminal.Size{Cols: queryUint16(r, "cols"), Rows: queryUint16(r, "rows")}
	unlock := s.lockSession(session.ID)
	if err := s.ensureManagedWorktree(r.Context(), session); err != nil {
		unlock()
		_ = writeTerminalMessage(conn, terminal.Message{Type: "error", Err: err.Error()})
		return
	}
	term, err := s.terminalManager().Open(session.ID, cwd, size)
	unlock()
	if err != nil {
		_ = writeTerminalMessage(conn, terminal.Message{Type: "error", Err: err.Error()})
		return
	}
	serveTerminal(conn, term, term.Close, nil)
}

func serveTerminal(conn *websocket.Conn, term *terminal.Session, terminate func(), finalize func(terminal.Message) terminal.Message) {
	events, unsubscribe := term.Subscribe()
	if err := writeTerminalMessage(conn, terminal.Message{Type: "ready", Cwd: term.Cwd()}); err != nil {
		unsubscribe()
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		for msg := range events {
			if msg.Type == "exit" && finalize != nil {
				msg = finalize(msg)
			}
			if err := writeTerminalMessage(conn, msg); err != nil {
				return
			}
		}
	}()
	defer func() {
		unsubscribe()
		_ = conn.Close()
		<-done
	}()
	for {
		var msg terminalClientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		switch msg.Type {
		case "input":
			_ = term.Write(msg.Data)
		case "resize":
			_ = term.Resize(terminal.Size{Cols: msg.Cols, Rows: msg.Rows})
		case "terminate":
			terminate()
		case "restart":
			terminate()
			return
		}
	}
}

func writeTerminalMessage(conn *websocket.Conn, msg terminal.Message) error {
	_ = conn.SetWriteDeadline(time.Now().Add(terminalWriteTimeout))
	return conn.WriteJSON(msg)
}

func queryUint16(r *http.Request, key string) uint16 {
	n, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if n <= 0 || n > 65535 {
		return 0
	}
	return uint16(n)
}
