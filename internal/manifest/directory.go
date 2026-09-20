package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	manifestSuffixYAML = ".agoraform.yaml"
	manifestSuffixYML  = ".agoraform.yml"
)

// loadDir discovers *.agoraform.yaml and *.agoraform.yml files in dir,
// parses each independently, and merges them into one logical configuration.
func loadDir(dir string) (*Manifest, error) {
	files, err := discoverManifestFiles(dir)
	if err != nil {
		return nil, err
	}

	parsed := make([]*Manifest, 0, len(files))
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", path, err)
		}
		m, err := parseDocument(data, path)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, m)
	}

	merged, err := mergeManifests(dir, parsed)
	if err != nil {
		return nil, err
	}

	if _, err := graph.Build(merged.Resources); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration directory %s: %w", dir, err)
	}
	merged.BaseDir = abs
	if err := BindLocalAssets(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func discoverManifestFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read configuration %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !isManifestFilename(entry.Name()) {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no Agoraform configuration files matching *%s or *%s", dir, manifestSuffixYAML, manifestSuffixYML)
	}
	return files, nil
}

func isManifestFilename(name string) bool {
	return strings.HasSuffix(name, manifestSuffixYAML) || strings.HasSuffix(name, manifestSuffixYML)
}

func looksLikeManifestFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func mergeManifests(dir string, files []*Manifest) (*Manifest, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no Agoraform configuration files matching *%s or *%s", dir, manifestSuffixYAML, manifestSuffixYML)
	}

	merged := &Manifest{
		Origin:            dir,
		APIVersion:        files[0].APIVersion,
		Providers:         make(map[string]resource.Attributes),
		ApplicationEvents: make(map[string]ApplicationEvent),
		Files:             make([]string, 0, len(files)),
	}

	providerOrigin := make(map[string]string)
	eventOrigin := make(map[string]string)
	resourceOrigin := make(map[string]string)
	var assetsOrigin string

	for _, src := range files {
		if src.APIVersion != merged.APIVersion {
			return nil, fmt.Errorf("%s: apiVersion %q is incompatible with %s (%q)", src.Origin, src.APIVersion, files[0].Origin, merged.APIVersion)
		}
		merged.Files = append(merged.Files, src.Origin)

		if err := mergeProviders(merged, src, providerOrigin); err != nil {
			return nil, err
		}
		if err := mergeAssets(merged, src, &assetsOrigin); err != nil {
			return nil, err
		}
		if err := mergeApplicationEvents(merged, src, eventOrigin); err != nil {
			return nil, err
		}
		if err := mergeResources(merged, src, resourceOrigin); err != nil {
			return nil, err
		}
	}

	sort.Slice(merged.Resources, func(i, j int) bool {
		return merged.Resources[i].Address.String() < merged.Resources[j].Address.String()
	})
	if len(merged.ApplicationEvents) == 0 {
		merged.ApplicationEvents = nil
	}
	return merged, nil
}

func mergeProviders(merged, src *Manifest, origin map[string]string) error {
	names := make([]string, 0, len(src.Providers))
	for name := range src.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		attrs := src.Providers[name]
		if existing, ok := merged.Providers[name]; ok {
			if !reflect.DeepEqual(existing, attrs) {
				return fmt.Errorf("%s: providers.%s: conflicting declarations in %s and %s", merged.Origin, name, displayName(origin[name]), displayName(src.Origin))
			}
			continue
		}
		merged.Providers[name] = attrs.Clone()
		origin[name] = src.Origin
	}
	return nil
}

func mergeAssets(merged, src *Manifest, origin *string) error {
	if src.Assets.Root == "" {
		return nil
	}
	if merged.Assets.Root != "" && merged.Assets.Root != src.Assets.Root {
		return fmt.Errorf("%s: assets.root: conflicting declarations in %s and %s", merged.Origin, displayName(*origin), displayName(src.Origin))
	}
	if merged.Assets.Root == "" {
		merged.Assets.Root = src.Assets.Root
		*origin = src.Origin
	}
	return nil
}

func mergeApplicationEvents(merged, src *Manifest, origin map[string]string) error {
	names := make([]string, 0, len(src.ApplicationEvents))
	for name := range src.ApplicationEvents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		evt := src.ApplicationEvents[name]
		if first, dup := origin[name]; dup {
			return fmt.Errorf("%s: applicationEvents.%s: duplicate event (defined in %s and %s)", merged.Origin, name, displayName(first), displayName(src.Origin))
		}
		merged.ApplicationEvents[name] = evt
		origin[name] = src.Origin
	}
	return nil
}

func mergeResources(merged, src *Manifest, origin map[string]string) error {
	for _, res := range src.Resources {
		key := res.Address.String()
		if first, dup := origin[key]; dup {
			return fmt.Errorf("%s: duplicate resource address %q (defined in %s and %s)", merged.Origin, key, displayName(first), displayName(src.Origin))
		}
		merged.Resources = append(merged.Resources, resource.Resource{
			Address:    res.Address,
			Attributes: res.Attributes.Clone(),
		})
		origin[key] = src.Origin
	}
	return nil
}

func displayName(path string) string {
	name := filepath.Base(path)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}
