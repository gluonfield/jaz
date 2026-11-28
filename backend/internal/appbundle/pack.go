package appbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func PackFiles(manifest Manifest, rawFiles map[string][]byte) (Bundle, error) {
	if err := ValidateManifest(&manifest); err != nil {
		return Bundle{}, err
	}
	canonicalManifest, err := canonicalManifestBytes(manifest)
	if err != nil {
		return Bundle{}, fmt.Errorf("encode canonical manifest: %w", err)
	}

	seenLower := make(map[string]string, len(rawFiles)+1)
	seenLower[strings.ToLower(ManifestPath)] = ManifestPath

	entries := make([]FileEntry, 0, len(rawFiles)+1)
	manifestSize := int64(len(canonicalManifest))
	if manifestSize > MaxFileBytes {
		return Bundle{}, ErrFileTooLarge
	}
	entries = append(entries, FileEntry{
		Path:        ManifestPath,
		SizeBytes:   manifestSize,
		SHA256:      sha256HexBytes(canonicalManifest),
		ContentType: ResolveContentType(ManifestPath),
		Data:        append([]byte(nil), canonicalManifest...),
	})
	totalBytes := manifestSize

	keys := make([]string, 0, len(rawFiles))
	for k := range rawFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var (
		hasEntry bool
		hasIcon  = manifest.Icon == nil || manifest.Icon.Kind != "path"
	)

	for _, rawPath := range keys {
		if rawPath == ManifestPath {
			continue
		}
		cleanPath, err := ValidatePath(rawPath)
		if err != nil {
			return Bundle{}, err
		}
		lower := strings.ToLower(cleanPath)
		if prev, exists := seenLower[lower]; exists {
			return Bundle{}, fmt.Errorf("%w: %q collides with %q", ErrDuplicatePath, cleanPath, prev)
		}
		seenLower[lower] = cleanPath

		if len(entries)+1 > MaxFileCount {
			return Bundle{}, ErrTooManyFiles
		}
		data := rawFiles[rawPath]
		fileSize := int64(len(data))
		if fileSize > MaxFileBytes {
			return Bundle{}, fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, cleanPath, fileSize)
		}
		totalBytes += fileSize
		if totalBytes > MaxBundleBytes {
			return Bundle{}, ErrBundleTooLarge
		}

		copied := append([]byte(nil), data...)
		entries = append(entries, FileEntry{
			Path:        cleanPath,
			SizeBytes:   fileSize,
			SHA256:      sha256HexBytes(copied),
			ContentType: ResolveContentType(cleanPath),
			Data:        copied,
		})
		if cleanPath == manifest.Entry {
			hasEntry = true
		}
		if manifest.Icon != nil && manifest.Icon.Kind == "path" && cleanPath == manifest.Icon.Value {
			hasIcon = true
		}
	}

	if !hasEntry {
		return Bundle{}, fmt.Errorf("%w: %s", ErrMissingEntry, manifest.Entry)
	}
	if !hasIcon {
		return Bundle{}, fmt.Errorf("%w: icon path %q not found in bundle", ErrInvalidManifest, manifest.Icon.Value)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	externalURLs, warnings := AuditFiles(entries, manifest)
	bundleHash := ComputeBundleSHA256(entries)

	return Bundle{
		Manifest:     manifest,
		ManifestJSON: canonicalManifest,
		BundleSHA256: bundleHash,
		SizeBytes:    totalBytes,
		FileCount:    len(entries),
		Files:        entries,
		ExternalURLs: externalURLs,
		Warnings:     warnings,
	}, nil
}

func PackZipBytes(raw []byte) (Bundle, error) {
	return PackZip(bytes.NewReader(raw), int64(len(raw)))
}

