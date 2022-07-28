package timerunner

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func makeTestRunner() (func(), func() []time.Time) {
	times := syncro.Var[[]time.Time]{}
	return func() {
		times.WorkWith(func(tp *[]time.Time) {
			*tp = append(*tp, time.Now())
		})
	}, times.Get
}

func TestTimeRunner(t *testing.T) {
	defer goleak.VerifyNone(t)
	f, getTimes := makeTestRunner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := New(ctx, f)
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
