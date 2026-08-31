package googleads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestDestroyRemovesEachRemovableType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		collection string
		res        func(*testing.T) resource.Resource
		seed       func(*destroyFake)
	}{
		{
			name:       "conversion_action",
			collection: "conversionActions",
			res: func(t *testing.T) resource.Resource {
				res := conversionActionResource(t, "trial_started", resource.Attributes{
					googleads.AttrName:     "Trial Started",
					googleads.AttrCategory: "SIGNUP",
				})
				res.Identity = resource.Identity{ID: "31"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedConversionAction(map[string]any{"id": "31", "name": "Trial Started", "status": "ENABLED", "category": "SIGNUP"})
			},
		},
		{
			name:       "campaign_budget",
			collection: "campaignBudgets",
			res: func(t *testing.T) resource.Resource {
				res := campaignBudgetResource(t, "brand", resource.Attributes{
					googleads.AttrName:             "Brand daily budget",
					googleads.AttrAmount:           50,
					googleads.AttrExplicitlyShared: false,
				})
				res.Identity = resource.Identity{ID: "11"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "status": "ENABLED", "referenceCount": 0, "amountMicros": "50000000", "explicitlyShared": false})
			},
		},
		{
			name:       "campaign",
			collection: "campaigns",
			res: func(t *testing.T) resource.Resource {
				res := campaignResource(t, "brand", defaultCampaignAttrs(t))
				res.Identity = resource.Identity{ID: "21"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "PAUSED", "campaignBudget": "customers/" + testCustomerID + "/campaignBudgets/11"})
			},
		},
		{
			name:       "ad_group",
			collection: "adGroups",
			res: func(t *testing.T) resource.Resource {
				res := adGroupResource(t, "brand", defaultAdGroupAttrs(t))
				res.Identity = resource.Identity{ID: "41"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedAdGroup(map[string]any{"id": "41", "name": "Brand", "status": "PAUSED", "campaign": "customers/" + testCustomerID + "/campaigns/21"})
			},
		},
		{
			name:       "keyword",
			collection: "adGroupCriteria",
			res: func(t *testing.T) resource.Resource {
				res := keywordResource(t, "brand_exact", resource.Attributes{
					googleads.AttrAdGroup:   adGroupRef(t, "brand"),
					googleads.AttrText:      "brand",
					googleads.AttrMatchType: "EXACT",
				})
				res.Identity = resource.Identity{ID: "41~51"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedKeyword(map[string]any{"criterionId": "51", "adGroup": "customers/" + testCustomerID + "/adGroups/41", "status": "PAUSED", "type": "KEYWORD", "keyword": map[string]any{"text": "brand", "matchType": "EXACT"}})
			},
		},
		{
			name:       "negative_keyword",
			collection: "adGroupCriteria",
			res: func(t *testing.T) resource.Resource {
				res := keywordResource(t, "jobs_neg", resource.Attributes{
					googleads.AttrAdGroup:   adGroupRef(t, "brand"),
					googleads.AttrText:      "jobs",
					googleads.AttrMatchType: "PHRASE",
					googleads.AttrNegative:  true,
				})
				res.Identity = resource.Identity{ID: "41~52"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedKeyword(map[string]any{"criterionId": "52", "adGroup": "customers/" + testCustomerID + "/adGroups/41", "status": "ENABLED", "negative": true, "type": "KEYWORD", "keyword": map[string]any{"text": "jobs", "matchType": "PHRASE"}})
			},
		},
		{
			name:       "responsive_search_ad",
			collection: "adGroupAds",
			res: func(t *testing.T) resource.Resource {
				res := rsaResource(t, "brand", defaultRSAAttrs(t))
				res.Identity = resource.Identity{ID: "41~61"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedRSA(map[string]any{"adGroup": "customers/" + testCustomerID + "/adGroups/41", "status": "PAUSED", "ad": map[string]any{"id": "61", "type": "RESPONSIVE_SEARCH_AD"}})
			},
		},
		{
			name:       "campaign_location",
			collection: "campaignCriteria",
			res: func(t *testing.T) resource.Resource {
				res := campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t))
				res.Identity = resource.Identity{ID: "21~71"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedCriterion(map[string]any{"criterionId": "71", "campaign": "customers/" + testCustomerID + "/campaigns/21", "status": "ENABLED", "type": "LOCATION", "location": map[string]any{"geoTargetConstant": "geoTargetConstants/2840"}})
			},
		},
		{
			name:       "campaign_language",
			collection: "campaignCriteria",
			res: func(t *testing.T) resource.Resource {
				res := campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t))
				res.Identity = resource.Identity{ID: "21~72"}
				return res
			},
			seed: func(f *destroyFake) {
				f.seedCriterion(map[string]any{"criterionId": "72", "campaign": "customers/" + testCustomerID + "/campaigns/21", "status": "ENABLED", "type": "LANGUAGE", "language": map[string]any{"languageConstant": "languageConstants/1000"}})
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newDestroyFake()
			tc.seed(fake)
			p := testDestroyProvider(t, fake)
			res := tc.res(t)

			result, err := p.Destroy(context.Background(), res)
			if err != nil {
				t.Fatalf("Destroy: %v", err)
			}
			if result.Status != provider.DestroyStatusRemoved {
				t.Fatalf("status = %q, want removed", result.Status)
			}
			ops := fake.operations()
			if len(ops) != 1 || ops[0].collection != tc.collection || ops[0].kind != "remove" {
				t.Fatalf("operations = %+v, want one %s remove", ops, tc.collection)
			}
			assertRemoveOnlyMutate(t, ops[0].raw)

			again, err := p.Destroy(context.Background(), res)
			if err != nil {
				t.Fatalf("Destroy already terminal: %v", err)
			}
			if again.Status != provider.DestroyStatusAlreadyAbsent {
				t.Fatalf("status = %q, want already-absent", again.Status)
			}
			if got := fake.operations(); len(got) != 1 {
				t.Fatalf("already-terminal issued another mutate: %+v", got)
			}
		})
	}
}

