package googleads_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

type assetFake struct {
	mu sync.Mutex

	assets         map[string]map[string]any
	campaignAssets map[string]map[string]any
	campaigns      map[string]map[string]any

	nextAssetID int64
	creates     int
	uploads     int
	ops         []string

	searchStatus int
	mutateStatus int
	searchBody   string
	lastMutate   string
}

func newAssetFake() *assetFake {
	return &assetFake{
		assets:         map[string]map[string]any{},
		campaignAssets: map[string]map[string]any{},
		campaigns:      map[string]map[string]any{},
		nextAssetID:    80,
	}
}

func (f *assetFake) handler(w http.ResponseWriter, r *http.Request) {
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
		f.lastMutate = string(body)
		if f.mutateStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.mutateStatus)
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		resourceName, kind, err := f.mutateLocked(collection, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.ops = append(f.ops, collection+":"+kind)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"resourceName": resourceName}}})
		return
	}
	http.NotFound(w, r)
}

func (f *assetFake) searchLocked(query string) []any {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "from campaign_asset"):
		return f.searchCampaignAssetsLocked(query)
	case strings.Contains(q, "from asset"):
		return f.searchAssetsLocked(query)
	case strings.Contains(q, "from campaign"):
		return f.searchCampaignsLocked(query)
	default:
		return nil
	}
}

func (f *assetFake) searchAssetsLocked(query string) []any {
	want := queryValue(query, "asset.id = ")
	var out []any
	for id, item := range f.assets {
		if stringify(item["status"]) == "REMOVED" {
			continue
		}
		if want != "" && id != want {
			continue
		}
		out = append(out, map[string]any{"asset": cloneMap(item)})
	}
	return out
}

func (f *assetFake) searchCampaignAssetsLocked(query string) []any {
	var out []any
	for _, item := range f.campaignAssets {
		if stringify(item["status"]) == "REMOVED" && strings.Contains(strings.ToLower(query), "status != ") {
			continue
		}
		campaign := stringify(item["campaign"])
		campaignID := strings.TrimPrefix(campaign, "customers/"+testCustomerID+"/campaigns/")
		asset := stringify(item["asset"])
		fieldType := stringify(item["fieldType"])
		if strings.Contains(query, "campaign.id = ") {
			want := queryValue(query, "campaign.id = ")
			if want != "" && want != campaignID {
				continue
			}
		}
		if strings.Contains(query, "campaign_asset.asset = ") {
			want := gaqlQuoted(query, "campaign_asset.asset = ")
			if want != "" && !strings.EqualFold(want, asset) {
				continue
			}
		}
		if strings.Contains(query, "campaign_asset.field_type = ") {
			want := gaqlQuoted(query, "campaign_asset.field_type = ")
			if want != "" && !strings.EqualFold(want, fieldType) {
				continue
			}
		}
		out = append(out, map[string]any{"campaignAsset": cloneMap(item)})
	}
	return out
}

func (f *assetFake) searchCampaignsLocked(query string) []any {
	want := queryValue(query, "campaign.id = ")
	var out []any
	for id, item := range f.campaigns {
		if want != "" && id != want {
			continue
		}
		out = append(out, map[string]any{"campaign": cloneMap(item)})
	}
	return out
}

func (f *assetFake) mutateLocked(collection string, body []byte) (string, string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if raw, ok := op["create"]; ok {
		item, _ := raw.(map[string]any)
		switch collection {
		case "assets":
			return f.createAssetLocked(item, body)
		case "campaignAssets":
			return f.createCampaignAssetLocked(item)
		default:
			return "", "", errors.New("unknown collection")
		}
	}
	if raw, ok := op["update"]; ok {
		item, _ := raw.(map[string]any)
		switch collection {
		case "assets":
			return f.updateAssetLocked(item)
		case "campaignAssets":
			return f.updateCampaignAssetLocked(item)
		default:
			return "", "", errors.New("unknown collection")
		}
	}
	if raw, ok := op["remove"]; ok {
		resourceName := stringify(raw)
		switch collection {
		case "assets":
			id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/assets/")
			item, ok := f.assets[id]
			if !ok {
				return "", "", errors.New("missing asset")
			}
			item["status"] = "REMOVED"
		case "campaignAssets":
			id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignAssets/")
			item, ok := f.campaignAssets[id]
			if !ok {
				return "", "", errors.New("missing campaign asset")
			}
			item["status"] = "REMOVED"
		default:
			return "", "", errors.New("unknown collection")
		}
		return resourceName, "remove", nil
	}
	return "", "", errors.New("unsupported mutate")
}

