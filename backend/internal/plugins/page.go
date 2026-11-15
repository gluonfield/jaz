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

	"github.com/wins/jaz/backend/internal/storage"
)

type Page struct {
	Title string `json:"title"`
	Icon  string `json:"icon"`
	View  string `json:"view"`
	File  string `json:"file"`
}

var filename = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

func (p Page) validate(views []string) error {
	if strings.TrimSpace(p.Title) == "" || !name.MatchString(p.View) || !slices.Contains(views, p.View) || !filename.MatchString(p.File) {
		return fmt.Errorf("%w: page needs a title, an existing view name and a plain filename", ErrInvalid)
	}
	return nil
}

type SessionStore interface {
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
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
	if !name.MatchString(plugin) {
		return PageContent{}, fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	installed, err := read(filepath.Join(s.dir, plugin))
	if errors.Is(err, fs.ErrNotExist) {
		return PageContent{}, ErrNotFound
	}
	if err != nil {
		return PageContent{}, err
	}
	if installed.Name != plugin || installed.Page == nil {
		return PageContent{}, ErrNotFound
	}
	html, err := s.View(plugin + "/" + installed.Page.View)
	if err != nil {
		return PageContent{}, err
	}
	var threads []storage.Session
	for _, archived := range []bool{false, true} {
		batch, err := sessions.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
		if err != nil {
			return PageContent{}, err
		}
		threads = append(threads, batch...)
	}
	slices.SortFunc(threads, func(a, b storage.Session) int {
		if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	page := PageContent{View: html, Items: []PageItem{}}
	folders := map[string]int{}
	for _, thread := range threads {
		if thread.RuntimeRef == nil || !filepath.IsAbs(thread.RuntimeRef.Cwd) {
			continue
		}
		folder := filepath.Clean(thread.RuntimeRef.Cwd)
		index, found := folders[folder]
		if !found {
			index = len(page.Items)
			folders[folder] = index
			data, err := os.ReadFile(filepath.Join(folder, installed.Page.File))
			var value any
			if err != nil || json.Unmarshal(data, &value) != nil {
				folders[folder] = -1
				continue
			}
			page.Items = append(page.Items, PageItem{Folder: folder, Data: value, Threads: []PageThread{}})
		}
		if index >= 0 {
			page.Items[index].Threads = append(page.Items[index].Threads, PageThread{ID: thread.ID, Title: thread.Title, Status: thread.Status, UpdatedAt: thread.UpdatedAt})
		}
	}
	return page, nil
}
