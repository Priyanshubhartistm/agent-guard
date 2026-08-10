package guard

import (
	"sync"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// Record is one decision the Guard made, kept in memory when WithHistory is
// used (e.g. to drive a dashboard).
type Record struct {
	Call      policy.ToolCall `json:"call"`
	Decision  policy.Decision `json:"decision"`
	DecidedAt time.Time       `json:"decided_at"`
}

// Stats summarizes decision counts and the current approval backlog.
type Stats struct {
	Allowed int `json:"allowed"`
	Denied  int `json:"denied"`
	Pending int `json:"pending"`
}

// history is an in-memory, mutex-guarded ring of the most recent decisions,
// plus running Allow/Deny counters. A RequireApproval record is kept in the
// feed (it marks the moment a call was parked for a human) but does not
// count toward either the Allowed or Denied tally — only the call's
// eventual, final decision does.
type history struct {
	mu      sync.Mutex
	records []Record // newest first, capped at max
	max     int
	allowed int
	denied  int
}

func newHistory(max int) *history {
	if max <= 0 {
		max = 100
	}
	return &history{max: max}
}

func (h *history) record(call policy.ToolCall, d policy.Decision) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch d.Type {
	case policy.Allow:
		h.allowed++
	case policy.Deny:
		h.denied++
	}

	rec := Record{Call: call, Decision: d, DecidedAt: time.Now()}
	h.records = append([]Record{rec}, h.records...)
	if len(h.records) > h.max {
		h.records = h.records[:h.max]
	}
}

func (h *history) recent() []Record {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Record, len(h.records))
	copy(out, h.records)
	return out
}

func (h *history) counts() (allowed, denied int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.allowed, h.denied
}
