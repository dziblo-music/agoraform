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
	// TypeTag is the Matomo Tag Manager tag type used in addresses such
	// as matomo.tag.trial_started.
	TypeTag = "tag"

	// AttrTrigger is a $ref to the matomo.trigger that fires the tag.
	AttrTrigger = "trigger"
	// AttrEventCategory is the Matomo event category for a matomoAnalytics tag.
	AttrEventCategory = "eventCategory"
	// AttrEventAction is the Matomo event action for a matomoAnalytics tag.
	AttrEventAction = "eventAction"
	// AttrEventName is an optional Matomo event name for a matomoAnalytics tag.
	AttrEventName = "eventName"
	// AttrEventValue is an optional Matomo event value for a matomoAnalytics tag.
	AttrEventValue = "eventValue"

	tagTypeMatomoAnalytics  = "matomoAnalytics"
	matomoTypeMatomo        = "Matomo"
	matomoTypeMatomoConfig  = "MatomoConfiguration"
	paramTrackingType       = "trackingType"
	trackingTypeEvent       = "event"
	paramMatomoConfig       = "matomoConfig"
	defaultMatomoConfigName = "Matomo Configuration"
	defaultFireLimit        = "unlimited"
	defaultFireDelay        = "0"
	defaultPriority         = "999"

	// MaxEventFieldLen is Matomo's maximum length for event category,
	// action, name, and value parameters.
	MaxEventFieldLen = 500
	// MaxTagNameLen is Matomo's maximum length for a Tag Manager tag
	// display name.
	MaxTagNameLen = 255
)

var (
	supportedTagAttrs = map[string]struct{}{
		AttrType:          {},
		AttrTrigger:       {},
		AttrEventCategory: {},
		AttrEventAction:   {},
		AttrEventName:     {},
		AttrEventValue:    {},
		AttrName:          {},
		AttrContainer:     {},
	}

	computedTagAttrs = map[string]struct{}{
		"idtag":              {},
		"idTag":              {},
		"idcontainertag":     {},
		"idcontainerversion": {},
		"idcontainer":        {},
		"idsite":             {},
		"status":             {},
		"typeMetadata":       {},
		"parameters":         {},
		"fireTriggerIds":     {},
		"fire_trigger_ids":   {},
		"blockTriggerIds":    {},
		"block_trigger_ids":  {},
		"fireLimit":          {},
		"fire_limit":         {},
		"fireDelay":          {},
		"fire_delay":         {},
		"priority":           {},
		"startDate":          {},
		"start_date":         {},
		"endDate":            {},
		"end_date":           {},
		"description":        {},
		"created_date":       {},
		"updated_date":       {},
		"matomoConfig":       {},
		"trackingType":       {},
	}

	supportedTagTypes = map[string]string{
		tagTypeMatomoAnalytics: matomoTypeMatomo,
	}

	optionalTagEventAttrs = []string{AttrEventName, AttrEventValue}
)

func (p *Provider) validateTag(res resource.Resource) error {
	if err := p.requireTagManagerConfig(res); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}

	for key := range attrs {
		if _, ok := supportedTagAttrs[key]; ok {
			continue
		}
		if _, computed := computedTagAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; matomo.tag supports %s, %s, %s, %s, optional %s, optional %s, optional %s, and optional %s", res.Address, key, AttrType, AttrTrigger, AttrEventCategory, AttrEventAction, AttrEventName, AttrEventValue, AttrName, AttrContainer)
	}

	if _, _, err := optionalContainerRef(res); err != nil {
		return err
	}

	typ, err := requiredString(res, AttrType)
	if err != nil {
		return err
	}
	if _, ok := supportedTagTypes[typ]; !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrType, joinSorted(keys(supportedTagTypes)))
	}

	if _, err := requiredTriggerRef(res); err != nil {
		return err
	}
	if err := requiredEventField(res, AttrEventCategory); err != nil {
		return err
	}
	if err := requiredEventField(res, AttrEventAction); err != nil {
		return err
	}
	for _, key := range optionalTagEventAttrs {
		if err := optionalEventField(res, key); err != nil {
			return err
		}
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

	effectiveName := tagName(res)
	if utf8.RuneCountInString(effectiveName) > MaxTagNameLen {
		if nameSet {
			return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrName, MaxTagNameLen)
		}
		return fmt.Errorf("resource %s: tag display name must be at most %d characters; set %s or shorten %s", res.Address, MaxTagNameLen, AttrName, AttrEventAction)
	}

	return nil
}