func (f *assetFake) createAssetLocked(item map[string]any, raw []byte) (string, string, error) {
	f.creates++
	created := cloneMap(item)
	if _, ok := created["imageAsset"]; ok {
		f.uploads++
		if imageAsset, ok := created["imageAsset"].(map[string]any); ok {
			delete(imageAsset, "data")
		}
	}
	if !strings.Contains(string(raw), `"data"`) && created["type"] == "IMAGE" {
		return "", "", errors.New("image create missing data")
	}
	f.nextAssetID++
	id := strconv.FormatInt(f.nextAssetID, 10)
	created["id"] = id
	created["resourceName"] = "customers/" + testCustomerID + "/assets/" + id
	if stringify(created["source"]) == "" {
		created["source"] = "ADVERTISER"
	}
	f.assets[id] = created
	return stringify(created["resourceName"]), "create", nil
}

func (f *assetFake) updateAssetLocked(item map[string]any) (string, string, error) {
	resourceName := stringify(item["resourceName"])
	id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/assets/")
	existing, ok := f.assets[id]
	if !ok {
		return "", "", errors.New("missing asset")
	}
	if name := stringify(item["name"]); name != "" {
		existing["name"] = name
	}
	if _, ok := item["imageAsset"]; ok {
		return "", "", errors.New("unexpected image update")
	}
	if _, ok := item["textAsset"]; ok {
		return "", "", errors.New("unexpected text update")
	}
	if sitelink, ok := item["sitelinkAsset"].(map[string]any); ok {
		current, _ := existing["sitelinkAsset"].(map[string]any)
		if current == nil {
			current = map[string]any{}
		}
		for key, value := range sitelink {
			current[key] = value
		}
		existing["sitelinkAsset"] = current
	}
	if callout, ok := item["calloutAsset"].(map[string]any); ok {
		current, _ := existing["calloutAsset"].(map[string]any)
		if current == nil {
			current = map[string]any{}
		}
		for key, value := range callout {
			current[key] = value
		}
		existing["calloutAsset"] = current
	}
	if urls, ok := item["finalUrls"]; ok {
		existing["finalUrls"] = urls
	}
	return resourceName, "update", nil
}

func (f *assetFake) createCampaignAssetLocked(item map[string]any) (string, string, error) {
	created := cloneMap(item)
	campaign := stringify(created["campaign"])
	assetName := stringify(created["asset"])
	fieldType := stringify(created["fieldType"])
	campaignID := strings.TrimPrefix(campaign, "customers/"+testCustomerID+"/campaigns/")
	assetID := strings.TrimPrefix(assetName, "customers/"+testCustomerID+"/assets/")
	id := campaignID + "~" + assetID + "~" + fieldType
	created["resourceName"] = "customers/" + testCustomerID + "/campaignAssets/" + id
	if stringify(created["status"]) == "" {
		created["status"] = "ENABLED"
	}
	f.campaignAssets[id] = created
	return stringify(created["resourceName"]), "create", nil
}

func (f *assetFake) updateCampaignAssetLocked(item map[string]any) (string, string, error) {
	resourceName := stringify(item["resourceName"])
	id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignAssets/")
	existing, ok := f.campaignAssets[id]
	if !ok {
		return "", "", errors.New("missing campaign asset")
	}
	if status := stringify(item["status"]); status != "" {
		existing["status"] = status
	}
	return resourceName, "update", nil
}

func (f *assetFake) seedAsset(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/assets/" + id
	if stringify(item["source"]) == "" {
		item["source"] = "ADVERTISER"
	}
	f.assets[id] = item
}

