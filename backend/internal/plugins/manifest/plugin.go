package manifest

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

const Schema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var ErrNotPlugin = errors.New("not a plugin: no supported manifest")

type Manifest struct {
	Name            string
	DisplayName     string
	Description     string
	Version         string
	IconPath        string
	Page            *Page
	Warnings        []string
	ManifestPath    string
	ExtraSkillPaths []string
	MCPServers      json.RawMessage
}

type Page struct {
	View string
	File string
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$`)
var portableName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func SafeName(value string) bool {
	return safeName.MatchString(value)
}

func validName(value string) bool {
	return len(value) <= 64 && portableName.MatchString(value) && !strings.Contains(value, "--") && !strings.Contains(value, "..")
}
