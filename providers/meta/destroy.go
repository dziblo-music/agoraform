package meta

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const pixelDestroyGuidance = "Pixel/Dataset event sources are owned in Events Manager / Business Manager; Agoraform imports or adopts them and never deletes the remote object"

// defaultDestroyConfirmDelays spaces the re-reads after an accepted DELETE.
// Meta can keep serving the pre-delete object briefly, so a single read that
// still finds it is not proof the delete failed.
var defaultDestroyConfirmDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// DestroyCapability implements provider.Destroyer.
func (p *Provider) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	spec, ok := Lifecycle(res.Address.Type)
	if !ok {
		return provider.DestroyUnsupported, nil
	}
	return spec.Destroy, nil
}

// Destroy implements provider.Destroyer.
//
// Destroy reads only what deletion needs: whether the object exists, which ad
// account owns it, and whether it is already terminal. It never maps the full
// object into the manifest schema, so provider-added fields cannot block a
// delete.
func (p *Provider) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	spec, ok := Lifecycle(res.Address.Type)
	if !ok {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: unsupported resource type", res.Address)
	}
	if spec.Destroy == provider.DestroyProviderOwned {
		if res.Address.Type == TypeImage {
			// meta.image assets are uploaded by Agoraform but owned by Meta's
			// ad account. Destroying the resource removes only the local state
			// binding; the uploaded image is retained in Meta so existing ad
			// creatives referencing the image hash continue to work.
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %s", res.Address, spec.ServingSafety)
	}
	var target destroyTarget
	switch res.Address.Type {
	case TypeCustomConversion:
		target = destroyTarget{kind: "custom conversion", probe: p.statusProbe("id,account_id,is_archived")}
	case TypeCampaign:
		target = destroyTarget{kind: "campaign", probe: p.statusProbe("id,account_id,status,configured_status", campaignStatusDeleted, campaignStatusArchived)}
	case TypeAdSet:
		target = destroyTarget{kind: "ad set", probe: p.statusProbe("id,account_id,status,configured_status", adSetStatusDeleted, adSetStatusArchived)}
	case TypeAdCreative:
		target = destroyTarget{kind: "ad creative", probe: p.statusProbe("id,account_id,status", adCreativeStatusDeleted)}
	case TypeAd:
		target = destroyTarget{kind: "ad", probe: p.statusProbe("id,account_id,status,configured_status", adStatusDeleted, adStatusArchived)}
	case TypeVideo:
		target = destroyTarget{kind: "video", probe: p.videoProbe}
	default:
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: unsupported resource type", res.Address)
	}
	target.done = provider.DestroyStatusDestroyed
	if spec.Destroy == provider.DestroyRemove {
		target.done = provider.DestroyStatusRemoved
	}
	return p.destroyObject(ctx, res, target)
}

// existenceProbe reports whether a Meta object still exists in a
// non-terminal state. Authentication, permission, malformed-response, and
// wrong-account errors are returned as errors, never as absence.
type existenceProbe func(ctx context.Context, id string) (bool, error)

type destroyTarget struct {
	kind  string
	probe existenceProbe
	done  provider.DestroyStatus
}

func (p *Provider) destroyObject(ctx context.Context, res resource.Resource, target destroyTarget) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}

	exists, err := target.probe(ctx, id)
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if !exists {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}

	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	var result map[string]any
	if err := c.Delete(ctx, id, nil, &result); err != nil {
		if client.IsNotFound(err) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: API did not report success", res.Address)
	}

	delays := p.destroyConfirmDelays
	if delays == nil {
		delays = defaultDestroyConfirmDelays
	}
	sleep := p.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	var waited time.Duration
	for attempt := 0; ; attempt++ {
		exists, err = target.probe(ctx, id)
		if err == nil && !exists {
			return provider.DestroyResult{Status: target.done}, nil
		}
		if attempt == len(delays) {
			break
		}
		if sleepErr := sleep(ctx, delays[attempt]); sleepErr != nil {
			return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE was accepted, but confirmation was interrupted: %w; the state binding was kept, rerun agoraform destroy to confirm", res.Address, sleepErr)
		}
		waited += delays[attempt]
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE was accepted, but confirming %s %s is gone failed: %w; the state binding was kept, rerun agoraform destroy to confirm", res.Address, target.kind, id, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE was accepted, but %s %s was still readable after %d checks over %s; Meta may still be processing the deletion. The state binding was kept; rerun agoraform destroy to confirm it is gone", res.Address, target.kind, id, len(delays)+1, waited)
}

// destroyProbeObject is the minimal Graph API read destroy needs.
type destroyProbeObject struct {
	ID               string `json:"id"`
	AccountID        string `json:"account_id"`
	Status           string `json:"status"`
	ConfiguredStatus string `json:"configured_status"`
	IsArchived       bool   `json:"is_archived"`
}

// statusProbe reads only fields, verifies the object belongs to the
// configured ad account, and treats is_archived or any terminal status as
// gone.
func (p *Provider) statusProbe(fields string, terminal ...string) existenceProbe {
	return func(ctx context.Context, id string) (bool, error) {
		c, err := p.Client()
		if err != nil {
			return false, err
		}
		var item destroyProbeObject
		if err := c.Get(ctx, id, url.Values{"fields": {fields}}, &item); err != nil {
			if client.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if got := strings.TrimSpace(item.ID); got != id {
			return false, fmt.Errorf("remote object id %q does not match requested %s", got, id)
		}
		if err := p.ensureDestroyAccount(item.AccountID); err != nil {
			return false, err
		}
		if item.IsArchived {
			return false, nil
		}
		status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
		if status == "" {
			status = strings.ToUpper(strings.TrimSpace(item.Status))
		}
		for _, t := range terminal {
			if status == t {
				return false, nil
			}
		}
		return true, nil
	}
}

func (p *Provider) videoProbe(ctx context.Context, id string) (bool, error) {
	_, err := p.readAdVideo(ctx, id)
	if errors.Is(err, provider.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ensureDestroyAccount rejects objects owned by another ad account. Meta may
// omit account_id from minimal reads; the identity then comes only from
// local state, which was bound to the configured account.
func (p *Provider) ensureDestroyAccount(accountID string) error {
	got := strings.TrimPrefix(strings.TrimSpace(accountID), "act_")
	if got == "" {
		return nil
	}
	want := strings.TrimPrefix(strings.TrimSpace(p.cfg.AdAccountID), "act_")
	if got != want {
		return fmt.Errorf("remote object belongs to ad account act_%s, not configured account act_%s", got, want)
	}
	return nil
}
