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
	// TypeTrigger is the Matomo Tag Manager trigger type used in
	// addresses such as matomo.trigger.trial_started.
	TypeTrigger = "trigger"

	// AttrEvent is the Data Layer event name matched by a customEvent trigger.
	AttrEvent = "event"

	triggerTypeCustomEvent = "customEvent"
	matomoTypeCustomEvent  = "CustomEvent"
	paramEventName         = "eventName"

	// MaxEventNameLen is Matomo's maximum length for a Custom Event
	// trigger event name (eventName).
	MaxEventNameLen = 300
	// MaxTriggerNameLen is Matomo's maximum length for a Tag Manager
	// trigger display name.
	MaxTriggerNameLen = 255
)

var (
	supportedTriggerAttrs = map[string]struct{}{
		AttrType:      {},
		AttrEvent:     {},
		AttrName:      {},
		AttrContainer: {},
	}

	computedTriggerAttrs = map[string]struct{}{
		"idtrigger":          {},
		"idTrigger":          {},
		"idcontainertrigger": {},
		"idcontainerversion": {},
		"idcontainer":        {},
		"idsite":             {},
		"status":             {},
		"typeMetadata":       {},
		"parameters":         {},
		"conditions":         {},
		"description":        {},
		"created_date":       {},
		"updated_date":       {},
	}

	supportedTriggerTypes = map[string]string{
		triggerTypeCustomEvent: matomoTypeCustomEvent,
	}
)

func (p *Provider) validateTrigger(res resource.Resource) error {
	if err := p.requireTagManagerConfig(res); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}

	for key := range attrs {
		if _, ok := supportedTriggerAttrs[key]; ok {
			continue
		}
		if _, computed := computedTriggerAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; matomo.trigger supports %s, %s, optional %s, and optional %s", res.Address, key, AttrType, AttrEvent, AttrName, AttrContainer)
	}

	if _, _, err := optionalContainerRef(res); err != nil {
		return err
	}

	typ, err := requiredString(res, AttrType)
	if err != nil {
		return err
	}
	if _, ok := supportedTriggerTypes[typ]; !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrType, joinSorted(keys(supportedTriggerTypes)))
	}

	event, err := requiredString(res, AttrEvent)
	if err != nil {
		return err
	}
	if event == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string when %s is %q", res.Address, AttrEvent, AttrType, typ)
	}
	if err := rejectEdgeWhitespace(res.Address, AttrEvent, event); err != nil {
		return err
	}
	if utf8.RuneCountInString(event) > MaxEventNameLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrEvent, MaxEventNameLen)
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

	effectiveName := event
	if nameSet {
		effectiveName = name
	}
	if utf8.RuneCountInString(effectiveName) > MaxTriggerNameLen {
		if nameSet {
			return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrName, MaxTriggerNameLen)
		}
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters when %s is omitted because it is used as the Matomo trigger name", res.Address, AttrEvent, MaxTriggerNameLen, AttrName)
	}

	return nil
}

func (p *Provider) readTrigger(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTrigger(res); err != nil {
		return resource.RemoteResource{}, err
	}

	name := triggerName(res.Attributes)
	triggers, err := p.listDraftTriggers(ctx, res)
	if err != nil {
		if mapped := mapUnavailableContainer(res.Address, err); mapped != err {
			return resource.RemoteResource{}, mapped
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, err)
	}

	matches := findTriggersByName(triggers, name)
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := remoteTrigger(res.Address, matches[0])
		if err == nil {
			live = attachContainerRef(live, res.Attributes)
			p.rememberBinding(res.Address, live.Identity.ID, matches[0].Name)
		}
		return live, err
	default:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: multiple remote triggers named %q (ids %s); names must be unique", res.Address, name, joinTriggerIDs(matches))
	}
}

