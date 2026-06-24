package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || origin == "null" || strings.HasPrefix(origin, "file://") || allowedRequestOrigin(r)
	},
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
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	size := terminal.Size{Cols: queryUint16(r, "cols"), Rows: queryUint16(r, "rows")}
	term, err := s.terminalManager().Open(session.ID, cwd, size)
	if err != nil {
		_ = writeTerminalMessage(conn, terminal.Message{Type: "error", Err: err.Error()})
		return
	}
	events, unsubscribe := term.Subscribe()
	defer unsubscribe()
	var writeMu sync.Mutex
	send := func(msg terminal.Message) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeTerminalMessage(conn, msg)
	}
	if err := send(terminal.Message{Type: "ready", Cwd: term.Cwd()}); err != nil {
		return
	}
	if replay := term.Replay(); replay != "" {
		if err := send(terminal.Message{Type: "output", Data: replay}); err != nil {
			return
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		for msg := range events {
			if err := send(msg); err != nil {
				return
			}
		}
	}()
	for {
		var msg terminalClientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			unsubscribe()
			_ = conn.Close()
			<-done
			return
		}
		switch msg.Type {
		case "input":
			if err := term.Write(msg.Data); err != nil {
				_ = send(terminal.Message{Type: "error", Err: err.Error()})
			}
		case "resize":
			if err := term.Resize(terminal.Size{Cols: msg.Cols, Rows: msg.Rows}); err != nil {
				_ = send(terminal.Message{Type: "error", Err: err.Error()})
			}
		case "terminate":
			s.terminalManager().Kill(session.ID)
		case "restart":
			s.terminalManager().Kill(session.ID)
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
