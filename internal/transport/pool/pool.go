package pool

import (
	"context"
	"sync"
	"time"

	"localaihub/internal/config"
	"localaihub/internal/transport/contract"
	"localaihub/internal/transport/registry"
	"localaihub/internal/wire"
)

type Entry struct {
	ID        string
	Target    *config.Resolved
	Transport contract.Transport
	State     string
	LastUsed  time.Time
	leases    int
}

type Pool struct {
	reg  *registry.Registry
	opts contract.Options
	idle time.Duration
	mu   sync.Mutex
	m    map[string]*Entry
}

func New(reg *registry.Registry, opts contract.Options, idle time.Duration) *Pool {
	p := &Pool{reg: reg, opts: opts, idle: idle, m: map[string]*Entry{}}
	go p.reaper()
	return p
}

func (p *Pool) Acquire(ctx context.Context, t *config.Resolved) (*Entry, error) {
	key := t.ConnKey
	for {
		if err := ctx.Err(); err != nil {
			return nil, wire.E("REMOTE_UNREACHABLE", err.Error())
		}
		p.mu.Lock()
		if e, ok := p.m[key]; ok {
			switch e.State {
			case "ready", "busy":
				if e.Transport != nil {
					e.leases++
					e.State = "busy"
					e.LastUsed = time.Now()
					p.mu.Unlock()
					return e, nil
				}
			case "connecting":
				p.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, wire.E("REMOTE_UNREACHABLE", ctx.Err().Error())
				case <-time.After(25 * time.Millisecond):
				}
				continue
			default:
				delete(p.m, key)
			}
		}
		e := &Entry{ID: wire.NewConnID(), Target: t, State: "connecting", LastUsed: time.Now()}
		p.m[key] = e
		p.mu.Unlock()
		tr, err := p.reg.Open(ctx, t, p.opts)
		p.mu.Lock()
		if err != nil {
			e.State = "backoff"
			delete(p.m, key)
			p.mu.Unlock()
			return nil, err
		}
		e.Transport = tr
		e.State = "busy"
		e.leases = 1
		e.LastUsed = time.Now()
		p.mu.Unlock()
		return e, nil
	}
}

func (p *Pool) Release(e *Entry) {
	if e == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e.leases--
	if e.leases < 0 {
		e.leases = 0
	}
	if e.leases == 0 && e.State != "closed" && e.State != "closing" {
		e.State = "ready"
	}
	e.LastUsed = time.Now()
}

func (p *Pool) List() []*Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Entry, 0, len(p.m))
	for _, e := range p.m {
		cp := *e
		out = append(out, &cp)
	}
	return out
}

func (p *Pool) Find(idOrTarget string) *Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.m {
		if e.ID == idOrTarget || e.Target.Ref == idOrTarget || e.Target.Name == idOrTarget {
			cp := *e
			return &cp
		}
	}
	return nil
}

func (p *Pool) Close(idOrTarget string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.m {
		if e.ID == idOrTarget || e.Target.Ref == idOrTarget || e.Target.Name == idOrTarget {
			e.State = "closing"
			if e.Transport != nil {
				_ = e.Transport.Close()
			}
			e.State = "closed"
			delete(p.m, k)
			return nil
		}
	}
	return wire.E("CONNECTION_NOT_FOUND", "unknown connection")
}

func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.m {
		e.State = "closing"
		if e.Transport != nil {
			_ = e.Transport.Close()
		}
		e.State = "closed"
		delete(p.m, k)
	}
}

func (p *Pool) reaper() {
	if p.idle <= 0 {
		return
	}
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		p.mu.Lock()
		for k, e := range p.m {
			if e.leases == 0 && e.State == "ready" && time.Since(e.LastUsed) > p.idle {
				e.State = "closing"
				if e.Transport != nil {
					_ = e.Transport.Close()
				}
				e.State = "closed"
				delete(p.m, k)
			}
		}
		p.mu.Unlock()
	}
}

func IdleSeconds(e *Entry) int {
	if e == nil {
		return 0
	}
	return int(time.Since(e.LastUsed).Seconds())
}