func (p *Provider) createTrigger(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTrigger(res); err != nil {
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

	id, err := tm.AddContainerTrigger(ctx, version, triggerInput(res.Attributes))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	live, err := p.readTriggerByID(ctx, res, id)
	if err == nil {
		return live, nil
	}
	p.rememberBinding(res.Address, id, triggerName(res.Attributes))
	fallback, ferr := remoteTrigger(res.Address, client.Trigger{
		IDTrigger:          id,
		IDContainerVersion: version,
		Type:               matomoTriggerType(stringAttr(res.Attributes, AttrType)),
		Name:               triggerName(res.Attributes),
		Parameters:         map[string]string{paramEventName: stringAttr(res.Attributes, AttrEvent)},
	})
	if ferr != nil {
		return resource.RemoteResource{}, ferr
	}
	return attachContainerRef(fallback, res.Attributes), nil
}

func (p *Provider) updateTrigger(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateTrigger(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	current, err := p.readTriggerByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote trigger %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableTriggerType(desired, current); err != nil {
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

	preserved := client.TriggerPreservedFields{
		Description: computedString(current.Computed, "description"),
		Conditions:  conditionsValue(current.Computed["conditions"]),
	}
	if err := tm.UpdateContainerTrigger(ctx, version, actual.Identity.ID, triggerInput(desired.Attributes), preserved); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readTriggerByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing trigger %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readTriggerByID(ctx context.Context, res resource.Resource, id string) (resource.RemoteResource, error) {
	triggers, err := p.listDraftTriggers(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	for _, tr := range triggers {
		if strings.EqualFold(tr.Status, "deleted") {
			continue
		}
		if tr.IDTrigger == id {
			live, err := remoteTrigger(res.Address, tr)
			if err == nil {
				live = attachContainerRef(live, res.Attributes)
				p.rememberBinding(res.Address, live.Identity.ID, tr.Name)
			}
			return live, err
		}
	}
	return resource.RemoteResource{}, provider.ErrNotFound
}

func (p *Provider) listDraftTriggers(ctx context.Context, res resource.Resource) ([]client.Trigger, error) {
	tm, err := p.tagManagerFor(res)
	if err != nil {
		return nil, err
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		return nil, err
	}
	return tm.GetContainerTriggers(ctx, version)
}

func (p *Provider) normalizeTriggerComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableTrigger(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	got, err := comparableTrigger(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func comparableTrigger(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	typ, err := coerceString(attrs[AttrType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrType, err)
	}
	event, err := coerceString(attrs[AttrEvent])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrEvent, err)
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	if name == "" {
		name = event
	}
	out := resource.Attributes{
		AttrType:  typ,
		AttrEvent: event,
		AttrName:  name,
	}
	return withComparableContainer(out, attrs)
}

func remoteTrigger(addr resource.Address, tr client.Trigger) (resource.RemoteResource, error) {
	agoraType, ok := agoraTriggerType(tr.Type)
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote trigger %q has unsupported type %q; v0.2 supports %s", addr, tr.IDTrigger, tr.Type, joinSorted(keys(supportedTriggerTypes)))
	}

	attrs := resource.Attributes{
		AttrType: agoraType,
		AttrName: tr.Name,
	}
	if event := tr.Parameters[paramEventName]; event != "" {
		attrs[AttrEvent] = event
	}

	computed := resource.Attributes{}
	setComputed(computed, "idtrigger", tr.IDTrigger)
	setComputed(computed, "idcontainerversion", tr.IDContainerVersion)
	setComputed(computed, "idsite", tr.IDSite)
	setComputed(computed, "status", tr.Status)
	setComputed(computed, "description", tr.Description)
	if len(tr.Conditions) > 0 {
		computed["conditions"] = string(tr.Conditions)
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: tr.IDTrigger},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func triggerInput(attrs resource.Attributes) client.TriggerInput {
	return client.TriggerInput{
		Type: matomoTriggerType(stringAttr(attrs, AttrType)),
		Name: triggerName(attrs),
		Parameters: map[string]string{
			paramEventName: stringAttr(attrs, AttrEvent),
		},
	}
}

func triggerName(attrs resource.Attributes) string {
	if name := stringAttr(attrs, AttrName); name != "" {
		return name
	}
	return stringAttr(attrs, AttrEvent)
}

func matomoTriggerType(agoraType string) string {
	if matomoType, ok := supportedTriggerTypes[agoraType]; ok {
		return matomoType
	}
	return agoraType
}

func agoraTriggerType(matomoType string) (string, bool) {
	for agora, remote := range supportedTriggerTypes {
		if remote == matomoType {
			return agora, true
		}
	}
	return "", false
}

func findTriggersByName(triggers []client.Trigger, name string) []client.Trigger {
	var matches []client.Trigger
	for _, tr := range triggers {
		if strings.EqualFold(tr.Status, "deleted") {
			continue
		}
		if tr.Name == name {
			matches = append(matches, tr)
		}
	}
	return matches
}

func joinTriggerIDs(triggers []client.Trigger) string {
	ids := make([]string, 0, len(triggers))
	for _, tr := range triggers {
		ids = append(ids, tr.IDTrigger)
	}
	return joinSorted(ids)
}

func conditionsValue(v any) []byte {
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return nil
	}
	return []byte(s)
}

func ensureImmutableTriggerType(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrType)
	got := stringAttr(live.Attributes, AttrType)
	if want == got {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for Matomo trigger %q; remote type is %q and configuration requests %q", desired.Address, AttrType, live.Identity.ID, got, want)
}
