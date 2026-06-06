package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/chat"
	"github.com/wins/jaz/backend/internal/tui"
)

func runChat(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	serverURL := fs.String("server", "http://127.0.0.1:8080", "Jaz server URL")
	sessionID := fs.String("session", "", "existing session ID")
	last := fs.Bool("last", false, "connect to the last root session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *last && *sessionID != "" {
		return fmt.Errorf("use either --last or --session, not both")
	}

	client := http.DefaultClient
	session, err := openChatSession(client, *serverURL, *sessionID, *last)
	if err != nil {
		return err
	}
	return tui.Run(context.Background(), tui.Config{
		Chat:    chat.NewClient(client, *serverURL),
		Session: chatSession(session),
	})
}

func openChatSession(client *http.Client, serverURL, sessionID string, last bool) (sessionResponse, error) {
	if last {
		return lastSession(client, serverURL)
	}
	if sessionID != "" {
		return getSession(client, serverURL, sessionID)
	}
	return createSession(client, serverURL)
}

func chatSession(session sessionResponse) chat.Session {
	out := chat.Session{ID: session.ID, Slug: session.Slug, Runtime: session.Runtime}
	if session.RuntimeRef != nil {
		out.ACPAgent = session.RuntimeRef.Agent
	}
	return out
}
