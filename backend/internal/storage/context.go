package storage

import (
	"encoding/json"
	"strings"

	"github.com/wins/jaz/backend/internal/messagepayload"
)

const (
	ContextTypeSelection         = messagepayload.ContextTypeSelection
	ContextTypeBrowserAnnotation = messagepayload.ContextTypeBrowserAnnotation
)

type MessageContext = messagepayload.MessageContext
type BrowserAnnotation = messagepayload.BrowserAnnotation
type BrowserAnnotationPosition = messagepayload.BrowserAnnotationPosition
type BrowserAnnotationViewport = messagepayload.BrowserAnnotationViewport
type Attachment = messagepayload.Attachment

func SelectionContexts(selections []string) []MessageContext {
	return messagepayload.SelectionContexts(selections)
}

func NormalizeMessageContexts(contexts []MessageContext) []MessageContext {
	return messagepayload.NormalizeMessageContexts(contexts)
}

func BrowserAnnotationFromBlock(block Block) *BrowserAnnotation {
	if block.Type != BlockTypeBrowserAnnotation || strings.TrimSpace(block.InputJSON) == "" {
		return nil
	}
	var annotation BrowserAnnotation
	if err := json.Unmarshal([]byte(block.InputJSON), &annotation); err != nil {
		return nil
	}
	return messagepayload.NormalizeBrowserAnnotation(&annotation)
}
