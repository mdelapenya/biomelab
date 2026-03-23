package provider

// UnsupportedProvider is a no-op provider for hosting platforms without CLI support.
type UnsupportedProvider struct {
	provider Provider
}

// NewUnsupportedProvider creates a provider that always returns CLIUnsupportedProvider.
func NewUnsupportedProvider(p Provider) *UnsupportedProvider {
	return &UnsupportedProvider{provider: p}
}

// CheckCLI always returns CLIUnsupportedProvider.
func (u *UnsupportedProvider) CheckCLI() CLIAvailability {
	return CLIUnsupportedProvider
}

// FetchPRs always returns an empty result.
func (u *UnsupportedProvider) FetchPRs(_ string, _ []string) PRResult {
	return make(PRResult)
}

// Name returns the provider name.
func (u *UnsupportedProvider) Name() string {
	return u.provider.String()
}

// Provider returns the provider type.
func (u *UnsupportedProvider) Provider() Provider {
	return u.provider
}
