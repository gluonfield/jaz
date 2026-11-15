package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
)

type Page struct {
	View string `json:"view"`
	File string `json:"file"`
}

var filename = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

func validatePage(p *Page) error {
	if !name.MatchString(p.View) || !filename.MatchString(p.File) {
		return fmt.Errorf("%w: page needs a view name and a plain filename", ErrInvalid)
	}
	return nil
}

type SessionStore interface {
	ListSessionFolders() ([]storage.SessionFolder, error)
}

type PageContent struct {
	View  string     `json:"view"`
	Items []PageItem `json:"items"`
}

type PageItem struct {
	Folder  string       `json:"folder"`
	Data    any          `json:"data"`
	Threads []PageThread `json:"threads"`
}

type PageThread struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) Page(plugin string, sessions SessionStore) (PageContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !manifest.SafeName(plugin) {
		return PageContent{}, fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	installed, err := manifest.Read(filepath.Join(s.dir, plugin))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, manifest.ErrNotPlugin) {
		return PageContent{}, ErrNotFound
	}
	if err != nil {
		return PageContent{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if installed.Name != plugin || installed.Page == nil {
		return PageContent{}, ErrNotFound
	}
	if err := validatePage(&Page{View: installed.Page.View, File: installed.Page.File}); err != nil {
		return PageContent{}, err
	}
	html, err := s.view(plugin + "/" + installed.Page.View)
	if err != nil {
		return PageContent{}, err
	}
	threads, err := sessions.ListSessionFolders()
	if err != nil {
		return PageContent{}, err
	}
	slices.SortFunc(threads, func(a, b storage.SessionFolder) int {
		if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	folders := map[string][]PageThread{}
	var ordered []string
	for _, thread := range threads {
		if !filepath.IsAbs(thread.Folder) {
			continue
		}
		folder := filepath.Clean(thread.Folder)
		if _, found := folders[folder]; !found {
			ordered = append(ordered, folder)
		}
		folders[folder] = append(folders[folder], PageThread{ID: thread.ID, Title: thread.Title, Status: thread.Status, UpdatedAt: thread.UpdatedAt})
	}
	page := PageContent{View: html, Items: []PageItem{}}
	for _, folder := range ordered {
		data, err := os.ReadFile(filepath.Join(folder, installed.Page.File))
		var value any
		if err != nil || json.Unmarshal(data, &value) != nil {
			continue
		}
		page.Items = append(page.Items, PageItem{Folder: folder, Data: value, Threads: folders[folder]})
	}
	return page, nil
}
