package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

const (
	cliGoogleAdsCustomerID     = "1234567890"
	cliGoogleAdsDeveloperToken = "cli-test-developer-token"
	cliGoogleAdsClientSecret   = "cli-test-client-secret"
	cliGoogleAdsRefreshToken   = "cli-test-refresh-token"
	cliGoogleAdsAccessToken    = "cli-test-access-token"
)

func TestImportGoogleAdsConversionActionThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":             "12",
		"name":           "Trial Started",
		"category":       "SIGNUP",
		"status":         "ENABLED",
		"type":           "WEBPAGE",
		"origin":         "WEBSITE",
		"countingType":   "ONE_PER_CLICK",
		"primaryForGoal": true,
		"valueSettings": map[string]any{
			"defaultValue":          0.0,
			"defaultCurrencyCode":   "USD",
			"alwaysUseDefaultValue": true,
		},
		"tagSnippets": []any{
			map[string]any{"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-1/" + cliGoogleAdsAccessToken + "'});"},
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.conversion_action.trial_started (remote identity 12).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}

	yamlText := extractYAML(out)
	parsed, err := manifest.Parse([]byte(yamlText), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, yamlText)
	}
	if parsed.Resources[0].Attributes["name"] != "Trial Started" {
		t.Fatalf("imported name = %v", parsed.Resources[0].Attributes["name"])
	}
	for _, key := range []string{"id", "resourceName", "type", "tagSnippets", "conversionId", "conversionLabel"} {
		if _, ok := parsed.Resources[0].Attributes[key]; ok {
			t.Fatalf("computed %s present in generated attributes:\n%s", key, yamlText)
		}
	}
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.conversion_action.trial_started", "12")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("conversion action import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionByResourceName(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "12",
		"name":     "Trial Started",
		"category": "SIGNUP",
		"type":     "WEBPAGE",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{
		"import", "-f", manifestPath, "googleads.conversion_action.trial_started",
		"customers/" + cliGoogleAdsCustomerID + "/conversionActions/12",
	}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "(remote identity 12)") {
		t.Fatalf("stdout should report canonical numeric identity:\n%s", stdout.String())
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.conversion_action.trial_started", "12")
	if srv.mutateCount() != 0 {
		t.Fatalf("resource-name import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsCustomerConversionGoalThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.customer_conversion_goal.signup", "signup~website"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.customer_conversion_goal.signup (remote identity SIGNUP~WEBSITE).") {
		t.Fatalf("stdout missing canonical identity confirmation:\n%s", out)
	}
	if strings.Contains(out, "conversionAction") || strings.Contains(out, "$ref") {
		t.Fatalf("goal import reconstructed conversionAction:\n%s", out)
	}

	yamlText := extractYAML(out)
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.customer_conversion_goal.signup", "SIGNUP~WEBSITE")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("conversion goal import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionUnsupportedType(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "3",
		"name":     "App Install",
		"category": "DOWNLOAD",
		"type":     "GOOGLE_PLAY_DOWNLOAD",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.app_install", "3"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "WEBPAGE") {
		t.Fatalf("stderr = %q, want WEBPAGE guidance", stderr.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("unsupported import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "abc"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not a valid Google Ads conversion action id") {
		t.Fatalf("stderr = %q, want invalid id diagnostic", stderr.String())
	}
}

func TestImportGoogleAdsConversionActionNotFound(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "was not found") {
		t.Fatalf("stderr = %q, want not found", stderr.String())
	}
}

func TestImportGoogleAdsConversionActionAPIError(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.searchStatus = http.StatusForbidden
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if strings.Contains(stderr.String(), cliGoogleAdsAccessToken) || strings.Contains(stderr.String(), cliGoogleAdsDeveloperToken) {
		t.Fatalf("API error leaked secret:\n%s", stderr.String())
	}
}

func TestImportGoogleAdsYAMLIsDeterministic(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "12",
		"name":     "Trial Started",
		"category": "SIGNUP",
		"type":     "WEBPAGE",
		"status":   "ENABLED",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	first := importGoogleAdsYAML(t, p, "googleads.conversion_action.trial_started", "12")
	second := importGoogleAdsYAML(t, p, "googleads.conversion_action.trial_started", "12")
	if first != second {
		t.Fatalf("YAML differed:\n%s\n---\n%s", first, second)
	}
}

func importGoogleAdsYAML(t *testing.T, p *googleads.Provider, address, remoteID string) string {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, address, remoteID}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	return extractYAML(stdout.String())
}

func assertGoogleAdsImportOutputClean(t *testing.T, out string) {
	t.Helper()
	for _, secret := range []string{cliGoogleAdsDeveloperToken, cliGoogleAdsClientSecret, cliGoogleAdsRefreshToken, cliGoogleAdsAccessToken} {
		if strings.Contains(out, secret) {
			t.Fatalf("import output leaked secret %q:\n%s", secret, out)
		}
	}
}

func googleAdsImportProvider(t *testing.T) (*googleads.Provider, *cliGoogleAdsFake) {
	t.Helper()
	srv := newCLIGoogleAdsServer(t)
	p := googleads.NewWithHTTPClient(googleads.Config{
		DeveloperToken: cliGoogleAdsDeveloperToken,
		ClientID:       "cli-test-client-id",
		ClientSecret:   cliGoogleAdsClientSecret,
		RefreshToken:   cliGoogleAdsRefreshToken,
		CustomerID:     cliGoogleAdsCustomerID,
		BaseURL:        srv.URL,
		TokenURL:       srv.URL + "/oauth/token",
		HTTPClient:     srv.Client(),
	}, srv.Client())
	return p, srv.fake
}

type cliGoogleAdsServer struct {
	*httptest.Server
	fake *cliGoogleAdsFake
}

func newCLIGoogleAdsServer(t *testing.T) *cliGoogleAdsServer {
	t.Helper()
	fake := &cliGoogleAdsFake{
		actions: map[string]map[string]any{},
		goals:   map[string]map[string]any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	return &cliGoogleAdsServer{Server: srv, fake: fake}
}

type cliGoogleAdsFake struct {
	mu           sync.Mutex
	actions      map[string]map[string]any
	goals        map[string]map[string]any
	searchStatus int
	mutates      int
}

func (f *cliGoogleAdsFake) seedAction(action map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(action)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/conversionActions/" + id
	}
	f.actions[id] = cloned
}

func (f *cliGoogleAdsFake) seedGoal(goal map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(goal)
	id := strings.ToUpper(stringifyAny(cloned["category"])) + "~" + strings.ToUpper(stringifyAny(cloned["origin"]))
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/customerConversionGoals/" + id
	}
	f.goals[id] = cloned
}

