package manifest

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

const Schema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var ErrNotPlugin = errors.New("not a plugin: no supported manifest")

type Plugin struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	Version      string          `json:"version"`
	Skills       []string        `json:"skills"`
	Views        []string        `json:"views"`
	IconDataURL  string          `json:"icon_data_url,omitempty"`
	Page         *Page           `json:"page,omitempty"`
	Warnings     []string        `json:"warnings,omitempty"`
	IconPath     string          `json:"-"`
	ManifestPath string          `json:"-"`
	SkillDirs    []string        `json:"-"`
	MCPServers   json.RawMessage `json:"-"`
}

type Page struct {
	View string `json:"view"`
	File string `json:"file"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$`)
var portableName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func SafeName(value string) bool {
	return safeName.MatchString(value)
}

func validName(value string) bool {
	return len(value) <= 64 && portableName.MatchString(value) && !strings.Contains(value, "--") && !strings.Contains(value, "..")
}
