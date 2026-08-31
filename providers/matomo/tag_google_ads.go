package matomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func validateGoogleAdsConversionTag(res resource.Resource) error {
	if err := requiredConversionField(res, AttrConversionID); err != nil {
		return err
	}
	if err := requiredConversionField(res, AttrConversionLabel); err != nil {
		return err
	}
	for _, key := range optionalGoogleAdsConversionAttrs {
		if err := optionalConversionField(res, key); err != nil {
			return err
		}
	}
	return nil
}

func requiredConversionField(res resource.Resource, key string) error {
	v, ok := res.Attributes[key]
	if !ok {
		return fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	return validateConversionField(res.Address, key, v, true)
}

func optionalConversionField(res resource.Resource, key string) error {
	v, ok := res.Attributes[key]
	if !ok {
		return nil
	}
	return validateConversionField(res.Address, key, v, false)
}

func validateConversionField(addr resource.Address, key string, v any, required bool) error {
	if resolved, ok := resource.AsResolved(v); ok {
		return validateVariableRef(addr, key, resolved.Address)
	}
	if ref, ok := resource.AsRef(v); ok {
		if ref.HasOutput() {
			return nil
		}
		return validateVariableRef(addr, key, ref.Address)
	}
	s, err := coerceString(v)
	if err != nil {
		return fmt.Errorf("resource %s: attribute %q must be a string, an output reference, or a resource reference to a %s.%s resource", addr, key, Name, TypeVariable)
	}
	if required && s == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string or an output reference", addr, key)
	}
	if s == "" {
		return nil
	}
	return rejectEdgeWhitespace(addr, key, s)
}

func (p *Provider) comparableGoogleAdsConversionTag(attrs resource.Attributes, addr resource.Address) (resource.Attributes, error) {
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
	id, err := comparableConversionAttr(attrs[AttrConversionID])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrConversionID, err)
	}
	label, err := comparableConversionAttr(attrs[AttrConversionLabel])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrConversionLabel, err)
	}
	out := resource.Attributes{
		AttrType:            typ,
		AttrName:            name,
		AttrTrigger:         trigger,
		AttrConversionID:    normalizeConversionIDValue(id),
		AttrConversionLabel: label,
	}
	for _, key := range optionalGoogleAdsConversionAttrs {
		if _, ok := attrs[key]; !ok {
			continue
		}
		v, err := comparableConversionAttr(attrs[key])
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

func comparableConversionAttr(v any) (any, error) {
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

func (p *Provider) remoteGoogleAdsConversionTag(addr resource.Address, tag client.Tag, desired resource.Attributes) (resource.RemoteResource, error) {
	id, err := templateParameterString(tag.Parameters[paramGoogleAdsConversionID])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q has malformed %s: %w", addr, tag.IDTag, paramGoogleAdsConversionID, err)
	}
	label, err := templateParameterString(tag.Parameters[paramGoogleAdsConversionLabel])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q has malformed %s: %w", addr, tag.IDTag, paramGoogleAdsConversionLabel, err)
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(label) == "" {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote Google Ads conversion tag %q is missing %s or %s", addr, tag.IDTag, paramGoogleAdsConversionID, paramGoogleAdsConversionLabel)
	}

	attrs := resource.Attributes{
		AttrType: tagTypeGoogleAdsConversion,
		AttrName: tag.Name,
	}
	if trigger := p.liveTriggerAttr(tag.FireTriggerIDs, desired[AttrTrigger]); trigger != nil {
		attrs[AttrTrigger] = trigger
	}
	if v := p.liveConversionAttr(id, desired[AttrConversionID]); v != nil {
		attrs[AttrConversionID] = v
	}
	if v := p.liveConversionAttr(label, desired[AttrConversionLabel]); v != nil {
		attrs[AttrConversionLabel] = v
	}
	for attr, param := range googleAdsConversionParamByAttr {
		if attr == AttrConversionID || attr == AttrConversionLabel {
			continue
		}
		desiredValue, managed := desired[attr]
		if !managed {
			continue
		}
		raw, err := templateParameterString(tag.Parameters[param])
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q has malformed %s: %w", addr, tag.IDTag, param, err)
		}
		if v := p.liveConversionAttr(raw, desiredValue); v != nil && v != "" {
			attrs[attr] = v
		}
	}

	computed := tagComputed(tag)
	cloned, err := client.CloneJSONMap(tag.Parameters)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: remote tag %q template parameters cannot be represented: %w", addr, tag.IDTag, err)
	}
	computed[computedTagParameters] = cloned

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: tag.IDTag},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) googleAdsConversionTagInput(ctx context.Context, res resource.Resource, live *resource.RemoteResource) (client.TagInput, error) {
	triggerID, err := p.resolvedTriggerID(res.Attributes[AttrTrigger])
	if err != nil {
		return client.TagInput{}, err
	}
	id, err := p.conversionFieldValue(ctx, res, res.Attributes[AttrConversionID])
	if err != nil {
		return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrConversionID, err)
	}
	if strings.TrimSpace(id) == "" {
		return client.TagInput{}, fmt.Errorf("attribute %q: conversion id is empty", AttrConversionID)
	}
	label, err := p.conversionFieldValue(ctx, res, res.Attributes[AttrConversionLabel])
	if err != nil {
		return client.TagInput{}, fmt.Errorf("attribute %q: %w", AttrConversionLabel, err)
	}
	if strings.TrimSpace(label) == "" {
		return client.TagInput{}, fmt.Errorf("attribute %q: conversion label is empty", AttrConversionLabel)
	}
	params := map[string]any{
		paramGoogleAdsConversionID:    id,
		paramGoogleAdsConversionLabel: label,
	}
	for _, key := range optionalGoogleAdsConversionAttrs {
		v, ok := res.Attributes[key]
		if !ok {
			continue
		}
		value, err := p.conversionFieldValue(ctx, res, v)
		if err != nil {
			return client.TagInput{}, fmt.Errorf("attribute %q: %w", key, err)
		}
		params[googleAdsConversionParamByAttr[key]] = value
	}
	_ = live
	return client.TagInput{
		Type:           matomoTypeGoogleAdsConversion,
		Name:           tagName(res),
		FireTriggerIDs: []string{triggerID},
		Parameters:     params,
	}, nil
}

