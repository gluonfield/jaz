package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

const defaultAttachmentMaxBytes int64 = 32 << 20

func (s *Server) resolveAttachments(sessionID string, ids []string) ([]storage.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	root, err := s.attachmentRoot()
	if err != nil {
		return nil, err
	}
	out := make([]storage.Attachment, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !validAttachmentID(id) {
			return nil, fmt.Errorf("attachment %s not found for this session", id)
		}
		sessionDir := filepath.Join(root, sessionID)
		attachment, err := readAttachmentMetadata(sessionDir, id)
		if err != nil {
			return nil, fmt.Errorf("attachment %s not found for this session", id)
		}
		path, err := trustedAttachmentPath(sessionDir, &attachment)
		if err != nil {
			return nil, fmt.Errorf("attachment %s not found for this session", id)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("attachment %s is not readable", id)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("attachment %s is not readable", id)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("attachment %s is a directory", id)
		}
		attachment.Size = info.Size()
		out = append(out, attachment)
	}
	return out, nil
}

func (s *Server) attachmentRoot() (string, error) {
	base := strings.TrimSpace(s.Workspace)
	if base == "" {
		base = strings.TrimSpace(s.Root)
	}
	if base == "" {
		return "", fmt.Errorf("workspace is not configured")
	}
	root, err := filepath.Abs(filepath.Join(base, ".jaz-attachments"))
	if err != nil {
		return "", err
	}
	return root, nil
}

func attachmentMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("JAZ_ATTACHMENT_MAX_BYTES"))
	if raw == "" {
		return defaultAttachmentMaxBytes
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultAttachmentMaxBytes
	}
	return value
}

func newAttachmentID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func validAttachmentID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func safeAttachmentName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name = strings.Trim(b.String(), ". ")
	if name == "" {
		name = "attachment"
	}
	if len(name) <= 120 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if len(ext) > 24 {
		ext = ""
	}
	limit := 120 - len(ext)
	if limit < 1 {
		limit = 120
	}
	if len(base) > limit {
		base = base[:limit]
	}
	return strings.Trim(base, ". ") + ext
}

func attachmentMimeType(uploaded, name string) string {
	if uploaded = strings.TrimSpace(uploaded); uploaded != "" {
		return uploaded
	}
	if byExt := mime.TypeByExtension(filepath.Ext(name)); byExt != "" {
		return byExt
	}
	return "application/octet-stream"
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func writeAttachmentMetadata(dir string, attachment storage.Attachment) error {
	data, err := json.MarshalIndent(attachment, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, attachment.ID+".json"), data, 0o600)
}

func readAttachmentMetadata(dir, id string) (storage.Attachment, error) {
	if !validAttachmentID(id) {
		return storage.Attachment{}, fmt.Errorf("invalid attachment id")
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return storage.Attachment{}, err
	}
	var attachment storage.Attachment
	if err := json.Unmarshal(data, &attachment); err != nil {
		return storage.Attachment{}, err
	}
	if attachment.ID != id || strings.TrimSpace(attachment.Name) == "" {
		return storage.Attachment{}, fmt.Errorf("invalid metadata")
	}
	return attachment, nil
}

func trustedAttachmentPath(sessionDir string, attachment *storage.Attachment) (string, error) {
	if attachment == nil || !validAttachmentID(attachment.ID) {
		return "", fmt.Errorf("invalid attachment metadata")
	}
	absDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", err
	}
	name := safeAttachmentName(attachment.Name)
	path, err := filepath.Abs(filepath.Join(absDir, attachment.ID+"-"+name))
	if err != nil {
		return "", err
	}
	if !pathWithin(absDir, path) {
		return "", fmt.Errorf("attachment path escapes session")
	}
	attachment.Name = name
	attachment.ServerPath = path
	attachment.URI = fileURI(path)
	return path, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