func (f *cliGoogleAdsFake) mutateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mutates
}

func (f *cliGoogleAdsFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)

	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+cliGoogleAdsAccessToken+`","expires_in":3600,"token_type":"Bearer"}`)
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		f.mutates++
		http.Error(w, `{"error":{"message":"unexpected mutate `+cliGoogleAdsDeveloperToken+`"}}`, http.StatusBadRequest)
		return
	}
	if !strings.Contains(r.URL.Path, "/googleAds:search") {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(body, &req)
	query := strings.ToLower(req.Query)
	w.Header().Set("Content-Type", "application/json")
	if f.searchStatus >= 400 && !strings.Contains(query, "from customer") {
		w.WriteHeader(f.searchStatus)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"query failed `+cliGoogleAdsAccessToken+`","status":"PERMISSION_DENIED"}}`)
		return
	}
	if strings.Contains(query, "from customer_conversion_goal") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchGoalsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from customer") {
		_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+cliGoogleAdsCustomerID+`"}}]}`)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchActionsLocked(req.Query)})
}

func (f *cliGoogleAdsFake) searchActionsLocked(query string) []any {
	var out []any
	for id, action := range f.actions {
		if strings.Contains(query, "conversion_action.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "conversion_action.id = ")+len("conversion_action.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			if want != id {
				continue
			}
		}
		out = append(out, map[string]any{"conversionAction": cloneAnyMap(action)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchGoalsLocked(query string) []any {
	var out []any
	for id, goal := range f.goals {
		if strings.Contains(query, "customer_conversion_goal.category = ") || strings.Contains(query, "customer_conversion_goal.origin = ") {
			if !strings.Contains(strings.ToUpper(query), strings.Split(id, "~")[0]) {
				continue
			}
		}
		out = append(out, map[string]any{"customerConversionGoal": cloneAnyMap(goal)})
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringifyAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}
