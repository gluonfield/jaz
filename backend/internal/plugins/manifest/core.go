package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type coreManifest struct {
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		URL   string `json:"url"`
	} `json:"author"`
	Homepage   string                     `json:"homepage"`
	Repository string                     `json:"repository"`
	License    string                     `json:"license"`
	Keywords   []string                   `json:"keywords"`
	Extensions map[string]json.RawMessage `json:"extensions"`
}

type jazExtension struct {
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
	Page        *struct {
		View string `json:"view"`
		File string `json:"file"`
	} `json:"page"`
}

func readCore(data []byte) (Manifest, error) {
	var input coreManifest
	if err := json.Unmarshal(data, &input); err != nil {
		return Manifest{}, err
	}
	if input.Schema != Schema || !validName(input.Name) {
		return Manifest{}, errors.New("unsupported $schema or invalid plugin name")
	}
	plugin := Manifest{Name: input.Name, Description: input.Description, Version: input.Version}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Manifest{}, err
	}
	for field := range fields {
		switch field {
		case "$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions":
		default:
			plugin.Warnings = append(plugin.Warnings, fmt.Sprintf("Ignoring unknown manifest field %q", field))
		}
	}
	sort.Strings(plugin.Warnings)
	if value, exists := input.Extensions["chat.jaz"]; exists {
		var jaz jazExtension
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&jaz); err != nil {
			return Manifest{}, fmt.Errorf("chat.jaz: %w", err)
		}
		plugin.DisplayName = jaz.DisplayName
		plugin.IconPath = jaz.Icon
		if jaz.Page != nil {
			plugin.Page = &Page{View: jaz.Page.View, File: jaz.Page.File}
		}
	}
	return plugin, nil
}
