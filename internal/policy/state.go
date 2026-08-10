package policy

import (
	"sync"

	"golang.org/x/time/rate"
)

// SessionState is the per-session state the engine needs to enforce rate
// and spend limits. Implementations must be safe for concurrent use.
//
// The interface is expressed as atomic, storage-agnostic operations rather
// than exposing raw counters or a *rate.Limiter, so an implementation can
// later be backed by Redis (e.g. INCRBYFLOAT + a Lua check-and-set) instead
// of memory without changing the engine.
type SessionState interface {
	// AllowRate reports whether session may make another call right now
	// under a max-calls-per-minute budget, recording the call if allowed.
	AllowRate(session string, maxPerMinute int) bool

	// AddSpend atomically adds amount to session's running total for meter,
	// but only if the result would not exceed max. It returns the total
	// that would result (whether or not it was applied) and whether amount
	// was actually applied.
	AddSpend(session, meter string, amount, max float64) (total float64, ok bool)
}

// memoryState is an in-memory, mutex-guarded SessionState.
type memoryState struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	spend    map[string]float64 // key: session + "\x00" + meter
}

// NewMemoryState returns a SessionState backed by an in-memory map.
func NewMemoryState() SessionState {
	return &memoryState{
		limiters: make(map[string]*rate.Limiter),
		spend:    make(map[string]float64),
	}
}

func (s *memoryState) AllowRate(session string, maxPerMinute int) bool {
	s.mu.Lock()
	lim, ok := s.limiters[session]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(float64(maxPerMinute)/60), maxPerMinute)
		s.limiters[session] = lim
	}
	s.mu.Unlock()

	// rate.Limiter is itself safe for concurrent use, so this can run
	// outside the map lock.
	return lim.Allow()
}

func (s *memoryState) AddSpend(session, meter string, amount, max float64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := session + "\x00" + meter
	total := s.spend[key] + amount
	if total > max {
		return total, false
	}
	s.spend[key] = total
	return total, true
}
