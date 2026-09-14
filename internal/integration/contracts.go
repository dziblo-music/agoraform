package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// ContractChange describes a change to an externally consumed application
// instrumentation contract since the last successful apply.
type ContractChange struct {
	Name   string
	Action string
}

const (
	ContractCreate = "create"
	ContractUpdate = "update"
	ContractDelete = "delete"
)

// ContractFingerprints returns deterministic, non-secret fingerprints for the
// externally consumed application event contracts. Matomo fingerprints include
// the event name derived from the referenced managed trigger, so changing the
// trigger changes both the provider plan and the integration-contract plan.
func ContractFingerprints(events map[string]manifest.ApplicationEvent, resources []resource.Resource) (map[string]string, error) {
	out := make(map[string]string, len(events))
	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}

	for _, name := range manifest.ApplicationEventNames(events) {
		evt := events[name]
		var parts []string

		if evt.Matomo != nil {
			eventName := strings.TrimSpace(evt.Matomo.Event)
			triggerAddr := evt.Matomo.Trigger.Address.String()
			if !evt.Matomo.Trigger.IsZero() {
				res, ok := byAddr[triggerAddr]
				if !ok {
					return nil, fmt.Errorf("applicationEvents.%s.matomo: references unknown resource %q", name, triggerAddr)
				}
				v, ok := res.Attributes["event"].(string)
				if !ok || strings.TrimSpace(v) == "" {
					return nil, fmt.Errorf("applicationEvents.%s.matomo: trigger %q has no event", name, triggerAddr)
				}
				eventName = strings.TrimSpace(v)
			}
			fields := append([]string(nil), evt.Matomo.Fields...)
			sort.Strings(fields)
			parts = append(parts, "matomo|trigger="+triggerAddr+"|event="+eventName+"|fields="+strings.Join(fields, ","))
		}
		if evt.GoogleAds != nil {
			parts = append(parts, "googleAds|conversion="+evt.GoogleAds.Conversion.Address.String())
		}
		if evt.Meta != nil {
			parts = append(parts, "meta|eventSource="+evt.Meta.EventSource.Address.String()+"|eventName="+evt.Meta.EventName+"|delivery="+string(evt.Meta.Delivery))
		}

		sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		out[name] = hex.EncodeToString(sum[:])
	}
	return out, nil
}

// DiffContractFingerprints compares the last-applied and desired contract
// fingerprints in deterministic event-name order.
func DiffContractFingerprints(previous, desired map[string]string) []ContractChange {
	names := make(map[string]struct{}, len(previous)+len(desired))
	for name := range previous {
		names[name] = struct{}{}
	}
	for name := range desired {
		names[name] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	changes := make([]ContractChange, 0)
	for _, name := range ordered {
		oldFP, hadOld := previous[name]
		newFP, hasNew := desired[name]
		switch {
		case !hadOld && hasNew:
			changes = append(changes, ContractChange{Name: name, Action: ContractCreate})
		case hadOld && !hasNew:
			changes = append(changes, ContractChange{Name: name, Action: ContractDelete})
		case hadOld && hasNew && oldFP != newFP:
			changes = append(changes, ContractChange{Name: name, Action: ContractUpdate})
		}
	}
	return changes
}

// FormatContractChanges renders application integration changes separately
// from provider-resource changes because contracts are local/external outputs,
// not remote provider resources.
func FormatContractChanges(changes []ContractChange) string {
	if len(changes) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Application integration changes:\n")
	for _, change := range changes {
		symbol := "~"
		switch change.Action {
		case ContractCreate:
			symbol = "+"
		case ContractDelete:
			symbol = "-"
		}
		fmt.Fprintf(&sb, "  %s applicationEvents.%s\n", symbol, change.Name)
	}
	return sb.String()
}
