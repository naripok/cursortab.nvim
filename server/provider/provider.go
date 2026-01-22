package provider

import (
	"cursortab/provider/autocomplete"
	"cursortab/provider/sweep"
	"cursortab/provider/zeta"
	"cursortab/types"
	"fmt"
)

// NewProvider creates a new provider instance based on the type
func NewProvider(providerType types.ProviderType, config *types.ProviderConfig) (types.Provider, error) {
	switch providerType {
	case types.ProviderTypeZeta:
		return zeta.NewProvider(config)
	case types.ProviderTypeAutoComplete:
		return autocomplete.NewProvider(config)
	case types.ProviderTypeSweep:
		return sweep.NewProvider(config)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}
