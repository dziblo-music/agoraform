package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckConnectionSuccessIsReadOnly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/me/permissions"):
			_, _ = io.WriteString(w, `{"data":[{"permission":"ads_management","status":"granted"}]}`)
		case strings.HasSuffix(r.URL.Path, "/act_"+testAccountID):
			_, _ = io.WriteString(w, `{"id":"`+testAccountID+`","account_status":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	if err := c.CheckConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCheckConnectionMissingPermission(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"permission":"ads_read","status":"granted"}]}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	err := c.CheckConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ads_management") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckConnectionReportsAuthenticationFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad token `+testToken+`","code":190}}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	err := c.CheckConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "access token was rejected") {
		t.Fatalf("error = %v", err)
	}
	assertNoToken(t, err.Error())
}

func TestCheckConnectionReportsInaccessibleAccount(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me/permissions") {
			_, _ = io.WriteString(w, `{"data":[{"permission":"ads_management","status":"granted"}]}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"object unavailable","code":200}}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	err := c.CheckConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "insufficient permission") {
		t.Fatalf("error = %v", err)
	}
}
