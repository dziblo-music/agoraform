package client

import (
	"context"
	"fmt"
	"strings"
)

// GoalPreservedFields is the unmanaged portion of a Matomo Goal record that
// must be carried forward when calling Goals.updateGoal. Matomo's update API
// applies default values to omitted parameters, so leaving these fields out
// can silently reset existing goal behavior.
type GoalPreservedFields struct {
	CaseSensitive                    string
	Revenue                          string
	AllowMultipleConversionsPerVisit string
	Description                      string
	UseEventValueAsRevenue           string
}

// UpdateGoalPreserving updates managed goal fields while explicitly carrying
// forward the current values of v0.1-unmanaged Matomo Goal settings.
func (a *Analytics) UpdateGoalPreserving(ctx context.Context, idGoal string, in GoalInput, preserved GoalPreservedFields) error {
	if a == nil || a.c == nil {
		return fmt.Errorf("matomo: analytics client is nil")
	}
	idGoal = strings.TrimSpace(idGoal)
	if idGoal == "" {
		return fmt.Errorf("matomo: idGoal is required")
	}

	params := goalInputValues(in)
	params.Set("idGoal", idGoal)
	params.Set("caseSensitive", preserved.CaseSensitive)
	params.Set("revenue", preserved.Revenue)
	params.Set("allowMultipleConversionsPerVisit", preserved.AllowMultipleConversionsPerVisit)
	params.Set("description", preserved.Description)
	params.Set("useEventValueAsRevenue", preserved.UseEventValueAsRevenue)
	_, err := a.Call(ctx, "Goals.updateGoal", params)
	return err
}
