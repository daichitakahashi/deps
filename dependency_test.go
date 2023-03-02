package deps_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daichitakahashi/deps"
)

func ExampleNew() {
	root := deps.New()

	go func(dep deps.Dependency) {
		defer dep.Stop(nil)
		for {
			select {
			case <-dep.Aborted():
				return
			default:
				for i := 0; i < 3; i++ {
					time.Sleep(time.Second)
					fmt.Printf("...%d", i)
				}
				fmt.Println()
			}
		}
	}(root.Dependent())

	time.Sleep(time.Second * 4)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := root.Abort(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	// ...0...1...2
	// ...0...1...2
}

func TestRoot_Abort(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		const count = 10 // 20 dependencies will be created
		var abortDetected int32

		root := deps.New()
		for i := 0; i < count; i++ {
			created := make(chan struct{})
			go func() {
				dep := root.Dependent()
				defer dep.Stop(nil)

				go func() {
					dep := dep.Dependent()
					defer dep.Stop(nil)

					close(created)

					<-dep.Aborted()
					atomic.AddInt32(&abortDetected, 1)
				}()

				<-dep.Aborted()
				<-dep.Wait() // wait for all dependencies stopped

				atomic.AddInt32(&abortDetected, 1)
			}()
			<-created // wait all goroutine launched
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		if err := root.Abort(ctx); err != nil {
			t.Fatalf("graceful abort failed: %s", err)
		}

		// Check if all dependencies detected root.Abort or not
		if detected := atomic.LoadInt32(&abortDetected); detected != 20 {
			t.Fatalf("abortDetected: want %d, got %d", 20, detected)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		root := deps.New()
		created := make(chan struct{})
		go func() {
			dep := root.Dependent()
			defer dep.Stop(nil)

			close(created)

			<-time.After(time.Second)
		}()
		<-created

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
		defer cancel()

		err := root.Abort(ctx)
		if err == nil {
			t.Fatal("unexpected success")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got unexpected error: %s", err)
		}
	})

	t.Run("Abort after Abort", func(t *testing.T) {
		t.Parallel()

		root := deps.New()
		created := make(chan struct{})
		go func() {
			dep := root.Dependent()
			defer dep.Stop(nil)

			close(created)

			<-time.After(time.Millisecond)
		}()
		<-created

		if err := root.Abort(context.Background()); err != nil {
			t.Fatalf("graceful abort failed: %s", err)
		}
		if err := root.Abort(context.Background()); err == nil {
			t.Fatal("unexpected success")
		}
	})
}

func TestDependency_AbortContext(t *testing.T) {
	t.Parallel()

	var (
		root              = deps.New()
		m                 sync.Mutex
		detectedDeadlines []time.Time
	)
	const count = 5 // 10 dependencies will be created

	for i := 0; i < count; i++ {
		created := make(chan struct{})
		go func() {
			dep := root.Dependent()
			defer dep.Stop(nil)

			go func() {
				dep := dep.Dependent()
				defer dep.Stop(nil)

				close(created)

				<-dep.Aborted()
				abortCtx := dep.AbortContext()
				deadline, _ := abortCtx.Deadline()
				m.Lock()
				defer m.Unlock()
				detectedDeadlines = append(detectedDeadlines, deadline)
			}()

			<-dep.Aborted()
			<-dep.Wait() // wait for all dependencies stopped

			abortCtx := dep.AbortContext()
			deadline, _ := abortCtx.Deadline()
			m.Lock()
			defer m.Unlock()
			detectedDeadlines = append(detectedDeadlines, deadline)
		}()
		<-created
	}

	expectedDeadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), expectedDeadline)
	defer cancel()

	if err := root.Abort(ctx); err != nil {
		t.Fatalf("graceful abort failed: %s", err)
	}
	m.Lock()
	defer m.Unlock()
	if len(detectedDeadlines) != 10 {
		t.Fatalf("abortDetected: want %d, got %d", 10, len(detectedDeadlines))
	}
	for _, d := range detectedDeadlines {
		if !expectedDeadline.Equal(d) {
			t.Fatalf("unexpected deadline detected: want %s, got %s", expectedDeadline, d)
		}
	}
}

func earlyStopParentDependent(t *testing.T, stop func(deps.Dependency) func(*error)) (childDependentFinished bool) {
	t.Helper()

	var (
		root    = deps.New()
		stopped atomic.Bool
	)
	go func() {
		var (
			dep = root.Dependent() // Dependent A
			err error
		)
		defer stop(dep)(&err)

		go func() {
			dep := dep.Dependent() // Dependent B
			defer stop(dep)(nil)

			time.Sleep(time.Second * 2)
			stopped.Store(true)
		}()

		time.Sleep(time.Millisecond * 500)
		err = errors.New("stop early")
		_ = err
	}()

	select {
	case <-root.AbortRequested():
	case <-time.After(time.Second):
		t.Fatal("abort not requested")
	}
	err := root.Abort(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	return stopped.Load()
}

func TestDependency_Stop(t *testing.T) {
	t.Parallel()

	childDependencyStopped := earlyStopParentDependent(t, func(dep deps.Dependency) func(*error) {
		return dep.Stop
	})
	if !childDependencyStopped {
		t.Fatal("Dependent B not stopped")
	}
}

func TestDependency_StopImmediately(t *testing.T) {
	t.Parallel()

	childDependencyStopped := earlyStopParentDependent(t, func(dep deps.Dependency) func(*error) {
		return dep.StopImmediately
	})
	if childDependencyStopped {
		t.Fatal("Dependent B stopped unexpectedly")
	}
}
