package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

type coreManifest struct {
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Author      struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
		URL   string `json:"url,omitempty"`
	} `json:"author,omitempty"`
	Homepage   string   `json:"homepage,omitempty"`
	Repository string   `json:"repository,omitempty"`
	License    string   `json:"license,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
}

type jazExtension struct {
	DisplayName string `json:"displayName,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Page        *Page  `json:"page,omitempty"`
}

var coreSchema = sync.OnceValues(schemaFor[coreManifest])
var jazSchema = sync.OnceValues(schemaFor[jazExtension])

func schemaFor[T any]() (*jsonschema.Resolved, error) {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		return nil, err
	}
	return schema.Resolve(nil)
}

func readCore(data []byte) (Plugin, error) {
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return Plugin{}, errors.New("manifest must be a JSON object")
	}
	plugin := Plugin{}
	extensions, hasExtensions := fields["extensions"]
	delete(fields, "extensions")
	for field := range fields {
		switch field {
		case "$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords":
		default:
			plugin.Warnings = append(plugin.Warnings, fmt.Sprintf("Ignoring unknown manifest field %q", field))
			delete(fields, field)
		}
	}
	sort.Strings(plugin.Warnings)
	schema, err := coreSchema()
	if err != nil {
		return Plugin{}, err
	}
	if err := schema.Validate(fields); err != nil {
		return Plugin{}, err
	}
	cleaned, err := json.Marshal(fields)
	if err != nil {
		return Plugin{}, err
	}
	var input coreManifest
	if err := json.Unmarshal(cleaned, &input); err != nil {
		return Plugin{}, err
	}
	if input.Schema != Schema || !validName(input.Name) {
		return Plugin{}, errors.New("unsupported $schema or invalid plugin name")
	}
	plugin.Name = input.Name
	plugin.Description = input.Description
	plugin.Version = input.Version
	if !hasExtensions {
		return plugin, nil
	}
	namespaces, ok := extensions.(map[string]any)
	if !ok {
		plugin.Warnings = append(plugin.Warnings, "Ignoring non-object extensions")
		return plugin, nil
	}
	if value, exists := namespaces["chat.jaz"]; exists {
		schema, err := jazSchema()
		if err != nil {
			return Plugin{}, err
		}
		if err := schema.Validate(value); err != nil {
			return Plugin{}, fmt.Errorf("chat.jaz: %w", err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return Plugin{}, err
		}
		var jaz jazExtension
		if err := json.Unmarshal(encoded, &jaz); err != nil {
			return Plugin{}, err
		}
		plugin.DisplayName = jaz.DisplayName
		plugin.IconPath = jaz.Icon
		plugin.Page = jaz.Page
	}
	return plugin, nil
}
