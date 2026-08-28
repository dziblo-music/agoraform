package matomo

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// TypeVariable is the Matomo Tag Manager variable type used in
	// addresses such as matomo.variable.user_id.
	TypeVariable = "variable"

	// AttrType is the Tag Manager template selector for a variable.
	AttrType = "type"
	// AttrKey is the Data Layer key read by a dataLayer variable.
	AttrKey = "key"

	variableTypeDataLayer = "dataLayer"
	matomoTypeDataLayer   = "DataLayer"
	paramDataLayerName    = "dataLayerName"

	// MaxDataLayerKeyLen is Matomo's maximum length for a Data Layer
	// variable key (dataLayerName).
	MaxDataLayerKeyLen = 300
	// MaxVariableNameLen is Matomo's maximum length for a Tag Manager
	// variable display name.
	MaxVariableNameLen = 255
)

var (
	supportedVariableAttrs = map[string]struct{}{
		AttrType:      {},
		AttrKey:       {},
		AttrName:      {},
		AttrContainer: {},
	}

	computedVariableAttrs = map[string]struct{}{
		"idvariable":          {},
		"idVariable":          {},
		"idcontainervariable": {},
		"idcontainerversion":  {},
		"idcontainer":         {},
		"idsite":              {},
		"status":              {},
		"typeMetadata":        {},
		"parameters":          {},
		"lookup_table":        {},
		"lookupTable":         {},
		"default_value":       {},
		"defaultValue":        {},
		"description":         {},
		"created_date":        {},
		"updated_date":        {},
	}

	supportedVariableTypes = map[string]string{
		variableTypeDataLayer: matomoTypeDataLayer,
	}
)

func (p *Provider) validateVariable(res resource.Resource) error {
	if err := p.requireTagManagerConfig(res); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}

	for key := range attrs {
		if _, ok := supportedVariableAttrs[key]; ok {
			continue
		}
		if _, computed := computedVariableAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; matomo.variable supports %s, %s, optional %s, and optional %s", res.Address, key, AttrType, AttrKey, AttrName, AttrContainer)
	}

	if _, _, err := optionalContainerRef(res); err != nil {
		return err
	}

	typ, err := requiredString(res, AttrType)
	if err != nil {
		return err
	}
	if _, ok := supportedVariableTypes[typ]; !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrType, joinSorted(keys(supportedVariableTypes)))
	}

	key, err := requiredString(res, AttrKey)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string when %s is %q", res.Address, AttrKey, AttrType, typ)
	}
	if err := rejectEdgeWhitespace(res.Address, AttrKey, key); err != nil {
		return err
	}
	if utf8.RuneCountInString(key) > MaxDataLayerKeyLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrKey, MaxDataLayerKeyLen)
	}

	name, nameSet, err := optionalString(res, AttrName)
	if err != nil {
		return err
	}
	if nameSet {
		if name == "" {
			return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
		}
		if err := rejectEdgeWhitespace(res.Address, AttrName, name); err != nil {
			return err
		}
	}

	effectiveName := key
	if nameSet {
		effectiveName = name
	}
	if utf8.RuneCountInString(effectiveName) > MaxVariableNameLen {
		if nameSet {
			return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrName, MaxVariableNameLen)
		}
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters when %s is omitted because it is used as the Matomo variable name", res.Address, AttrKey, MaxVariableNameLen, AttrName)
	}

	return nil
}

func rejectEdgeWhitespace(addr resource.Address, attr, value string) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("resource %s: attribute %q must not have leading or trailing whitespace", addr, attr)
	}
	return nil
}

func (p *Provider) readVariable(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateVariable(res); err != nil {
		return resource.RemoteResource{}, err
	}

	name := variableName(res.Attributes)
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		if mapped := mapUnavailableContainer(res.Address, err); mapped != err {
			return resource.RemoteResource{}, mapped
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, err)
	}

	matches := findVariablesByName(vars, name)
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := remoteVariable(res.Address, matches[0])
		if err == nil {
			live = attachContainerRef(live, res.Attributes)
			p.rememberBinding(res.Address, live.Identity.ID, matches[0].Name)
		}
		return live, err
	default:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: multiple remote variables named %q (ids %s); names must be unique", res.Address, name, joinVariableIDs(matches))
	}
}

func (p *Provider) createVariable(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateVariable(res); err != nil {
		return resource.RemoteResource{}, err
	}

	tm, err := p.tagManagerFor(res)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	id, err := tm.AddContainerVariable(ctx, version, variableInput(res.Attributes))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	live, err := p.readVariableByID(ctx, res, id)
	if err == nil {
		return live, nil
	}
	p.rememberBinding(res.Address, id, variableName(res.Attributes))
	fallback, ferr := remoteVariable(res.Address, client.Variable{
		IDVariable:         id,
		IDContainerVersion: version,
		Type:               matomoVariableType(stringAttr(res.Attributes, AttrType)),
		Name:               variableName(res.Attributes),
		Parameters:         map[string]string{paramDataLayerName: stringAttr(res.Attributes, AttrKey)},
	})
	if ferr != nil {
		return resource.RemoteResource{}, ferr
	}
	return attachContainerRef(fallback, res.Attributes), nil
}

