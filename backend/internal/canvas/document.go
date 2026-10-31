package canvas

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

const FileName = "canvas.json"

var ErrInvalid = errors.New("invalid canvas")

// Item is one JSON Canvas node or edge. It stays a loose map so fields the
// agent adds survive a client patch untouched.
type Item map[string]any

type Board struct {
	Nodes []Item `json:"nodes"`
	Edges []Item `json:"edges"`
}

type Page struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Board
}

type Document struct {
	Pages []Page `json:"pages"`
}

func readDocument(dir string) ([]byte, Document, error) {
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Document{Pages: []Page{{ID: "page-1", Name: "Page 1", Board: Board{Nodes: []Item{}, Edges: []Item{}}}}}, nil
	}
	if err != nil {
		return nil, Document{}, err
	}
	var stored struct {
		Pages json.RawMessage `json:"pages"`
		Board
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, Document{}, fmt.Errorf("%w: %s: %v", ErrInvalid, FileName, err)
	}
	document := Document{}
	if len(stored.Pages) == 0 {
		document.Pages = []Page{{ID: "page-1", Name: "Page 1", Board: stored.Board}}
	} else {
		if len(stored.Nodes) > 0 || len(stored.Edges) > 0 {
			return nil, Document{}, fmt.Errorf("%w: move root nodes and edges into a page", ErrInvalid)
		}
		if err := json.Unmarshal(stored.Pages, &document.Pages); err != nil || len(document.Pages) == 0 {
			return nil, Document{}, fmt.Errorf("%w: pages must be a nonempty array of pages", ErrInvalid)
		}
	}
	ids := map[string]bool{}
	for i := range document.Pages {
		page := &document.Pages[i]
		if !validPageID(page.ID) || ids[page.ID] {
			return nil, Document{}, fmt.Errorf("%w: every page needs a unique nonempty id without surrounding whitespace", ErrInvalid)
		}
		ids[page.ID] = true
		page.Name = strings.TrimSpace(page.Name)
		if page.Name == "" {
			page.Name = fmt.Sprintf("Page %d", i+1)
		}
		if err := validateBoard(page.Board); err != nil {
			return nil, Document{}, err
		}
		if page.Nodes == nil {
			page.Nodes = []Item{}
		}
		if page.Edges == nil {
			page.Edges = []Item{}
		}
	}
	return raw, document, nil
}

func validPageID(id string) bool {
	return id != "" && id == strings.TrimFunc(id, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\ufeff'
	})
}

func validateBoard(board Board) error {
	ids := map[string]bool{}
	for _, items := range [][]Item{board.Nodes, board.Edges} {
		for _, item := range items {
			id, _ := item["id"].(string)
			if strings.TrimSpace(id) == "" || ids[id] {
				return fmt.Errorf("%w: every node and edge needs a unique string id within its page", ErrInvalid)
			}
			ids[id] = true
		}
	}
	return nil
}

func nextPageName(pages []Page) string {
	for n := len(pages) + 1; ; n++ {
		name := fmt.Sprintf("Page %d", n)
		if !slices.ContainsFunc(pages, func(page Page) bool { return page.Name == name }) {
			return name
		}
	}
}

// writeDocument keeps one item per line for the agent's line-based edits.
func writeDocument(dir string, document Document) error {
	var out bytes.Buffer
	out.WriteString("{\n  \"pages\": [")
	for i, page := range document.Pages {
		if i > 0 {
			out.WriteString(",")
		}
		id, _ := json.Marshal(page.ID)
		name, _ := json.Marshal(page.Name)
		fmt.Fprintf(&out, "\n    {\"id\":%s,\"name\":%s,\"nodes\":[", id, name)
		if err := writeItems(&out, page.Nodes); err != nil {
			return err
		}
		out.WriteString("],\"edges\":[")
		if err := writeItems(&out, page.Edges); err != nil {
			return err
		}
		out.WriteString("]}")
	}
	out.WriteString("\n  ]\n}\n")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".canvas-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(out.Bytes()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, FileName))
}

// Markdown in text nodes stays readable: no HTML escaping of < > &.
func writeItems(out *bytes.Buffer, items []Item) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	for i, item := range items {
		if i > 0 {
			out.WriteString(",")
		}
		out.WriteString("\n      ")
		if err := encoder.Encode(item); err != nil {
			return err
		}
		out.Truncate(out.Len() - 1)
	}
	if len(items) > 0 {
		out.WriteString("\n    ")
	}
	return nil
}
