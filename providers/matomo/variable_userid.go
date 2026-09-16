package matomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func optionalUserIDRef(res resource.Resource) (resource.Ref, bool, error) {
	v, ok := res.Attributes[AttrUserID]
	if !ok {
		return resource.Ref{}, false, nil
	}
	if resolved, ok := resource.AsResolved(v); ok {
		if err := validateVariableRef(res.Address, AttrUserID, resolved.Address); err != nil {
			return resource.Ref{}, true, err
		}
		return resource.Ref{Address: resolved.Address}, true, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, true, fmt.Errorf("resource %s: attribute %q must be a resource reference ($ref) to a %s.%s resource of type %q", res.Address, AttrUserID, Name, TypeVariable, variableTypeDataLayer)
	}
	if err := validateVariableRef(res.Address, AttrUserID, ref.Address); err != nil {
		return resource.Ref{}, true, err
	}
	return ref, true, nil
}

func (p *Provider) userIDParameter(ctx context.Context, res resource.Resource) (string, bool, error) {
	ref, set, err := optionalUserIDRef(res)
	if err != nil || !set {
		return "", set, err
	}
	name := p.lookupName(ref.Address)
	id := ""
	if resolved, ok := resource.AsResolved(res.Attributes[AttrUserID]); ok {
		id = resolved.Identity.ID
	}
	if id == "" {
		id = p.lookupID(ref.Address)
	}
	if name == "" && id != "" {
		var nameErr error
		name, nameErr = p.variableNameByID(ctx, res, id)
		if nameErr != nil {
			return "", true, nameErr
		}
	}
	if name == "" {
		return "", true, fmt.Errorf("variable %s has no Tag Manager name", ref.Address)
	}
	if id != "" {
		if err := p.ensureRemoteDataLayerVariable(ctx, res, ref.Address, id); err != nil {
			return "", true, err
		}
	}
	return "{{" + name + "}}", true, nil
}

func (p *Provider) ensureRemoteDataLayerVariable(ctx context.Context, res resource.Resource, addr resource.Address, id string) error {
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		return err
	}
	for _, v := range vars {
		if strings.EqualFold(v.Status, "deleted") {
			continue
		}
		if v.IDVariable != id {
			continue
		}
		if v.Type != matomoTypeDataLayer {
			return fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource with type %q; %s is type %q", res.Address, AttrUserID, Name, TypeVariable, variableTypeDataLayer, addr, v.Type)
		}
		return nil
	}
	return fmt.Errorf("variable identity %q was not found", id)
}

func (p *Provider) reconcileVariableUserID(res resource.Resource, live resource.RemoteResource) resource.RemoteResource {
	if stringAttr(live.Attributes, AttrType) != variableTypeMatomoConfiguration {
		return live
	}
	raw := remoteUserIDParameter(live)
	var desired any
	if res.Attributes != nil {
		desired = res.Attributes[AttrUserID]
	}
	attrs := live.Attributes.Clone()
	if v := p.liveUserIDAttr(raw, desired); v != nil {
		attrs[AttrUserID] = v
	} else {
		delete(attrs, AttrUserID)
	}
	live.Attributes = attrs
	return live
}

func (p *Provider) liveUserIDAttr(raw string, desired any) any {
	want := logicalRef(desired)
	if want.IsZero() {
		return nil
	}
	name := p.lookupName(want.Address)
	if name != "" && (raw == "{{"+name+"}}" || raw == name) {
		return want
	}
	if raw == "" {
		return nil
	}
	return raw
}

func (p *Provider) reconstructVariableImportRefs(ctx context.Context, res resource.Resource, live resource.RemoteResource) (resource.RemoteResource, error) {
	if stringAttr(live.Attributes, AttrType) != variableTypeMatomoConfiguration {
		return live, nil
	}
	raw := remoteUserIDParameter(live)
	if raw == "" {
		return live, nil
	}
	ref, found, err := p.importUserIDRef(ctx, res, raw)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !found {
		return live, nil
	}
	attrs := live.Attributes.Clone()
	attrs[AttrUserID] = ref
	live.Attributes = attrs
	return live, nil
}

func (p *Provider) importUserIDRef(ctx context.Context, res resource.Resource, raw string) (resource.Ref, bool, error) {
	addr := res.Address
	name, isTemplate := parseMatomoVariableTemplate(raw)
	if !isTemplate {
		return resource.Ref{}, false, nil
	}
	vars, err := p.listDraftVariables(ctx, res)
	if err != nil {
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: list variables to reconstruct %s: %w", addr, AttrUserID, err)
	}
	matches := findVariablesByName(vars, name)
	switch len(matches) {
	case 0:
		return resource.Ref{}, false, nil
	case 1:
		// continue
	default:
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: %s template %q matches multiple active remote variables (ids %s); variable names must be unique before Agoraform can reconstruct a logical $ref", addr, AttrUserID, "{{"+name+"}}", joinVariableIDs(matches))
	}
	match := matches[0]
	if match.Type != matomoTypeDataLayer {
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: %s template %q matches remote variable %q of type %q; %s must reference a managed %s variable", addr, AttrUserID, "{{"+name+"}}", match.IDVariable, match.Type, AttrUserID, variableTypeDataLayer)
	}
	managed, ok, err := p.lookupManagedAddress(TypeVariable, match.IDVariable)
	if err != nil {
		return resource.Ref{}, false, fmt.Errorf("matomo: import %s: look up %s variable %q: %w", addr, AttrUserID, match.IDVariable, err)
	}
	if !ok {
		return resource.Ref{}, false, nil
	}
	return resource.Ref{Address: managed}, true, nil
}

func remoteUserIDParameter(live resource.RemoteResource) string {
	params, _ := live.Computed[computedVariableParameters].(map[string]any)
	return strings.TrimSpace(parameterString(params, paramUserID))
}