func TestDestroyAlreadyAbsentAndAlreadyRemoved(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		result, err := p.Destroy(context.Background(), res)
		if err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		if result.Status != provider.DestroyStatusAlreadyAbsent {
			t.Fatalf("status = %q, want already-absent", result.Status)
		}
		if len(fake.operations()) != 0 {
			t.Fatalf("missing resource issued mutate: %+v", fake.operations())
		}
	})

	t.Run("removed", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "REMOVED"})
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		result, err := p.Destroy(context.Background(), res)
		if err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		if result.Status != provider.DestroyStatusAlreadyAbsent {
			t.Fatalf("status = %q, want already-absent for REMOVED", result.Status)
		}
		if len(fake.operations()) != 0 {
			t.Fatalf("REMOVED resource issued mutate: %+v", fake.operations())
		}
	})

	t.Run("paused is not terminal", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "PAUSED"})
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		result, err := p.Destroy(context.Background(), res)
		if err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		if result.Status != provider.DestroyStatusRemoved {
			t.Fatalf("paused status = %q, want removed", result.Status)
		}
	})

	t.Run("enabled is removed not enabled", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "ENABLED"})
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		result, err := p.Destroy(context.Background(), res)
		if err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		if result.Status != provider.DestroyStatusRemoved {
			t.Fatalf("enabled status = %q, want removed", result.Status)
		}
		ops := fake.operations()
		if len(ops) != 1 {
			t.Fatalf("operations = %+v", ops)
		}
		assertRemoveOnlyMutate(t, ops[0].raw)
		if stringify(fake.campaign("21")["status"]) != "REMOVED" {
			t.Fatalf("remote status = %v, want REMOVED", fake.campaign("21")["status"])
		}
	})

	t.Run("hidden conversion action is not terminal", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedConversionAction(map[string]any{"id": "31", "name": "Trial Started", "status": "HIDDEN"})
		p := testDestroyProvider(t, fake)
		res := conversionActionResource(t, "trial_started", resource.Attributes{
			googleads.AttrName:     "Trial Started",
			googleads.AttrCategory: "SIGNUP",
		})
		res.Identity = resource.Identity{ID: "31"}
		result, err := p.Destroy(context.Background(), res)
		if err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		if result.Status != provider.DestroyStatusRemoved {
			t.Fatalf("hidden status = %q, want removed", result.Status)
		}
	})
}

