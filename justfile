default:
    @just --list

# Build the richmond binary
build:
    go build -o bin/richmond .

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-v:
    go test -v ./...

# Run fuzz tests (30s default)
fuzz duration="30s":
    go test -fuzz=Fuzz -fuzztime={{duration}} ./internal/scim/

# Run golangci-lint
lint:
    golangci-lint run ./...

# Run go vet
vet:
    go vet ./...

# Run govulncheck
vulncheck:
    govulncheck ./...

# Run semgrep
semgrep:
    semgrep --config auto .

# Run all quality gates
check: lint vet test vulncheck

# Build container image
docker-build tag="richmond:latest":
    docker build -t {{tag}} .

# Remove build artifacts
clean:
    rm -rf bin/

# Run sync (pass args after --)
run *args:
    go run . {{args}}
