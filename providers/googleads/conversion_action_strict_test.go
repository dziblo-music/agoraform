package googleads_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateConversionActionWebpageClickWindowBoundary(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	valid := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:                           "Trial Started",
		googleads.AttrCategory:                       "SIGNUP",
		googleads.AttrClickThroughLookbackWindowDays: 30,
	})
	if err := p.Validate(context.Background(), valid); err != nil {
		t.Fatalf("Validate 30-day WEBPAGE window: %v", err)
	}

	invalid := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:                           "Trial Started",
		googleads.AttrCategory:                       "SIGNUP",
		googleads.AttrClickThroughLookbackWindowDays: 31,
	})
	err := p.Validate(context.Background(), invalid)
	if err == nil {
		t.Fatal("Validate 31-day WEBPAGE window succeeded, want error")
	}
	if !strings.Contains(err.Error(), "between 1 and 30") {
		t.Fatalf("error = %q, want WEBPAGE 1-30 day guidance", err)
	}
}

func TestReadConversionActionRejectsIncompleteAPIObjects(t *testing.T) {
	t.Parallel()

	base := `"id":"11","resourceName":"customers/` + testCustomerID + `/conversionActions/11","name":"Trial Started","category":"SIGNUP","type":"WEBPAGE"`
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing type",
			body: `{"results":[{"conversionAction":{"id":"11","resourceName":"customers/` + testCustomerID + `/conversionActions/11","name":"Trial Started","category":"SIGNUP"}}]}`,
			want: "missing type",
		},
		{
			name: "missing category",
			body: `{"results":[{"conversionAction":{"id":"11","resourceName":"customers/` + testCustomerID + `/conversionActions/11","name":"Trial Started","type":"WEBPAGE"}}]}`,
			want: "missing category",
		},
		{
			name: "malformed resource name",
			body: `{"results":[{"conversionAction":{"id":"11","resourceName":"conversionActions/11","name":"Trial Started","category":"SIGNUP","type":"WEBPAGE"}}]}`,
			want: "resourceName",
		},
		{
			name: "non-integral click window",
			body: `{"results":[{"conversionAction":{` + base + `,"clickThroughLookbackWindowDays":1.5}}]}`,
			want: "clickThroughLookbackWindowDays",
		},
		{
			name: "webpage click window above maximum",
			body: `{"results":[{"conversionAction":{` + base + `,"clickThroughLookbackWindowDays":"31"}}]}`,
			want: "clickThroughLookbackWindowDays",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newConversionActionFake()
			fake.searchBody = tc.body
			p, _ := testConversionActionProvider(t, fake)
			_, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
			}))
			if err == nil {
				t.Fatal("Read succeeded, want malformed response error")
			}
			if errors.Is(err, provider.ErrNotFound) {
				t.Fatalf("Read = %v, malformed response must not be ErrNotFound", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestImportConversionActionRejectsMalformedRemote(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchBody = `{"results":[{"conversionAction":{"id":"12","resourceName":"customers/` + testCustomerID + `/conversionActions/12","name":"Trial Started","category":"SIGNUP"}}]}`
	p, _ := testConversionActionProvider(t, fake)
	_, err := p.Import(context.Background(), mustConversionActionAddress(t, "trial_started"), "12")
	if err == nil {
		t.Fatal("Import succeeded, want malformed response error")
	}
	if !strings.Contains(err.Error(), "missing type") {
		t.Fatalf("error = %q, want missing type", err)
	}
}
