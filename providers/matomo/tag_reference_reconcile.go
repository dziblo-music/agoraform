package matomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func (p *Provider) reconcileTagVariableRefs(ctx context.Context, res resource.Resource, live resource.RemoteResource) (resource.RemoteResource, error) {
	if !tagUsesVariableRefs(res.Attributes) {
		return live, nil
	}

	tags, err := p.listDraftTags(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: verify variable references: %w", res.Address, err)
	}
	var raw *client.Tag
	for i := range tags {
		if strings.EqualFold(tags[i].Status, "deleted") {
			continue
		}
		if tags[i].IDTag == live.Identity.ID {
			raw = &tags[i]
			break
		}
	}
	if raw == nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: tag %q disappeared while verifying variable references", res.Address, live.Identity.ID)
	}

	attrs := live.Attributes.Clone()
	for _, key := range []string{AttrEventCategory, AttrEventAction, AttrEventName, AttrEventValue} {
		desired := res.Attributes[key]
		want := logicalRef(desired)
		if want.IsZero() {
			continue
		}

		name := p.lookupName(want.Address)
		if name == "" {
			if resolved, ok := resource.AsResolved(desired); ok && resolved.Identity.ID != "" {
				name, err = p.variableNameByID(ctx, resolved.Identity.ID)
				if err != nil {
					return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: resolve %s reference: %w", res.Address, key, err)
				}
			}
		}

		remoteValue := parameterString(raw.Parameters, key)
		if name != "" && remoteValue == "{{"+name+"}}" {
			attrs[key] = want
			continue
		}
		if remoteValue == "" {
			delete(attrs, key)
			continue
		}
		attrs[key] = remoteValue
	}
	live.Attributes = attrs
	return live, nil
}

func tagUsesVariableRefs(attrs resource.Attributes) bool {
	for _, key := range []string{AttrEventCategory, AttrEventAction, AttrEventName, AttrEventValue} {
		if !logicalRef(attrs[key]).IsZero() {
			return true
		}
	}
	return false
}
