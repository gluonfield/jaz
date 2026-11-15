package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	plugin.Warnings = unknownFields(fields, "", "$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions")
	if value, exists := input.Extensions["chat.jaz"]; exists {
		var jaz jazExtension
		if err := json.Unmarshal(value, &jaz); err != nil {
			return Manifest{}, fmt.Errorf("chat.jaz: %w", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(value, &fields); err != nil {
			return Manifest{}, err
		}
		plugin.Warnings = append(plugin.Warnings, unknownFields(fields, "extensions.chat.jaz.", "displayName", "icon", "page")...)
		plugin.DisplayName = jaz.DisplayName
		plugin.IconPath = jaz.Icon
		if jaz.Page != nil {
			plugin.Page = &Page{View: jaz.Page.View}
			var pageFields map[string]json.RawMessage
			if err := json.Unmarshal(fields["page"], &pageFields); err != nil {
				return Manifest{}, err
			}
			plugin.Warnings = append(plugin.Warnings, unknownFields(pageFields, "extensions.chat.jaz.page.", "view")...)
		}
	}
	slices.Sort(plugin.Warnings)
	return plugin, nil
}

func unknownFields(fields map[string]json.RawMessage, prefix string, allowed ...string) []string {
	var warnings []string
	for field := range fields {
		if !slices.Contains(allowed, field) {
			warnings = append(warnings, fmt.Sprintf("Ignoring unknown manifest field %q", prefix+field))
		}
	}
	return warnings
}