func TestDestroyBudgetStillReferenced(t *testing.T) {
	t.Parallel()

	fake := newDestroyFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "status": "ENABLED", "referenceCount": 1, "amountMicros": "50000000"})
	p := testDestroyProvider(t, fake)
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	res.Identity = resource.Identity{ID: "11"}

	_, err := p.Destroy(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "referenced") {
		t.Fatalf("Destroy = %v, want referenced-budget refusal", err)
	}
	if len(fake.operations()) != 0 {
		t.Fatalf("referenced budget issued mutate: %+v", fake.operations())
	}
}

func TestDestroyMissingIdentity(t *testing.T) {
	t.Parallel()

	p := testDestroyProvider(t, newDestroyFake())
	_, err := p.Destroy(context.Background(), campaignResource(t, "brand", defaultCampaignAttrs(t)))
	if err == nil || !strings.Contains(err.Error(), "missing identity") {
		t.Fatalf("Destroy = %v, want missing identity", err)
	}
}

func TestDestroyAuthAndMalformedAreNotAlreadyAbsent(t *testing.T) {
	t.Parallel()

	t.Run("auth", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "PAUSED"})
		fake.searchStatus = http.StatusForbidden
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		_, err := p.Destroy(context.Background(), res)
		if err == nil {
			t.Fatal("Destroy succeeded, want auth error")
		}
		assertNoProviderSecret(t, err.Error())
		if strings.Contains(strings.ToLower(err.Error()), "already") {
			t.Fatalf("auth error treated as already-absent: %v", err)
		}
		if len(fake.operations()) != 0 {
			t.Fatalf("auth error issued mutate: %+v", fake.operations())
		}
	})

	t.Run("malformed", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.searchBody = `{"results":[{"campaign":{"status":"PAUSED"}}]}`
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		_, err := p.Destroy(context.Background(), res)
		if err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Fatalf("Destroy = %v, want malformed inspect error", err)
		}
		assertNoProviderSecret(t, err.Error())
		if strings.Contains(strings.ToLower(err.Error()), "already") {
			t.Fatalf("malformed error treated as already-absent: %v", err)
		}
	})

	t.Run("mutate secret redaction", func(t *testing.T) {
		t.Parallel()
		fake := newDestroyFake()
		fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "PAUSED"})
		fake.mutateStatus = http.StatusBadRequest
		p := testDestroyProvider(t, fake)
		res := campaignResource(t, "brand", defaultCampaignAttrs(t))
		res.Identity = resource.Identity{ID: "21"}
		_, err := p.Destroy(context.Background(), res)
		if err == nil {
			t.Fatal("Destroy succeeded, want mutate error")
		}
		assertNoProviderSecret(t, err.Error())
	})
}

func TestDestroyRunReverseOrderProviderOwnedAndRetry(t *testing.T) {
	t.Parallel()

	fake := newDestroyFake()
	seedSearchGraph(fake)
	p := testDestroyProvider(t, fake)
	desired := searchDestroyGraph(t)
	st := mustGoogleAdsImportStore(t)
	bindDestroyGraph(t, st, searchDestroyIDs())

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupDestroy(p), st, &out, nil)
	if err == nil {
		t.Fatal("Run succeeded, want remaining provider-owned error")
	}
	var remaining *destroy.RemainingError
	if !errors.As(err, &remaining) || remaining == nil {
		t.Fatalf("Run err = %v, want RemainingError", err)
	}
	if result.Removed != 8 || result.Remaining != 2 {
		t.Fatalf("result = %+v, want 8 removed and 2 remaining", result)
	}

	ops := fake.operations()
	got := collections(ops)
	assertBefore(t, got, "adGroupAds", "adGroups")
	assertBefore(t, got, "adGroupCriteria", "adGroups")
	assertBefore(t, got, "campaignCriteria", "campaigns")
	assertBefore(t, got, "adGroups", "campaigns")
	assertBefore(t, got, "campaigns", "campaignBudgets")
	if len(got) != 8 {
		t.Fatalf("collections = %v, want 8 removable mutations", got)
	}
	for _, op := range ops {
		if op.collection == "customerConversionGoals" || op.collection == "campaignConversionGoals" {
			t.Fatalf("provider-owned mutate issued: %+v", op)
		}
		assertRemoveOnlyMutate(t, op.raw)
	}

	for _, addr := range []string{
		"googleads.responsive_search_ad.brand",
		"googleads.keyword.brand_exact",
		"googleads.campaign_location.united_states",
		"googleads.campaign_language.english",
		"googleads.ad_group.brand",
		"googleads.campaign.brand",
		"googleads.conversion_action.trial_started",
		"googleads.campaign_budget.brand",
	} {
		assertStateMissing(t, st, addr)
	}
	for _, addr := range []string{
		"googleads.customer_conversion_goal.signup",
		"googleads.campaign_conversion_goal.trial_signup",
	} {
		assertStatePresent(t, st, addr)
	}
	if !strings.Contains(out.String(), "provider-owned") {
		t.Fatalf("plan output missing provider-owned remnants:\n%s", out.String())
	}
}

