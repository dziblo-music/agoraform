package client

import (
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
	return &Error{Method: method, StatusCode: status, Message: "malformed JSON response"}
}

func unexpectedStatusError(method string, status int) *Error {
	return &Error{Method: method, StatusCode: status, Message: fmt.Sprintf("unexpected HTTP status %d", status)}
}