func PackZip(r io.ReaderAt, size int64) (Bundle, error) {
	if size <= 0 || size > MaxBundleBytes*2 {
		return Bundle{}, fmt.Errorf("%w: invalid zip archive size", ErrBundleTooLarge)
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Bundle{}, fmt.Errorf("open zip bundle: %w", err)
	}

	files := make(map[string][]byte)
	seenLower := make(map[string]string)
	var (
		totalBytes int64
		fileCount  int
		manifest   Manifest
		hasManif   bool
	)

	for _, f := range zr.File {
		mode := f.Mode()
		if mode&fs.ModeSymlink != 0 {
			return Bundle{}, fmt.Errorf("%w: %q", ErrSymlinkBlocked, f.Name)
		}
		if f.FileInfo().IsDir() {
			trimmedDir := strings.TrimSuffix(f.Name, "/")
			if trimmedDir != "" {
				if _, err := ValidatePath(trimmedDir); err != nil {
					return Bundle{}, err
				}
			}
			continue
		}
		if mode.Type() != 0 {
			return Bundle{}, fmt.Errorf("%w: non-regular file %q", ErrSymlinkBlocked, f.Name)
		}

		cleanPath, err := ValidatePath(f.Name)
		if err != nil {
			return Bundle{}, err
		}
		lower := strings.ToLower(cleanPath)
		if prev, exists := seenLower[lower]; exists {
			return Bundle{}, fmt.Errorf("%w: %q collides with %q", ErrDuplicatePath, cleanPath, prev)
		}
		seenLower[lower] = cleanPath

		fileCount++
		if fileCount > MaxFileCount {
			return Bundle{}, ErrTooManyFiles
		}
		if f.UncompressedSize64 > uint64(MaxFileBytes) {
			return Bundle{}, fmt.Errorf("%w: %s", ErrFileTooLarge, cleanPath)
		}

		rc, err := f.Open()
		if err != nil {
			return Bundle{}, fmt.Errorf("read zip entry %q: %w", cleanPath, err)
		}
		remaining := MaxBundleBytes - totalBytes
		limit := MaxFileBytes
		if remaining < limit {
			limit = remaining
		}
		data, rerr := io.ReadAll(io.LimitReader(rc, limit+1))
		_ = rc.Close()
		if rerr != nil {
			return Bundle{}, fmt.Errorf("read zip entry %q: %w", cleanPath, rerr)
		}
		if int64(len(data)) > MaxFileBytes {
			return Bundle{}, fmt.Errorf("%w: %s", ErrFileTooLarge, cleanPath)
		}
		totalBytes += int64(len(data))
		if totalBytes > MaxBundleBytes {
			return Bundle{}, ErrBundleTooLarge
		}

		if cleanPath == ManifestPath {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return Bundle{}, fmt.Errorf("%w: parse %s: %v", ErrInvalidManifest, ManifestPath, err)
			}
			hasManif = true
			continue
		}
		files[cleanPath] = data
	}

	if !hasManif {
		return Bundle{}, fmt.Errorf("%w: missing %s", ErrInvalidManifest, ManifestPath)
	}
	return PackFiles(manifest, files)
}

func PackDir(rootDir string) (Bundle, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return Bundle{}, err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return Bundle{}, err
	}

	files := make(map[string][]byte)
	var (
		totalBytes int64
		fileCount  int
		manifest   Manifest
		hasManif   bool
	)

	walkErr := filepath.WalkDir(realRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == realRoot {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlinkBlocked, p)
		}
		rel, err := filepath.Rel(realRoot, p)
		if err != nil {
			return err
		}
		slashRel := filepath.ToSlash(rel)
		if d.IsDir() {
			if _, err := ValidatePath(slashRel); err != nil {
				return err
			}
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrSymlinkBlocked, slashRel)
		}
		cleanPath, err := ValidatePath(slashRel)
		if err != nil {
			return err
		}
		fileCount++
		if fileCount > MaxFileCount {
			return ErrTooManyFiles
		}
		if info.Size() > MaxFileBytes {
			return fmt.Errorf("%w: %s", ErrFileTooLarge, cleanPath)
		}
		totalBytes += info.Size()
		if totalBytes > MaxBundleBytes {
			return ErrBundleTooLarge
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if int64(len(data)) > MaxFileBytes {
			return fmt.Errorf("%w: %s", ErrFileTooLarge, cleanPath)
		}
		if cleanPath == ManifestPath {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return fmt.Errorf("%w: parse %s: %v", ErrInvalidManifest, ManifestPath, err)
			}
			hasManif = true
			return nil
		}
		files[cleanPath] = data
		return nil
	})
	if walkErr != nil {
		return Bundle{}, walkErr
	}
	if !hasManif {
		return Bundle{}, fmt.Errorf("%w: missing %s", ErrInvalidManifest, ManifestPath)
	}
	return PackFiles(manifest, files)
}
