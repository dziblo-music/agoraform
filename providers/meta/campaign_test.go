package meta_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateCampaignAndSafeDefaults(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := campaignResource(t, "acquisition", standardCampaignAttrs())
	if err := p.Validate(context.Background(), res); err != nil { t.Fatal(err) }
	want, _, err := p.NormalizeComparable(res, nil)
	if err != nil { t.Fatal(err) }
	if want[meta.AttrStatus] != "PAUSED" || want[meta.AttrBuyingType] != "AUCTION" { t.Fatalf("defaults = %#v", want) }

	tests := []struct{name string; mutate func(resource.Attributes); contains string}{
		{"legacy objective", func(a resource.Attributes){ a[meta.AttrObjective] = "CONVERSIONS" }, "objective"},
		{"terminal status", func(a resource.Attributes){ a[meta.AttrStatus] = "DELETED" }, "status"},
		{"reserved", func(a resource.Attributes){ a[meta.AttrBuyingType] = "RESERVED" }, "out of scope"},
		{"bad category", func(a resource.Attributes){ a[meta.AttrSpecialAdCategories] = []any{"ALCOHOL"} }, "specialAdCategories"},
		{"duplicate category", func(a resource.Attributes){ a[meta.AttrSpecialAdCategories] = []any{"CREDIT", "credit"} }, "duplicate"},
		{"missing categories", func(a resource.Attributes){ delete(a, meta.AttrSpecialAdCategories) }, "empty list"},
		{"double budget", func(a resource.Attributes){ a[meta.AttrDailyBudget]=1000; a[meta.AttrLifetimeBudget]=5000 }, "mutually exclusive"},
		{"fractional budget", func(a resource.Attributes){ a[meta.AttrDailyBudget]=10.5 }, "smallest unit"},
		{"bid without budget", func(a resource.Attributes){ a[meta.AttrBidStrategy]="COST_CAP" }, "requires"},
	}
	for _, tc := range tests { t.Run(tc.name, func(t *testing.T){ attrs:=standardCampaignAttrs(); tc.mutate(attrs); err:=p.Validate(context.Background(), campaignResource(t,"bad",attrs)); if err==nil || !strings.Contains(err.Error(),tc.contains){ t.Fatalf("error = %v, want %q",err,tc.contains) } }) }
}

func TestCreateReadUpdateCampaign(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t); httpSrv := srv.start(); defer httpSrv.Close(); p := testProvider(t,httpSrv)
	attrs := standardCampaignAttrs(); attrs[meta.AttrDailyBudget]=5000; attrs[meta.AttrBidStrategy]="LOWEST_COST_WITHOUT_CAP"
	created, err := p.Create(context.Background(), campaignResource(t,"acquisition",attrs)); if err != nil { t.Fatal(err) }
	if created.Identity.ID != testCampaignID || created.Attributes[meta.AttrStatus] != "PAUSED" { t.Fatalf("created = %#v",created) }
	bound := campaignResource(t,"acquisition",attrs); bound.Identity=created.Identity
	live, err := p.Read(context.Background(),bound); if err != nil { t.Fatal(err) }
	updatedAttrs:=attrs.Clone(); updatedAttrs[meta.AttrName]="Acquisition 2026"; updatedAttrs[meta.AttrStatus]="ACTIVE"; updatedAttrs[meta.AttrDailyBudget]=6000
	desired:=campaignResource(t,"acquisition",updatedAttrs); desired.Identity=created.Identity
	updated,err:=p.Update(context.Background(),desired,live); if err!=nil{t.Fatal(err)}
	if updated.Attributes[meta.AttrName]!="Acquisition 2026" || updated.Attributes[meta.AttrStatus]!="ACTIVE" || updated.Attributes[meta.AttrDailyBudget]!=int64(6000){t.Fatalf("updated=%#v",updated.Attributes)}
	posts,_:=srv.mutationCounts(); if posts!=2{t.Fatalf("posts=%d, want create+update",posts)}
	if _,err:=p.Update(context.Background(),desired,updated);err!=nil{t.Fatal(err)}
	posts,_=srv.mutationCounts();if posts!=2{t.Fatalf("no-op mutated: posts=%d",posts)}
}

