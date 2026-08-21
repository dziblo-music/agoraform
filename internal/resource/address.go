package resource

import (
	"fmt"
	"strings"
)

// Address is a stable logical resource address of the form provider.type.name.
//
// Example: matomo.goal.trial_started
//
// Addresses are parseable, deterministic, and intended for plans, identity
// mapping, diagnostics, and imports. Each segment must be a lowercase
// identifier: start with a letter, then letters, digits, or underscores.
type Address struct {
	Provider string
	Type     string
	Name     string
}

// ParseAddress parses a dotted resource address.
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Address{}, fmt.Errorf("resource address is empty")
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Address{}, fmt.Errorf("resource address %q must be provider.type.name", s)
	}

	addr := Address{
		Provider: parts[0],
		Type:     parts[1],
		Name:     parts[2],
	}
	if err := addr.Validate(); err != nil {
		return Address{}, err
	}
	return addr, nil
}

// String returns the canonical dotted address.
func (a Address) String() string {
	return a.Provider + "." + a.Type + "." + a.Name
}

// IsZero reports whether the address is the zero value.
func (a Address) IsZero() bool {
	return a.Provider == "" && a.Type == "" && a.Name == ""
}

// Validate reports whether the address segments are well-formed.
func (a Address) Validate() error {
	if err := validateSegment("provider", a.Provider); err != nil {
		return err
	}
	if err := validateSegment("type", a.Type); err != nil {
		return err
	}
	if err := validateSegment("name", a.Name); err != nil {
		return err
	}
	return nil
}

func validateSegment(label, value string) error {
	if value == "" {
		return fmt.Errorf("resource address %s is empty", label)
	}
	if !isIdentifier(value) {
		return fmt.Errorf("resource address %s %q is invalid: use a lowercase letter followed by letters, digits, or underscores", label, value)
	}
	return nil
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return false
			}
			continue
		}
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
