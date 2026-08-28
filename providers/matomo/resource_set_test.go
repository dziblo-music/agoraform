package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestValidateResourceSetAllowsSingleManagedContainer(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	container := containerResource(t, "main", defaultContainerAttrs())
	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{container, variable}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestValidateResourceSetRejectsMultipleContainers(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		containerResource(t, "main", defaultContainerAttrs()),
		containerResource(t, "other", defaultContainerAttrs()),
	})
	if err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("ValidateResourceSet = %v, want at most one container", err)
	}
}

func TestValidateResourceSetRejectsMixingManagedAndEnvContainer(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
	})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		containerResource(t, "main", defaultContainerAttrs()),
	})
	if err == nil || !strings.Contains(err.Error(), matomo.EnvContainerID) {
		t.Fatalf("ValidateResourceSet = %v, want mix rejection", err)
	}
}

func TestValidateResourceSetRequiresChildContainerRefInManagedMode(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		containerResource(t, "main", defaultContainerAttrs()),
		variableResource(t, "user_id", resource.Attributes{
			matomo.AttrType: "dataLayer",
			matomo.AttrKey:  "userId",
		}),
	})
	if err == nil || !strings.Contains(err.Error(), `attribute "container" is required`) {
		t.Fatalf("ValidateResourceSet = %v, want required container $ref", err)
	}
}

func TestValidateResourceSetRejectsChildRefToOtherContainer(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		containerResource(t, "main", defaultContainerAttrs()),
		variableResource(t, "user_id", resource.Attributes{
			matomo.AttrType:      "dataLayer",
			matomo.AttrKey:       "userId",
			matomo.AttrContainer: resource.Ref{Address: mustContainerAddress(t, "other")},
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "must reference matomo.container.main") {
		t.Fatalf("ValidateResourceSet = %v, want reference to declared container", err)
	}
}

func TestValidateResourceSetRejectsContainerRefWithoutManagedContainer(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
	})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		variableResource(t, "user_id", resource.Attributes{
			matomo.AttrType:      "dataLayer",
			matomo.AttrKey:       "userId",
			matomo.AttrContainer: resource.Ref{Address: mustContainerAddress(t, "main")},
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "requires a declared matomo.container") {
		t.Fatalf("ValidateResourceSet = %v, want external-mode rejection of container $ref", err)
	}
}

func TestValidateResourceSetAllowsExternalContainerMode(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
	})
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		variableResource(t, "user_id", resource.Attributes{
			matomo.AttrType: "dataLayer",
			matomo.AttrKey:  "userId",
		}),
	}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestValidateResourceSetPublicationRequiresContainerSelector(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	err := p.ValidateResourceSet(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), matomo.EnvContainerID) {
		t.Fatalf("ValidateResourceSet = %v, want %s when publication is enabled", err, matomo.EnvContainerID)
	}
}

func TestValidateResourceSetRejectsMatomoConfigurationRefToDataLayer(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
	})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		variableResource(t, "user_id", resource.Attributes{
			matomo.AttrType: "dataLayer",
			matomo.AttrKey:  "userId",
		}),
		tagResource(t, "trial_started", resource.Attributes{
			matomo.AttrType:          "matomoAnalytics",
			matomo.AttrTrigger:       resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
			matomo.AttrEventCategory: "signup",
			matomo.AttrEventAction:   "trialStarted",
			matomo.AttrMatomoConfiguration: resource.Ref{
				Address: mustVariableAddress(t, "user_id"),
			},
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "matomoConfiguration") {
		t.Fatalf("ValidateResourceSet = %v, want type rejection", err)
	}
}

func TestValidateResourceSetAllowsManagedMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
	})
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		variableResource(t, "config", configVariableAttrs(nil)),
		tagResource(t, "trial_started", resource.Attributes{
			matomo.AttrType:          "matomoAnalytics",
			matomo.AttrTrigger:       resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
			matomo.AttrEventCategory: "signup",
			matomo.AttrEventAction:   "trialStarted",
			matomo.AttrMatomoConfiguration: resource.Ref{
				Address: mustVariableAddress(t, "config"),
			},
		}),
	}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestValidateResourceSetPublicationWithManagedContainerOmitsEnvID(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "3"})
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{
		containerResource(t, "main", defaultContainerAttrs()),
	}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestValidateVariableRejectsLiteralContainerID(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	err := p.Validate(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: variableContainerID,
	}))
	if err == nil || !strings.Contains(err.Error(), "$ref") {
		t.Fatalf("Validate = %v, want $ref error", err)
	}
}
