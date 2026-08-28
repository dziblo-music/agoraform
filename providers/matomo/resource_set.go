package matomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// ValidateResourceSet implements provider.ResourceSetValidator.
//
// v0.5.0 supports at most one managed Matomo container per manifest. Managed
// Tag Manager children must reference that container. Mixing a managed
// container with MATOMO_CONTAINER_ID is rejected before mutation.
func (p *Provider) ValidateResourceSet(_ context.Context, resources []resource.Resource) error {
	var containers []resource.Resource
	var children []resource.Resource
	for _, res := range resources {
		if res.Address.Provider != Name {
			continue
		}
		switch res.Address.Type {
		case TypeContainer:
			containers = append(containers, res)
		case TypeVariable, TypeTrigger, TypeTag:
			children = append(children, res)
		}
	}

	if len(containers) > 1 {
		addrs := make([]string, 0, len(containers))
		for _, c := range containers {
			addrs = append(addrs, c.Address.String())
		}
		return fmt.Errorf("matomo: at most one matomo.container resource is supported; found %s", strings.Join(addrs, ", "))
	}

	if len(containers) == 1 {
		p.setManagedContainer(containers[0].Address)
		if p != nil && strings.TrimSpace(p.cfg.ContainerID) != "" {
			return fmt.Errorf("matomo: cannot mix a managed matomo.container resource with %s; omit %s when declaring %s, or omit the container resource and use %s to select an existing container", EnvContainerID, EnvContainerID, containers[0].Address, EnvContainerID)
		}
		for _, child := range children {
			ref, set, err := optionalContainerRef(child)
			if err != nil {
				return err
			}
			if !set {
				return fmt.Errorf("resource %s: attribute %q is required when a matomo.container resource is declared", child.Address, AttrContainer)
			}
			if ref.Address != containers[0].Address {
				return fmt.Errorf("resource %s: attribute %q must reference %s", child.Address, AttrContainer, containers[0].Address)
			}
		}
		return validateMatomoConfigurationRefs(resources)
	}

	p.clearManagedContainer()
	for _, child := range children {
		_, set, err := optionalContainerRef(child)
		if err != nil {
			return err
		}
		if set {
			return fmt.Errorf("resource %s: attribute %q requires a declared matomo.container resource; omit the reference and set %s to manage resources in an existing container", child.Address, AttrContainer, EnvContainerID)
		}
	}

	if err := validateMatomoConfigurationRefs(resources); err != nil {
		return err
	}

	enabled, _ := p.publicationSettings()
	if enabled && (p == nil || strings.TrimSpace(p.cfg.ContainerID) == "") {
		return fmt.Errorf("matomo: %s is required when provider publication is enabled without a managed matomo.container resource", EnvContainerID)
	}
	return nil
}

func validateMatomoConfigurationRefs(resources []resource.Resource) error {
	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}
	for _, res := range resources {
		if res.Address.Provider != Name || res.Address.Type != TypeTag {
			continue
		}
		ref, set, err := optionalMatomoConfigurationRef(res)
		if err != nil {
			return err
		}
		if !set {
			continue
		}
		target, ok := byAddr[ref.Address.String()]
		if !ok {
			continue
		}
		if stringAttr(target.Attributes, AttrType) != variableTypeMatomoConfiguration {
			return fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource with type %q", res.Address, AttrMatomoConfiguration, Name, TypeVariable, variableTypeMatomoConfiguration)
		}
	}
	return nil
}
