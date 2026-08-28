package matomo

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
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
	// AttrMatomoURL is the Matomo instance URL for a matomoConfiguration variable.
	AttrMatomoURL = "matomoUrl"
	// AttrSiteID is the Matomo site identifier for a matomoConfiguration variable.
	AttrSiteID = "siteId"
	// AttrEnableLinkTracking is the optional link-tracking flag for a
	// matomoConfiguration variable.
	AttrEnableLinkTracking = "enableLinkTracking"

	variableTypeDataLayer           = "dataLayer"
	variableTypeMatomoConfiguration = "matomoConfiguration"
	matomoTypeDataLayer             = "DataLayer"
	matomoTypeMatomoConfiguration   = "MatomoConfiguration"
	paramDataLayerName              = "dataLayerName"
	paramMatomoURL                  = "matomoUrl"
	paramIDSite                     = "idSite"
	paramEnableLinkTracking         = "enableLinkTracking"
	computedVariableParameters      = "parameters"

	// MaxDataLayerKeyLen is Matomo's maximum length for a Data Layer
	// variable key (dataLayerName).
	MaxDataLayerKeyLen = 300
	// MaxVariableNameLen is Matomo's maximum length for a Tag Manager
	// variable display name.
	MaxVariableNameLen = 255
)

var (
	supportedVariableAttrs = map[string]struct{}{
		AttrType:               {},
		AttrKey:                {},
		AttrName:               {},
		AttrContainer:          {},
		AttrMatomoURL:          {},
		AttrSiteID:             {},
		AttrEnableLinkTracking: {},
	}

	dataLayerVariableAttrs = map[string]struct{}{
		AttrType:      {},
		AttrKey:       {},
		AttrName:      {},
		AttrContainer: {},
	}

	matomoConfigurationVariableAttrs = map[string]struct{}{
		AttrType:               {},
		AttrName:               {},
		AttrContainer:          {},
		AttrMatomoURL:          {},
		AttrSiteID:             {},
		AttrEnableLinkTracking: {},
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
		variableTypeDataLayer:           matomoTypeDataLayer,
		variableTypeMatomoConfiguration: matomoTypeMatomoConfiguration,
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
		return fmt.Errorf("resource %s: unsupported attribute %q; matomo.variable supports type %s (%s, optional %s, optional %s) and type %s (%s, %s, %s, optional %s, optional %s)", res.Address, key, variableTypeDataLayer, AttrKey, AttrName, AttrContainer, variableTypeMatomoConfiguration, AttrName, AttrMatomoURL, AttrSiteID, AttrEnableLinkTracking, AttrContainer)
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

	switch typ {
	case variableTypeDataLayer:
		return validateDataLayerVariable(res)
	case variableTypeMatomoConfiguration:
		return validateMatomoConfigurationVariable(res)
	default:
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrType, joinSorted(keys(supportedVariableTypes)))
	}
}

func validateDataLayerVariable(res resource.Resource) error {
	if err := rejectTypeSpecificAttrs(res, dataLayerVariableAttrs, variableTypeDataLayer); err != nil {
		return err
	}

	key, err := requiredString(res, AttrKey)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string when %s is %q", res.Address, AttrKey, AttrType, variableTypeDataLayer)
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

func validateMatomoConfigurationVariable(res resource.Resource) error {
	if err := rejectTypeSpecificAttrs(res, matomoConfigurationVariableAttrs, variableTypeMatomoConfiguration); err != nil {
		return err
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string when %s is %q", res.Address, AttrName, AttrType, variableTypeMatomoConfiguration)
	}
	if err := rejectEdgeWhitespace(res.Address, AttrName, name); err != nil {
		return err
	}
	if utf8.RuneCountInString(name) > MaxVariableNameLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrName, MaxVariableNameLen)
	}

	if _, err := requiredHTTPURL(res, AttrMatomoURL); err != nil {
		return err
	}
	if _, err := requiredPositiveSiteID(res, AttrSiteID); err != nil {
		return err
	}
	if _, _, err := optionalBoolAttr(res, AttrEnableLinkTracking); err != nil {
		return err
	}
	return nil
}

