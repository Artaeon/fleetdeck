package server

import (
	"sync"
	"testing"
	"time"
)

// TestDedupFirstSeenIsFalse pins the contract: a new delivery ID is NOT
// deduplicated. Forgetting this would cause every legitimate webhook to
// be dropped.
func TestDedupFirstSeenIsFalse(t *testing.T) {
	d := newWebhookDedup(time.Minute)
	if d.seenRecently("delivery-1") {
		t.Error("first observation should return false")
	}
}

// TestDedupRepeatWithinTTL is the load-bearing assertion: a redelivery
// within the TTL window must return true so the handler can 202-skip.
func TestDedupRepeatWithinTTL(t *testing.T) {
	d := newWebhookDedup(time.Minute)
	d.seenRecently("delivery-1")
	if !d.seenRecently("delivery-1") {
		t.Error("repeat within TTL should return true")
	}
}

// TestDedupExpiresAfterTTL verifies that an old entry ages out and a
// later redelivery is treated as new. Uses a short TTL to keep the
// test fast.
func TestDedupExpiresAfterTTL(t *testing.T) {
	d := newWebhookDedup(30 * time.Millisecond)
	d.seenRecently("delivery-1")
	time.Sleep(50 * time.Millisecond)
	if d.seenRecently("delivery-1") {
		t.Error("entry should have aged out after TTL")
	}
}

// TestDedupEmptyIDIsNeverCached guards against deduplicating requests
// with a missing X-GitHub-Delivery header into each other.
func TestDedupEmptyIDIsNeverCached(t *testing.T) {
	d := newWebhookDedup(time.Minute)
	if d.seenRecently("") || d.seenRecently("") {
		t.Error("empty delivery IDs must not dedupe")
	}
}

// TestDedupIsConcurrencySafe stresses the mutex with many parallel
// callers on the same ID. Only one goroutine should observe "first
// seen" (return false); the rest must see true.
func TestDedupIsConcurrencySafe(t *testing.T) {
	d := newWebhookDedup(time.Minute)

	const n = 64
	var wg sync.WaitGroup
	firsts := make(chan bool, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			firsts <- d.seenRecently("delivery-hot")
		}()
	}
	wg.Wait()
	close(firsts)

	var firstCount int
	for result := range firsts {
		if !result {
			firstCount++
		}
	}
	if firstCount != 1 {
		t.Errorf("expected exactly one 'first seen' observation, got %d", firstCount)
	}
}

// TestDedupRefreshesTimestamp confirms that a hit within the window
// re-anchors the TTL. Prevents a slow trickle of redeliveries from
// leaking past the window on the boundary.
func TestDedupRefreshesTimestamp(t *testing.T) {
	d := newWebhookDedup(60 * time.Millisecond)
	d.seenRecently("delivery-slow")
	time.Sleep(40 * time.Millisecond)
	// Hit within window — should refresh timestamp
	if !d.seenRecently("delivery-slow") {
		t.Fatal("hit within window should return true")
	}
	// Sleep another 40 ms — total 80 ms since first seen, but only
	// 40 ms since refresh, so should still be within window.
	time.Sleep(40 * time.Millisecond)
	if !d.seenRecently("delivery-slow") {
		t.Error("refresh should have extended the window; redelivery still within TTL was not deduped")
	}
}
