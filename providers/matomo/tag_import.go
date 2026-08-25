package matomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func (p *Provider) reconstructTagImportRefs(ctx context.Context, addr resource.Address, live resource.RemoteResource) (resource.RemoteResource, error) {
	attrs := live.Attributes.Clone()

	trigger, err := p.importTriggerRef(addr, splitComputedIDs(computedString(live.Computed, "fire_trigger_ids")))
	if err != nil {
		return resource.RemoteResource{}, err
	}
	attrs[AttrTrigger] = trigger

	for _, key := range []string{AttrEventCategory, AttrEventAction, AttrEventName, AttrEventValue} {
		raw, ok := attrs[key].(string)
		if !ok {
			continue
		}
		name, isTemplate := parseMatomoVariableTemplate(raw)
		if !isTemplate {
			continue
		}
		ref, found, err := p.importVariableRefByName(ctx, addr, name)
		if err != nil {
			return resource.RemoteResource{}, err
		}
		if found {
			attrs[key] = ref
		}
	}

	live.Attributes = attrs
	return live, nil
}

func (p *Provider) importTriggerRef(addr resource.Address, fireIDs []string) (resource.Ref, error) {
	switch len(fireIDs) {
	case 0:
		return resource.Ref{}, fmt.Errorf("matomo: import %s: remote tag has no fire trigger; v0.2 matomo.tag requires exactly one fire trigger", addr)
	case 1:
		// continue
	default:
		return resource.Ref{}, fmt.Errorf("matomo: import %s: remote tag fires on %d triggers (%s); v0.2 supports exactly one fire trigger represented as a logical $ref", addr, len(fireIDs), strings.Join(fireIDs, ", "))
	}

	triggerID := fireIDs[0]
	managed, ok, err := p.lookupManagedAddress(TypeTrigger, triggerID)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("matomo: import %s: look up fire trigger %q: %w", addr, triggerID, err)
	}
	if !ok {
		return resource.Ref{}, fmt.Errorf("matomo: import %s: fire trigger id %q is not bound in local state; import the matomo.trigger resource first (or apply it), then re-import this tag", addr, triggerID)
	}
	return resource.Ref{Address: managed}, nil
}

func (p *Provider) importVariableRefByName(ctx context.Context, tagAddr resource.Address, name string) (resource.Ref, bool, error) {
	vars, err := p.listDraftVariables(ctx)
	if err != nil {
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: list variables to reconstruct references: %w", tagAddr, err)
	}

	matches := findVariablesByName(vars, name)
	switch len(matches) {
	case 0:
		return resource.Ref{}, false, nil
	case 1:
		// continue
	default:
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: variable template %q matches multiple active remote variables (ids %s); variable names must be unique before Agoraform can reconstruct a logical $ref", tagAddr, "{{"+name+"}}", joinVariableIDs(matches))
	}

	match := matches[0]
	managed, ok, err := p.lookupManagedAddress(TypeVariable, match.IDVariable)
	if err != nil {
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: look up variable %q: %w", tagAddr, match.IDVariable, err)
	}
	if !ok {
		return resource.Ref{}, false, nil
	}
	return resource.Ref{Address: managed}, true, nil
}

func (p *Provider) lookupManagedAddress(resourceType, remoteID string) (resource.Address, bool, error) {
	if p == nil {
		return resource.Address{}, false, nil
	}
	p.mu.Lock()
	catalog := p.identities
	p.mu.Unlock()
	if catalog == nil {
		return resource.Address{}, false, nil
	}
	return catalog.AddressByRemoteID(Name, resourceType, remoteID)
}

func parseMatomoVariableTemplate(s string) (string, bool) {
	if s != strings.TrimSpace(s) {
		return "", false
	}
	if len(s) < 5 || !strings.HasPrefix(s, "{{") || !strings.HasSuffix(s, "}}") {
		return "", false
	}
	name := s[2 : len(s)-2]
	if name == "" || strings.ContainsAny(name, "{}") {
		return "", false
	}
	return name, true
}
