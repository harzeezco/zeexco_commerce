package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Provider string

const ProviderWhatsApp Provider = "whatsapp"

var (
	ErrProviderRequired       = errors.New("identity provider is required")
	ErrProviderUserIDRequired = errors.New("provider user ID is required")
	ErrUnsupportedProvider    = errors.New("identity provider is not supported")
)

type Resolution struct {
	Registered bool
	UserID     string
}

type ProviderResolver interface {
	Resolve(ctx context.Context, providerUserID string) (Resolution, error)
}

// ScopedProviderResolver lets a provider resolve through a caller-owned
// database scope, such as the transaction holding a message processing lock.
// The scope remains provider-neutral; each adapter validates the concrete
// capability it needs.
type ScopedProviderResolver interface {
	ResolveWithScope(
		ctx context.Context,
		scope any,
		providerUserID string,
	) (Resolution, error)
}

type Resolver struct {
	providers map[Provider]ProviderResolver
}

func NewResolver(providers map[Provider]ProviderResolver) *Resolver {
	providerCopy := make(map[Provider]ProviderResolver, len(providers))
	for provider, resolver := range providers {
		providerCopy[provider] = resolver
	}

	return &Resolver{providers: providerCopy}
}

func (r *Resolver) ResolveIdentity(
	ctx context.Context,
	provider Provider,
	providerUserID string,
) (Resolution, error) {
	providerResolver, err := r.providerResolver(provider, providerUserID)
	if err != nil {
		return Resolution{}, err
	}

	return providerResolver.Resolve(ctx, providerUserID)
}

func (r *Resolver) ResolveIdentityWithScope(
	ctx context.Context,
	scope any,
	provider Provider,
	providerUserID string,
) (Resolution, error) {
	providerResolver, err := r.providerResolver(provider, providerUserID)
	if err != nil {
		return Resolution{}, err
	}

	scopedResolver, ok := providerResolver.(ScopedProviderResolver)
	if !ok {
		return Resolution{}, fmt.Errorf(
			"%w: provider %s does not support scoped resolution",
			ErrUnsupportedProvider,
			provider,
		)
	}

	return scopedResolver.ResolveWithScope(ctx, scope, providerUserID)
}

func (r *Resolver) providerResolver(
	provider Provider,
	providerUserID string,
) (ProviderResolver, error) {
	if strings.TrimSpace(string(provider)) == "" {
		return nil, ErrProviderRequired
	}
	if strings.TrimSpace(providerUserID) == "" {
		return nil, ErrProviderUserIDRequired
	}

	providerResolver, ok := r.providers[provider]
	if !ok || providerResolver == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}

	return providerResolver, nil
}
