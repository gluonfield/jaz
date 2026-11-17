package sessionevents

import (
	"encoding/json"
	"errors"
)

type PresentationKind string

const (
	PresentationArtifact   PresentationKind = "artifact"
	PresentationPluginView PresentationKind = "plugin-view"
	PresentationCanvas     PresentationKind = "canvas"
)

type PresentationEvent struct {
	Kind PresentationKind `json:"kind"`
	// Presented records host acceptance for the user's chat, not a client render acknowledgement.
	Presented bool           `json:"presented,omitempty"`
	Artifact  *ArtifactEvent `json:"artifact,omitempty"`
	View      *PluginView    `json:"view,omitempty"`
}

type PluginView struct {
	Reference  string `json:"reference,omitempty"`
	Title      string `json:"title"`
	WidgetCode string `json:"widget_code"`
	File       string `json:"file"`
}

func (p *PresentationEvent) UnmarshalJSON(data []byte) error {
	type current PresentationEvent
	if err := json.Unmarshal(data, (*current)(p)); err != nil {
		return err
	}
	if p.Kind == "" {
		var old struct {
			ArtifactEvent
			File string `json:"file"`
		}
		if err := json.Unmarshal(data, &old); err != nil {
			return err
		}
		switch {
		case old.ArtifactType == "canvas":
			p.Kind = PresentationCanvas
		case old.File != "":
			p.Kind = PresentationPluginView
			p.View = &PluginView{Title: old.Title, WidgetCode: old.WidgetCode, File: old.File}
		default:
			p.Kind = PresentationArtifact
			p.Artifact = &old.ArtifactEvent
		}
	}
	switch p.Kind {
	case PresentationCanvas:
		p.Artifact = nil
		p.View = nil
		return nil
	case PresentationArtifact:
		if p.Artifact != nil && p.Artifact.Title != "" && p.Artifact.WidgetCode != "" {
			p.View = nil
			return nil
		}
	case PresentationPluginView:
		if p.View != nil && p.View.Title != "" && p.View.WidgetCode != "" && p.View.File != "" {
			p.Artifact = nil
			return nil
		}
	}
	return errors.New("invalid presentation payload")
}

func LatestPluginViews(events []Event) []PluginView {
	views := []PluginView{}
	seen := map[string]bool{}
	for i := len(events) - 1; i >= 0; i-- {
		presentation := events[i].Artifact
		if presentation == nil || presentation.Kind != PresentationPluginView || presentation.View == nil {
			continue
		}
		view := *presentation.View
		key := view.Reference + "\x00" + view.File
		if view.File != "" && !seen[key] {
			views = append(views, view)
			seen[key] = true
		}
	}
	return views
}
