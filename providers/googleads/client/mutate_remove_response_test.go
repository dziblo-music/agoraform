package client_test

import (
	"context"
	"strings"
	"testing"
)

func TestMutateRemoveRequiresConfirmedResourceName(t *testing.T) {
	t.Parallel()

	resourceName := "customers/" + testCustomerID + "/conversionActions/1"
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "empty object", body: `{}`, wantErr: true},
		{name: "empty results", body: `{"results":[]}`, wantErr: true},
		{name: "missing resource name", body: `{"results":[{}]}`, wantErr: true},
		{name: "mismatched resource name", body: `{"results":[{"resourceName":"customers/` + testCustomerID + `/conversionActions/2"}]}`, wantErr: true},
		{name: "matching resource name", body: `{"results":[{"resourceName":"` + resourceName + `"}]}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeAds{mutateBody: tc.body}
			c := mustClient(t, startFakeAds(t, f))

			_, err := c.Mutate(context.Background(), "conversionActions", []map[string]any{
				{"remove": resourceName},
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("Mutate succeeded without a confirmed remove result")
				}
				if !strings.Contains(err.Error(), "malformed") {
					t.Fatalf("error = %q, want malformed response", err)
				}
				assertNoSecret(t, err.Error())
				return
			}
			if err != nil {
				t.Fatalf("Mutate: %v", err)
			}
		})
	}
}
