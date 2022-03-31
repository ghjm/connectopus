package timerunner

import (
	"context"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func makeTestRunner() (func(), func() []time.Time) {
	times := make([]time.Time, 0)
	return func() {
		times = append(times, time.Now())
	},
	func() []time.Time {
		return times
	}
}

func TestTimeRunner(t *testing.T) {
	defer goleak.VerifyNone(t)
	f, getTimes := makeTestRunner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := NewTimeRunner(ctx, f)
	tr.RunWithin(200 * time.Millisecond)
	tr.RunWithin(time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(300 * time.Millisecond)
	times := getTimes()
	if len(times) != 1 {
		t.Fatalf("function ran %d times, expecting 1", len(times))
	}
}