func TestDestroyRunPartialFailurePreservesRetryableState(t *testing.T) {
	t.Parallel()

	fake := newDestroyFake()
	seedSearchGraph(fake)
	fake.failCollection = "campaigns"
	p := testDestroyProvider(t, fake)
	desired := searchDestroyGraph(t)
	st := mustGoogleAdsImportStore(t)
	bindDestroyGraph(t, st, searchDestroyIDs())

	var out bytes.Buffer
	_, err := destroy.Run(context.Background(), desired, lookupDestroy(p), st, &out, nil)
	if err == nil {
		t.Fatal("Run succeeded, want campaign mutate failure")
	}
	assertNoProviderSecret(t, err.Error())

	for _, addr := range []string{
		"googleads.responsive_search_ad.brand",
		"googleads.keyword.brand_exact",
		"googleads.campaign_location.united_states",
		"googleads.campaign_language.english",
		"googleads.ad_group.brand",
		"googleads.conversion_action.trial_started",
	} {
		assertStateMissing(t, st, addr)
	}
	for _, addr := range []string{
		"googleads.campaign.brand",
		"googleads.campaign_budget.brand",
		"googleads.customer_conversion_goal.signup",
		"googleads.campaign_conversion_goal.trial_signup",
	} {
		assertStatePresent(t, st, addr)
	}

	fake.failCollection = ""
	out.Reset()
	result, err := destroy.Run(context.Background(), desired, lookupDestroy(p), st, &out, nil)
	if err == nil {
		t.Fatal("retry succeeded, want remaining provider-owned error")
	}
	if result.Removed != 2 || result.Remaining != 2 {
		t.Fatalf("retry result = %+v, want 2 removed (campaign, budget) and 2 remaining", result)
	}
	assertStateMissing(t, st, "googleads.campaign.brand")
	assertStateMissing(t, st, "googleads.campaign_budget.brand")
	assertStateMissing(t, st, "googleads.conversion_action.trial_started")
	assertStatePresent(t, st, "googleads.customer_conversion_goal.signup")
	assertStatePresent(t, st, "googleads.campaign_conversion_goal.trial_signup")
}

func TestDestroyProviderOwnedDestroyIsNotCalledByPlan(t *testing.T) {
	t.Parallel()

	fake := newDestroyFake()
	fake.seedCustomerGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p := testDestroyProvider(t, fake)
	desired := []resource.Resource{
		customerConversionGoalResource(t, "signup", resource.Attributes{
			googleads.AttrCategory: "SIGNUP",
			googleads.AttrOrigin:   "WEBSITE",
			googleads.AttrBiddable: true,
		}),
	}
	st := mustGoogleAdsImportStore(t)
	bindDestroyGraph(t, st, map[string]string{"googleads.customer_conversion_goal.signup": "SIGNUP~WEBSITE"})

	result, err := destroy.Run(context.Background(), desired, lookupDestroy(p), st, io.Discard, nil)
	if err == nil {
		t.Fatal("Run succeeded, want remaining error")
	}
	if result.Removed != 0 || result.Remaining != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(fake.operations()) != 0 {
		t.Fatalf("provider-owned resource was mutated: %+v", fake.operations())
	}
	assertStatePresent(t, st, "googleads.customer_conversion_goal.signup")
}

