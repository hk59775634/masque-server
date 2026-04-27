//go:build linux

package main

import (
	"context"
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(1*time.Second, 10*time.Second); got != 2*time.Second {
		t.Fatalf("nextBackoff 1s->10s = %s", got)
	}
	if got := nextBackoff(8*time.Second, 10*time.Second); got != 10*time.Second {
		t.Fatalf("nextBackoff 8s->10s = %s", got)
	}
	if got := nextBackoff(12*time.Second, 10*time.Second); got != 10*time.Second {
		t.Fatalf("nextBackoff cap = %s", got)
	}
}

func TestSleepWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepWithContext(ctx, 200*time.Millisecond) {
		t.Fatal("sleepWithContext should return false on canceled context")
	}
}

func TestLogThrottleZeroIntervalAlwaysEmits(t *testing.T) {
	th := &logThrottle{minInterval: 0}
	n := 0
	for i := 0; i < 3; i++ {
		th.maybeLog(func(suppressed int) {
			if suppressed != 0 {
				t.Fatalf("unexpected suppressed=%d", suppressed)
			}
			n++
		})
	}
	if n != 3 {
		t.Fatalf("emits=%d want 3", n)
	}
}

func TestLogThrottleCoalesces(t *testing.T) {
	th := &logThrottle{minInterval: 20 * time.Millisecond}
	emits := 0
	var lastSup int
	th.maybeLog(func(sup int) {
		emits++
		lastSup = sup
	})
	th.maybeLog(func(int) { t.Fatal("should not emit") })
	th.maybeLog(func(int) { t.Fatal("should not emit") })
	time.Sleep(25 * time.Millisecond)
	th.maybeLog(func(sup int) {
		emits++
		lastSup = sup
	})
	if emits != 2 {
		t.Fatalf("emits=%d want 2", emits)
	}
	if lastSup < 2 {
		t.Fatalf("last suppressed=%d want >=2", lastSup)
	}
}