func rejectTypeSpecificAttrs(res resource.Resource, allowed map[string]struct{}, typ string) error {
	for key := range res.Attributes {
		if _, ok := allowed[key]; ok {
			continue
		}
		return fmt.Errorf("resource %s: attribute %q is not used when %s is %q", res.Address, key, AttrType, typ)
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
		Parameters:         variableInput(res.Attributes).Parameters,
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

	preservedParams, err := parametersFromComputed(desired.Address, current.Computed)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	preserved := client.VariablePreservedFields{
		Description:  computedString(current.Computed, "description"),
		DefaultValue: computedString(current.Computed, "default_value"),
		LookupTable:  lookupTableValue(current.Computed["lookup_table"]),
		Parameters:   preservedParams,
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
	if _, ok := desired.Attributes[AttrEnableLinkTracking]; !ok {
		delete(got, AttrEnableLinkTracking)
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
	switch typ {
	case variableTypeMatomoConfiguration:
		return comparableMatomoConfiguration(attrs)
	default:
		return comparableDataLayerVariable(attrs)
	}
}

func comparableDataLayerVariable(attrs resource.Attributes) (resource.Attributes, error) {
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
		AttrType: variableTypeDataLayer,
		AttrKey:  key,
		AttrName: name,
	}
	return withComparableContainer(out, attrs)
}

func comparableMatomoConfiguration(attrs resource.Attributes) (resource.Attributes, error) {
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	matomoURL, err := coerceString(attrs[AttrMatomoURL])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrMatomoURL, err)
	}
	siteID := ""
	if _, ok := attrs[AttrSiteID]; ok {
		siteID, err = coercePositiveSiteID(attrs[AttrSiteID])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrSiteID, err)
		}
	}
	out := resource.Attributes{
		AttrType:      variableTypeMatomoConfiguration,
		AttrName:      name,
		AttrMatomoURL: matomoURL,
		AttrSiteID:    siteID,
	}
	if _, ok := attrs[AttrEnableLinkTracking]; ok {
		enabled, err := coerceBool(attrs[AttrEnableLinkTracking])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrEnableLinkTracking, err)
		}
		out[AttrEnableLinkTracking] = enabled
	}
	return withComparableContainer(out, attrs)
}

func remoteVariable(addr resource.Address, v client.Variable) (resource.RemoteResource, error) {
	agoraType, ok := agoraVariableType(v.Type)
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote variable %q has unsupported type %q; supported types are %s", addr, v.IDVariable, v.Type, joinSorted(keys(supportedVariableTypes)))
	}

	attrs := resource.Attributes{
		AttrType: agoraType,
		AttrName: v.Name,
	}
	switch agoraType {
	case variableTypeDataLayer:
		if key := parameterString(v.Parameters, paramDataLayerName); key != "" {
			attrs[AttrKey] = key
		}
	case variableTypeMatomoConfiguration:
		if err := setRemoteMatomoConfigurationAttrs(addr, v, attrs); err != nil {
			return resource.RemoteResource{}, err
		}
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
	cloned, err := client.CloneJSONMap(v.Parameters)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote variable %q has template parameters that cannot be represented without loss: %w", addr, v.IDVariable, err)
	}
	computed[computedVariableParameters] = cloned

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: v.IDVariable},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func setRemoteMatomoConfigurationAttrs(addr resource.Address, v client.Variable, attrs resource.Attributes) error {
	if raw, ok := v.Parameters[paramMatomoURL]; ok {
		s, err := coerceString(raw)
		if err != nil {
			return fmt.Errorf("matomo: read %s: remote variable %q has an unreadable %s: %w", addr, v.IDVariable, AttrMatomoURL, err)
		}
		if s != "" {
			attrs[AttrMatomoURL] = s
		}
	}
	if raw, ok := v.Parameters[paramIDSite]; ok {
		id, err := coercePositiveSiteID(raw)
		if err == nil && id != "" {
			if n, convErr := strconv.Atoi(id); convErr == nil {
				attrs[AttrSiteID] = n
			} else {
				attrs[AttrSiteID] = id
			}
		} else if s, sErr := coerceString(raw); sErr == nil && strings.TrimSpace(s) != "" {
			return fmt.Errorf("matomo: read %s: remote variable %q has an unreadable %s", addr, v.IDVariable, AttrSiteID)
		}
	}
	if raw, ok := v.Parameters[paramEnableLinkTracking]; ok && raw != nil && raw != "" {
		enabled, err := coerceBool(raw)
		if err != nil {
			return fmt.Errorf("matomo: read %s: remote variable %q has an unreadable %s: %w", addr, v.IDVariable, AttrEnableLinkTracking, err)
		}
		attrs[AttrEnableLinkTracking] = enabled
	}
	return nil
}

