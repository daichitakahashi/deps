package deps_test

import (
	"context"
	"fmt"
	"log"
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

func TestNew(t *testing.T) {
}

func TestDependency_AbortContext(t *testing.T) {
}
