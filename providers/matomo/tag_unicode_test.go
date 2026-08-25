package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateTagUnicodeCharacterLimits(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	field500 := strings.Repeat("é", matomo.MaxEventFieldLen)
	field501 := strings.Repeat("é", matomo.MaxEventFieldLen+1)
	name255 := strings.Repeat("é", matomo.MaxTagNameLen)
	name256 := strings.Repeat("é", matomo.MaxTagNameLen+1)

	cases := []struct {
		name    string
		attrs   resource.Attributes
		wantErr string
	}{
		{
			name: "500-character unicode eventCategory",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: field500,
				matomo.AttrEventAction:   "trialStarted",
			},
		},
		{
			name: "501-character unicode eventCategory",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: field501,
				matomo.AttrEventAction:   "trialStarted",
			},
			wantErr: "500",
		},
		{
			name: "255-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: "signup",
				matomo.AttrEventAction:   "trialStarted",
				matomo.AttrName:          name255,
			},
		},
		{
			name: "256-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: "signup",
				matomo.AttrEventAction:   "trialStarted",
				matomo.AttrName:          name256,
			},
			wantErr: "255",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), tagResource(t, "unicode", tc.attrs))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tc.wantErr)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}
