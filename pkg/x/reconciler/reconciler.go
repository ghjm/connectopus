package reconciler

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type ConfigItem interface {
	ShallowEqual(ConfigItem) bool
	Start(context.Context, *RunningItem, func()) (any, error)
	Children() map[string]ConfigItem
	Type() string
}

type RunningItem struct {
	isFake   bool
	name     string
	parent   *RunningItem
	config   ConfigItem
	ctx      context.Context
	cancel   context.CancelFunc
	status   syncro.Var[error]
	instance syncro.Var[any]
	children syncro.Map[string, *RunningItem]
	doneChan chan struct{}
}

func NewRootRunningItem(ctx context.Context, name string) *RunningItem {
	return &RunningItem{
		name: name,
		parent: &RunningItem{
			isFake: true,
			ctx:    ctx,
		},
		cancel:   func() {},
		status:   syncro.Var[error]{},
		children: syncro.Map[string, *RunningItem]{},
	}
}

func NewRunningItem(name string, parent *RunningItem) *RunningItem {
	return &RunningItem{
		name:     name,
		parent:   parent,
		status:   syncro.Var[error]{},
		children: syncro.Map[string, *RunningItem]{},
	}
}

func (ri *RunningItem) waitForStop() {
	wg := sync.WaitGroup{}
	ri.children.WorkWithReadOnly(func(c map[string]*RunningItem) {
		for _, v := range c {
			wg.Add(1)
			go func(v *RunningItem) {
				v.waitForStop()
				wg.Done()
			}(v)
		}
	})
	wg.Wait()
	log.Infof("Stopping %s %s\n", ri.config.Type(), ri.QualifiedName())
	t := time.NewTimer(10 * time.Second)
	select {
	case <-ri.doneChan:
		t.Stop()
	case <-t.C:
		log.Warnf("%s %s is taking a long time to stop", ri.config.Type(), ri.QualifiedName())
	}
	<-ri.doneChan
}

func (ri *RunningItem) Reconcile(ci ConfigItem) {
	oldConfig := ri.config
	ri.config = ci
	if !ci.ShallowEqual(oldConfig) {
		if ri.ctx != nil {
			ri.cancel()
			ri.waitForStop()
		}
		ri.children = syncro.Map[string, *RunningItem]{}
		ri.ctx, ri.cancel = context.WithCancel(ri.parent.ctx)
		startChan := make(chan struct{})
		once := sync.Once{}
		go func() {
			myType := ri.config.Type()
			for {
				ri.doneChan = make(chan struct{})
				childInstance, err := ci.Start(ri.ctx, ri, func() { close(ri.doneChan) })
				ri.instance.Set(childInstance)
				ri.status.Set(err)
				once.Do(func() { close(startChan) })
				if err == nil {
					log.Infof("Starting %s %s", myType, ri.QualifiedName())
					return
				}
				log.Warnf("Failed to start %s %s: %s", myType, ri.QualifiedName(), err)
				timer := time.NewTimer(5 * time.Second)
				select {
				case <-ri.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
		<-startChan
	}
	ri.children.WorkWith(func(_rc *map[string]*RunningItem) {
		rc := *_rc
		ciChildren := ci.Children()
		for name, cci := range ciChildren {
			cri, ok := rc[name]
			if !ok {
				cri = NewRunningItem(name, ri)
			}
			cri.Reconcile(cci)
			rc[name] = cri
		}
		for name, rci := range rc {
			_, ok := ciChildren[name]
			if !ok {
				if rci.ctx != nil {
					log.Infof("Removing %s %s", rci.config.Type(), rci.QualifiedName())
				}
				rci.cancel()
				delete(rc, name)
			}
		}
	})
}

func (ri *RunningItem) Status() error {
	err := ri.status.Get()
	if err != nil {
		return err
	}
	ri.children.WorkWithReadOnly(func(c map[string]*RunningItem) {
		for k, v := range c {
			cErr := v.Status()
			if cErr != nil {
				err = fmt.Errorf("%s: %w", k, cErr)
				return
			}
		}
	})
	return err
}

func (ri *RunningItem) SetFailed(err error) {
	ri.status.Set(err)
	go func() {
		time.Sleep(50 * time.Millisecond)
		pctx := ri.parent.ctx
		if ri.ctx != nil {
			ri.cancel()
			ri.waitForStop()
		}
		t := time.NewTimer(5 * time.Second)
		select {
		case <-pctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		config := ri.config
		ri.config = nil // force restart
		ri.Reconcile(config)
	}()
}

func (ri *RunningItem) Instance() any {
	return ri.instance.Get()
}

func (ri *RunningItem) Config() ConfigItem {
	return ri.config
}

func (ri *RunningItem) Name() string {
	return ri.name
}

func (ri *RunningItem) QualifiedName() string {
	name := ri.name
	rp := ri.parent
	for rp != nil && !rp.isFake {
		name = rp.name + "." + name
		rp = rp.parent
	}
	return name
}

func (ri *RunningItem) Parent() *RunningItem {
	p := ri.parent
	if p == nil || p.isFake {
		return nil
	}
	return p
}

func (ri *RunningItem) Children() map[string]*RunningItem {
	ret := make(map[string]*RunningItem)
	ri.children.WorkWithReadOnly(func(c map[string]*RunningItem) {
		for k, v := range c {
			ret[k] = v
		}
	})
	return ret
}

func (ri *RunningItem) Context() context.Context {
	return ri.ctx
}
