package storage

import (
	"encoding/json"
	"strings"
)

const (
	ContextTypeSelection         = "selection"
	ContextTypeBrowserAnnotation = "browser_annotation"
)

type MessageContext struct {
	Type              string             `json:"type"`
	Text              string             `json:"text,omitempty"`
	BrowserAnnotation *BrowserAnnotation `json:"browser_annotation,omitempty"`
}

type BrowserAnnotation struct {
	URL                    string                    `json:"url,omitempty"`
	Frame                  string                    `json:"frame,omitempty"`
	Target                 string                    `json:"target,omitempty"`
	Selector               string                    `json:"selector,omitempty"`
	Path                   string                    `json:"path,omitempty"`
	NodePosition           BrowserAnnotationPosition `json:"node_position,omitempty"`
	Viewport               BrowserAnnotationViewport `json:"viewport,omitempty"`
	RequestedChanges       string                    `json:"requested_changes,omitempty"`
	Comment                string                    `json:"comment,omitempty"`
	ScreenshotAttachmentID string                    `json:"screenshot_attachment_id,omitempty"`
}

type BrowserAnnotationPosition struct {
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

type BrowserAnnotationViewport struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

func SelectionContexts(selections []string) []MessageContext {
	if len(selections) == 0 {
		return nil
	}
	out := make([]MessageContext, 0, len(selections))
	for _, selection := range selections {
		if selection = strings.TrimSpace(selection); selection != "" {
			out = append(out, MessageContext{Type: ContextTypeSelection, Text: selection})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeMessageContexts(contexts []MessageContext) []MessageContext {
	if len(contexts) == 0 {
		return nil
	}
	out := make([]MessageContext, 0, len(contexts))
	for _, context := range contexts {
		switch context.Type {
		case ContextTypeSelection:
			text := strings.TrimSpace(context.Text)
			if text != "" {
				out = append(out, MessageContext{Type: ContextTypeSelection, Text: text})
			}
		case ContextTypeBrowserAnnotation:
			annotation := normalizeBrowserAnnotation(context.BrowserAnnotation)
			if annotation != nil {
				out = append(out, MessageContext{Type: ContextTypeBrowserAnnotation, BrowserAnnotation: annotation})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeBrowserAnnotation(annotation *BrowserAnnotation) *BrowserAnnotation {
	if annotation == nil {
		return nil
	}
	next := *annotation
	next.URL = strings.TrimSpace(next.URL)
	next.Frame = strings.TrimSpace(next.Frame)
	next.Target = strings.TrimSpace(next.Target)
	next.Selector = strings.TrimSpace(next.Selector)
	next.Path = strings.TrimSpace(next.Path)
	next.RequestedChanges = strings.TrimSpace(next.RequestedChanges)
	next.Comment = strings.TrimSpace(next.Comment)
	next.ScreenshotAttachmentID = strings.TrimSpace(next.ScreenshotAttachmentID)
	if next.Frame == "" {
		next.Frame = "top document"
	}
	if next.URL == "" && next.Target == "" && next.Selector == "" && next.RequestedChanges == "" && next.Comment == "" {
		return nil
	}
	return &next
}

func BrowserAnnotationFromBlock(block Block) *BrowserAnnotation {
	if block.Type != BlockTypeBrowserAnnotation || strings.TrimSpace(block.InputJSON) == "" {
		return nil
	}
	var annotation BrowserAnnotation
	if err := json.Unmarshal([]byte(block.InputJSON), &annotation); err != nil {
		return nil
	}
	return normalizeBrowserAnnotation(&annotation)
}
