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
	PresentationMap        PresentationKind = "map"
)

type PresentationEvent struct {
	Kind PresentationKind `json:"kind"`
	// Presented records host acceptance for the user's chat, not a client render acknowledgement.
	Presented bool           `json:"presented,omitempty"`
	Artifact  *ArtifactEvent `json:"artifact,omitempty"`
	View      *PluginView    `json:"view,omitempty"`
	Map       *PlaceMap      `json:"map,omitempty"`
}

// PlaceMap is a set of places drawn on an inline map. Every place has
// coordinates; the tool that presents it resolves the ones given only by address.
type PlaceMap struct {
	Title  string  `json:"title"`
	Places []Place `json:"places"`
}

type Place struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address,omitempty"`
	Category  string  `json:"category,omitempty"`
	Note      string  `json:"note,omitempty"`
	Rating    float64 `json:"rating,omitempty"`
	Reviews   int     `json:"reviews,omitempty"`
	Price     string  `json:"price,omitempty"`
	URL       string  `json:"url,omitempty"`
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
	// A presentation keeps only the payload its kind names.
	artifact, view, placeMap := p.Artifact, p.View, p.Map
	p.Artifact, p.View, p.Map = nil, nil, nil
	switch p.Kind {
	case PresentationCanvas:
		return nil
	case PresentationArtifact:
		if artifact != nil && artifact.Title != "" && artifact.WidgetCode != "" {
			p.Artifact = artifact
			return nil
		}
	case PresentationPluginView:
		if view != nil && view.Title != "" && view.WidgetCode != "" && view.File != "" {
			p.View = view
			return nil
		}
	case PresentationMap:
		if placeMap != nil && len(placeMap.Places) > 0 {
			p.Map = placeMap
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
