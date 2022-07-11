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
	ParentEqual(ConfigItem) bool
	Start(context.Context, string, any) (any, error)
	Children() map[string]ConfigItem
}

type RunningItem struct {
	name      string
	parentCtx context.Context
	config    ConfigItem
	ctx       context.Context
	cancel    context.CancelFunc
	status    syncro.Var[error]
	instance  syncro.Var[any]
	children  syncro.Map[string, *RunningItem]
}

func NewRunningItem(ctx context.Context, name string) *RunningItem {
	return &RunningItem{
		name:      name,
		parentCtx: ctx,
		cancel:    func() {},
		status:    syncro.Var[error]{},
		children:  syncro.Map[string, *RunningItem]{},
	}
}

func (ri *RunningItem) Reconcile(ci ConfigItem, instance any) {
	oldConfig := ri.config
	ri.config = ci
	if !ci.ParentEqual(oldConfig) {
		if ri.ctx != nil {
			log.Infof("Restarting %s", ri.name)
		}
		ri.cancel()
		ri.children = syncro.Map[string, *RunningItem]{}
		ri.ctx, ri.cancel = context.WithCancel(ri.parentCtx)
		startChan := make(chan struct{})
		once := sync.Once{}
		go func() {
			for {
				childInstance, err := ci.Start(ri.ctx, ri.name, instance)
				ri.instance.Set(childInstance)
				ri.status.Set(err)
				once.Do(func() { close(startChan) })
				if err == nil {
					log.Infof("Started %s", ri.name)
					return
				} else {
					log.Infof("Failed to start %s: %s", ri.name, err)
				}
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
		parentInstance := ri.instance.Get()
		for name, cci := range ciChildren {
			cri, ok := rc[name]
			if !ok {
				cri = NewRunningItem(ri.ctx, fmt.Sprintf("%s.%s", ri.name, name))
			}
			cri.Reconcile(cci, parentInstance)
			rc[name] = cri
		}
		for name, rci := range rc {
			_, ok := ciChildren[name]
			if !ok {
				if rci.ctx != nil {
					log.Infof("Removing %s", rci.name)
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

func (ri *RunningItem) Instance() any {
	return ri.instance.Get()
}

func (ri *RunningItem) Config() ConfigItem {
	return ri.config
}

func (ri *RunningItem) Name() string {
	return ri.name
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