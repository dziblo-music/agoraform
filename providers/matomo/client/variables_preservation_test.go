package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestUpdateContainerVariableRejectsLossyPreservedParametersBeforeMutation(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "null", value: nil, want: "is null"},
		{name: "empty array", value: []any{}, want: "is an empty array"},
		{name: "empty object", value: map[string]any{}, want: "is an empty object"},
		{
			name: "nested empty object",
			value: []any{
				map[string]any{"name": "kept"},
				map[string]any{},
			},
			want: "parameters[unowned][1] is an empty object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_, _ = w.Write([]byte(`null`))
			}))
			t.Cleanup(srv.Close)

			c := mustTagClient(t, srv)
			err := c.TagManager().UpdateContainerVariable(context.Background(), "9", "20", client.VariableInput{
				Type: "MatomoConfiguration",
				Name: "Matomo Configuration",
				Parameters: map[string]any{
					"matomoUrl":          "https://matomo.example.com",
					"idSite":             "1",
					"enableLinkTracking": true,
				},
			}, client.VariablePreservedFields{
				Parameters: map[string]any{"unowned": tc.value},
			})
			if err == nil {
				t.Fatal("expected preservation error")
			}
			if !strings.Contains(err.Error(), "remote variable parameters cannot be preserved without loss") {
				t.Fatalf("error = %q, want preservation failure", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			if calls != 0 {
				t.Fatalf("update made %d remote requests; want zero", calls)
			}
		})
	}
}
