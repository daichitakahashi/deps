// Package deps helps managing lifecycle of the application's dependencies, with minimalistic API.
package deps

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type (
	// Dependency is a controller of the worker depends on the parent.
	// After receiving abort signal from the parent, wait its dependent's stop and
	// notify the parent of its Stop.
	Dependency interface {
		// Aborted returns a channel that's closed when its Root aborted.
		// After the close of Aborted channel, the worker on behalf of this controller
		// will have to start shutdown including its dependents.
		Aborted() <-chan struct{}

		// AbortContext returns a context given to (*Root).Abort.
		// The worker on behalf of this controller can get the deadline of shutdown
		// from the context, if specified.
		AbortContext() context.Context

		// Wait returns a channel that's closed when its all dependents stopped.
		// To shutdown gracefully, the worker on behalf of this controller have to
		// wait the stop of its children before its Stop.
		Wait() <-chan struct{}

		// Stop marks the worker on behalf of this controller shut down, even if its
		// any dependents still working.
		Stop()

		// Dependent creates the controller depends on this controller.
		Dependent() Dependency
	}

	// Root is a root controller and describe its dependents using (*Root).Dependent.
	// Root can send signal of shutdown to all its dependents.
	Root struct {
		aborted chan struct{}
		wg      sync.WaitGroup

		abortCtx context.Context
		rw       sync.RWMutex
	}
)

// New creates Root controller.
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

// Abort fires shutdown of the application.
// When all dependents stopped successfully, it returns nil.
// The context given as argument can be accessed via (Dependency).AbortContext.
func (r *Root) Abort(ctx context.Context) error {
	select {
	case <-r.Aborted():
		return errors.New("already aborted")
	default:
	}
	close(r.aborted)
	r.rw.Lock()
	r.abortCtx = ctx
	r.rw.Unlock()
	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to wait all dependents to stop: %w", ctx.Err())
	case <-wait(&r.wg):
		return nil
	}
}

type dependency struct {
	aborted  <-chan struct{}
	abortCtx *context.Context
	rw       *sync.RWMutex

	m    sync.Mutex
	wait <-chan struct{}
	wg   sync.WaitGroup
	stop func() // notify parent
}

func dependent(wg *sync.WaitGroup, aborted <-chan struct{}, abortCtx *context.Context, rw *sync.RWMutex) Dependency {
	wg.Add(1)
	var once sync.Once
	return &dependency{
		aborted:  aborted,
		abortCtx: abortCtx,
		rw:       rw,
		stop: func() {
			once.Do(wg.Done)
		},
	}
}

// Dependent creates the controller depends on this root.
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
	d.m.Lock()
	defer d.m.Unlock()
	if d.wait == nil {
		d.wait = wait(&d.wg)
	}
	return d.wait
}

func (d *dependency) Stop() {
	d.stop()
}

func (d *dependency) Dependent() Dependency {
	return dependent(&d.wg, d.aborted, d.abortCtx, d.rw)
}
