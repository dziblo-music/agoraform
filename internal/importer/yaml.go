package importer

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
	"gopkg.in/yaml.v3"
)

func manifestYAML(addr resource.Address, attrs resource.Attributes) (string, error) {
	attrNode, err := valueNode(attrs)
	if err != nil {
		return "", err
	}

	item := mapping(
		scalarString("address"), scalarString(addr.String()),
		scalarString("attributes"), attrNode,
	)
	resources := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Tag:     "!!seq",
		Content: []*yaml.Node{item},
	}
	root := mapping(
		scalarString("apiVersion"), scalarString(manifest.APIVersion),
		scalarString("resources"), resources,
	)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func valueNode(v any) (*yaml.Node, error) {
	switch x := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	case resource.Attributes:
		return mapNode(map[string]any(x))
	case map[string]any:
		return mapNode(x)
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for i, item := range x {
			child, err := valueNode(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			n.Content = append(n.Content, child)
		}
		return n, nil
	case string:
		return scalarString(x), nil
	case bool:
		val := "false"
		if x {
			val = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: val}, nil
	case int:
		return scalarInt(int64(x)), nil
	case int8:
		return scalarInt(int64(x)), nil
	case int16:
		return scalarInt(int64(x)), nil
	case int32:
		return scalarInt(int64(x)), nil
	case int64:
		return scalarInt(x), nil
	case uint:
		return scalarUint(uint64(x)), nil
	case uint8:
		return scalarUint(uint64(x)), nil
	case uint16:
		return scalarUint(uint64(x)), nil
	case uint32:
		return scalarUint(uint64(x)), nil
	case uint64:
		return scalarUint(x), nil
	case float32:
		return scalarFloat(float64(x)), nil
	case float64:
		return scalarFloat(x), nil
	default:
		return nil, fmt.Errorf("unsupported attribute type %T", v)
	}
}

func mapNode(m map[string]any) (*yaml.Node, error) {
	if m == nil {
		m = map[string]any{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "" {
			return nil, fmt.Errorf("attribute name is empty")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		child, err := valueNode(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		n.Content = append(n.Content, scalarString(k), child)
	}
	return n, nil
}

func mapping(content ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: content}
}

func scalarString(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func scalarInt(n int64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(n, 10)}
}

func scalarUint(n uint64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatUint(n, 10)}
}

func scalarFloat(n float64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(n, 'g', -1, 64)}
}
