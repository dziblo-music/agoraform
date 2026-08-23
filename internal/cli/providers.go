package cli

import (
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func newProviderRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	_ = reg.Register(matomo.NewFromEnv())
	return reg
}
