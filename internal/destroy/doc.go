// Package destroy owns Agoraform's provider-neutral destroy lifecycle.
//
// Build constructs a deterministic destroy plan from the manifest graph and
// local identity state. Execute carries out supported destructive operations
// in reverse dependency order, removes confirmed identities from local state,
// and runs planned provider finalizations only after those mutations succeed.
//
// Providers that do not implement provider.Destroyer, or that report
// unsupported/provider-owned capability, are planned non-mutations: they
// remain in state and do not block supported teardown, but the command
// finishes non-zero when any such remnant remains. State identities that are
// absent from the manifest are preserved and are not destroyed.
package destroy
