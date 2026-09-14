# leroymerlin-cli — build & maintenance.

BIN ?= leroymerlin

.PHONY: build test cover fmt vet tidy check release-dry clean

build:
	go build -o $(BIN) ./cmd/leroymerlin

test:
	go test ./...

# -coverpkg=./... so a package's code counts as covered when another package's
# tests exercise it (the cmd end-to-end tests drive internal/client). The second
# stage folds in the subprocess profile: a test binary never runs main(), so the
# only way to cover it is to execute an instrumented build and merge what it
# writes to GOCOVERDIR. -count=1 because a cached test run never executes
# TestMain, which would leave .covdata empty and main() reported as uncovered.
cover:
	rm -rf .covdata && mkdir -p .covdata
	LEROYMERLIN_TEST_COVERDIR=$(CURDIR)/.covdata \
		go test -count=1 -coverpkg=./... -coverprofile=coverage.out -covermode=atomic ./...
	go tool covdata textfmt -i=.covdata -o coverage.subproc.out
	tail -n +2 coverage.subproc.out >> coverage.out
	go tool cover -func=coverage.out | tail -1

fmt:
	@test -z "$$(gofmt -l .)" || { echo "unformatted:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

tidy:
	go mod tidy

# Pre-release gate: formatting, vet, tests, build.
check: fmt vet test build
	@echo "ok"

# Dry-run the release the way CI will build it: cross-compiled archives +
# checksums into dist/, nothing published. Names must stay what install.sh
# reconstructs — leroymerlin_<version>_<os>_<arch>.tar.gz — so check dist/ after.
release-dry:
	go run github.com/goreleaser/goreleaser/v2@v2.12.7 release --snapshot --clean --skip=publish
	@ls dist/*.tar.gz dist/*.zip dist/checksums.txt

clean:
	rm -f $(BIN)
	rm -rf .covdata coverage.out coverage.subproc.out dist
