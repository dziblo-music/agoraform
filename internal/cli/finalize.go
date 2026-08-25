package cli

import (
	"context"
	"io"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
)

// attachFinalizations remains as a thin CLI adapter for plan command/tests;
// finalization planning itself belongs to the provider-neutral apply layer.
func attachFinalizations(ctx context.Context, reg *provider.Registry, p *plan.Plan) error {
	return apply.AttachFinalizations(ctx, reg, p)
}

// executeFinalizations remains as a thin compatibility adapter for CLI tests.
// The apply command itself uses apply.Run, which owns the complete lifecycle.
func executeFinalizations(ctx context.Context, reg *provider.Registry, plans []provider.FinalizationPlan, out io.Writer) error {
	_, err := apply.ExecuteFinalizations(ctx, reg, plans, out)
	return err
}