type mutateOp struct {
	collection string
	kind       string
	raw        string
}

type destroyFake struct {
	mu sync.Mutex

	conversionActions map[string]map[string]any
	customerGoals     map[string]map[string]any
	budgets           map[string]map[string]any
	campaigns         map[string]map[string]any
	campaignGoals     map[string]map[string]any
	adGroups          map[string]map[string]any
	keywords          map[string]map[string]any
	ads               map[string]map[string]any
	criteria          map[string]map[string]any

	searchStatus   int
	searchBody     string
	mutateStatus   int
	failCollection string

	ops []mutateOp
}

func newDestroyFake() *destroyFake {
	return &destroyFake{
		conversionActions: map[string]map[string]any{},
		customerGoals:     map[string]map[string]any{},
		budgets:           map[string]map[string]any{},
		campaigns:         map[string]map[string]any{},
		campaignGoals:     map[string]map[string]any{},
		adGroups:          map[string]map[string]any{},
		keywords:          map[string]map[string]any{},
		ads:               map[string]map[string]any{},
		criteria:          map[string]map[string]any{},
	}
}

func (f *destroyFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		writeToken(w)
		return
	}
	if strings.Contains(r.URL.Path, "googleAds:search") {
		if f.searchStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.searchStatus)
			_, _ = io.WriteString(w, `{"error":{"code":`+strconv.Itoa(f.searchStatus)+`,"message":"query failed `+testAccessToken+`","status":"PERMISSION_DENIED"}}`)
			return
		}
		if f.searchBody != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, f.searchBody)
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		if strings.Contains(strings.ToLower(req.Query), "from customer ") {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"customer": map[string]any{"id": testCustomerID}}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchLocked(req.Query)})
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		collection := mutateCollection(r.URL.Path)
		if f.mutateStatus >= 400 || (f.failCollection != "" && collection == f.failCollection) {
			f.ops = append(f.ops, mutateOp{collection: collection, kind: "remove", raw: string(body)})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		resourceName, kind, err := f.mutateLocked(collection, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.ops = append(f.ops, mutateOp{collection: collection, kind: kind, raw: string(body)})
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"resourceName": resourceName}}})
		return
	}
	http.NotFound(w, r)
}

func (f *destroyFake) searchLocked(query string) []any {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "from conversion_action"):
		return f.match(f.conversionActions, query, "conversion_action.id = ", "id", "conversionAction")
	case strings.Contains(q, "from customer_conversion_goal"):
		return f.matchGoals(f.customerGoals, "customerConversionGoal")
	case strings.Contains(q, "from campaign_budget"):
		return f.match(f.budgets, query, "campaign_budget.id = ", "id", "campaignBudget")
	case strings.Contains(q, "from campaign_conversion_goal"):
		return f.matchGoals(f.campaignGoals, "campaignConversionGoal")
	case strings.Contains(q, "from campaign_criterion"):
		return f.matchComposite(f.criteria, query, "campaign.id = ", "campaign_criterion.criterion_id = ", "campaign", "criterionId", "campaignCriterion")
	case strings.Contains(q, "from ad_group_criterion"):
		return f.matchComposite(f.keywords, query, "ad_group.id = ", "ad_group_criterion.criterion_id = ", "adGroup", "criterionId", "adGroupCriterion")
	case strings.Contains(q, "from ad_group_ad"):
		return f.matchRSA(query)
	case strings.Contains(q, "from ad_group"):
		return f.match(f.adGroups, query, "ad_group.id = ", "id", "adGroup")
	case strings.Contains(q, "from campaign"):
		return f.match(f.campaigns, query, "campaign.id = ", "id", "campaign")
	default:
		return nil
	}
}

func (f *destroyFake) match(items map[string]map[string]any, query, field, idKey, envelope string) []any {
	want := queryValue(query, field)
	var out []any
	for _, item := range items {
		if want != "" && stringify(item[idKey]) != want {
			continue
		}
		out = append(out, map[string]any{envelope: cloneMap(item)})
	}
	return out
}

func (f *destroyFake) matchGoals(items map[string]map[string]any, envelope string) []any {
	var out []any
	for _, item := range items {
		out = append(out, map[string]any{envelope: cloneMap(item)})
	}
	return out
}

