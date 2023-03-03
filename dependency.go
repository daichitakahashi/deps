// Package deps helps managing lifecycle of the application's dependencies and shutting down gracefully, with minimalistic API.
package deps

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type (
	// Root is a root controller and describe its dependents using (*Root).Dependent.
	// Root can send signal of shutdown to all its dependents.
	Root struct {
		abortRequested chan struct{}
		requestAbort   func() // request abort
		aborted        chan struct{}
		wg             sync.WaitGroup

		abortCtx context.Context
		rw       sync.RWMutex
	}

	// Dependency is a controller of the worker depends on the parent.
	// After receiving abort signal from the parent, wait its dependent's stop and
	// notify the parent of its Stop.
	Dependency struct {
		requestAbort func()
		aborted      <-chan struct{}
		abortCtx     *context.Context
		rw           *sync.RWMutex

		m    sync.Mutex
		wait <-chan struct{}
		wg   sync.WaitGroup
		stop func() // notify parent
	}
)

// New creates Root controller.
func New() *Root {
	r := make(chan struct{})
	var once sync.Once
	request := func() {
		once.Do(func() {
			close(r)
		})
	}
	return &Root{
		abortRequested: r,
		requestAbort:   request,
		aborted:        make(chan struct{}),
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

func (r *Root) AbortRequested() <-chan struct{} {
	return r.abortRequested
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

func dependent(wg *sync.WaitGroup, requestAbort func(), aborted <-chan struct{}, abortCtx *context.Context, rw *sync.RWMutex) *Dependency {
	wg.Add(1)
	var once sync.Once
	return &Dependency{
		requestAbort: requestAbort,
		aborted:      aborted,
		abortCtx:     abortCtx,
		rw:           rw,
		stop: func() {
			once.Do(wg.Done)
		},
	}
}

// Dependent creates the controller depends on this root.
// Dependency should be created before the statement creating the goroutine or other event
// to be waited for. Otherwise, a data race could occur.
// Root uses [sync.WaitGroup] internally. For detail, see [sync.WaitGroup.Add].
func (r *Root) Dependent() *Dependency {
	return dependent(&r.wg, r.requestAbort, r.aborted, &r.abortCtx, &r.rw)
}

// Aborted returns a channel that's closed when its Root aborted.
// After the close of Aborted channel, the worker on behalf of this controller
// will have to start shutdown process including its dependents.
func (d *Dependency) Aborted() <-chan struct{} {
	return d.aborted
}

// AbortContext returns a context given to (*Root).Abort.
// The worker on behalf of this controller can get the deadline of shutdown
// from the context, if specified.
func (d *Dependency) AbortContext() context.Context {
	d.rw.RLock()
	defer d.rw.RUnlock()
	return *d.abortCtx
}

// Wait returns a channel that's closed when its all dependents stopped.
// To shutdown gracefully, the worker on behalf of this controller have to
// wait the stop of its children before starting its shutdown process.
func (d *Dependency) Wait() <-chan struct{} {
	d.m.Lock()
	defer d.m.Unlock()
	if d.wait == nil {
		d.wait = wait(&d.wg)
	}
	return d.wait
}

// Stop marks the worker on behalf of this controller stopped after all dependents
// stopped.
// If abortOnError indicates error, this requests Root to abort.
func (d *Dependency) Stop(abortOnError *error) {
	if abortOnError != nil && *abortOnError != nil {
		d.requestAbort()
	}
	<-d.Wait()
	d.stop()
}

// StopImmediately marks the worker on behalf of this controller stopped, even if its
// any dependents still working.
// If abortOnError indicates error, this requests Root to abort.
func (d *Dependency) StopImmediately(abortOnError *error) {
	if abortOnError != nil && *abortOnError != nil {
		d.requestAbort()
	}
	d.stop()
}

// Dependent creates the controller depends on this controller.
// Dependency should be created before the statement creating the goroutine or other event
// to be waited for. Otherwise, a data race could occur.
// Dependency uses [sync.WaitGroup] internally. For detail, see [sync.WaitGroup.Add].
func (d *Dependency) Dependent() *Dependency {
	return dependent(&d.wg, d.requestAbort, d.aborted, d.abortCtx, d.rw)
}
