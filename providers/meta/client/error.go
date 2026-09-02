package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Error is a secret-safe Meta API or transport failure.
type Error struct {
	Operation   string
	StatusCode  int
	Type        string
	Code        int
	Subcode     int
	Message     string
	UserTitle   string
	UserMessage string
	RequestID   string
	TraceID     string
	Transient   bool
	err         error
}

func (e *Error) Error() string {
	if e == nil {
		return "meta: unknown error"
	}
	var b strings.Builder
	b.WriteString("meta")
	if e.Operation != "" {
		b.WriteString(": ")
		b.WriteString(e.Operation)
	}
	if e.StatusCode != 0 {
		b.WriteString(": HTTP ")
		b.WriteString(strconv.Itoa(e.StatusCode))
	}
	if e.Code != 0 {
		b.WriteString(": API code ")
		b.WriteString(strconv.Itoa(e.Code))
		if e.Subcode != 0 {
			b.WriteString(" subcode ")
			b.WriteString(strconv.Itoa(e.Subcode))
		}
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	} else if e.err != nil {
		b.WriteString(": ")
		b.WriteString(e.err.Error())
	}
	if e.RequestID != "" {
		b.WriteString(" (request-id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	if e.TraceID != "" {
		b.WriteString(" (trace-id ")
		b.WriteString(e.TraceID)
		b.WriteString(")")
	}
	return b.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// IsAuthentication reports an invalid, expired, or missing token.
func (e *Error) IsAuthentication() bool {
	return e != nil && (e.StatusCode == 401 || e.Code == 190 || e.Code == 102)
}

// IsPermission reports that authentication succeeded but the operation or
// target is not allowed for the token.
func (e *Error) IsPermission() bool {
	if e == nil || e.IsAuthentication() {
		return false
	}
	return e.StatusCode == 403 || e.Code == 10 || e.Code == 200 || e.Code == 294
}

// IsTransient reports failures that a caller may choose to retry. The client
// itself never automatically retries mutations.
func (e *Error) IsTransient() bool {
	return e != nil && e.Transient
}

type errorEnvelope struct {
	Error struct {
		Message          string `json:"message"`
		Type             string `json:"type"`
		Code             int    `json:"code"`
		ErrorSubcode     int    `json:"error_subcode"`
		IsTransient      bool   `json:"is_transient"`
		ErrorUserTitle   string `json:"error_user_title"`
		ErrorUserMessage string `json:"error_user_msg"`
		TraceID          string `json:"fbtrace_id"`
	} `json:"error"`
}

func parseAPIError(operation string, status int, requestID, traceID string, body []byte, secrets ...string) *Error {
	var envelope errorEnvelope
	_ = json.Unmarshal(body, &envelope)
	api := envelope.Error
	message := strings.TrimSpace(Redact(api.Message, secrets...))
	if message == "" {
		message = fmt.Sprintf("unexpected HTTP status %d", status)
	}
	if traceID == "" {
		traceID = api.TraceID
	}
	requestID = strings.TrimSpace(Redact(requestID, secrets...))
	traceID = strings.TrimSpace(Redact(traceID, secrets...))
	return &Error{
		Operation: operation, StatusCode: status, Type: strings.TrimSpace(api.Type),
		Code: api.Code, Subcode: api.ErrorSubcode, Message: message,
		UserTitle:   strings.TrimSpace(Redact(api.ErrorUserTitle, secrets...)),
		UserMessage: strings.TrimSpace(Redact(api.ErrorUserMessage, secrets...)),
		RequestID:   requestID, TraceID: traceID,
		Transient: api.IsTransient || transientStatus(status) || transientCode(api.Code),
	}
}

func transportError(operation string, err error, secrets ...string) *Error {
	message := "network error"
	if errors.Is(err, context.Canceled) {
		message = "request canceled"
	} else if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		message = "request timed out"
	}
	return &Error{Operation: operation, Message: message, Transient: message == "network error" || message == "request timed out", err: safeWrappedError(err, secrets...)}
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr) && urlErr.Timeout()
}

func safeWrappedError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	return errors.New(Redact(err.Error(), secrets...))
}

func transientStatus(status int) bool { return status == 408 || status == 429 || status >= 500 }

func transientCode(code int) bool {
	switch code {
	case 1, 2, 4, 17, 32, 341, 613:
		return true
	default:
		return false
	}
}
