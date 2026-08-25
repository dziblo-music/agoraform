package apply

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

// Partial-apply failure stages. These distinguish failures after a successful
// provider mutation from state-write and provider-finalization failures.
const (
	StageMutation = "mutation"
	StagePersist  = "persist"
	StageFinalize = "finalize"
)

// PartialApplyError reports that apply failed after remote provider state may
// already have changed. Failures that occur before any remote mutation are
// ordinary errors and must not use this type.
type PartialApplyError struct {
	Address         resource.Address
	Operation       string
	RemoteMutation  bool
	RemoteIdentity  resource.Identity
	Stage           string
	ResourceChanges bool
	Details         []string
	Err             error
}

func (e *PartialApplyError) Error() string {
	if e == nil {
		return "partial apply failure"
	}
	switch e.Stage {
	case StageMutation:
		return e.mutationMessage()
	case StagePersist:
		return e.persistMessage()
	case StageFinalize:
		return e.finalizeMessage()
	default:
		if e.Err != nil {
			return e.Err.Error()
		}
		return "partial apply failure"
	}
}

func (e *PartialApplyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *PartialApplyError) mutationMessage() string {
	cause := e.Err
	if cause == nil {
		cause = errors.New("unknown error")
	}

	switch e.Operation {
	case "create":
		if e.RemoteIdentity.IsZero() {
			return fmt.Sprintf("%s was created remotely, but Agoraform could not safely accept the provider result: %v\nRemote state may already have changed; it was not rolled back. Identify the created remote resource before retrying, then bind it with agoraform import.", e.Address, cause)
		}
		return fmt.Sprintf("%s was created remotely with id %s, but Agoraform could not safely accept the provider result: %v\nRemote state may already have changed; it was not rolled back. Fix the provider issue, then inspect the remote resource and re-bind it with agoraform import if needed.", e.Address, e.RemoteIdentity.ID, cause)
	case "update":
		return fmt.Sprintf("%s was updated remotely, but Agoraform could not safely accept the provider result: %v\nThe existing identity binding remains unchanged. Inspect the remote resource and fix the provider issue before rerunning agoraform plan and agoraform apply.", e.Address, cause)
	default:
		return fmt.Sprintf("apply %s: %s: %v\nRemote provider state may already have changed; it was not rolled back.", e.Address, e.Operation, cause)
	}
}

func (e *PartialApplyError) persistMessage() string {
	cause := e.Err
	if cause == nil {
		cause = errors.New("unknown error")
	}

	if e.Operation == "update" {
		return fmt.Sprintf("%s was updated remotely, but the local state write failed: %v\nThe existing identity binding remains valid. Fix the state-file problem, then rerun agoraform plan and agoraform apply.", e.Address, cause)
	}

	id := e.RemoteIdentity.ID
	var conflict *state.DuplicateIdentityError
	if errors.As(cause, &conflict) {
		if id == "" {
			id = conflict.RemoteID
		}
		owner := conflict.OwnerOtherThan(e.Address)
		if owner == "" {
			owner = conflict.Existing
		}
		return fmt.Sprintf("%s was created remotely with id %s, but its state binding could not be saved: %v\nIdentity %q is already bound to %s. Resolve that ownership conflict before retrying; importing the same identity onto %s will fail for the same reason.", e.Address, id, cause, id, owner, e.Address)
	}

	if id == "" {
		return fmt.Sprintf("%s was created remotely, but its state binding could not be saved: %v\nFix the state-file problem, then bind the remote identity with agoraform import.", e.Address, cause)
	}
	return fmt.Sprintf("%s was created remotely with id %s, but its state binding could not be saved: %v\nFix the state-file problem, then run:\n  agoraform import %s %s", e.Address, id, cause, e.Address, id)
}

func (e *PartialApplyError) finalizeMessage() string {
	var b strings.Builder
	op := e.Operation
	if op == "" {
		op = "finalize"
	}
	fmt.Fprintf(&b, "apply %s: %s: %v", e.Address, op, e.Err)
	b.WriteByte('\n')
	if e.ResourceChanges {
		b.WriteString("Earlier resource changes remain applied; they were not rolled back.")
	} else {
		b.WriteString("Remote provider state may already have changed; it was not rolled back.")
	}
	if isUncertainOutcome(e.Err) {
		b.WriteString("\nInspect the remote provider state to determine whether the operation completed before retrying; do not create another version until that status is known.")
	} else {
		b.WriteString("\nFix the provider error, then rerun agoraform plan and agoraform apply.")
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

// IsPartial reports whether err is a post-mutation apply failure.
func IsPartial(err error) bool {
	var partial *PartialApplyError
	return errors.As(err, &partial)
}

func postMutationError(addr resource.Address, operation string, id resource.Identity, err error) error {
	return &PartialApplyError{
		Address:         addr,
		Operation:       operation,
		RemoteMutation:  true,
		RemoteIdentity:  id,
		Stage:           StageMutation,
		ResourceChanges: true,
		Err:             err,
	}
}

func persistCreateError(addr resource.Address, id resource.Identity, err error) error {
	return &PartialApplyError{
		Address:         addr,
		Operation:       "create",
		RemoteMutation:  true,
		RemoteIdentity:  id,
		Stage:           StagePersist,
		ResourceChanges: true,
		Err:             err,
	}
}

func persistUpdateError(addr resource.Address, id resource.Identity, err error) error {
	return &PartialApplyError{
		Address:         addr,
		Operation:       "update",
		RemoteMutation:  true,
		RemoteIdentity:  id,
		Stage:           StagePersist,
		ResourceChanges: true,
		Err:             err,
	}
}

func finalizeError(addr resource.Address, operation string, details []string, resourceChanges bool, err error) error {
	copied := append([]string(nil), details...)
	return &PartialApplyError{
		Address:         addr,
		Operation:       operation,
		RemoteMutation:  true,
		Stage:           StageFinalize,
		ResourceChanges: resourceChanges,
		Details:         copied,
		Err:             err,
	}
}
