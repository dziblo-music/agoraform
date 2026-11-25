package destroy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
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

// Failure is a planned destroy/remove that the provider did not complete.
// Its state binding is kept.
type Failure struct {
	Address resource.Address
	Err     error
}

// Skipped is a planned change that was not attempted because Cause, a
// resource that depends on it, failed or was itself skipped. Its state
// binding is kept.
type Skipped struct {
	Address resource.Address
	Cause   resource.Address
}

// IncompleteError reports that destroy continued past one or more failures.
// Every resource that was not torn down keeps its state binding, so a rerun
// retries only those resources. It unwraps to each failure's error.
type IncompleteError struct {
	Failures             []Failure
	Skipped              []Skipped
	SkippedFinalizations []provider.FinalizationPlan
}

func (e *IncompleteError) Error() string {
	if e == nil {
		return "destroy incomplete"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "destroy incomplete: %s failed", countNoun(len(e.Failures), "resource"))
	if n := len(e.Skipped); n > 0 {
		fmt.Fprintf(&b, ", %d skipped", n)
	}
	b.WriteString("; their state bindings were kept")
	for _, f := range e.Failures {
		fmt.Fprintf(&b, "\n  - %s: %v", f.Address, f.Err)
	}
	if len(e.Skipped) > 0 {
		b.WriteString("\nSkipped because a resource that depends on them was not destroyed:")
		for _, s := range e.Skipped {
			fmt.Fprintf(&b, "\n  - %s (blocked by %s)", s.Address, s.Cause)
		}
	}
	if len(e.SkippedFinalizations) > 0 {
		b.WriteString("\nProvider actions skipped because that provider's teardown is incomplete:")
		for _, f := range e.SkippedFinalizations {
			action := strings.TrimSpace(f.Action)
			if action == "" {
				action = "finalize"
			}
			fmt.Fprintf(&b, "\n  - %s: %s", f.Address, action)
		}
	}
	b.WriteString("\nFix the failures above, then rerun agoraform destroy; only resources still in state are retried.")
	return b.String()
}

func (e *IncompleteError) Unwrap() []error {
	if e == nil {
		return nil
	}
	out := make([]error, 0, len(e.Failures))
	for _, f := range e.Failures {
		out = append(out, f.Err)
	}
	return out
}

func countNoun(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// RemainingError reports that supported teardown finished but one or more
// requested resources remain in state because destroy is unsupported or
// provider-owned.
type RemainingError struct {
	Addresses []resource.Address
}

func (e *RemainingError) Error() string {
	if e == nil || len(e.Addresses) == 0 {
		return "Some resources remain in state because Agoraform cannot destroy them."
	}
	names := make([]string, 0, len(e.Addresses))
	for _, addr := range e.Addresses {
		names = append(names, addr.String())
	}
	noun, pronoun := "resource remains", "it"
	if len(names) != 1 {
		noun, pronoun = "resources remain", "them"
	}
	return fmt.Sprintf("%d %s in state because Agoraform cannot destroy %s: %s", len(names), noun, pronoun, strings.Join(names, ", "))
}

func remainingError(p *Plan) error {
	return &RemainingError{Addresses: p.remainingAddresses()}
}