func (p *Provider) readTag(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTag(res); err != nil {
		return resource.RemoteResource{}, err
	}

	name := tagName(res)
	tags, err := p.listDraftTags(ctx, res)
	if err != nil {
		if mapped := mapUnavailableContainer(res.Address, err); mapped != err {
			return resource.RemoteResource{}, mapped
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, err)
	}

	matches := findTagsByName(tags, name)
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteTag(res.Address, matches[0], res.Attributes)
		if err == nil {
			live = attachContainerRef(live, res.Attributes)
			p.rememberBinding(res.Address, live.Identity.ID, matches[0].Name)
		}
		return live, err
	default:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: multiple remote tags named %q (ids %s); names must be unique", res.Address, name, joinTagIDs(matches))
	}
}

func (p *Provider) createTag(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTag(res); err != nil {
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

	in, err := p.tagInput(ctx, res, nil)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}
	id, err := tm.AddContainerTag(ctx, version, in)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	live, err := p.readTagByID(ctx, res, id)
	if err == nil {
		return live, nil
	}
	p.rememberBinding(res.Address, id, tagName(res))
	fallback, ferr := p.remoteTag(res.Address, client.Tag{
		IDTag:              id,
		IDContainerVersion: version,
		Type:               matomoTagType(stringAttr(res.Attributes, AttrType)),
		Name:               tagName(res),
		FireTriggerIDs:     in.FireTriggerIDs,
		Parameters:         in.Parameters,
	}, res.Attributes)
	if ferr != nil {
		return resource.RemoteResource{}, ferr
	}
	return attachContainerRef(fallback, res.Attributes), nil
}

func (p *Provider) updateTag(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateTag(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	current, err := p.readTagByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote tag %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableTagType(desired, current); err != nil {
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

	in, err := p.tagInput(ctx, desired, &current)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}
	preserved := tagPreserved(current)
	if err := tm.UpdateContainerTag(ctx, version, actual.Identity.ID, in, preserved); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readTagByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing tag %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readTagByID(ctx context.Context, res resource.Resource, id string) (resource.RemoteResource, error) {
	tags, err := p.listDraftTags(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	for _, tag := range tags {
		if strings.EqualFold(tag.Status, "deleted") {
			continue
		}
		if tag.IDTag == id {
			live, err := p.remoteTag(res.Address, tag, res.Attributes)
			if err == nil {
				live = attachContainerRef(live, res.Attributes)
				p.rememberBinding(res.Address, live.Identity.ID, tag.Name)
			}
			return live, err
		}
	}
	return resource.RemoteResource{}, provider.ErrNotFound
}

func (p *Provider) listDraftTags(ctx context.Context, res resource.Resource) ([]client.Tag, error) {
	tm, err := p.tagManagerFor(res)
	if err != nil {
		return nil, err
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		return nil, err
	}
	return tm.GetContainerTags(ctx, version)
}

func (p *Provider) normalizeTagComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := p.comparableTag(desired.Attributes, desired.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	got, err := p.comparableTag(live.Attributes, desired.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func (p *Provider) comparableTag(attrs resource.Attributes, addr resource.Address) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	typ, err := coerceString(attrs[AttrType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrType, err)
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	if name == "" {
		name = defaultTagName(addr, attrs)
	}
	trigger, err := comparableTriggerAttr(attrs[AttrTrigger])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrTrigger, err)
	}
	category, err := comparableEventAttr(attrs[AttrEventCategory])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrEventCategory, err)
	}
	action, err := comparableEventAttr(attrs[AttrEventAction])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrEventAction, err)
	}
	out := resource.Attributes{
		AttrType:          typ,
		AttrName:          name,
		AttrTrigger:       trigger,
		AttrEventCategory: category,
		AttrEventAction:   action,
	}
	for _, key := range optionalTagEventAttrs {
		if _, ok := attrs[key]; !ok {
			continue
		}
		v, err := comparableEventAttr(attrs[key])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", key, err)
		}
		if v == nil || v == "" {
			continue
		}
		out[key] = v
	}
	return withComparableContainer(out, attrs)
}

