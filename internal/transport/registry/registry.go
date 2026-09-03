package registry

import (
	"context"
	"sync"

	"localaihub/internal/config"
	"localaihub/internal/transport/contract"
	"localaihub/internal/wire"
)

type Registry struct {
	mu sync.RWMutex
	m  map[string]contract.Factory
}

func New() *Registry { return &Registry{m: map[string]contract.Factory{}} }

func (r *Registry) Register(name string, f contract.Factory) {
	r.mu.Lock()
	r.m[name] = f
	r.mu.Unlock()
}

func (r *Registry) Open(ctx context.Context, t *config.Resolved, opts contract.Options) (contract.Transport, error) {
	r.mu.RLock()
	f := r.m[t.Transport]
	r.mu.RUnlock()
	if f == nil {
		return nil, wire.Ef("CAPABILITY_UNSUPPORTED", "unknown transport %s", t.Transport)
	}
	return f(ctx, t, opts)
}