func TestCampaignImmutableChangesFailPlanning(t *testing.T) {
	t.Parallel()
	srv:=newGraphServer(t); srv.seedCampaign(testCampaignID,graphObject{"name":"Acquisition","objective":"OUTCOME_SALES"}); httpSrv:=srv.start(); defer httpSrv.Close(); p:=testProvider(t,httpSrv)
	st,err:=state.Load(filepath.Join(t.TempDir(),"agoraform.state.json"));if err!=nil{t.Fatal(err)};if err:=st.Bind(campaignAddress(t,"acquisition"),resource.Identity{ID:testCampaignID});err!=nil{t.Fatal(err)}
	for _,tc:=range []struct{name string; mutate func(resource.Attributes); contains string}{
		{"objective",func(a resource.Attributes){a[meta.AttrObjective]="OUTCOME_TRAFFIC"},"objective is immutable"},
		{"budget ownership",func(a resource.Attributes){a[meta.AttrDailyBudget]=1000},"budget ownership/type"},
	}{t.Run(tc.name,func(t *testing.T){attrs:=standardCampaignAttrs();tc.mutate(attrs);_,err:=plan.BuildWithState(context.Background(),[]resource.Resource{campaignResource(t,"acquisition",attrs)},func(resource.Address)(provider.Reader,error){return p,nil},st);if err==nil||!strings.Contains(err.Error(),tc.contains){t.Fatalf("plan error=%v",err)}})}
	posts,deletes:=srv.mutationCounts();if posts!=0||deletes!=0{t.Fatalf("plan mutated state posts=%d deletes=%d",posts,deletes)}
}

func TestImportCampaignPreservesActiveStatusAndPlansCleanly(t *testing.T) {
	t.Parallel()
	srv:=newGraphServer(t);srv.seedCampaign(testCampaignID,graphObject{"name":"Existing","objective":"OUTCOME_TRAFFIC","status":"ACTIVE","configured_status":"ACTIVE","effective_status":"ACTIVE","daily_budget":"2500","bid_strategy":"LOWEST_COST_WITHOUT_CAP","special_ad_categories":[]string{"HOUSING"}});httpSrv:=srv.start();defer httpSrv.Close();p:=testProvider(t,httpSrv)
	live,err:=p.Import(context.Background(),campaignAddress(t,"existing"),testCampaignID);if err!=nil{t.Fatal(err)}
	if live.Attributes[meta.AttrStatus]!="ACTIVE"{t.Fatalf("status=%v",live.Attributes[meta.AttrStatus])}
	st,err:=state.Load(filepath.Join(t.TempDir(),"agoraform.state.json"));if err!=nil{t.Fatal(err)};if err:=st.Bind(live.Address,live.Identity);err!=nil{t.Fatal(err)}
	got,err:=plan.BuildWithState(context.Background(),[]resource.Resource{{Address:live.Address,Attributes:live.Attributes.Clone()}},func(resource.Address)(provider.Reader,error){return p,nil},st);if err!=nil{t.Fatal(err)};if got.HasChanges(){t.Fatalf("imported plan changed:\n%s",plan.Format(got))}
	posts,deletes:=srv.mutationCounts();if posts!=0||deletes!=0{t.Fatalf("import mutated posts=%d deletes=%d",posts,deletes)}
}

func TestDestroyCampaignIsIdempotent(t *testing.T) {
	t.Parallel()
	srv:=newGraphServer(t);srv.seedCampaign(testCampaignID,graphObject{"name":"Acquisition","objective":"OUTCOME_SALES"});httpSrv:=srv.start();defer httpSrv.Close();p:=testProvider(t,httpSrv)
	res:=campaignResource(t,"acquisition",standardCampaignAttrs());res.Identity=resource.Identity{ID:testCampaignID}
	got,err:=p.Destroy(context.Background(),res);if err!=nil{t.Fatal(err)};if got.Status!=provider.DestroyStatusRemoved{t.Fatalf("status=%q",got.Status)}
	got,err=p.Destroy(context.Background(),res);if err!=nil{t.Fatal(err)};if got.Status!=provider.DestroyStatusAlreadyAbsent{t.Fatalf("second status=%q",got.Status)}
	if _,err:=p.Read(context.Background(),res);!errors.Is(err,provider.ErrNotFound){t.Fatalf("read after delete=%v",err)}
}

func TestCampaignAPIErrorDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv:=newGraphServer(t);httpSrv:=srv.start();defer httpSrv.Close();p:=testProvider(t,httpSrv)
	res:=campaignResource(t,"missing",standardCampaignAttrs());res.Identity=resource.Identity{ID:"123"}
	_,err:=p.Read(context.Background(),res);if !errors.Is(err,provider.ErrNotFound){t.Fatalf("read=%v",err)};if strings.Contains(err.Error(),testToken){t.Fatalf("token leaked: %v",err)}
}
