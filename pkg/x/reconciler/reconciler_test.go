package reconciler

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/expector"
	"go.uber.org/goleak"
	"testing"
	"time"
)

type Config struct {
	MainCfg string
	SubCfg  map[string]Config
}

var exp *expector.Expector[string]

func (c Config) ParentEqual(item ConfigItem) bool {
	ci, ok := item.(Config)
	if !ok {
		return false
	}
	return c.MainCfg == ci.MainCfg
}

func (c Config) Start(ctx context.Context, _ string, _ any) (any, error) {
	exp.Produce(fmt.Sprintf("started %s", c.MainCfg))
	go func() {
		<-ctx.Done()
		exp.Produce(fmt.Sprintf("stopped %s", c.MainCfg))
	}()
	return nil, nil
}

func (c Config) Children() map[string]ConfigItem {
	ci := make(map[string]ConfigItem)
	for n, v := range c.SubCfg {
		ci[n] = v
	}
	return ci
}

var config1 = Config{
	MainCfg: "main 1",
	SubCfg: map[string]Config{
		"A": {MainCfg: "A1"},
		"B": {MainCfg: "B1"},
		"C": {MainCfg: "C1"},
	},
}

var config2 = Config{
	MainCfg: "main 2",
	SubCfg: map[string]Config{
		"A": {MainCfg: "A1"},
		"B": {MainCfg: "B1"},
		"C": {MainCfg: "C1"},
	},
}

var config3 = Config{
	MainCfg: "main 2",
	SubCfg: map[string]Config{
		"A": {MainCfg: "A1"},
		"B": {MainCfg: "B2"},
		"C": {MainCfg: "C1"},
	},
}

func TestComponents(t *testing.T) {
	goleak.VerifyNone(t)
	mainCtx, mainCancel := context.WithTimeout(context.Background(), time.Second)
	defer mainCancel()
	exp = expector.NewExpector[string](mainCtx)
	testCtx, testCancel := context.WithCancel(mainCtx)
	exp.Expect("started main 1")
	exp.Expect("started A1")
	exp.Expect("started B1")
	exp.Expect("started C1")
	main := NewRunningItem(testCtx, "test")
	main.Reconcile(config1, nil)
	exp.WaitClear()
	exp.Expect("stopped main 1")
	exp.Expect("stopped A1")
	exp.Expect("stopped B1")
	exp.Expect("stopped C1")
	exp.Expect("started main 2")
	exp.Expect("started A1")
	exp.Expect("started B1")
	exp.Expect("started C1")
	main.Reconcile(config2, nil)
	exp.WaitClear()
	exp.Expect("stopped B1")
	exp.Expect("started B2")
	main.Reconcile(config3, nil)
	exp.WaitClear()
	exp.Expect("stopped main 2")
	exp.Expect("stopped A1")
	exp.Expect("stopped B2")
	exp.Expect("stopped C1")
	testCancel()
	exp.WaitClear()
	if mainCtx.Err() != nil {
		t.Fatal(mainCtx.Err())
	}
}
