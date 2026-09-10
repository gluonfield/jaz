package browsercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type axValue struct {
	Value any `json:"value"`
}

type axProperty struct {
	Name  string  `json:"name"`
	Value axValue `json:"value"`
}

type axNode struct {
	ID          string       `json:"nodeId"`
	ParentID    string       `json:"parentId"`
	ChildIDs    []string     `json:"childIds"`
	Ignored     bool         `json:"ignored"`
	BackendID   int64        `json:"backendDOMNodeId"`
	Role        axValue      `json:"role"`
	Name        axValue      `json:"name"`
	Description axValue      `json:"description"`
	Value       axValue      `json:"value"`
	Properties  []axProperty `json:"properties"`
}

type axFrame struct {
	cdpFrame
	ParentID  string `json:"parentId"`
	SessionID string
	OwnerID   int64
}

type axTree struct {
	Frame    axFrame
	Nodes    []axNode `json:"nodes"`
	Children map[int64]*axTree
}

type axTarget struct {
	Frame     axFrame
	BackendID int64
}

type axLine struct {
	Index  int
	Parent int
	Text   string
}

type axState struct {
	next      int
	indices   map[string]int
	targets   map[string]axTarget
	lines     []axLine
	forceFull bool
}

func (p *browserPage) accessibilityState(ctx context.Context, full bool) (ActionOutput, error) {
	if err := p.waitReady(ctx); err != nil {
		return ActionOutput{}, err
	}
	frames, err := p.accessibilityFrames(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	if len(frames) == 0 {
		return ActionOutput{}, fmt.Errorf("browser has no accessibility document")
	}
	trees := make(map[string]*axTree, len(frames))
	for _, frame := range frames {
		if err := p.conn.callSession(ctx, frame.SessionID, "Accessibility.enable", nil, nil); err != nil {
			return ActionOutput{}, err
		}
		tree := &axTree{Frame: frame, Children: map[int64]*axTree{}}
		if err := p.conn.callSession(ctx, frame.SessionID, "Accessibility.getFullAXTree", map[string]any{"frameId": frame.ID}, tree); err != nil {
			return ActionOutput{}, fmt.Errorf("read accessibility frame %s: %w", frame.ID, err)
		}
		trees[frame.ID] = tree
	}
	for _, frame := range frames[1:] {
		trees[frame.ParentID].Children[frame.OwnerID] = trees[frame.ID]
	}
	next := axState{next: p.ax.next, indices: map[string]int{}, targets: map[string]axTarget{}}
	next.appendTree(trees[frames[0].ID], p.ax.indices, 0, -1)
	info, err := p.info(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	text := info + "\n" + renderAXRevision(p.ax.lines, next.lines, full || p.ax.forceFull || len(p.ax.lines) == 0)
	p.ax = next
	data, err := json.Marshal(text)
	return ActionOutput{Status: "ok", Text: text, Data: data}, err
}

func (s *axState) appendTree(tree *axTree, previous map[string]int, depth, parentIndex int) {
	frame := tree.Frame
	byID := make(map[string]axNode, len(tree.Nodes))
	for _, node := range tree.Nodes {
		byID[node.ID] = node
	}
	visited := map[string]bool{}
	var walk func(string, int, int)
	walk = func(id string, depth, parent int) {
		if visited[id] {
			return
		}
		visited[id] = true
		node, ok := byID[id]
		if !ok {
			return
		}
		if !node.Ignored && node.Role.Value != "InlineTextBox" {
			key := frame.ID + ":" + frame.LoaderID + ":" + node.ID + ":" + strconv.FormatInt(node.BackendID, 10)
			index, exists := previous[key]
			if !exists {
				index = s.next
				s.next++
			}
			s.indices[key] = index
			if node.BackendID != 0 {
				s.targets["ax:"+strconv.Itoa(index)] = axTarget{Frame: frame, BackendID: node.BackendID}
			}
			s.lines = append(s.lines, axLine{Index: index, Parent: parent, Text: strings.Repeat("\t", depth) + formatAXNode(index, node)})
			parent = index
			depth++
			if child := tree.Children[node.BackendID]; child != nil {
				s.appendTree(child, previous, depth, parent)
			}
		}
		for _, child := range node.ChildIDs {
			walk(child, depth, parent)
		}
	}
	for _, node := range tree.Nodes {
		if _, ok := byID[node.ParentID]; !ok {
			walk(node.ID, depth, parentIndex)
		}
	}
}

func formatAXNode(index int, node axNode) string {
	line := strconv.Itoa(index) + " " + axText(node.Role.Value)
	if name := axText(node.Name.Value); name != "" {
		line += " " + strconv.Quote(name)
	}
	if description := axText(node.Description.Value); description != "" {
		line += ", Description: " + strconv.Quote(description)
	}
	if value := axText(node.Value.Value); value != "" {
		line += ", Value: " + strconv.Quote(value)
	}
	for _, property := range node.Properties {
		switch property.Name {
		case "checked", "disabled", "expanded", "focused", "selected", "pressed", "required", "readonly", "level", "valuemin", "valuemax", "valuetext", "invalid", "hasPopup", "multiselectable", "multiline", "orientation", "keyshortcuts", "url", "busy", "live":
			line += ", " + property.Name + ": " + axText(property.Value.Value)
		}
	}
	return line
}

func axText(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func renderAXRevision(previous, current []axLine, full bool) string {
	var lines []string
	positions := make(map[int]int, len(previous))
	for i, line := range previous {
		positions[line.Index] = i
	}
	last, shared := -1, false
	for _, line := range current {
		if position, exists := positions[line.Index]; exists {
			shared = true
			full = full || position < last || previous[position].Parent != line.Parent
			last = position
		}
	}
	full = full || !shared
	if full {
		for _, line := range current {
			lines = append(lines, line.Text)
		}
		return strings.Join(lines, "\n")
	}
	old := make(map[int]string, len(previous))
	for _, line := range previous {
		old[line.Index] = line.Text
	}
	for _, line := range current {
		text, exists := old[line.Index]
		if !exists {
			lines = append(lines, "Added: "+line.Text)
		} else if text != line.Text {
			lines = append(lines, "Changed: "+line.Text)
		}
		delete(old, line.Index)
	}
	for _, line := range previous {
		if _, exists := old[line.Index]; exists {
			lines = append(lines, "Removed: "+line.Text)
		}
	}
	if len(lines) == 0 {
		return "Accessibility tree unchanged."
	}
	return strings.Join(lines, "\n")
}
