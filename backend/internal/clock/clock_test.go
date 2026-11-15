package clock

import (
	"context"
	"testing"
	"time"
)

func TestFakeSleepAdvancesWithoutBlocking(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := NewFake(start)
	if err := f.Sleep(context.Background(), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := f.Now().Sub(start); got != 10*time.Second {
		t.Fatalf("advanced %v, want 10s", got)
	}
}

func TestFakeSleepHonoursCancelledContext(t *testing.T) {
	f := NewFake(time.Unix(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Sleep(ctx, time.Second); err == nil {
		t.Fatal("expected context error")
	}
	if !f.Now().Equal(time.Unix(0, 0)) {
		t.Fatal("time must not advance on a cancelled sleep")
	}
}

func TestRealSleepHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (Real{}).Sleep(ctx, time.Hour); err == nil {
		t.Fatal("expected context error")
	}
}
