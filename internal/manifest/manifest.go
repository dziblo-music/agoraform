package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/resource"
	"gopkg.in/yaml.v3"
)

const (
	// APIVersion is the v0.1 manifest schema identifier.
	APIVersion = "agoraform.io/v1alpha1"

	// DefaultFilename is the manifest path used when none is specified.
	DefaultFilename = "agoraform.yaml"
)

// Manifest is a parsed Agoraform configuration document.
type Manifest struct {
	// Origin is the source path or label used in diagnostics.
	Origin string

	// BaseDir is the directory containing the manifest file. It is populated
	// by LoadFile and used to resolve relative file paths in resource
	// attributes (such as file: in meta.image). Empty when the manifest is
	// parsed from an in-memory source.
	BaseDir string

	// APIVersion is the schema version declared in the file.
	APIVersion string

	// Providers contains non-secret, declarative provider-specific desired
	// state. Credentials and connection settings do not belong here.
	Providers map[string]resource.Attributes

	// Resources are the desired resources from configuration.
	Resources []resource.Resource

	// Assets is optional local-file source configuration. An omitted or
	// empty block is valid and keeps existing manifests backward compatible.
	Assets Assets

	// ApplicationEvents is the optional provider-neutral instrumentation
	// contract. Each key is a logical application event name; the value
	// describes the provider-side bindings the application must satisfy to
	// connect its event emission to the managed marketing infrastructure.
	// An omitted or empty block is valid.
	ApplicationEvents map[string]ApplicationEvent
}

// Assets is provider-neutral local file source configuration.
type Assets struct {
	// Root is a directory relative to the manifest file. When empty, local
	// source paths resolve against the manifest directory itself.
	Root string
}

type rawManifest struct {
	APIVersion        string                    `yaml:"apiVersion"`
	Providers         map[string]map[string]any `yaml:"providers"`
	Assets            map[string]any            `yaml:"assets"`
	Resources         []rawResource             `yaml:"resources"`
	ApplicationEvents map[string]any            `yaml:"applicationEvents"`
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

	assets, err := parseAssets(origin, raw.Assets)
	if err != nil {
		return nil, err
	}

	providers := make(map[string]resource.Attributes, len(raw.Providers))
	for name, attrs := range raw.Providers {
		addr := resource.Address{Provider: name, Type: "config", Name: "main"}
		if err := addr.Validate(); err != nil {
			return nil, fmt.Errorf("%s: providers.%s: invalid provider name", origin, name)
		}
		normalized, err := normalizeAttributes(attrs)
		if err != nil {
			return nil, fmt.Errorf("%s: providers.%s: %w", origin, name, err)
		}
		providers[name] = normalized
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

	if _, err := graph.Build(resources); err != nil {
		return nil, fmt.Errorf("%s: %w", origin, err)
	}

	appEvents, err := parseApplicationEvents(origin, raw.ApplicationEvents)
	if err != nil {
		return nil, err
	}

	return &Manifest{
		Origin:            origin,
		APIVersion:        raw.APIVersion,
		Providers:         providers,
		Resources:         resources,
		Assets:            assets,
		ApplicationEvents: appEvents,
	}, nil
}

// LoadFile reads and parses a manifest from disk.
//
// The manifest's BaseDir is set to the absolute directory of path so that
// callers can resolve relative resource file paths against the manifest
// location. This mirrors how users run agoraform: from the directory
// containing agoraform.yaml, making relative paths resolve naturally.
func LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	m, err := Parse(data, path)
	if err != nil {
		return m, err
	}
	abs, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return m, fmt.Errorf("resolve manifest directory %s: %w", path, err)
	}
	m.BaseDir = abs
	if err := BindLocalAssets(m); err != nil {
		return m, err
	}
	return m, nil
}

// BindLocalAssets resolves source.file attributes against the manifest
// directory and optional assets.root. It is safe to call more than once.
func BindLocalAssets(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	origin := m.Origin
	if origin == "" {
		origin = "manifest"
	}
	return asset.Bind(origin, m.BaseDir, m.Assets.Root, m.Resources)
}

func parseAssets(origin string, raw map[string]any) (Assets, error) {
	if len(raw) == 0 {
		return Assets{}, nil
	}
	for key := range raw {
		if key != "root" {
			return Assets{}, fmt.Errorf("%s: assets: unknown field %q (only root is supported)", origin, key)
		}
	}
	if _, ok := raw["root"]; !ok {
		return Assets{}, nil
	}
	if raw["root"] == nil {
		return Assets{}, nil
	}
	s, ok := raw["root"].(string)
	if !ok {
		return Assets{}, fmt.Errorf("%s: assets.root must be a string", origin)
	}
	return Assets{Root: strings.TrimSpace(s)}, nil
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
		if rawRef, ok := x["$ref"]; ok {
			if err := validateRefKeys(x); err != nil {
				return nil, err
			}
			s, ok := rawRef.(string)
			if !ok {
				return nil, fmt.Errorf("resource reference $ref must be a string")
			}
			addr, err := resource.ParseAddress(s)
			if err != nil {
				return nil, fmt.Errorf("resource reference $ref: %w", err)
			}
			ref := resource.Ref{Address: addr}
			if rawOut, exists := x["output"]; exists {
				out, ok := rawOut.(string)
				if !ok {
					return nil, fmt.Errorf("resource reference output must be a string")
				}
				out = strings.TrimSpace(out)
				if out == "" {
					return nil, fmt.Errorf("resource reference output must be a non-empty string")
				}
				ref.Output = out
			}
			return ref, nil
		}

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
			out[ks] = val
		}
		return normalizeValue(out)
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

func validateRefKeys(x map[string]any) error {
	for k := range x {
		switch k {
		case "$ref", "output":
		default:
			return fmt.Errorf("resource reference may only contain $ref and optional output")
		}
	}
	return nil
}
