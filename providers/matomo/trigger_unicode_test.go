package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateTriggerUnicodeCharacterLimits(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	event255 := strings.Repeat("é", matomo.MaxTriggerNameLen)
	event256 := strings.Repeat("é", matomo.MaxTriggerNameLen+1)
	event300 := strings.Repeat("é", matomo.MaxEventNameLen)
	event301 := strings.Repeat("é", matomo.MaxEventNameLen+1)
	name255 := strings.Repeat("é", matomo.MaxTriggerNameLen)
	name256 := strings.Repeat("é", matomo.MaxTriggerNameLen+1)

	cases := []struct {
		name    string
		attrs   resource.Attributes
		wantErr string
	}{
		{
			name: "255-character unicode event without name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event255,
			},
		},
		{
			name: "256-character unicode event without name exceeds effective name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event256,
			},
			wantErr: "255",
		},
		{
			name: "300-character unicode event with short name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event300,
				matomo.AttrName:  "Trial Started",
			},
		},
		{
			name: "301-character unicode event exceeds custom event limit",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event301,
				matomo.AttrName:  "Trial Started",
			},
			wantErr: "300",
		},
		{
			name: "255-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  name255,
			},
		},
		{
			name: "256-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  name256,
			},
			wantErr: "255",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), triggerResource(t, "unicode", tc.attrs))
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
