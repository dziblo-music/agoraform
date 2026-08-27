package googleads

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

const campaignCriteriaCollection = "campaignCriteria"

func campaignCriterionResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + campaignCriteriaCollection + "/" + id
}

func campaignCriterionID(campaignID, criterionID string) string {
	return campaignID + "~" + criterionID
}

func boundCampaignCriterionIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCampaignCriterionIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCampaignCriterionIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads campaign criterion id of the form campaignId~criterionId is required", addr)
	}
	campaignID, criterionID, err := parseCampaignCriterionID(addr, raw)
	if err != nil {
		return "", err
	}
	return campaignCriterionID(campaignID, criterionID), nil
}

func parseCampaignCriterionID(addr resource.Address, raw string) (campaignID, criterionID string, err error) {
	raw = strings.TrimSpace(raw)
	if _, campID, crID, ok := splitCampaignCriterionResourceName(raw); ok {
		return campID, crID, nil
	}
	parts := strings.Split(raw, "~")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign criterion id; expected campaignId~criterionId", addr, raw)
	}
	campaignID = strings.TrimSpace(parts[0])
	criterionID = strings.TrimSpace(parts[1])
	if n, parseErr := strconv.ParseInt(campaignID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign criterion id; expected campaignId~criterionId", addr, raw)
	}
	if n, parseErr := strconv.ParseInt(criterionID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign criterion id; expected campaignId~criterionId", addr, raw)
	}
	return campaignID, criterionID, nil
}

func (p *Provider) canonicalCampaignCriterionImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportCampaignCriterionID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, campaignID, criterionID, ok := splitCampaignCriterionResourceName(id); ok {
		configured, err := p.configuredCustomerID()
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		got, err := NormalizeCustomerID(customerID)
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		if got != configured {
			return "", fmt.Errorf("googleads: import %s: resource name customer %s does not match configured %s", addr, got, configured)
		}
		return campaignCriterionID(campaignID, criterionID), nil
	}
	return id, nil
}

func parseImportCampaignCriterionID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected campaignId~criterionId or resource name customers/{customerId}/campaignCriteria/{campaignId}~{criterionId}", addr)
	}
	if _, campaignID, criterionID, ok := splitCampaignCriterionResourceName(raw); ok {
		if err := campaignCriterionPartIDError(addr, campaignID); err != nil {
			return "", err
		}
		if err := campaignCriterionPartIDError(addr, criterionID); err != nil {
			return "", err
		}
		return raw, nil
	}
	campaignID, criterionID, err := parseCampaignCriterionID(addr, raw)
	if err != nil {
		return "", fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign criterion id; expected campaignId~criterionId or resource name customers/{customerId}/campaignCriteria/{campaignId}~{criterionId}", addr, raw)
	}
	return campaignCriterionID(campaignID, criterionID), nil
}

func campaignCriterionPartIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign criterion id; expected campaignId~criterionId or resource name customers/{customerId}/campaignCriteria/{campaignId}~{criterionId}", addr, id)
	}
	return nil
}

func splitCampaignCriterionResourceName(name string) (customerID, campaignID, criterionID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != campaignCriteriaCollection {
		return "", "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", "", false
	}
	ids := strings.Split(parts[3], "~")
	if len(ids) != 2 {
		return "", "", "", false
	}
	if n, err := strconv.ParseInt(ids[0], 10, 64); err != nil || n <= 0 {
		return "", "", "", false
	}
	if n, err := strconv.ParseInt(ids[1], 10, 64); err != nil || n <= 0 {
		return "", "", "", false
	}
	return parts[1], ids[0], ids[1], true
}

func parseCampaignCriterionMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, campaignID, criterionID, ok := splitCampaignCriterionResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned campaign criterion %s for a different customer", campaignCriterionID(campaignID, criterionID))
	}
	return campaignCriterionID(campaignID, criterionID), nil
}

func (p *Provider) ensureCampaignCriterionIdentityMatches(res resource.Resource) error {
	id, bound, err := boundCampaignCriterionIdentity(res)
	if err != nil || !bound {
		return err
	}
	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return nil
	}
	gotCampaignID, _, err := parseCampaignCriterionID(res.Address, id)
	if err != nil {
		return err
	}
	if gotCampaignID != campaignID {
		return fmt.Errorf("resource %s: persisted identity %q does not match referenced campaign %s", res.Address, id, campaignID)
	}
	return nil
}