func (p *Provider) conversionFieldValue(ctx context.Context, res resource.Resource, v any) (string, error) {
	return p.eventFieldValue(ctx, res, v)
}

func (p *Provider) liveConversionAttr(raw string, desired any) any {
	want := logicalRef(desired)
	if want.HasOutput() {
		if raw == "" {
			return want
		}
		return raw
	}
	return p.liveEventAttr(raw, desired)
}

func googleAdsTagPreserved(live resource.RemoteResource) client.TagPreservedFields {
	block := splitComputedIDs(computedString(live.Computed, "block_trigger_ids"))
	params := map[string]any{}
	if raw, ok := live.Computed[computedTagParameters]; ok {
		if cloned, err := client.CloneJSONMap(asStringMap(raw)); err == nil {
			params = cloned
		}
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

func asStringMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func templateParameterString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	s, err := coerceString(v)
	if err == nil {
		return s, nil
	}
	if wrapped, ok := client.NormalizeMatomoConfig(v).(string); ok {
		return wrapped, nil
	}
	return "", fmt.Errorf("must be a string or a Tag Manager variable template")
}

func normalizeConversionIDValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	return stripGoogleAdsConversionPrefix(s)
}

func stripGoogleAdsConversionPrefix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 3 && strings.EqualFold(s[:3], "AW-") {
		return s[3:]
	}
	return s
}