func variableInput(attrs resource.Attributes) client.VariableInput {
	typ := stringAttr(attrs, AttrType)
	in := client.VariableInput{
		Type:       matomoVariableType(typ),
		Name:       variableName(attrs),
		Parameters: map[string]any{},
	}
	switch typ {
	case variableTypeMatomoConfiguration:
		in.Parameters[paramMatomoURL] = stringAttr(attrs, AttrMatomoURL)
		if id, err := coercePositiveSiteID(attrs[AttrSiteID]); err == nil {
			in.Parameters[paramIDSite] = id
		}
		if _, ok := attrs[AttrEnableLinkTracking]; ok {
			if enabled, err := coerceBool(attrs[AttrEnableLinkTracking]); err == nil {
				in.Parameters[paramEnableLinkTracking] = enabled
			}
		}
	default:
		in.Parameters[paramDataLayerName] = stringAttr(attrs, AttrKey)
	}
	return in
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

func parametersFromComputed(addr resource.Address, computed resource.Attributes) (map[string]any, error) {
	if computed == nil {
		return nil, nil
	}
	v, ok := computed[computedVariableParameters]
	if !ok || v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("matomo: update %s: remote variable template parameters cannot be preserved without loss", addr)
	}
	return m, nil
}

func requiredHTTPURL(res resource.Resource, key string) (string, error) {
	s, err := requiredString(res, key)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty http or https URL", res.Address, key)
	}
	if err := rejectEdgeWhitespace(res.Address, key, s); err != nil {
		return "", err
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("resource %s: attribute %q must be an http or https URL", res.Address, key)
	}
	if u.User != nil || u.Query().Get("token_auth") != "" || u.Query().Get("token") != "" {
		return "", fmt.Errorf("resource %s: attribute %q must not contain credentials", res.Address, key)
	}
	return s, nil
}

func requiredPositiveSiteID(res resource.Resource, key string) (string, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	id, err := coercePositiveSiteID(v)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	if id == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a positive site identifier", res.Address, key)
	}
	return id, nil
}

func optionalBoolAttr(res resource.Resource, key string) (bool, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return false, false, nil
	}
	b, err := coerceBool(v)
	if err != nil {
		return false, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return b, true, nil
}

func coercePositiveSiteID(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("must be a positive site identifier")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return "", fmt.Errorf("must be a positive site identifier")
	}
	return strconv.FormatInt(n, 10), nil
}

func coerceBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		default:
			return false, fmt.Errorf("must be a boolean")
		}
	case int, int8, int16, int32, int64:
		n, _ := coerceString(x)
		switch n {
		case "1":
			return true, nil
		case "0":
			return false, nil
		default:
			return false, fmt.Errorf("must be a boolean")
		}
	case uint, uint8, uint16, uint32, uint64:
		n, _ := coerceString(x)
		switch n {
		case "1":
			return true, nil
		case "0":
			return false, nil
		default:
			return false, fmt.Errorf("must be a boolean")
		}
	case float32, float64:
		n, err := coerceString(x)
		if err != nil {
			return false, fmt.Errorf("must be a boolean")
		}
		switch n {
		case "1":
			return true, nil
		case "0":
			return false, nil
		default:
			return false, fmt.Errorf("must be a boolean")
		}
	default:
		return false, fmt.Errorf("must be a boolean")
	}
}

func ensureImmutableVariableType(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrType)
	got := stringAttr(live.Attributes, AttrType)
	if want == got {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for Matomo variable %q; remote type is %q and configuration requests %q", desired.Address, AttrType, live.Identity.ID, got, want)
}