func (f *assetFake) seedCampaignAsset(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	campaignID := strings.TrimPrefix(stringify(item["campaign"]), "customers/"+testCustomerID+"/campaigns/")
	assetID := strings.TrimPrefix(stringify(item["asset"]), "customers/"+testCustomerID+"/assets/")
	fieldType := stringify(item["fieldType"])
	id := campaignID + "~" + assetID + "~" + fieldType
	item["resourceName"] = "customers/" + testCustomerID + "/campaignAssets/" + id
	if stringify(item["status"]) == "" {
		item["status"] = "ENABLED"
	}
	f.campaignAssets[id] = item
}

func (f *assetFake) seedCampaign(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := stringify(item["id"])
	item["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	f.campaigns[id] = item
}

func (f *assetFake) uploadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads
}

func (f *assetFake) lastMutateBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastMutate
}

func testAssetProvider(t *testing.T, fake *assetFake) *googleads.Provider {
	t.Helper()
	if fake == nil {
		fake = newAssetFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	return googleads.NewWithHTTPClient(cfg, srv.Client())
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 80, B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func localPNG(t *testing.T, path string, width, height int) resource.LocalAsset {
	t.Helper()
	data := testPNG(t, width, height)
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	payload := data
	return resource.NewLocalAsset(path, digest, int64(len(payload)), "image/png", func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	})
}

func imageAssetResource(t *testing.T, name string, local resource.LocalAsset) resource.Resource {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	res := resource.Resource{
		Address: addr,
		Attributes: resource.Attributes{
			googleads.AttrType: "IMAGE",
			asset.AttrName:     map[string]any{asset.AttrFile: local.Path},
		},
	}
	res.LocalAsset = &local
	return res
}

func textAssetResource(t *testing.T, name, text string) resource.Resource {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return resource.Resource{
		Address: addr,
		Attributes: resource.Attributes{
			googleads.AttrType: "TEXT",
			googleads.AttrText: text,
		},
	}
}

func campaignAssetResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return resource.Resource{Address: addr, Attributes: attrs}
}

func mustAssetAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustCampaignAssetAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func assetRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustAssetAddress(t, name)}
}

func resolvedCampaign(t *testing.T, name, id string) resource.Resolved {
	t.Helper()
	return resource.Resolved{Address: mustCampaignAddress(t, name), Identity: resource.Identity{ID: id}}
}

func resolvedAsset(t *testing.T, name, id string) resource.Resolved {
	t.Helper()
	return resource.Resolved{Address: mustAssetAddress(t, name), Identity: resource.Identity{ID: id}}
}

func defaultCampaignAssetAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign:  campaignRef(t, "brand"),
		googleads.AttrAsset:     assetRef(t, "product_image"),
		googleads.AttrFieldType: "AD_IMAGE",
	}
}

func sampleImageAsset(id, name string) map[string]any {
	return map[string]any{
		"id":   id,
		"name": name,
		"type": "IMAGE",
		"imageAsset": map[string]any{
			"fileSize": "1200",
			"mimeType": "IMAGE_PNG",
			"fullSize": map[string]any{"widthPixels": "128", "heightPixels": "128"},
		},
	}
}

func sampleTextAsset(id, text string) map[string]any {
	return map[string]any{
		"id":        id,
		"type":      "TEXT",
		"textAsset": map[string]any{"text": text},
	}
}

func sitelinkAssetResource(t *testing.T, name, linkText string, urls []any) resource.Resource {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return resource.Resource{
		Address: addr,
		Attributes: resource.Attributes{
			googleads.AttrType:      "SITELINK",
			googleads.AttrLinkText:  linkText,
			googleads.AttrFinalUrls: urls,
		},
	}
}

func calloutAssetResource(t *testing.T, name, text string) resource.Resource {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.asset." + name)
	if err != nil {
		t.Fatal(err)
	}
	return resource.Resource{
		Address: addr,
		Attributes: resource.Attributes{
			googleads.AttrType:        "CALLOUT",
			googleads.AttrCalloutText: text,
		},
	}
}

func sampleSitelinkAsset(id, linkText string, urls []any) map[string]any {
	return map[string]any{
		"id":        id,
		"type":      "SITELINK",
		"finalUrls": urls,
		"sitelinkAsset": map[string]any{
			"linkText": linkText,
		},
	}
}

func sampleCalloutAsset(id, text string) map[string]any {
	return map[string]any{
		"id":   id,
		"type": "CALLOUT",
		"calloutAsset": map[string]any{
			"calloutText": text,
		},
	}
}
