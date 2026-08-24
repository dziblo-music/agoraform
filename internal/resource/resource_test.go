package resource

import "testing"

func TestAttributesCloneIndependent(t *testing.T) {
	t.Parallel()

	orig := Attributes{
		"title": "Homepage",
		"meta": map[string]any{
			"color": "blue",
		},
		"tags":   []any{"a", "b"},
		"parent": Ref{Address: Address{Provider: "fake", Type: "widget", Name: "homepage"}},
	}

	cloned := orig.Clone()
	cloned["title"] = "Changed"
	cloned["meta"].(map[string]any)["color"] = "red"
	cloned["tags"].([]any)[0] = "z"
	cloned["parent"] = Ref{Address: Address{Provider: "fake", Type: "widget", Name: "other"}}

	if orig["title"] != "Homepage" {
		t.Fatalf("clone mutated original title: %v", orig["title"])
	}
	if orig["meta"].(map[string]any)["color"] != "blue" {
		t.Fatalf("clone mutated nested map: %v", orig["meta"])
	}
	if orig["tags"].([]any)[0] != "a" {
		t.Fatalf("clone mutated nested slice: %v", orig["tags"])
	}
	if orig["parent"].(Ref).Address.Name != "homepage" {
		t.Fatalf("clone mutated original parent: %v", orig["parent"])
	}
}

func TestAttributesCloneNil(t *testing.T) {
	t.Parallel()

	var a Attributes
	cloned := a.Clone()
	if cloned == nil {
		t.Fatal("Clone of nil Attributes should return an empty map")
	}
	if len(cloned) != 0 {
		t.Fatalf("Clone of nil Attributes length = %d, want 0", len(cloned))
	}
}

func TestIdentityIsZero(t *testing.T) {
	t.Parallel()

	if !(Identity{}).IsZero() {
		t.Fatal("empty Identity should be zero")
	}
	if (Identity{ID: "widget-1"}).IsZero() {
		t.Fatal("non-empty Identity should not be zero")
	}
}
