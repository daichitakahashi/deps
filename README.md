# deps
[![Go Reference](https://pkg.go.dev/badge/github.com/daichitakahashi/deps.svg)](https://pkg.go.dev/github.com/daichitakahashi/deps)
[![coverage](https://img.shields.io/endpoint?style=flat-square&url=https%3A%2F%2Fdaichitakahashi.github.io%2Fdeps%2Fcoverage.json)](https://daichitakahashi.github.io/deps/coverage.html)

Manage lifecycle of the application's dependencies and shutdown gracefully, with minimalistic API.

## How to use
Create application entrypoint using `deps.New()` and describe dependencies with `Dependent()`.
```go
func main() {
	root := deps.New()

	// Worker #1 (Web server)
	go func(dep *deps.Dependency) {
		var (
			svr http.Server
			err error
			e   = make(chan error, 1)
		)
		defer dep.Stop(&err) // request abort if err!=nil

		go func() {
			e <- svr.ListenAndServe()
		}()
		select {
		case err = <-e:
			log.Println("server stopped unexpectedly: ", err)
		case <-dep.Aborted():
			log.Println("start shutdown server")
			// do not pass shutdown error to dep.Stop()
			if shutdownErr := svr.Shutdown(dep.AbortContext()); shutdownErr != nil { // timeout=1m
				log.Println("failed to shutdown server gracefully: ", shutdownErr)
			}
		}
	}(root.Dependent())

	// Worker #2 (Periodic task runner or something)
	go func(dep *deps.Dependency) {
		defer dep.Stop(nil)

		// Start worker and describe shutdown flow as same as Worker #1...
	}(root.Dependent())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	select {
	case <-root.AbortRequested():
		log.Println("abort: server error")
	case s := <-sig:
		log.Printf("abort: signal received (%s)", s.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := root.Abort(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
```
