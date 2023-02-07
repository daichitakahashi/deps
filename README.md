# deps
[![Go Reference](https://pkg.go.dev/badge/github.com/daichitakahashi/deps.svg)](https://pkg.go.dev/github.com/daichitakahashi/deps)
[![coverage](https://img.shields.io/endpoint?style=flat-square&url=https%3A%2F%2Fdaichitakahashi.github.io%2Fdeps%2Fcoverage.json)](https://daichitakahashi.github.io/deps/coverage.html)

Manage lifecycle of the application's dependencies, with minimalistic API.

## How to use
Create application entrypoint using `deps.New()` and describe dependencies with `Dependent()`.
```go
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
```