func (f *destroyFake) matchComposite(items map[string]map[string]any, query, parentField, childField, parentKey, childKey, envelope string) []any {
	wantParent := queryValue(query, parentField)
	wantChild := queryValue(query, childField)
	var out []any
	for _, item := range items {
		parent := strings.TrimPrefix(stringify(item[parentKey]), "customers/"+testCustomerID+"/"+parentCollection(parentKey)+"/")
		if wantParent != "" && parent != wantParent {
			continue
		}
		if wantChild != "" && stringify(item[childKey]) != wantChild {
			continue
		}
		out = append(out, map[string]any{envelope: cloneMap(item)})
	}
	return out
}

func (f *destroyFake) matchRSA(query string) []any {
	wantGroup := queryValue(query, "ad_group.id = ")
	wantAd := queryValue(query, "ad_group_ad.ad.id = ")
	var out []any
	for _, item := range itemsValues(f.ads) {
		group := strings.TrimPrefix(stringify(item["adGroup"]), "customers/"+testCustomerID+"/adGroups/")
		ad, _ := item["ad"].(map[string]any)
		if wantGroup != "" && group != wantGroup {
			continue
		}
		if wantAd != "" && stringify(ad["id"]) != wantAd {
			continue
		}
		out = append(out, map[string]any{"adGroupAd": cloneMap(item)})
	}
	return out
}

func (f *destroyFake) mutateLocked(collection string, body []byte) (string, string, error) {
	if collection == "customerConversionGoals" || collection == "campaignConversionGoals" {
		return "", "", errors.New("unsupported conversion goal mutate")
	}
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if _, ok := op["create"]; ok {
		return "", "", errors.New("unexpected create")
	}
	if _, ok := op["update"]; ok {
		return "", "", errors.New("unexpected update")
	}
	raw, ok := op["remove"]
	if !ok {
		return "", "", errors.New("unsupported mutate")
	}
	resourceName := stringify(raw)
	if err := f.removeLocked(collection, resourceName); err != nil {
		return "", "", err
	}
	return resourceName, "remove", nil
}

func (f *destroyFake) removeLocked(collection, resourceName string) error {
	markRemoved := func(item map[string]any) {
		item["status"] = "REMOVED"
	}
	switch collection {
	case "conversionActions":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/conversionActions/")
		item, ok := f.conversionActions[id]
		if !ok {
			return errors.New("missing conversion action")
		}
		markRemoved(item)
	case "campaignBudgets":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignBudgets/")
		item, ok := f.budgets[id]
		if !ok {
			return errors.New("missing budget")
		}
		if n := stringify(item["referenceCount"]); n != "" && n != "0" {
			return errors.New("budget still referenced")
		}
		markRemoved(item)
	case "campaigns":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaigns/")
		item, ok := f.campaigns[id]
		if !ok {
			return errors.New("missing campaign")
		}
		markRemoved(item)
		if budgetName := stringify(item["campaignBudget"]); budgetName != "" {
			budgetID := strings.TrimPrefix(budgetName, "customers/"+testCustomerID+"/campaignBudgets/")
			if budget, ok := f.budgets[budgetID]; ok {
				budget["referenceCount"] = 0
			}
		}
	case "adGroups":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroups/")
		item, ok := f.adGroups[id]
		if !ok {
			return errors.New("missing ad group")
		}
		markRemoved(item)
	case "adGroupCriteria":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroupCriteria/")
		item, ok := f.keywords[id]
		if !ok {
			return errors.New("missing keyword")
		}
		markRemoved(item)
	case "adGroupAds":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroupAds/")
		item, ok := f.ads[id]
		if !ok {
			return errors.New("missing ad")
		}
		markRemoved(item)
	case "campaignCriteria":
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignCriteria/")
		item, ok := f.criteria[id]
		if !ok {
			return errors.New("missing criterion")
		}
		markRemoved(item)
	default:
		return errors.New("unknown collection")
	}
	return nil
}

func (f *destroyFake) seedConversionAction(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/conversionActions/" + id
	f.conversionActions[id] = item
}

