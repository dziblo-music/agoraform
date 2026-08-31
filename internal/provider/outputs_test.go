package provider_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestValidateOutputRefsAcceptsDeclaredNonSensitive(t *testing.T) {
	t.Parallel()

	parent := resource.Resource{
		Address:    mustOutputAddress(t, "fake.widget.homepage"),
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}
	child := resource.Resource{
		Address: mustOutputAddress(t, "alt.note.banner"),
		Attributes: resource.Attributes{
			fake.AttrText: resource.Ref{Address: parent.Address, Output: fake.OutputToken},
		},
	}

	err := provider.ValidateOutputRefs([]resource.Resource{parent, child}, func(addr resource.Address) (provider.Reader, error) {
		if addr.Provider == fake.AltName {
			return fake.NewAlt(), nil
		}
		return fake.New(), nil
	})
	if err != nil {
		t.Fatalf("ValidateOutputRefs: %v", err)
	}
}

func TestValidateOutputRefsRejectsUnknownSensitiveAndUndeclared(t *testing.T) {
	t.Parallel()

	parent := resource.Resource{
		Address:    mustOutputAddress(t, "fake.widget.homepage"),
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}

	cases := []struct {
		name   string
		output string
		want   string
	}{
		{name: "unknown", output: "etag", want: "no declared output"},
		{name: "sensitive", output: fake.OutputSecret, want: "sensitive"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			child := resource.Resource{
				Address: mustOutputAddress(t, "alt.note.banner"),
				Attributes: resource.Attributes{
					fake.AttrText: resource.Ref{Address: parent.Address, Output: tc.output},
				},
			}
			err := provider.ValidateOutputRefs([]resource.Resource{parent, child}, func(resource.Address) (provider.Reader, error) {
				return fake.New(), nil
			})
			if err == nil {
				t.Fatal("ValidateOutputRefs succeeded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Fatalf("error leaked a secret: %q", err)
			}
		})
	}
}

func TestKindOfAndKindMatches(t *testing.T) {
	t.Parallel()

	if kind, ok := provider.KindOf("tok"); !ok || kind != provider.OutputKindString {
		t.Fatalf("KindOf(string) = (%q, %v)", kind, ok)
	}
	if kind, ok := provider.KindOf(3); !ok || kind != provider.OutputKindNumber {
		t.Fatalf("KindOf(int) = (%q, %v)", kind, ok)
	}
	if kind, ok := provider.KindOf(true); !ok || kind != provider.OutputKindBool {
		t.Fatalf("KindOf(bool) = (%q, %v)", kind, ok)
	}
	if _, ok := provider.KindOf([]any{"x"}); ok {
		t.Fatal("KindOf(slice) should fail")
	}
	if provider.KindMatches(4, provider.OutputKindString) {
		t.Fatal("number should not match string kind")
	}
	if !provider.KindMatches(4, provider.OutputKindNumber) {
		t.Fatal("number should match number kind")
	}
}

func TestFindOutput(t *testing.T) {
	t.Parallel()

	specs := []provider.OutputSpec{
		{Name: fake.AttrSerial, Kind: provider.OutputKindNumber},
		{Name: fake.OutputSecret, Kind: provider.OutputKindString, Sensitive: true},
	}
	got, ok := provider.FindOutput(specs, fake.OutputSecret)
	if !ok || !got.Sensitive {
		t.Fatalf("FindOutput(secret) = (%v, %v)", got, ok)
	}
	if _, ok := provider.FindOutput(specs, "missing"); ok {
		t.Fatal("FindOutput(missing) should fail")
	}
}

func mustOutputAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