func (p *Provider) reconstructGoogleAdsConversionImport(ctx context.Context, res resource.Resource, live resource.RemoteResource) (resource.RemoteResource, error) {
	attrs := live.Attributes.Clone()
	if rawParams, ok := live.Computed[computedTagParameters]; ok {
		params := asStringMap(rawParams)
		for _, key := range optionalGoogleAdsConversionAttrs {
			if _, exists := attrs[key]; exists {
				continue
			}
			param := googleAdsConversionParamByAttr[key]
			raw, err := templateParameterString(params[param])
			if err != nil {
				return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: remote tag %q has malformed %s: %w", res.Address, live.Identity.ID, param, err)
			}
			if raw != "" {
				attrs[key] = raw
			}
		}
	}
	reconstructed := map[string]bool{}
	if id, ok := attrs[AttrConversionID].(string); ok {
		label, labelOK := attrs[AttrConversionLabel].(string)
		_, idTemplate := parseMatomoVariableTemplate(id)
		_, labelTemplate := parseMatomoVariableTemplate(label)
		if labelOK && id != "" && label != "" && !idTemplate && !labelTemplate {
			ref, matched, err := p.matchGoogleAdsConversionOutputs(ctx, stripGoogleAdsConversionPrefix(id), label)
			if err != nil {
				return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", res.Address, err)
			}
			if matched {
				attrs[AttrConversionID] = resource.Ref{Address: ref.Address, Output: AttrConversionID}
				attrs[AttrConversionLabel] = resource.Ref{Address: ref.Address, Output: AttrConversionLabel}
				reconstructed[AttrConversionID] = true
				reconstructed[AttrConversionLabel] = true
			}
		}
	}
	for _, key := range append([]string{AttrConversionID, AttrConversionLabel}, optionalGoogleAdsConversionAttrs...) {
		if reconstructed[key] {
			continue
		}
		raw, ok := attrs[key].(string)
		if !ok || raw == "" {
			continue
		}
		name, isTemplate := parseMatomoVariableTemplate(raw)
		if isTemplate {
			ref, found, err := p.importVariableRefByName(ctx, res, name)
			if err != nil {
				return resource.RemoteResource{}, err
			}
			if found {
				attrs[key] = ref
			}
			continue
		}
		if key != AttrConversionID && key != AttrConversionLabel {
			continue
		}
		ref, ok, err := p.matchGoogleAdsConversionOutput(ctx, key, raw)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", res.Address, err)
		}
		if ok {
			attrs[key] = ref
		}
	}
	live.Attributes = attrs
	return live, nil
}

func (p *Provider) matchGoogleAdsConversionOutputs(ctx context.Context, conversionID, conversionLabel string) (resource.Ref, bool, error) {
	ref, result, err := p.matchOutput(ctx, provider.OutputMatchQuery{
		Provider:     googleAdsOutputProvider,
		ResourceType: googleAdsOutputConversionAction,
		Output:       AttrConversionID,
		Equals: map[string]string{
			AttrConversionID:    conversionID,
			AttrConversionLabel: conversionLabel,
		},
	})
	if err != nil {
		return resource.Ref{}, false, err
	}
	if result != provider.OutputMatchUnique {
		return resource.Ref{}, false, nil
	}
	return ref, true, nil
}

func (p *Provider) matchGoogleAdsConversionOutput(ctx context.Context, attr, value string) (resource.Ref, bool, error) {
	output := attr
	candidates := []string{value}
	if attr == AttrConversionID {
		if stripped := stripGoogleAdsConversionPrefix(value); stripped != value {
			candidates = append(candidates, stripped)
		}
	}
	var matched resource.Ref
	found := false
	for _, candidate := range candidates {
		ref, result, err := p.matchOutput(ctx, provider.OutputMatchQuery{
			Provider:     googleAdsOutputProvider,
			ResourceType: googleAdsOutputConversionAction,
			Output:       output,
			Value:        candidate,
		})
		if err != nil {
			return resource.Ref{}, false, err
		}
		switch result {
		case provider.OutputMatchNone:
			continue
		case provider.OutputMatchAmbiguous:
			return resource.Ref{}, false, nil
		case provider.OutputMatchUnique:
			if found && matched.Address != ref.Address {
				return resource.Ref{}, false, nil
			}
			matched = ref
			found = true
		}
	}
	if !found {
		return resource.Ref{}, false, nil
	}
	return matched, true, nil
}

func (p *Provider) matchOutput(ctx context.Context, query provider.OutputMatchQuery) (resource.Ref, provider.OutputMatch, error) {
	if p == nil {
		return resource.Ref{}, provider.OutputMatchNone, nil
	}
	p.mu.Lock()
	matcher := p.outputs
	p.mu.Unlock()
	if matcher == nil {
		return resource.Ref{}, provider.OutputMatchNone, nil
	}
	return matcher.Match(ctx, query)
}
