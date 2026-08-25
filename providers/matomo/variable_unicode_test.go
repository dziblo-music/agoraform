package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateVariableUnicodeCharacterLimits(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	key255 := strings.Repeat("é", matomo.MaxVariableNameLen)
	key256 := strings.Repeat("é", matomo.MaxVariableNameLen+1)
	key300 := strings.Repeat("é", matomo.MaxDataLayerKeyLen)
	key301 := strings.Repeat("é", matomo.MaxDataLayerKeyLen+1)
	name255 := strings.Repeat("é", matomo.MaxVariableNameLen)
	name256 := strings.Repeat("é", matomo.MaxVariableNameLen+1)

	cases := []struct {
		name    string
		attrs   resource.Attributes
		wantErr string
	}{
		{
			name: "255-character unicode key without name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key255,
			},
		},
		{
			name: "256-character unicode key without name exceeds effective name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key256,
			},
			wantErr: "255",
		},
		{
			name: "300-character unicode key with short name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key300,
				matomo.AttrName: "User ID",
			},
		},
		{
			name: "301-character unicode key exceeds data layer limit",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key301,
				matomo.AttrName: "User ID",
			},
			wantErr: "300",
		},
		{
			name: "255-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: name255,
			},
		},
		{
			name: "256-character unicode explicit name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: name256,
			},
			wantErr: "255",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), variableResource(t, "unicode", tc.attrs))
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