func (f *destroyFake) seedCustomerGoal(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["category"]) + "~" + stringify(item["origin"])
	item["resourceName"] = "customers/" + testCustomerID + "/customerConversionGoals/" + id
	f.customerGoals[id] = item
}

func (f *destroyFake) seedBudget(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/campaignBudgets/" + id
	f.budgets[id] = item
}

func (f *destroyFake) seedCampaign(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	f.campaigns[id] = item
}

func (f *destroyFake) seedCampaignGoal(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/campaignConversionGoals/" + id
	f.campaignGoals[id] = item
}

func (f *destroyFake) seedAdGroup(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/adGroups/" + id
	f.adGroups[id] = item
}

func (f *destroyFake) seedKeyword(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	group := strings.TrimPrefix(stringify(item["adGroup"]), "customers/"+testCustomerID+"/adGroups/")
	id := group + "~" + stringify(item["criterionId"])
	item["resourceName"] = "customers/" + testCustomerID + "/adGroupCriteria/" + id
	f.keywords[id] = item
}

func (f *destroyFake) seedRSA(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	group := strings.TrimPrefix(stringify(item["adGroup"]), "customers/"+testCustomerID+"/adGroups/")
	ad, _ := item["ad"].(map[string]any)
	id := group + "~" + stringify(ad["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/adGroupAds/" + id
	f.ads[id] = item
}

func (f *destroyFake) seedCriterion(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	campaign := strings.TrimPrefix(stringify(item["campaign"]), "customers/"+testCustomerID+"/campaigns/")
	id := campaign + "~" + stringify(item["criterionId"])
	item["resourceName"] = "customers/" + testCustomerID + "/campaignCriteria/" + id
	f.criteria[id] = item
}

func (f *destroyFake) operations() []mutateOp {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]mutateOp, len(f.ops))
	copy(out, f.ops)
	return out
}

func (f *destroyFake) campaign(id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.campaigns[id]
}

func testDestroyProvider(t *testing.T, fake *destroyFake) *googleads.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	return googleads.NewWithHTTPClient(cfg, srv.Client())
}

func seedSearchGraph(fake *destroyFake) {
	fake.seedConversionAction(map[string]any{"id": "31", "name": "Trial Started", "status": "ENABLED", "category": "SIGNUP", "origin": "WEBSITE"})
	fake.seedCustomerGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "status": "ENABLED", "referenceCount": 1, "amountMicros": "50000000", "explicitlyShared": false})
	fake.seedCampaign(map[string]any{"id": "21", "name": "Brand", "status": "PAUSED", "campaignBudget": "customers/" + testCustomerID + "/campaignBudgets/11"})
	fake.seedCampaignGoal(map[string]any{"id": "21~SIGNUP~WEBSITE", "campaign": "customers/" + testCustomerID + "/campaigns/21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	fake.seedAdGroup(map[string]any{"id": "41", "name": "Brand", "status": "PAUSED", "campaign": "customers/" + testCustomerID + "/campaigns/21"})
	fake.seedKeyword(map[string]any{"criterionId": "51", "adGroup": "customers/" + testCustomerID + "/adGroups/41", "status": "PAUSED", "type": "KEYWORD", "keyword": map[string]any{"text": "brand", "matchType": "EXACT"}})
	fake.seedRSA(map[string]any{"adGroup": "customers/" + testCustomerID + "/adGroups/41", "status": "PAUSED", "ad": map[string]any{"id": "61", "type": "RESPONSIVE_SEARCH_AD"}})
	fake.seedCriterion(map[string]any{"criterionId": "71", "campaign": "customers/" + testCustomerID + "/campaigns/21", "status": "ENABLED", "type": "LOCATION"})
	fake.seedCriterion(map[string]any{"criterionId": "72", "campaign": "customers/" + testCustomerID + "/campaigns/21", "status": "ENABLED", "type": "LANGUAGE"})
}

func searchDestroyGraph(t *testing.T) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		conversionActionResource(t, "trial_started", resource.Attributes{
			googleads.AttrName:     "Trial Started",
			googleads.AttrCategory: "SIGNUP",
		}),
		customerConversionGoalResource(t, "signup", resource.Attributes{
			googleads.AttrCategory:         "SIGNUP",
			googleads.AttrOrigin:           "WEBSITE",
			googleads.AttrBiddable:         true,
			googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
		}),
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		campaignConversionGoalResource(t, "trial_signup", resource.Attributes{
			googleads.AttrCampaign:         campaignRef(t, "brand"),
			googleads.AttrCategory:         "SIGNUP",
			googleads.AttrOrigin:           "WEBSITE",
			googleads.AttrBiddable:         true,
			googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
		}),
		campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t)),
		campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t)),
		adGroupResource(t, "brand", defaultAdGroupAttrs(t)),
		keywordResource(t, "brand_exact", resource.Attributes{
			googleads.AttrAdGroup:   adGroupRef(t, "brand"),
			googleads.AttrText:      "brand",
			googleads.AttrMatchType: "EXACT",
		}),
		rsaResource(t, "brand", defaultRSAAttrs(t)),
	}
}

