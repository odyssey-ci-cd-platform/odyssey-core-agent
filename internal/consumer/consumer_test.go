package consumer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/bus"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/consumer"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

// publish adds one event to the bus stream via the real publisher.
func publish(t *testing.T, client redis.UniversalClient, event domain.Event) {
	t.Helper()
	if err := bus.New(client).Publish(context.Background(), event); err != nil {
		t.Fatalf("publish %s: %v", event.Type, err)
	}
}

// collector records handled events with a Done channel per event.
type collector struct {
	mu      sync.Mutex
	events  []domain.Event
	seen    chan struct{}
	handler func(e domain.Event) error
}

func newCollector(n int) *collector {
	return &collector{seen: make(chan struct{}, n)}
}

func (c *collector) handle(_ context.Context, e domain.Event) error {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
	if c.handler != nil {
		if err := c.handler(e); err != nil {
			return err
		}
	}
	c.seen <- struct{}{}
	return nil
}

func (c *collector) recorded() []domain.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]domain.Event(nil), c.events...)
}

func TestConsumerDeliversAndAcks(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	// Events published before the consumer starts must all be delivered,
	// in stream order.
	for _, typ := range []string{"test.a", "test.b", "test.c"} {
		publish(t, client, domain.Event{Type: typ, Pipeline: "p"})
	}

	col := newCollector(3)
	c, err := consumer.New(client, col.handle, consumer.Options{Group: "svc", Consumer: "c1"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() { runErr <- c.Run(runCtx) }()

	var got []string
	for i := 0; i < 3; i++ {
		select {
		case <-col.seen:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for event %d; handled %v", i+1, col.recorded())
		}
	}
	for _, e := range col.recorded() {
		got = append(got, e.Type)
	}
	want := []string{"test.a", "test.b", "test.c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delivered %v, want %v", got, want)
		}
	}

	// Successful handling must ack every entry.
	pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: bus.StreamEvents, Group: "svc", Start: "-", End: "+", Count: 10,
	}).Result()
	if err != nil {
		t.Fatalf("XPendingExt: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending entries after successful handling = %d, want 0", len(pending))
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run returned error on shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}

func TestConsumerNewValidatesOptions(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	if _, err := consumer.New(client, func(context.Context, domain.Event) error { return nil }, consumer.Options{}, nil); err == nil {
		t.Error("New with empty Options: want error for missing group, got nil")
	}
	if _, err := consumer.New(client, nil, consumer.Options{Group: "g", Consumer: "c"}, nil); err == nil {
		t.Error("New with nil handler: want error, got nil")
	}
}
