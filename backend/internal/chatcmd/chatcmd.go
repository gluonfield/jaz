package chatcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/chat"
	"github.com/wins/jaz/backend/internal/serverclient"
	"github.com/wins/jaz/backend/internal/tui"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("jaz-chat", flag.ContinueOnError)
	serverURL := fs.String("server", "http://127.0.0.1:5299", "Jaz server URL")
	sessionID := fs.String("session", "", "existing session ID")
	last := fs.Bool("last", false, "connect to the last root session")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *last && *sessionID != "" {
		return fmt.Errorf("use either --last or --session, not both")
	}

	conn, err := serverclient.ParseConnection(*serverURL)
	if err != nil {
		return err
	}
	client := serverclient.HTTPClient(conn.Key)
	session, err := openSession(client, conn.URL, *sessionID, *last)
	if err != nil {
		return err
	}
	return tui.Run(context.Background(), tui.Config{
		Chat:    chat.NewClient(client, conn.URL),
		Session: chatSession(session),
	})
}

func openSession(client *http.Client, serverURL, sessionID string, last bool) (serverclient.Session, error) {
	if last {
		return serverclient.LastSession(client, serverURL)
	}
	if sessionID != "" {
		return serverclient.GetSession(client, serverURL, sessionID)
	}
	return serverclient.CreateSession(client, serverURL)
}

func chatSession(session serverclient.Session) chat.Session {
	out := chat.Session{ID: session.ID, Slug: session.Slug, Runtime: session.Runtime}
	if session.RuntimeRef != nil {
		out.ACPAgent = session.RuntimeRef.Agent
	}
	return out
}
