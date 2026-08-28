package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateTagMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	valid := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrMatomoConfiguration: resource.Ref{
			Address: mustVariableAddress(t, "config"),
		},
	})
	if err := p.Validate(context.Background(), valid); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateTagMatomoConfigurationRefErrors(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	addr := mustTagAddress(t, "trial_started")
	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name: "literal string",
			attrs: resource.Attributes{
				matomo.AttrType:                "matomoAnalytics",
				matomo.AttrTrigger:             resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
				matomo.AttrEventCategory:       "signup",
				matomo.AttrEventAction:         "trialStarted",
				matomo.AttrMatomoConfiguration: "Matomo Configuration",
			},
			want: "resource reference",
		},
		{
			name: "trigger ref",
			attrs: resource.Attributes{
				matomo.AttrType:                "matomoAnalytics",
				matomo.AttrTrigger:             resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
				matomo.AttrEventCategory:       "signup",
				matomo.AttrEventAction:         "trialStarted",
				matomo.AttrMatomoConfiguration: resource.Ref{Address: mustTriggerAddress(t, "trial_started")},
			},
			want: "matomo.variable",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestCreateTagUsesManagedMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	p := testTagProvider(t, srv)
	res := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrMatomoConfiguration: resource.Resolved{
			Address:  mustVariableAddress(t, "config"),
			Identity: resource.Identity{ID: "20"},
		},
	})

	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[matomoConfig]") != "{{Site Config}}" {
		t.Fatalf("matomoConfig = %v, want {{Site Config}}", srv.lastCreateValues())
	}
}

func TestCreateTagKeepsImplicitMatomoConfigurationDiscovery(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	})
	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[matomoConfig]") != "{{Matomo Configuration}}" {
		t.Fatalf("implicit matomoConfig = %v", srv.lastCreateValues())
	}
}

func TestCreateTagImplicitDiscoveryAmbiguous(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 21, Name: "Other Config", Type: "MatomoConfiguration"})
	p := testTagProvider(t, srv)
	res := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	})
	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create with default-named config among many: %v", err)
	}

	srv = newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 1, Name: "Primary", Type: "MatomoConfiguration"})
	srv.seedVariable(apiTagVariable{ID: 2, Name: "Secondary", Type: "MatomoConfiguration"})
	p = testTagProvider(t, srv)
	_, err := p.Create(context.Background(), res)
	if err == nil {
		t.Fatal("expected ambiguous configuration error")
	}
	if !strings.Contains(err.Error(), "multiple Matomo Configuration variables") {
		t.Fatalf("error = %q", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateTagImplicitDiscoveryMissing(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 1, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := testTagProvider(t, srv)
	_, err := p.Create(context.Background(), tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	}))
	if err == nil || !strings.Contains(err.Error(), "no Matomo Configuration variable") {
		t.Fatalf("Create = %v, want missing configuration guidance", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateTagRejectsDataLayerMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := testTagProvider(t, srv)
	_, err := p.Create(context.Background(), tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrMatomoConfiguration: resource.Resolved{
			Address:  mustVariableAddress(t, "user_id"),
			Identity: resource.Identity{ID: "2"},
		},
	}))
	if err == nil || !strings.Contains(err.Error(), "matomoConfiguration") {
		t.Fatalf("Create = %v, want type rejection", err)
	}
}

func TestPlanTagUnchangedWithManagedMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "signup",
		Action:        "trialStarted",
		MatomoConfig:  "Site Config",
	})
	p := testTagProvider(t, srv)

	config := variableResource(t, "config", configVariableAttrs(nil))
	config.Identity = resource.Identity{ID: "20"}
	trigger := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	trigger.Identity = resource.Identity{ID: "4"}
	tag := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrName:          "trialStarted",
		matomo.AttrTrigger:       resource.Ref{Address: trigger.Address},
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrMatomoConfiguration: resource.Ref{
			Address: config.Address,
		},
	})
	tag.Identity = resource.Identity{ID: "8"}

	got := mustPlanTag(t, p, config, trigger, tag)
	change := changeByAddr(t, got, "matomo.tag.trial_started")
	if change.Action != "unchanged" {
		t.Fatalf("tag change = %+v, want unchanged", change)
	}
}

func TestImportTagReconstructsMatomoConfigurationRef(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "signup",
		Action:        "trialStarted",
		MatomoConfig:  "Site Config",
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalogs(t, map[string]string{
		"matomo.variable.config":       "20",
		"matomo.trigger.trial_started": "4",
	}))

	live, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "8")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrMatomoConfiguration])
	if !ok || ref.Address.String() != "matomo.variable.config" {
		t.Fatalf("imported matomoConfiguration = %#v", live.Attributes[matomo.AttrMatomoConfiguration])
	}
}

func TestCreateTagExplicitRefWithMultipleConfigurations(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 21, Name: "Other Config", Type: "MatomoConfiguration"})
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	p := testTagProvider(t, srv)
	if _, err := p.Create(context.Background(), tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrMatomoConfiguration: resource.Resolved{
			Address:  mustVariableAddress(t, "config"),
			Identity: resource.Identity{ID: "20"},
		},
	})); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[matomoConfig]") != "{{Site Config}}" {
		t.Fatalf("matomoConfig = %v", srv.lastCreateValues())
	}
}
