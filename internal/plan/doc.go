// Package plan implements non-mutating reconciliation and execution plans.
//
// The engine reads live resources through provider.Reader, diffs configurable
// attributes, and produces a machine-usable Plan. Terminal rendering is a
// separate Format step so apply and CI tooling can consume the same change
// model. Plan never calls provider mutation methods.
package plan
