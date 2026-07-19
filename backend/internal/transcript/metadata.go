package transcript

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Metadata struct {
	Title           string `json:"title,omitempty"`
	Slug            string `json:"slug,omitempty"`
	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func metadata(events []sessionevents.Event, session storage.Session, children []acp.Job, references []storage.TranscriptSession) map[string]Metadata {
	ids := make(map[string]struct{})
	for _, event := range events {
		if event.ACP != nil && event.ACP.ID != "" {
			ids[event.ACP.ID] = struct{}{}
		}
	}
	childrenByID := make(map[string]acp.Job, len(children))
	for _, child := range children {
		childrenByID[child.ID] = child
	}
	referencesByID := make(map[string]storage.TranscriptSession, len(references))
	for _, reference := range references {
		referencesByID[reference.ID] = reference
	}
	meta := make(map[string]Metadata, len(ids))
	for id := range ids {
		switch {
		case id == session.ID:
			meta[id] = metadataFromSession(session)
		case childrenByID[id].ID != "":
			meta[id] = metadataFromJob(childrenByID[id])
		case referencesByID[id].ID != "":
			meta[id] = metadataFromTranscriptSession(referencesByID[id])
		}
	}
	return meta
}

func metadataFromSession(session storage.Session) Metadata {
	return Metadata{Title: session.Title, Slug: session.Slug, ModelProvider: session.ModelProvider, Model: session.Model, ReasoningEffort: session.ReasoningEffort}
}

func metadataFromTranscriptSession(session storage.TranscriptSession) Metadata {
	return Metadata{Title: session.Title, Slug: session.Slug, ModelProvider: session.ModelProvider, Model: session.Model, ReasoningEffort: session.ReasoningEffort}
}

func metadataFromJob(job acp.Job) Metadata {
	return Metadata{Title: job.Title, Slug: job.Slug, ModelProvider: job.ModelProvider, Model: job.Model, ReasoningEffort: job.ReasoningEffort}
}