func (p *Provider) updateVariable(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateVariable(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	current, err := p.readVariableByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote variable %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableVariableType(desired, current); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := ensureImmutableContainerRef(desired, current); err != nil {
		return resource.RemoteResource{}, err
	}

	tm, err := p.tagManagerFor(desired)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	preserved := client.VariablePreservedFields{
		Description:  computedString(current.Computed, "description"),
		DefaultValue: computedString(current.Computed, "default_value"),
		LookupTable:  lookupTableValue(current.Computed["lookup_table"]),
	}
	if err := tm.UpdateContainerVariable(ctx, version, actual.Identity.ID, variableInput(desired.Attributes), preserved); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readVariableByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing variable %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readVariableByID(ctx context.Context, res resource.Resource, id string) (resource.RemoteResource, error) {
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	for _, v := range vars {
		if strings.EqualFold(v.Status, "deleted") {
			continue
		}
		if v.IDVariable == id {
			live, err := remoteVariable(res.Address, v)
			if err == nil {
				live = attachContainerRef(live, res.Attributes)
				p.rememberBinding(res.Address, live.Identity.ID, v.Name)
			}
			return live, err
		}
	}
	return resource.RemoteResource{}, provider.ErrNotFound
}

func (p *Provider) listDraftVariables(ctx context.Context, res resource.Resource) ([]client.Variable, error) {
	tm, err := p.tagManagerFor(res)
	if err != nil {
		return nil, err
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		return nil, err
	}
	return tm.GetContainerVariables(ctx, version)
}

func (p *Provider) normalizeVariableComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableVariable(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	got, err := comparableVariable(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func comparableVariable(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	typ, err := coerceString(attrs[AttrType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrType, err)
	}
	key, err := coerceString(attrs[AttrKey])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrKey, err)
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	if name == "" {
		name = key
	}
	out := resource.Attributes{
		AttrType: typ,
		AttrKey:  key,
		AttrName: name,
	}
	return withComparableContainer(out, attrs)
}

func remoteVariable(addr resource.Address, v client.Variable) (resource.RemoteResource, error) {
	agoraType, ok := agoraVariableType(v.Type)
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote variable %q has unsupported type %q; v0.2 supports %s", addr, v.IDVariable, v.Type, joinSorted(keys(supportedVariableTypes)))
	}

	attrs := resource.Attributes{
		AttrType: agoraType,
		AttrName: v.Name,
	}
	if key := v.Parameters[paramDataLayerName]; key != "" {
		attrs[AttrKey] = key
	}

	computed := resource.Attributes{}
	setComputed(computed, "idvariable", v.IDVariable)
	setComputed(computed, "idcontainerversion", v.IDContainerVersion)
	setComputed(computed, "idsite", v.IDSite)
	setComputed(computed, "status", v.Status)
	setComputed(computed, "description", v.Description)
	setComputed(computed, "default_value", v.DefaultValue)
	if len(v.LookupTable) > 0 {
		computed["lookup_table"] = string(v.LookupTable)
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: v.IDVariable},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func variableInput(attrs resource.Attributes) client.VariableInput {
	return client.VariableInput{
		Type: matomoVariableType(stringAttr(attrs, AttrType)),
		Name: variableName(attrs),
		Parameters: map[string]string{
			paramDataLayerName: stringAttr(attrs, AttrKey),
		},
	}
}

func variableName(attrs resource.Attributes) string {
	if name := stringAttr(attrs, AttrName); name != "" {
		return name
	}
	return stringAttr(attrs, AttrKey)
}

func matomoVariableType(agoraType string) string {
	if matomoType, ok := supportedVariableTypes[agoraType]; ok {
		return matomoType
	}
	return agoraType
}

func agoraVariableType(matomoType string) (string, bool) {
	for agora, remote := range supportedVariableTypes {
		if remote == matomoType {
			return agora, true
		}
	}
	return "", false
}

func findVariablesByName(vars []client.Variable, name string) []client.Variable {
	var matches []client.Variable
	for _, v := range vars {
		if strings.EqualFold(v.Status, "deleted") {
			continue
		}
		if v.Name == name {
			matches = append(matches, v)
		}
	}
	return matches
}

func joinVariableIDs(vars []client.Variable) string {
	ids := make([]string, 0, len(vars))
	for _, v := range vars {
		ids = append(ids, v.IDVariable)
	}
	return joinSorted(ids)
}

func lookupTableValue(v any) []byte {
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return nil
	}
	return []byte(s)
}

func ensureImmutableVariableType(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrType)
	got := stringAttr(live.Attributes, AttrType)
	if want == got {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for Matomo variable %q; remote type is %q and configuration requests %q", desired.Address, AttrType, live.Identity.ID, got, want)
}
