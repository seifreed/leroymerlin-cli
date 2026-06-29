# leroymerlin-cli — build & maintenance.

BIN ?= leroymerlin

.PHONY: build test cover fmt vet tidy check clean

build:
	go build -o $(BIN) ./cmd/leroymerlin

test:
	go test ./...

cover:
	./coverage.sh

fmt:
	@test -z "$$(gofmt -l .)" || { echo "unformatted:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

tidy:
	go mod tidy

# Pre-release gate: formatting, vet, tests, build.
check: fmt vet test build
	@echo "ok"

clean:
	rm -f $(BIN)
