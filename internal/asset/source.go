package asset

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// AttrName is the provider-neutral local-source attribute.
	AttrName = "source"

	// AttrFile is the relative file path inside AttrName.
	AttrFile = "file"

	// AttrDigest is the reviewable content fingerprint injected into
	// comparable plan attributes. It is not a manifest field.
	AttrDigest = "digest"
)

// SourceFile returns the relative file path declared under source.file.
//
// present is false when the resource does not declare a local source.
func SourceFile(attrs resource.Attributes) (path string, present bool, err error) {
	if attrs == nil {
		return "", false, nil
	}
	raw, ok := attrs[AttrName]
	if !ok || raw == nil {
		return "", false, nil
	}
	src, ok := raw.(map[string]any)
	if !ok {
		return "", true, fmt.Errorf("attribute %q must be an object with %s", AttrName, AttrFile)
	}
	for key := range src {
		if key != AttrFile {
			return "", true, fmt.Errorf("attribute %q may only contain %s", AttrName, AttrFile)
		}
	}
	file, ok := src[AttrFile]
	if !ok || file == nil {
		return "", true, fmt.Errorf("attribute %q.%s is required", AttrName, AttrFile)
	}
	s, ok := file.(string)
	if !ok {
		return "", true, fmt.Errorf("attribute %q.%s must be a string", AttrName, AttrFile)
	}
	if s == "" {
		return "", true, fmt.Errorf("attribute %q.%s must be a non-empty string", AttrName, AttrFile)
	}
	return s, true, nil
}