func (p *Provider) remoteTag(addr resource.Address, tag client.Tag, desired resource.Attributes) (resource.RemoteResource, error) {
	agoraType, ok := agoraTagType(tag.Type)
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q has unsupported type %q; v0.2 supports %s", addr, tag.IDTag, tag.Type, joinSorted(keys(supportedTagTypes)))
	}
	if tracking := parameterString(tag.Parameters, paramTrackingType); tracking != "" && tracking != trackingTypeEvent {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q has unsupported trackingType %q; v0.2 supports %s", addr, tag.IDTag, tracking, trackingTypeEvent)
	}

	attrs := resource.Attributes{
		AttrType: agoraType,
		AttrName: tag.Name,
	}
	if trigger := p.liveTriggerAttr(tag.FireTriggerIDs, desired[AttrTrigger]); trigger != nil {
		attrs[AttrTrigger] = trigger
	}
	if category := p.liveEventAttr(parameterString(tag.Parameters, AttrEventCategory), desired[AttrEventCategory]); category != nil {
		attrs[AttrEventCategory] = category
	}
	if action := p.liveEventAttr(parameterString(tag.Parameters, AttrEventAction), desired[AttrEventAction]); action != nil {
		attrs[AttrEventAction] = action
	}
	if name := parameterString(tag.Parameters, AttrEventName); name != "" || desired[AttrEventName] != nil {
		if v := p.liveEventAttr(name, desired[AttrEventName]); v != nil {
			attrs[AttrEventName] = v
		}
	}
	if value := parameterString(tag.Parameters, AttrEventValue); value != "" || desired[AttrEventValue] != nil {
		if v := p.liveEventAttr(value, desired[AttrEventValue]); v != nil {
			attrs[AttrEventValue] = v
		}
	}

	computed := resource.Attributes{}
	setComputed(computed, "idtag", tag.IDTag)
	setComputed(computed, "idcontainerversion", tag.IDContainerVersion)
	setComputed(computed, "idsite", tag.IDSite)
	setComputed(computed, "status", tag.Status)
	setComputed(computed, "description", tag.Description)
	setComputed(computed, "fire_limit", tag.FireLimit)
	setComputed(computed, "fire_delay", tag.FireDelay)
	setComputed(computed, "priority", tag.Priority)
	setComputed(computed, "start_date", tag.StartDate)
	setComputed(computed, "end_date", tag.EndDate)
	if len(tag.FireTriggerIDs) > 0 {
		computed["fire_trigger_ids"] = strings.Join(tag.FireTriggerIDs, ",")
	}
	if len(tag.BlockTriggerIDs) > 0 {
		computed["block_trigger_ids"] = strings.Join(tag.BlockTriggerIDs, ",")
	}
	if cfg := parameterString(tag.Parameters, paramMatomoConfig); cfg != "" {
		setComputed(computed, paramMatomoConfig, cfg)
	} else if wrapped, ok := client.NormalizeMatomoConfig(tag.Parameters[paramMatomoConfig]).(string); ok {
		setComputed(computed, paramMatomoConfig, wrapped)
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: tag.IDTag},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) tagInput(ctx context.Context, res resource.Resource, live *resource.RemoteResource) (client.TagInput, error) {
	triggerID, err := p.resolvedTriggerID(res.Attributes[AttrTrigger])
	if err != nil {
		return client.TagInput{}, err
	}
	category, err := p.eventFieldValue(ctx, res, res.Attributes[AttrEventCategory])
	if err != nil {
		return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrEventCategory, err)
	}
	action, err := p.eventFieldValue(ctx, res, res.Attributes[AttrEventAction])
	if err != nil {
		return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrEventAction, err)
	}
	params := map[string]any{
		paramTrackingType: trackingTypeEvent,
		AttrEventCategory: category,
		AttrEventAction:   action,
	}
	if v, ok := res.Attributes[AttrEventName]; ok {
		name, err := p.eventFieldValue(ctx, res, v)
		if err != nil {
			return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrEventName, err)
		}
		params[AttrEventName] = name
	}
	if v, ok := res.Attributes[AttrEventValue]; ok {
		value, err := p.eventFieldValue(ctx, res, v)
		if err != nil {
			return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrEventValue, err)
		}
		params[AttrEventValue] = value
	}
	cfg, err := p.matomoConfigValue(ctx, res, live)
	if err != nil {
		return client.TagInput{}, err
	}
	if cfg != "" {
		params[paramMatomoConfig] = cfg
	}
	return client.TagInput{
		Type:           matomoTagType(stringAttr(res.Attributes, AttrType)),
		Name:           tagName(res),
		FireTriggerIDs: []string{triggerID},
		Parameters:     params,
	}, nil
}

