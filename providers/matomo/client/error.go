package client

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Error is a Matomo API or transport failure safe to print.
//
// Message and Error() never include authentication tokens or
// credential-bearing URLs.
type Error struct {
	Method     string
	StatusCode int
	Message    string
	err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "matomo: unknown error"
	}

	var b strings.Builder
	b.WriteString("matomo")
	if e.Method != "" {
		b.WriteString(": ")
		b.WriteString(e.Method)
	}
	if e.StatusCode > 0 {
		b.WriteString(": HTTP ")
		b.WriteString(strconv.Itoa(e.StatusCode))
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	} else if e.err != nil {
		b.WriteString(": ")
		b.WriteString(e.err.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *Error) IsUnauthorized() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == 401 || e.StatusCode == 403
}

func apiError(method string, status int, message string) *Error {
	if message == "" {
		message = "API error"
	}
	return &Error{Method: method, StatusCode: status, Message: message}
}

func fmtSafe(method, message string, err error) *Error {
	return &Error{Method: method, Message: message, err: err}
}

func unauthorizedError(method string, status int) *Error {
	return &Error{Method: method, StatusCode: status, Message: "authentication failed"}
}

func malformedResponseError(method string, status int) *Error {
	return &Error{
		Method:     method,
		StatusCode: status,
		Message:    "malformed JSON response",
		err:        unconfirmed("malformed response"),
	}
}

func unexpectedStatusError(method string, status int) *Error {
	return &Error{
		Method:     method,
		StatusCode: status,
		Message:    fmt.Sprintf("unexpected HTTP status %d", status),
		err:        unconfirmed("unexpected HTTP status"),
	}
}

func responseTooLargeError(method string, status int) *Error {
	return &Error{
		Method:     method,
		StatusCode: status,
		Message:    "response exceeded size limit",
		err:        unconfirmed("response exceeded size limit"),
	}
}

func responseReadError(method string, status int) *Error {
	return &Error{
		Method:     method,
		StatusCode: status,
		Message:    "response could not be read",
		err:        unconfirmed("response could not be read"),
	}
}

// errUnconfirmed marks a response-processing failure after the HTTP request
// may already have reached Matomo. It is distinct from an explicit Matomo
// {result:"error"} payload.
var errUnconfirmed = errors.New("unconfirmed response")

type unconfirmedFailure struct {
	reason string
}

func unconfirmed(reason string) error {
	return unconfirmedFailure{reason: reason}
}

func (u unconfirmedFailure) Error() string {
	if u.reason == "" {
		return errUnconfirmed.Error()
	}
	return u.reason
}

func (u unconfirmedFailure) Unwrap() error {
	return errUnconfirmed
}

func isUnconfirmed(err error) bool {
	return errors.Is(err, errUnconfirmed)
}

func unconfirmedReason(err error) string {
	var u unconfirmedFailure
	if errors.As(err, &u) && u.reason != "" {
		return u.reason
	}
	return "unconfirmed response"
}

// UncertainOutcomeError reports that a mutating request was sent but the
// response did not confirm whether Matomo completed the operation.
type UncertainOutcomeError struct {
	Method string
	Reason string
	err    error
}

func (e *UncertainOutcomeError) Error() string {
	if e == nil {
		return "matomo: publication outcome is uncertain"
	}

	var b strings.Builder
	b.WriteString("matomo")
	if e.Method != "" {
		b.WriteString(": ")
		b.WriteString(e.Method)
	}
	b.WriteString(": publication outcome is uncertain")
	if e.Reason != "" {
		b.WriteString(": ")
		b.WriteString(e.Reason)
	}
	b.WriteString("; inspect the remote container before retrying and do not create another version until publication status is known")
	return b.String()
}

func (e *UncertainOutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// UncertainOutcome marks post-request failures whose remote result is unknown.
// apply uses this to distinguish them from pre-request errors.
func (e *UncertainOutcomeError) UncertainOutcome() {}

// IsUncertainOutcome reports whether err is a post-request uncertain outcome.
func IsUncertainOutcome(err error) bool {
	var uncertain *UncertainOutcomeError
	return errors.As(err, &uncertain)
}

func uncertainOutcomeError(method, reason string, err error) error {
	return &UncertainOutcomeError{Method: method, Reason: reason, err: err}
}
