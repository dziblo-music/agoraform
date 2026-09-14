package resource

import (
	"fmt"
	"io"
)

// DigestAlgorithm is the content fingerprint algorithm used for local assets.
const DigestAlgorithm = "sha256"

// LocalAsset is a provider-neutral descriptor for a finished local file.
//
// Path is a deterministic project-relative path using forward slashes.
// Digest is the lowercase hex SHA-256 of the file bytes. Open streams those
// bytes at apply time without placing them in attributes, plans, logs, or
// state. Absolute host paths stay unexported.
type LocalAsset struct {
	Path      string
	Digest    string
	Size      int64
	MediaType string
	open      func() (io.ReadCloser, error)
}

// NewLocalAsset constructs a descriptor. open must stream the file bytes;
// it may be nil only in tests that do not call Open.
func NewLocalAsset(path, digest string, size int64, mediaType string, open func() (io.ReadCloser, error)) LocalAsset {
	return LocalAsset{
		Path:      path,
		Digest:    digest,
		Size:      size,
		MediaType: mediaType,
		open:      open,
	}
}

// DigestLabel returns the reviewable fingerprint, for example sha256:abc....
func (a LocalAsset) DigestLabel() string {
	if a.Digest == "" {
		return ""
	}
	return DigestAlgorithm + ":" + a.Digest
}

// Open returns a stream of the local file bytes. Callers must close it.
func (a LocalAsset) Open() (io.ReadCloser, error) {
	if a.open == nil {
		return nil, fmt.Errorf("local asset %q cannot be opened", a.Path)
	}
	return a.open()
}

// String returns a reviewable relative path and digest. It never includes
// file contents or an absolute host path.
func (a LocalAsset) String() string {
	if a.Path == "" && a.Digest == "" {
		return ""
	}
	if a.Digest == "" {
		return a.Path
	}
	if a.Path == "" {
		return a.DigestLabel()
	}
	return a.Path + " " + a.DigestLabel()
}

// GoString implements fmt.GoStringer so %#v does not leak the opener.
func (a LocalAsset) GoString() string {
	return fmt.Sprintf("resource.LocalAsset{Path:%q, Digest:%q, Size:%d, MediaType:%q}", a.Path, a.Digest, a.Size, a.MediaType)
}
