package asset

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Root is the directory used to resolve local asset file paths.
type Root struct {
	// Display is the project-relative root as declared, or "." when omitted.
	Display string
	dir     string
}

// NewRoot resolves the asset root against the manifest directory.
//
// configured is the optional assets.root value. Manifest paths always use
// slash semantics, independent of the host OS. Relative roots are joined with
// baseDir, which must be the directory containing the selected manifest.
// Absolute roots are rejected because they are not portable.
func NewRoot(baseDir, configured string) (Root, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return Root{}, fmt.Errorf("local assets require a file-backed manifest")
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return Root{}, fmt.Errorf("cannot resolve manifest directory")
	}
	info, err := os.Stat(baseAbs)
	if err != nil || !info.IsDir() {
		return Root{}, fmt.Errorf("manifest directory is not accessible")
	}

	configured = strings.TrimSpace(configured)
	display := "."
	dir := baseAbs
	if configured != "" {
		if isForbiddenAbsolute(configured) {
			return Root{}, fmt.Errorf("assets.root must be a path relative to the manifest directory; absolute paths are not portable")
		}
		cleaned := slashPath(configured)
		if cleaned == "." {
			return Root{Display: display, dir: dir}, nil
		}
		if escapesParent(cleaned) {
			return Root{}, fmt.Errorf("assets.root %q escapes the manifest directory", cleaned)
		}
		dir = filepath.Join(baseAbs, filepath.FromSlash(cleaned))
		abs, err := filepath.Abs(dir)
		if err != nil {
			return Root{}, fmt.Errorf("cannot resolve assets.root %q", cleaned)
		}
		if err := containPath(baseAbs, abs); err != nil {
			return Root{}, fmt.Errorf("assets.root %q escapes the manifest directory", cleaned)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return Root{}, fmt.Errorf("assets.root %q does not exist", cleaned)
			}
			return Root{}, fmt.Errorf("cannot access assets.root %q", cleaned)
		}
		if !info.IsDir() {
			return Root{}, fmt.Errorf("assets.root %q is not a directory", cleaned)
		}

		resolvedBase, err := evalExisting(baseAbs)
		if err != nil {
			return Root{}, fmt.Errorf("cannot resolve manifest directory")
		}
		resolvedRoot, err := evalExisting(abs)
		if err != nil {
			return Root{}, fmt.Errorf("cannot resolve assets.root %q", cleaned)
		}
		if err := containPath(resolvedBase, resolvedRoot); err != nil {
			return Root{}, fmt.Errorf("assets.root %q escapes the manifest directory through a symlink", cleaned)
		}
		dir = resolvedRoot
		display = cleaned
	}
	return Root{Display: display, dir: dir}, nil
}

func isForbiddenAbsolute(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	normalized := strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return true
	}
	if len(normalized) >= 2 && normalized[1] == ':' {
		return true
	}
	return false
}

func escapesParent(p string) bool {
	cleaned := slashPath(p)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

func slashPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}
