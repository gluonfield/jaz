package manifest

import (
	"encoding/json"
	"errors"
	"regexp"
)

var ErrNotPlugin = errors.New("not a plugin: no supported manifest")

type Manifest struct {
	Name             string
	DisplayName      string
	Description      string
	Version          string
	Category         string
	Author           string
	AuthorURL        string
	Homepage         string
	Repository       string
	Keywords         []string
	IconPath         string
	SuggestedPrompts []string
	Page             *Page
	Warnings         []string
	ManifestPath     string
	ExtraSkillPaths  []string
	MCPServers       json.RawMessage
}

type Page struct {
	View string `json:"view"`
}

var safeName = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

func SafeName(value string) bool {
	return len(value) > 0 && len(value) <= 64 && safeName.MatchString(value)
}
