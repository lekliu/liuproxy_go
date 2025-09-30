package tunnel

import (
	"fmt"
	"liuproxy_go/internal/shared/types"
	"liuproxy_go/internal/tunnel/goremote"
	"liuproxy_go/internal/tunnel/vless"
	"liuproxy_go/internal/tunnel/worker"
)

// NewStrategy is the factory function to create a new strategy based on the active profile.
// This is the single entry point for creating any strategy.
func NewStrategy(cfg *types.Config, profiles []*types.ServerProfile) (types.TunnelStrategy, error) {
	var activeProfile *types.ServerProfile
	for _, p := range profiles {
		if p.Active {
			activeProfile = p
			break
		}
	}

	if activeProfile == nil {
		return nil, nil
	}

	switch activeProfile.Type {
	case "vless":
		return vless.NewVlessStrategy(cfg, activeProfile)
	case "worker":
		return worker.NewWorkerStrategy(cfg, activeProfile)
	case "goremote", "remote", "": // "" is for backward compatibility
		return goremote.NewGoRemoteStrategy(cfg, activeProfile)
	default:
		return nil, fmt.Errorf("unknown or unsupported strategy type: '%s'", activeProfile.Type)
	}
}