func (p *Provider) resolvedTriggerID(v any) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		if resolved.Identity.ID != "" {
			return resolved.Identity.ID, nil
		}
		v = resource.Ref{Address: resolved.Address}
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return "", fmt.Errorf("attribute %q must be a resource reference", AttrTrigger)
	}
	if id := p.lookupID(ref.Address); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("trigger %s has no provider-native identity", ref.Address)
}

func (p *Provider) eventFieldValue(ctx context.Context, res resource.Resource, v any) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		name := p.lookupName(resolved.Address)
		if name == "" {
			var err error
			name, err = p.variableNameByID(ctx, res, resolved.Identity.ID)
			if err != nil {
				return "", err
			}
		}
		if name == "" {
			return "", fmt.Errorf("variable %s has no Tag Manager name", resolved.Address)
		}
		return "{{" + name + "}}", nil
	}
	if ref, ok := resource.AsRef(v); ok {
		name := p.lookupName(ref.Address)
		if name == "" {
			return "", fmt.Errorf("variable %s has no provider-native identity", ref.Address)
		}
		return "{{" + name + "}}", nil
	}
	s, err := coerceString(v)
	if err != nil {
		return "", err
	}
	return s, nil
}

func (p *Provider) variableNameByID(ctx context.Context, res resource.Resource, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("variable identity is empty")
	}
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		return "", err
	}
	for _, v := range vars {
		if strings.EqualFold(v.Status, "deleted") {
			continue
		}
		if v.IDVariable == id {
			return v.Name, nil
		}
	}
	return "", fmt.Errorf("variable identity %q was not found", id)
}

func (p *Provider) matomoConfigValue(ctx context.Context, res resource.Resource, live *resource.RemoteResource) (string, error) {
	if live != nil {
		if cfg := computedString(live.Computed, paramMatomoConfig); cfg != "" {
			if wrapped, ok := client.NormalizeMatomoConfig(cfg).(string); ok {
				return wrapped, nil
			}
		}
	}
	return p.defaultMatomoConfig(ctx, res)
}

func (p *Provider) defaultMatomoConfig(ctx context.Context, res resource.Resource) (string, error) {
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		return "", err
	}
	var matches []client.Variable
	for _, v := range vars {
		if strings.EqualFold(v.Status, "deleted") {
			continue
		}
		if v.Type == matomoTypeMatomoConfig {
			matches = append(matches, v)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("container has no Matomo Configuration variable; create one in Tag Manager before managing matomo.tag resources")
	case 1:
		return "{{" + matches[0].Name + "}}", nil
	default:
		for _, v := range matches {
			if v.Name == defaultMatomoConfigName {
				return "{{" + v.Name + "}}", nil
			}
		}
		return "", fmt.Errorf("container has multiple Matomo Configuration variables; keep a single %q variable", defaultMatomoConfigName)
	}
}

func (p *Provider) liveTriggerAttr(fireIDs []string, desired any) any {
	want := logicalRef(desired)
	if want.IsZero() {
		// Never copy provider-native trigger ids into comparable
		// attributes or import YAML. Plan always has a $ref.
		return nil
	}
	wantID := resolvedOrCachedID(p, desired, want.Address)
	if wantID != "" && equalSingletonIDs(fireIDs, wantID) {
		return want
	}
	if len(fireIDs) == 1 {
		return fireIDs[0]
	}
	if len(fireIDs) > 1 {
		return asAnyIDs(fireIDs)
	}
	return nil
}

func (p *Provider) liveEventAttr(raw string, desired any) any {
	want := logicalRef(desired)
	if !want.IsZero() {
		name := p.lookupName(want.Address)
		if name != "" && (raw == "{{"+name+"}}" || raw == name) {
			return want
		}
		if raw == "" {
			return want
		}
		return raw
	}
	if raw == "" {
		return nil
	}
	return raw
}

func tagPreserved(live resource.RemoteResource) client.TagPreservedFields {
	block := splitComputedIDs(computedString(live.Computed, "block_trigger_ids"))
	params := map[string]any{}
	if cfg := computedString(live.Computed, paramMatomoConfig); cfg != "" {
		params[paramMatomoConfig] = cfg
	}
	return client.TagPreservedFields{
		Description:     computedString(live.Computed, "description"),
		BlockTriggerIDs: block,
		FireLimit:       firstNonEmptyString(computedString(live.Computed, "fire_limit"), defaultFireLimit),
		FireDelay:       firstNonEmptyString(computedString(live.Computed, "fire_delay"), defaultFireDelay),
		Priority:        firstNonEmptyString(computedString(live.Computed, "priority"), defaultPriority),
		StartDate:       computedString(live.Computed, "start_date"),
		EndDate:         computedString(live.Computed, "end_date"),
		Parameters:      params,
	}
}

