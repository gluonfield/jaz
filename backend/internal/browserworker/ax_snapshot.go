package browserworker

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

type axNode struct {
	Ignored    bool     `json:"ignored"`
	Role       axValue  `json:"role"`
	Name       axValue  `json:"name"`
	Properties []axProp `json:"properties"`
	ChildIDs   []string `json:"childIds"`
}

type axProp struct {
	Name  string  `json:"name"`
	Value axValue `json:"value"`
}

type axValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

func (n axNode) snapshotLine() string {
	if n.Ignored {
		return ""
	}
	role := n.Role.String()
	name := n.Name.String()
	if role == "" || role == "none" || role == "generic" {
		return ""
	}
	if name == "" && role != "WebArea" {
		return ""
	}
	var parts []string
	parts = append(parts, "- "+role)
	if name != "" {
		parts = append(parts, strconv.Quote(trimForSnapshot(name, 160)))
	}
	for _, prop := range n.Properties {
		if prop.Name == "focused" || prop.Name == "selected" || prop.Name == "checked" || prop.Name == "expanded" {
			if value := prop.Value.String(); value != "" {
				parts = append(parts, prop.Name+"="+value)
			}
		}
	}
	return strings.Join(parts, " ")
}

func (v axValue) String() string {
	if len(v.Value) == 0 || bytes.Equal(v.Value, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var b bool
	if err := json.Unmarshal(v.Value, &b); err == nil {
		return strconv.FormatBool(b)
	}
	var f float64
	if err := json.Unmarshal(v.Value, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func trimForSnapshot(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
