package manifest

import (
	"fmt"
	"os"

	"github.com/dziblo-music/agoraform/internal/resource"
	"gopkg.in/yaml.v3"
)

const (
	// APIVersion is the v0.1 manifest schema identifier.
	APIVersion = "agoraform.io/v1alpha1"

	// DefaultFilename is the manifest path used when none is specified.
	DefaultFilename = "agoraform.yaml"
)

// Manifest is a parsed v0.1 Agoraform configuration document.
type Manifest struct {
	// Origin is the source path or label used in diagnostics.
	Origin string

	// APIVersion is the schema version declared in the file.
	APIVersion string

	// Resources are the desired resources from configuration.
	Resources []resource.Resource
}

type rawManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Resources  []rawResource `yaml:"resources"`
}

type rawResource struct {
	Address    string         `yaml:"address"`
	Attributes map[string]any `yaml:"attributes"`
}

// Parse decodes and structurally validates a YAML manifest.
func Parse(data []byte, origin string) (*Manifest, error) {
	if origin == "" {
		origin = "manifest"
	}
	if isEmptyYAML(data) {
		return nil, fmt.Errorf("%s: manifest is empty", origin)
	}

	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: malformed YAML: %w", origin, err)
	}

	if raw.APIVersion == "" {
		return nil, fmt.Errorf("%s: apiVersion is required", origin)
	}
	if raw.APIVersion != APIVersion {
		return nil, fmt.Errorf("%s: unsupported apiVersion %q (want %s)", origin, raw.APIVersion, APIVersion)
	}

	seen := make(map[string]int, len(raw.Resources))
	resources := make([]resource.Resource, 0, len(raw.Resources))
	for i, item := range raw.Resources {
		path := fmt.Sprintf("resources[%d]", i)
		if item.Address == "" {
			return nil, fmt.Errorf("%s: %s: address is required", origin, path)
		}

		addr, err := resource.ParseAddress(item.Address)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", origin, path, err)
		}

		key := addr.String()
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("%s: %s: duplicate resource address %q (first defined at resources[%d])", origin, path, key, first)
		}
		seen[key] = i

		attrs, err := normalizeAttributes(item.Attributes)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: attributes: %w", origin, path, err)
		}

		resources = append(resources, resource.Resource{
			Address:    addr,
			Attributes: attrs,
		})
	}

	return &Manifest{
		Origin:     origin,
		APIVersion: raw.APIVersion,
		Resources:  resources,
	}, nil
}

// LoadFile reads and parses a manifest from disk.
func LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return Parse(data, path)
}

func isEmptyYAML(data []byte) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}

func normalizeAttributes(in map[string]any) (resource.Attributes, error) {
	if in == nil {
		return resource.Attributes{}, nil
	}
	out := make(resource.Attributes, len(in))
	for k, v := range in {
		if k == "" {
			return nil, fmt.Errorf("attribute name is empty")
		}
		nv, err := normalizeValue(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = nv
	}
	return out, nil
}

func normalizeValue(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			nv, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("map key %v is not a string", k)
			}
			nv, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[ks] = nv
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			nv, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	default:
		return v, nil
	}
}
