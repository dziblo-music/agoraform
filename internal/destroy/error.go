package destroy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	StageMutation = "mutation"
	StagePersist  = "persist"
	StageFinalize = "finalize"
)

// PartialError reports that destroy failed after remote provider state may
// already have changed. Failures that occur before any remote mutation are
// ordinary errors and must not use this type.
type PartialError struct {
	Address         resource.Address
	Operation       string
	RemoteIdentity  resource.Identity
	Stage           string
	ResourceChanges bool
	Details         []string
	Err             error
}

func (e *PartialError) Error() string {
	if e == nil {
		return "partial destroy failure"
	}
	switch e.Stage {
	case StagePersist:
		return e.persistMessage()
	case StageFinalize:
		return e.finalizeMessage()
	default:
		if e.Err != nil {
			return e.Err.Error()
		}
		return "partial destroy failure"
	}
}

func (e *PartialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *PartialError) persistMessage() string {
	cause := e.Err
	if cause == nil {
		cause = errors.New("unknown error")
	}
	id := e.RemoteIdentity.ID
	if id == "" {
		return fmt.Sprintf("%s was destroyed remotely, but the local state write failed: %v\nThe identity binding was left unchanged so retry can confirm the remote resource is gone. Fix the state-file problem, then rerun agoraform destroy.", e.Address, cause)
	}
	return fmt.Sprintf("%s was destroyed remotely, but the local state write failed: %v\nIdentity %s remains in local state so retry can confirm the remote resource is gone. Fix the state-file problem, then rerun agoraform destroy.", e.Address, cause, id)
}

func (e *PartialError) finalizeMessage() string {
	var b strings.Builder
	op := e.Operation
	if op == "" {
		op = "finalize"
	}
	fmt.Fprintf(&b, "destroy %s: %s: %v", e.Address, op, e.Err)
	b.WriteByte('\n')
	if e.ResourceChanges {
		b.WriteString("Earlier resource changes remain applied; they were not rolled back.")
	} else {
		b.WriteString("Remote provider state may already have changed; it was not rolled back.")
	}
	if isUncertainOutcome(e.Err) {
		b.WriteString("\nInspect the remote provider state to determine whether the operation completed before retrying; do not create another version until that status is known.")
	} else {
		b.WriteString("\nFix the provider error, then rerun agoraform destroy.")
	}
	return b.String()
}

type uncertainOutcome interface {
	UncertainOutcome()
}

func isUncertainOutcome(err error) bool {
	var uncertain uncertainOutcome
	return errors.As(err, &uncertain)
}

// IsPartial reports whether err is a post-mutation destroy failure.
func IsPartial(err error) bool {
	var partial *PartialError
	return errors.As(err, &partial)
}

func persistDestroyError(addr resource.Address, id resource.Identity, err error) error {
	return &PartialError{
		Address:         addr,
		Operation:       "destroy",
		RemoteIdentity:  id,
		Stage:           StagePersist,
		ResourceChanges: true,
		Err:             err,
	}
}

func finalizeError(addr resource.Address, operation string, details []string, resourceChanges bool, err error) error {
	copied := append([]string(nil), details...)
	return &PartialError{
		Address:         addr,
		Operation:       operation,
		Stage:           StageFinalize,
		ResourceChanges: resourceChanges,
		Details:         copied,
		Err:             err,
	}
}

// RemainingError reports that supported teardown finished but one or more
// requested resources remain in state because destroy is unsupported or
// provider-owned.
type RemainingError struct {
	Addresses []resource.Address
}

func (e *RemainingError) Error() string {
	if e == nil || len(e.Addresses) == 0 {
		return "destroy left resources in state because destroy is unsupported"
	}
	names := make([]string, 0, len(e.Addresses))
	for _, addr := range e.Addresses {
		names = append(names, addr.String())
	}
	noun := "resource"
	if len(names) != 1 {
		noun = "resources"
	}
	return fmt.Sprintf("destroy left %d %s in state because destroy is unsupported: %s", len(names), noun, strings.Join(names, ", "))
}

func remainingError(p *Plan) error {
	return &RemainingError{Addresses: p.remainingAddresses()}
}
