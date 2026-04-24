package server

import (
	"sync"
	"time"
)

// webhookDedup remembers GitHub webhook delivery IDs for a bounded window
// so duplicate redeliveries (the same X-GitHub-Delivery UUID fired again
// by GitHub because our first response timed out) do not trigger parallel
// deploys of the same commit.
//
// GitHub documents webhooks as at-least-once — if the receiver doesn't
// respond within 10 seconds, GitHub retries. A long deploy easily
// exceeds that, so redelivery is the common case, not the edge case.
// Without dedup, two `runDeployment` goroutines race to update the same
// deployment record and one of them inevitably writes a stale status.
//
// The cache is intentionally in-memory rather than in SQLite: we want
// this to be cheap on the happy path, and an fleetdeck process restart
// losing the dedup window is acceptable (at worst, one duplicate deploy
// on the first webhook after a restart).
type webhookDedup struct {
	mu   sync.Mutex
	seen map[string]time.Time
	// ttl controls how long a delivery ID is remembered. Long enough to
	// outlast GitHub's retry window (which tops out around 5 minutes
	// total over multiple attempts) by a healthy margin.
	ttl time.Duration
}

func newWebhookDedup(ttl time.Duration) *webhookDedup {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &webhookDedup{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// seenRecently returns true when id was seen within ttl and bumps the
// entry's timestamp. Returns false the first time an id is observed
// (and for ids that have aged out), registering it for future calls.
//
// Empty id is always treated as "not seen" so callers that can't read
// the X-GitHub-Delivery header (misconfigured webhook, non-GitHub
// sender) aren't silently deduplicated to each other.
func (d *webhookDedup) seenRecently(id string) bool {
	if id == "" {
		return false
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	// Opportunistic sweep — cheap because we're already holding the lock
	// and the map is tiny (bounded by webhook rate over ttl).
	for k, t := range d.seen {
		if now.Sub(t) > d.ttl {
			delete(d.seen, k)
		}
	}

	if ts, ok := d.seen[id]; ok && now.Sub(ts) <= d.ttl {
		// Refresh the timestamp so a duplicate burst keeps extending the
		// window — prevents a slow processor from racing the TTL and
		// accepting the same delivery twice.
		d.seen[id] = now
		return true
	}
	d.seen[id] = now
	return false
}
