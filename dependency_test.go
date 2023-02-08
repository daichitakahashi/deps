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
		defer dep.Stop()
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
				defer dep.Stop()

				go func() {
					dep := dep.Dependent()
					defer dep.Stop()

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

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		if err := root.Abort(ctx); err != nil {
			t.Fatal("graceful abort failed", err)
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
			defer dep.Stop()

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
			defer dep.Stop()

			go func() {
				dep := dep.Dependent()
				defer dep.Stop()

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
		t.Fatal("graceful abort failed", err)
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
