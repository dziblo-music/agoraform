package asset

import (
	"fmt"
	"os"
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
// configured is the optional assets.root value. Relative roots are joined
// with baseDir, which must be the directory containing the selected
// manifest. Absolute roots are rejected because they are not portable.
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
	if configured != "" && configured != "." {
		if isForbiddenAbsolute(configured) {
			return Root{}, fmt.Errorf("assets.root must be a path relative to the manifest directory; absolute paths are not portable")
		}
		cleaned := filepath.Clean(configured)
		if escapesParent(cleaned) {
			return Root{}, fmt.Errorf("assets.root %q escapes the manifest directory", slashPath(configured))
		}
		dir = filepath.Join(baseAbs, cleaned)
		abs, err := filepath.Abs(dir)
		if err != nil {
			return Root{}, fmt.Errorf("cannot resolve assets.root %q", slashPath(cleaned))
		}
		if err := containPath(baseAbs, abs); err != nil {
			return Root{}, fmt.Errorf("assets.root %q escapes the manifest directory", slashPath(cleaned))
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return Root{}, fmt.Errorf("assets.root %q does not exist", slashPath(cleaned))
			}
			return Root{}, fmt.Errorf("cannot access assets.root %q", slashPath(cleaned))
		}
		if !info.IsDir() {
			return Root{}, fmt.Errorf("assets.root %q is not a directory", slashPath(cleaned))
		}
		dir = abs
		display = slashPath(cleaned)
	}
	return Root{Display: display, dir: dir}, nil
}

func isForbiddenAbsolute(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return false
}

func escapesParent(cleaned string) bool {
	if cleaned == ".." {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(cleaned, ".."+sep)
}

func slashPath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}
