test:
	go test -race -coverprofile=coverage.out.tmp -coverpkg=./... ./...

coverage.out:
	@# カバレッジの計測対象から、特定のファイルを除外する
	@cat coverage.out.tmp | grep -v ".pb." | grep -v ".gen." > coverage.out
	@rm coverage.out.tmp

test-cov: test coverage.out
	go tool cover -func=coverage.out
	@rm coverage.out

test-cov-visual: test coverage.out
	go tool cover -html=coverage.out
	@rm coverage.out

lint:
	docker run --rm -w /go/lint -v `pwd`:/go/lint:ro -v `go env GOMODCACHE`:/go/pkg/mod:ro golangci/golangci-lint:latest golangci-lint run ./...

.PHONY: test coverage.out test-cov test-cov-visual lint
