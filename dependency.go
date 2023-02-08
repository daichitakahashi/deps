// Package deps is intended to control dependency description
package deps

import (
	"context"
	"fmt"
	"sync"
)

type (
	Dependency interface {
		Aborted() <-chan struct{}
		AbortContext() context.Context
		Wait() <-chan struct{}
		Stop()
		Dependent() Dependency
	}

	Root struct {
		aborted chan struct{}
		wg      sync.WaitGroup

		abortCtx context.Context
		rw       sync.RWMutex
	}
)

func New() *Root {
	return &Root{
		aborted: make(chan struct{}),
	}
}

func (r *Root) Aborted() <-chan struct{} {
	return r.aborted
}

func wait(wg *sync.WaitGroup) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()
	return done
}

func (r *Root) Abort(ctx context.Context) error {
	close(r.aborted)
	r.rw.Lock()
	r.abortCtx = ctx
	r.rw.Unlock()
	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to wait all dependencies to stop: %w", ctx.Err())
	case <-wait(&r.wg):
		return nil
	}
}

type dependency struct {
	aborted  <-chan struct{}
	abortCtx *context.Context
	rw       *sync.RWMutex
	wg       sync.WaitGroup
	stop     func() // notify parent
}

func dependent(wg *sync.WaitGroup, aborted <-chan struct{}, abortCtx *context.Context, rw *sync.RWMutex) Dependency {
	wg.Add(1)

	return &dependency{
		aborted:  aborted,
		abortCtx: abortCtx,
		rw:       rw,
		stop: func() {
			wg.Done()
		},
	}
}

func (r *Root) Dependent() Dependency {
	return dependent(&r.wg, r.aborted, &r.abortCtx, &r.rw)
}

func (d *dependency) Aborted() <-chan struct{} {
	return d.aborted
}

func (d *dependency) AbortContext() context.Context {
	d.rw.RLock()
	defer d.rw.RUnlock()
	return *d.abortCtx
}

func (d *dependency) Wait() <-chan struct{} {
	return wait(&d.wg)
}

func (d *dependency) Stop() {
	d.stop()
}

func (d *dependency) Dependent() Dependency {
	return dependent(&d.wg, d.aborted, d.abortCtx, d.rw)
}
