package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Error is a Google Ads API, OAuth, or transport failure safe to print.
//
// Message and Error() never include developer tokens, OAuth secrets,
// access tokens, or credential-bearing URLs.
type Error struct {
	Operation  string
	StatusCode int
	Status     string
	Message    string
	RequestID  string
	err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "googleads: unknown error"
	}

	var b strings.Builder
	b.WriteString("googleads")
	if e.Operation != "" {
		b.WriteString(": ")
		b.WriteString(e.Operation)
	}
	if e.StatusCode > 0 {
		b.WriteString(": HTTP ")
		b.WriteString(strconv.Itoa(e.StatusCode))
	}
	if e.Status != "" {
		b.WriteString(" ")
		b.WriteString(e.Status)
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
	return b.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// IsUnauthorized reports an authentication or permission failure.
func (e *Error) IsUnauthorized() bool {
	if e == nil {
		return false
	}
	if e.StatusCode == 401 || e.StatusCode == 403 {
		return true
	}
	switch e.Status {
	case "UNAUTHENTICATED", "PERMISSION_DENIED":
		return true
	default:
		return false
	}
}

func fmtSafe(operation, message string, err error) *Error {
	return &Error{Operation: operation, Message: message, err: err}
}

func unauthorizedError(operation string, status int) *Error {
	return &Error{Operation: operation, StatusCode: status, Message: "authentication failed"}
}

func malformedResponseError(operation string, status int) *Error {
	return &Error{
		Operation:  operation,
		StatusCode: status,
		Message:    "malformed JSON response",
	}
}

func unexpectedStatusError(operation string, status int, message string) *Error {
	if message == "" {
		message = fmt.Sprintf("unexpected HTTP status %d", status)
	}
	return &Error{
		Operation:  operation,
		StatusCode: status,
		Message:    message,
	}
}

func responseTooLargeError(operation string, status int) *Error {
	return &Error{
		Operation:  operation,
		StatusCode: status,
		Message:    "response exceeded size limit",
	}
}

func responseReadError(operation string, status int) *Error {
	return &Error{
		Operation:  operation,
		StatusCode: status,
		Message:    "response could not be read",
	}
}

type googleRPCError struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Status  string            `json:"status"`
	Details []json.RawMessage `json:"details"`
}

type googleAdsFailure struct {
	Errors    []googleAdsError `json:"errors"`
	RequestID string           `json:"requestId"`
}

type googleAdsError struct {
	ErrorCode json.RawMessage `json:"errorCode"`
	Message   string          `json:"message"`
}

type oauthErrorBody struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func parseAPIError(operation string, status int, requestID string, body []byte, secrets []string) *Error {
	if operation == "oauth" && (status == 400 || status == 401 || status == 403) {
		return &Error{
			Operation:  operation,
			StatusCode: status,
			Status:     "UNAUTHENTICATED",
			Message:    "authentication failed: could not refresh access token",
		}
	}

	msg := ""
	rpcStatus := ""
	failureID := ""

	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Error) > 0 {
		var rpc googleRPCError
		if json.Unmarshal(envelope.Error, &rpc) == nil && (rpc.Message != "" || rpc.Status != "" || rpc.Code != 0) {
			rpcStatus = strings.TrimSpace(rpc.Status)
			msg = strings.TrimSpace(rpc.Message)
			if detail := formatGoogleAdsFailure(rpc.Details); detail != "" {
				if msg == "" {
					msg = detail
				} else {
					msg = msg + ": " + detail
				}
			}
			if id := googleAdsRequestID(rpc.Details); id != "" {
				failureID = id
			}
		} else {
			var oauth string
			if json.Unmarshal(envelope.Error, &oauth) == nil && oauth != "" {
				msg = oauth
				var oauthBody oauthErrorBody
				if json.Unmarshal(body, &oauthBody) == nil && oauthBody.ErrorDescription != "" {
					msg = oauth + ": " + oauthBody.ErrorDescription
				}
			}
		}
	}

	if msg == "" {
		var oauthBody oauthErrorBody
		if json.Unmarshal(body, &oauthBody) == nil && oauthBody.Error != "" {
			msg = oauthBody.Error
			if oauthBody.ErrorDescription != "" {
				msg = msg + ": " + oauthBody.ErrorDescription
			}
		}
	}

	msg = strings.TrimSpace(Redact(msg, secrets...))
	if msg == "" {
		msg = fmt.Sprintf("unexpected HTTP status %d", status)
	}
	if requestID == "" {
		requestID = failureID
	}

	if status == 401 || status == 403 || rpcStatus == "UNAUTHENTICATED" || rpcStatus == "PERMISSION_DENIED" {
		out := unauthorizedError(operation, status)
		out.Status = rpcStatus
		out.Message = "authentication failed"
		if msg != "" && msg != "authentication failed" && !looksLikeSecretDump(msg) {
			out.Message = "authentication failed: " + summarizeAuthMessage(msg)
		}
		out.RequestID = requestID
		return out
	}

	return &Error{
		Operation:  operation,
		StatusCode: status,
		Status:     rpcStatus,
		Message:    msg,
		RequestID:  requestID,
	}
}

func formatGoogleAdsFailure(details []json.RawMessage) string {
	var parts []string
	for _, raw := range details {
		var failure googleAdsFailure
		if json.Unmarshal(raw, &failure) != nil {
			continue
		}
		for _, item := range failure.Errors {
			msg := strings.TrimSpace(item.Message)
			if code := formatErrorCode(item.ErrorCode); code != "" {
				if msg == "" {
					msg = code
				} else {
					msg = code + ": " + msg
				}
			}
			if msg != "" {
				parts = append(parts, msg)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func googleAdsRequestID(details []json.RawMessage) string {
	for _, raw := range details {
		var failure googleAdsFailure
		if json.Unmarshal(raw, &failure) == nil && failure.RequestID != "" {
			return failure.RequestID
		}
	}
	return ""
}

func formatErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for k, v := range m {
		if k != "" && v != "" {
			return k + "=" + v
		}
	}
	return ""
}

func looksLikeSecretDump(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "bearer ") || strings.Contains(lower, "developer-token")
}

func summarizeAuthMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "invalid credentials"
	}
	return msg
}
