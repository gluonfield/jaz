package server

import (
	"context"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/m365copilot"
)

func (s *Server) startMicrosoftCopilotAuthLogin(w http.ResponseWriter, r *http.Request, auth acp.AgentAuthConfig) {
	if s.MicrosoftCopilot == nil {
		writeError(w, http.StatusBadRequest, m365copilot.ErrClientIDRequired)
		return
	}
	session, err := s.MicrosoftCopilot.StartLogin(r.Context(), s.microsoftCopilotCallbackURL(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := newACPAuthLoginID()
	if err != nil {
		s.MicrosoftCopilot.CancelLogin(session.State)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	job := &acpAuthLoginJob{
		ID:        id,
		Agent:     acp.AgentMicrosoftCopilot,
		Status:    "running",
		StartedAt: time.Now().UTC(),
		authURL:   session.AuthURL,
		onInput: func(raw string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return s.MicrosoftCopilot.CompleteFromInput(ctx, session.State, raw)
		},
	}
	s.acpAuthLoginJobs.Store(job.ID, job)
	go func() {
		timer := time.NewTimer(acpAuthLoginTimeout)
		defer timer.Stop()
		select {
		case err := <-session.Done:
			if err == nil {
				err = s.verifyACPAuthLogin(acp.AgentMicrosoftCopilot, auth)
			}
			job.finish(err, nil)
		case <-timer.C:
			s.MicrosoftCopilot.CancelLogin(session.State)
			job.finish(nil, context.DeadlineExceeded)
		}
	}()
	writeJSON(w, http.StatusOK, job.response())
}

func (s *Server) handleMicrosoftCopilotAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.MicrosoftCopilot == nil {
		writeMCPOAuthCallbackHTML(w, http.StatusServiceUnavailable, "Connection failed", "Microsoft Copilot OAuth is not configured", false)
		return
	}
	q := r.URL.Query()
	if err := s.MicrosoftCopilot.CompleteCallback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error"), q.Get("error_description")); err != nil {
		writeMCPOAuthCallbackHTML(w, http.StatusBadRequest, "Connection failed", err.Error(), false)
		return
	}
	writeMCPOAuthCallbackHTML(w, http.StatusOK, "Connected", "You can close this tab and return to Jaz.", true)
}

func (s *Server) microsoftCopilotCallbackURL(r *http.Request) string {
	base := s.ServerConfig.PublicBaseURL()
	if base == "" {
		base = httpapi.RequestBaseURL(r)
	}
	if base == "" {
		return ""
	}
	return base + m365copilot.OAuthCallbackPath
}
