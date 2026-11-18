package identity

import (
	"context"
	"errors"
	"testing"
)

func TestResolverResolvesRegisteredAndUnknownIdentities(t *testing.T) {
	providerResolver := &fakeProviderResolver{
		resolutions: map[string]Resolution{
			"known-provider-user": {
				Registered: true,
				UserID:     "user-id-1",
			},
		},
	}
	resolver := NewResolver(map[Provider]ProviderResolver{
		ProviderWhatsApp: providerResolver,
	})

	registered, err := resolver.ResolveIdentity(
		context.Background(),
		ProviderWhatsApp,
		"known-provider-user",
	)
	if err != nil {
		t.Fatalf("ResolveIdentity() registered error = %v", err)
	}
	if !registered.Registered || registered.UserID != "user-id-1" {
		t.Fatalf("registered resolution = %#v", registered)
	}

	unknown, err := resolver.ResolveIdentity(
		context.Background(),
		ProviderWhatsApp,
		"unknown-provider-user",
	)
	if err != nil {
		t.Fatalf("ResolveIdentity() unknown error = %v", err)
	}
	if unknown.Registered || unknown.UserID != "" {
		t.Fatalf("unknown resolution = %#v", unknown)
	}

	if providerResolver.resolveCalls != 2 || providerResolver.writeCalls != 0 {
		t.Fatalf(
			"resolver calls = %d, write calls = %d, want 2 reads and 0 writes",
			providerResolver.resolveCalls,
			providerResolver.writeCalls,
		)
	}
}

func TestResolverAPIIsProviderNeutral(t *testing.T) {
	futureProvider := Provider("future-channel")
	resolver := NewResolver(map[Provider]ProviderResolver{
		futureProvider: &fakeProviderResolver{
			resolutions: map[string]Resolution{
				"future-provider-user": {Registered: true, UserID: "user-id-2"},
			},
		},
	})

	resolution, err := resolver.ResolveIdentity(
		context.Background(),
		futureProvider,
		"future-provider-user",
	)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if !resolution.Registered || resolution.UserID != "user-id-2" {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestResolverUsesProviderNeutralScopedResolution(t *testing.T) {
	scope := &struct{ name string }{name: "transaction"}
	providerResolver := &fakeProviderResolver{
		resolutions: map[string]Resolution{
			"known-provider-user": {Registered: true, UserID: "user-id-1"},
		},
	}
	resolver := NewResolver(map[Provider]ProviderResolver{
		ProviderWhatsApp: providerResolver,
	})

	resolution, err := resolver.ResolveIdentityWithScope(
		context.Background(),
		scope,
		ProviderWhatsApp,
		"known-provider-user",
	)
	if err != nil {
		t.Fatalf("ResolveIdentityWithScope() error = %v", err)
	}
	if resolution.UserID != "user-id-1" || providerResolver.scope != scope {
		t.Fatalf("resolution = %#v, scope = %#v", resolution, providerResolver.scope)
	}
}

func TestResolverValidatesInputAndUnsupportedProviders(t *testing.T) {
	resolver := NewResolver(nil)

	tests := []struct {
		name       string
		provider   Provider
		providerID string
		wantErr    error
	}{
		{name: "missing provider", providerID: "provider-user", wantErr: ErrProviderRequired},
		{name: "missing provider user ID", provider: ProviderWhatsApp, wantErr: ErrProviderUserIDRequired},
		{name: "unsupported provider", provider: ProviderWhatsApp, providerID: "provider-user", wantErr: ErrUnsupportedProvider},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.ResolveIdentity(
				context.Background(),
				tt.provider,
				tt.providerID,
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

type fakeProviderResolver struct {
	resolutions  map[string]Resolution
	resolveCalls int
	writeCalls   int
	scope        any
}

func (f *fakeProviderResolver) ResolveWithScope(
	_ context.Context,
	scope any,
	providerUserID string,
) (Resolution, error) {
	f.resolveCalls++
	f.scope = scope
	return f.resolutions[providerUserID], nil
}

func (f *fakeProviderResolver) Resolve(
	_ context.Context,
	providerUserID string,
) (Resolution, error) {
	f.resolveCalls++
	return f.resolutions[providerUserID], nil
}