func tagName(res resource.Resource) string {
	return defaultTagName(res.Address, res.Attributes)
}

func defaultTagName(addr resource.Address, attrs resource.Attributes) string {
	if name := stringAttr(attrs, AttrName); name != "" {
		return name
	}
	if action, err := coerceString(attrs[AttrEventAction]); err == nil && action != "" {
		return action
	}
	return addr.Name
}

func matomoTagType(agoraType string) string {
	if matomoType, ok := supportedTagTypes[agoraType]; ok {
		return matomoType
	}
	return agoraType
}

func agoraTagType(matomoType string) (string, bool) {
	for agora, remote := range supportedTagTypes {
		if remote == matomoType {
			return agora, true
		}
	}
	return "", false
}

func findTagsByName(tags []client.Tag, name string) []client.Tag {
	var matches []client.Tag
	for _, tag := range tags {
		if strings.EqualFold(tag.Status, "deleted") {
			continue
		}
		if tag.Name == name {
			matches = append(matches, tag)
		}
	}
	return matches
}

func joinTagIDs(tags []client.Tag) string {
	ids := make([]string, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.IDTag)
	}
	return joinSorted(ids)
}

func ensureImmutableTagType(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrType)
	got := stringAttr(live.Attributes, AttrType)
	if want == got {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for Matomo tag %q; remote type is %q and configuration requests %q", desired.Address, AttrType, live.Identity.ID, got, want)
}

func requiredTriggerRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrTrigger]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrTrigger)
	}
	ref, err := triggerRefValue(v)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrTrigger, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeTrigger {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrTrigger, Name, TypeTrigger)
	}
	return ref, nil
}

func triggerRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeTrigger)
	}
	return ref, nil
}

func requiredEventField(res resource.Resource, key string) error {
	v, ok := res.Attributes[key]
	if !ok {
		return fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	return validateEventField(res.Address, key, v, true)
}

func optionalEventField(res resource.Resource, key string) error {
	v, ok := res.Attributes[key]
	if !ok {
		return nil
	}
	return validateEventField(res.Address, key, v, false)
}

func validateEventField(addr resource.Address, key string, v any, required bool) error {
	if resolved, ok := resource.AsResolved(v); ok {
		return validateVariableRef(addr, key, resolved.Address)
	}
	if ref, ok := resource.AsRef(v); ok {
		return validateVariableRef(addr, key, ref.Address)
	}
	s, err := coerceString(v)
	if err != nil {
		return fmt.Errorf("resource %s: attribute %q must be a string or a resource reference to a %s.%s resource", addr, key, Name, TypeVariable)
	}
	if required && s == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string or a resource reference", addr, key)
	}
	if s == "" {
		return nil
	}
	if err := rejectEdgeWhitespace(addr, key, s); err != nil {
		return err
	}
	if utf8.RuneCountInString(s) > MaxEventFieldLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", addr, key, MaxEventFieldLen)
	}
	return nil
}

func validateVariableRef(addr resource.Address, key string, target resource.Address) error {
	if target.Provider != Name || target.Type != TypeVariable {
		return fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", addr, key, Name, TypeVariable)
	}
	return nil
}

func comparableTriggerAttr(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	ref, err := triggerRefValue(v)
	if err != nil {
		if s, strErr := coerceString(v); strErr == nil {
			return s, nil
		}
		return nil, err
	}
	return ref, nil
}

func comparableEventAttr(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	if ref, ok := resource.AsRef(v); ok {
		return ref, nil
	}
	return coerceString(v)
}

func logicalRef(v any) resource.Ref {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}
	}
	ref, _ := resource.AsRef(v)
	return ref
}

func resolvedOrCachedID(p *Provider, v any, addr resource.Address) string {
	if resolved, ok := resource.AsResolved(v); ok && resolved.Identity.ID != "" {
		return resolved.Identity.ID
	}
	return p.lookupID(addr)
}

func equalSingletonIDs(ids []string, want string) bool {
	return len(ids) == 1 && ids[0] == want
}

func asAnyIDs(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func parameterString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	s, err := coerceString(params[key])
	if err != nil {
		if wrapped, ok := client.NormalizeMatomoConfig(params[key]).(string); ok {
			return wrapped
		}
		return ""
	}
	return s
}

func splitComputedIDs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
