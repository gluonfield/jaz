// Package canvas owns a canvas thread's pages. Each page is a JSON Canvas 1.0
// board that the agent edits with file tools and the client patches over HTTP.
package canvas

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

type File struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

type Component struct {
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type PageView struct {
	Page
	Files      map[string]File      `json:"files"`
	Components map[string]Component `json:"components"`
}

type View struct {
	Version string     `json:"version"`
	Dir     string     `json:"dir"`
	Pages   []PageView `json:"pages"`
}

// Patch targets one page. Item fields set to null are removed; unknown item
// ids are appended. New pages require Create; an unknown target is an error.
type Patch struct {
	Page   string   `json:"page"`
	Create bool     `json:"create,omitempty"`
	Name   *string  `json:"name,omitempty"`
	Nodes  []Item   `json:"nodes,omitempty"`
	Edges  []Item   `json:"edges,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

type Snapshot struct {
	Version string
	dir     string
	pages   []PageView
}

type Service struct {
	root string
	mu   sync.Mutex
}

func NewService(layout runtimefiles.Layout) *Service {
	return &Service{root: layout.Canvases}
}

func (s *Service) Dir(sessionID string) string {
	return Dir(s.root, sessionID)
}

func Dir(canvasesRoot, sessionID string) string {
	return filepath.Join(canvasesRoot, sessionID)
}

func (s *Service) Load(sessionID string) (Snapshot, error) {
	dir := s.Dir(sessionID)
	raw, document, err := readDocument(dir)
	if err != nil {
		return Snapshot{}, err
	}
	hash := sha256.New()
	hash.Write([]byte("pages\x00"))
	hash.Write(raw)
	pages := make([]PageView, len(document.Pages))
	for i, page := range document.Pages {
		files := map[string]File{}
		for _, node := range page.Nodes {
			id, _ := node["id"].(string)
			path, _ := node["file"].(string)
			if node["type"] != "file" || path == "" {
				continue
			}
			file := File{Path: resolve(dir, path)}
			if info, err := os.Stat(file.Path); err == nil {
				file.Version = fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
			}
			files[id] = file
			fmt.Fprintf(hash, "\x00%s\x00%s\x00%s\x00%s", page.ID, id, file.Path, file.Version)
		}
		pages[i] = PageView{Page: page, Files: files}
	}
	return Snapshot{Version: hex.EncodeToString(hash.Sum(nil))[:24], dir: dir, pages: pages}, nil
}

func (s Snapshot) View() View {
	pages := make([]PageView, len(s.pages))
	for i, page := range s.pages {
		page.Components = map[string]Component{}
		for id, file := range page.Files {
			if file.Version != "" && IsComponent(file.Path) {
				page.Components[id] = compile(file.Path)
			}
		}
		pages[i] = page
	}
	return View{Version: s.Version, Dir: s.dir, Pages: pages}
}

func (s *Service) Patch(sessionID string, patch Patch) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validPageID(patch.Page) {
		return "", fmt.Errorf("%w: page is required and must not have surrounding whitespace", ErrInvalid)
	}
	dir := s.Dir(sessionID)
	_, document, err := readDocument(dir)
	if err != nil {
		return "", err
	}
	at := slices.IndexFunc(document.Pages, func(page Page) bool { return page.ID == patch.Page })
	if patch.Create {
		if at >= 0 {
			return "", fmt.Errorf("%w: page %q already exists", ErrInvalid, patch.Page)
		}
		name := nextPageName(document.Pages)
		document.Pages = append(document.Pages, Page{ID: patch.Page, Name: name, Board: Board{Nodes: []Item{}, Edges: []Item{}}})
		at = len(document.Pages) - 1
	}
	if at < 0 {
		return "", fmt.Errorf("%w: page %q does not exist", ErrInvalid, patch.Page)
	}
	page := &document.Pages[at]
	if patch.Name != nil {
		page.Name = strings.TrimSpace(*patch.Name)
		if page.Name == "" {
			return "", fmt.Errorf("%w: page name is required", ErrInvalid)
		}
	}
	if page.Nodes, err = upsert(page.Nodes, patch.Nodes); err != nil {
		return "", err
	}
	if page.Edges, err = upsert(page.Edges, patch.Edges); err != nil {
		return "", err
	}
	removeItems(&page.Board, patch.Remove)
	if err := validateBoard(page.Board); err != nil {
		return "", err
	}
	if err := writeDocument(dir, document); err != nil {
		return "", err
	}
	snapshot, err := s.Load(sessionID)
	return snapshot.Version, err
}

func resolve(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func upsert(items, changes []Item) ([]Item, error) {
	for _, change := range changes {
		id, _ := change["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("%w: every node and edge needs a string id", ErrInvalid)
		}
		at := slices.IndexFunc(items, func(item Item) bool { return item["id"] == id })
		if at < 0 {
			items = append(items, Item{})
			at = len(items) - 1
		}
		for key, value := range change {
			if value == nil {
				delete(items[at], key)
				continue
			}
			items[at][key] = value
		}
	}
	return items, nil
}

func removeItems(board *Board, ids []string) {
	if len(ids) == 0 {
		return
	}
	gone := func(item Item) bool {
		return slices.Contains(ids, item["id"].(string))
	}
	board.Nodes = slices.DeleteFunc(board.Nodes, gone)
	board.Edges = slices.DeleteFunc(board.Edges, func(edge Item) bool {
		from, _ := edge["fromNode"].(string)
		to, _ := edge["toNode"].(string)
		return gone(edge) || slices.Contains(ids, from) || slices.Contains(ids, to)
	})
}
