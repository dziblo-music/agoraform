package cli

import (
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func newProviderRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	_ = reg.Register(matomo.NewFromEnv())
	_ = reg.Register(googleads.NewFromEnv())
	_ = reg.Register(meta.NewFromEnv())
	return reg
}
