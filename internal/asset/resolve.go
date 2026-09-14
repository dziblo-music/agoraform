package asset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Bind resolves source.file attributes onto resources in place.
//
// origin is the manifest path used in diagnostics. baseDir is the manifest
// directory. configuredRoot is the optional assets.root value. Missing,
// unreadable, directory, traversal, and symlink-escape paths fail before
// providers run.
func Bind(origin, baseDir, configuredRoot string, resources []resource.Resource) error {
	if origin == "" {
		origin = "manifest"
	}
	needRoot := false
	for _, res := range resources {
		_, present, err := SourceFile(res.Attributes)
		if err != nil {
			return fmt.Errorf("%s: resource %s: %w", origin, res.Address, err)
		}
		if present {
			needRoot = true
			break
		}
	}
	if strings.TrimSpace(configuredRoot) != "" && strings.TrimSpace(baseDir) != "" {
		needRoot = true
	}
	if !needRoot {
		return nil
	}

	root, err := NewRoot(baseDir, configuredRoot)
	if err != nil {
		return fmt.Errorf("%s: %w", origin, err)
	}

	for i := range resources {
		file, present, err := SourceFile(resources[i].Attributes)
		if err != nil {
			return fmt.Errorf("%s: resource %s: %w", origin, resources[i].Address, err)
		}
		if !present {
			continue
		}
		asset, err := root.Resolve(file)
		if err != nil {
			return fmt.Errorf("%s: resource %s: attribute %s.%s: %w", origin, resources[i].Address, AttrName, AttrFile, err)
		}
		resources[i].LocalAsset = &asset
	}
	return nil
}

// Resolve fingerprints a file declared relative to the asset root.
func (r Root) Resolve(file string) (resource.LocalAsset, error) {
	display, abs, err := r.safeFile(file)
	if err != nil {
		return resource.LocalAsset{}, err
	}

	f, err := os.Open(abs)
	if err != nil {
		return resource.LocalAsset{}, accessError(display, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return resource.LocalAsset{}, accessError(display, err)
	}
	if info.IsDir() {
		return resource.LocalAsset{}, fmt.Errorf("%q is a directory, not a file", display)
	}

	digest, mediaType, err := fingerprint(f, display)
	if err != nil {
		return resource.LocalAsset{}, err
	}

	openPath := abs
	return resource.NewLocalAsset(display, digest, info.Size(), mediaType, func() (io.ReadCloser, error) {
		opened, err := os.Open(openPath)
		if err != nil {
			return nil, accessError(display, err)
		}
		return opened, nil
	}), nil
}

func (r Root) safeFile(file string) (display, abs string, err error) {
	raw := strings.TrimSpace(file)
	if raw == "" {
		return "", "", fmt.Errorf("file path is empty")
	}
	if isForbiddenAbsolute(raw) {
		return "", "", fmt.Errorf("file path must be relative to the asset root; absolute paths are not portable")
	}

	cleaned := filepath.Clean(raw)
	display = slashPath(cleaned)
	if display == "." || strings.HasSuffix(display, "/") {
		return "", "", fmt.Errorf("%q is a directory, not a file", display)
	}
	if escapesParent(cleaned) || strings.Contains(display, "/../") || display == ".." {
		return "", "", fmt.Errorf("file %q escapes the asset root", display)
	}

	joined := filepath.Join(r.dir, cleaned)
	abs, err = filepath.Abs(joined)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve file %q", display)
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return "", "", accessError(display, err)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("%q is a directory, not a file", display)
	}

	resolvedRoot, err := evalExisting(r.dir)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve asset root")
	}
	resolvedFile, err := evalExisting(abs)
	if err != nil {
		return "", "", accessError(display, err)
	}
	resolvedInfo, err := os.Stat(resolvedFile)
	if err != nil {
		return "", "", accessError(display, err)
	}
	if resolvedInfo.IsDir() {
		return "", "", fmt.Errorf("%q is a directory, not a file", display)
	}
	if err := containPath(resolvedRoot, resolvedFile); err != nil {
		return "", "", fmt.Errorf("file %q escapes the asset root", display)
	}
	return display, resolvedFile, nil
}

func fingerprint(r io.Reader, display string) (digest, mediaType string, err error) {
	h := sha256.New()
	head := make([]byte, 512)
	n, readErr := io.ReadFull(r, head)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", "", fmt.Errorf("cannot read file %q", display)
	}
	head = head[:n]
	if _, err := h.Write(head); err != nil {
		return "", "", fmt.Errorf("cannot fingerprint file %q", display)
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", "", fmt.Errorf("cannot fingerprint file %q", display)
	}
	return hex.EncodeToString(h.Sum(nil)), detectMediaType(display, head), nil
}

func detectMediaType(display string, head []byte) string {
	detected := http.DetectContentType(head)
	if detected != "application/octet-stream" {
		return detected
	}
	if ext := strings.ToLower(filepath.Ext(display)); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}
	return detected
}

func evalExisting(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func containPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	rel = filepath.Clean(rel)
	if escapesParent(rel) {
		return fmt.Errorf("escapes root")
	}
	return nil
}

func accessError(display string, err error) error {
	if err == nil {
		return fmt.Errorf("cannot access file %q", display)
	}
	if os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", display)
	}
	if os.IsPermission(err) {
		return fmt.Errorf("file is unreadable: %s", display)
	}
	return fmt.Errorf("cannot access file %q", display)
}