func searchDestroyIDs() map[string]string {
	return map[string]string{
		"googleads.conversion_action.trial_started":       "31",
		"googleads.customer_conversion_goal.signup":       "SIGNUP~WEBSITE",
		"googleads.campaign_budget.brand":                 "11",
		"googleads.campaign.brand":                        "21",
		"googleads.campaign_conversion_goal.trial_signup": "21~SIGNUP~WEBSITE",
		"googleads.campaign_location.united_states":       "21~71",
		"googleads.campaign_language.english":             "21~72",
		"googleads.ad_group.brand":                        "41",
		"googleads.keyword.brand_exact":                   "41~51",
		"googleads.responsive_search_ad.brand":            "41~61",
	}
}

func bindDestroyGraph(t *testing.T, st *state.Store, ids map[string]string) {
	t.Helper()
	for addr, id := range ids {
		parsed, err := resource.ParseAddress(addr)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Bind(parsed, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
}

func lookupDestroy(p *googleads.Provider) destroy.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func assertRemoveOnlyMutate(t *testing.T, raw string) {
	t.Helper()
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal([]byte(raw), &req); err != nil || len(req.Operations) != 1 {
		t.Fatalf("mutate JSON: %v (%s)", err, raw)
	}
	op := req.Operations[0]
	if _, ok := op["remove"]; !ok {
		t.Fatalf("mutate missing remove: %s", raw)
	}
	if _, ok := op["create"]; ok {
		t.Fatalf("mutate included create: %s", raw)
	}
	if _, ok := op["update"]; ok {
		t.Fatalf("mutate included update: %s", raw)
	}
	if _, ok := op["updateMask"]; ok {
		t.Fatalf("mutate included updateMask: %s", raw)
	}
}

func assertBefore(t *testing.T, collections []string, earlier, later string) {
	t.Helper()
	earlierIdx, laterIdx := -1, -1
	for i, name := range collections {
		if name == earlier && earlierIdx < 0 {
			earlierIdx = i
		}
		if name == later {
			laterIdx = i
		}
	}
	if earlierIdx < 0 || laterIdx < 0 || earlierIdx >= laterIdx {
		t.Fatalf("expected %s before %s in %v", earlier, later, collections)
	}
}

func collections(ops []mutateOp) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.collection)
	}
	return out
}

func assertStateMissing(t *testing.T, st *state.Store, addr string) {
	t.Helper()
	parsed, err := resource.ParseAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.Identity(parsed); err != nil || ok {
		t.Fatalf("%s still in state (ok=%v err=%v)", addr, ok, err)
	}
}

func assertStatePresent(t *testing.T, st *state.Store, addr string) {
	t.Helper()
	parsed, err := resource.ParseAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.Identity(parsed); err != nil || !ok {
		t.Fatalf("%s missing from state (ok=%v err=%v)", addr, ok, err)
	}
}

func queryValue(query, field string) string {
	idx := strings.Index(strings.ToLower(query), strings.ToLower(field))
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(query[idx+len(field):])
	if i := strings.IndexAny(rest, " \n\t"); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(rest, `"'`)
}

func parentCollection(parentKey string) string {
	switch parentKey {
	case "adGroup":
		return "adGroups"
	case "campaign":
		return "campaigns"
	default:
		return parentKey
	}
}

func itemsValues(m map[string]map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(m))
	for _, item := range m {
		out = append(out, item)
	}
	return out
}
