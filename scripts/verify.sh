#!/bin/sh

set -e

COVERAGE_THRESHOLD=70
WEB_DIR=web

# Run a command quietly, show output only on failure
run_check() {
    title=$1
    shift
    echo ">> $title..."
    if ! output=$("$@" 2>&1); then
        echo "❌ ERROR: $title failed"
        echo "----------------------------------------"
        echo "$output"
        echo "----------------------------------------"
        exit 1
    fi
}

# ---------- Backend (Go) ----------
if [ -f go.mod ]; then
    run_check "Security audit" govulncheck ./...
    run_check "Dependency check" go mod verify

    echo ">> Running go fmt..."
    FMT_OUT=$(gofmt -l .)
    if [ -n "$FMT_OUT" ]; then
        echo "❌ ERROR: Code not formatted. Run 'go fmt ./...'"
        echo "Files needing format: $FMT_OUT"
        exit 1
    fi

    run_check "Linter" golangci-lint run
    run_check "Tests" go test ./...
    run_check "Tests with race detection" go test -race ./...

    echo ">> Checking test coverage..."
    go test -coverprofile=coverage.out ./... > /dev/null 2>&1 || true
    if [ -f coverage.out ]; then
        coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        is_low=$(awk -v cov="$coverage" -v thr="$COVERAGE_THRESHOLD" 'BEGIN {print (cov < thr ? 1 : 0)}')
        if [ "$is_low" -eq 1 ]; then
            echo "⚠️ Warning: Coverage is ${coverage}% (minimum: ${COVERAGE_THRESHOLD}%)"
        else
            echo "✅ Coverage is ${coverage}%"
        fi
        rm coverage.out
    fi

    run_check "Go build" go build -o /dev/null ./...
else
    echo ">> No go.mod yet, skipping backend checks"
fi

# ---------- Frontend (SvelteKit) ----------
if [ -f "$WEB_DIR/package.json" ]; then
    run_check "Frontend dependencies" npm --prefix "$WEB_DIR" ci
    run_check "Frontend security audit" npm --prefix "$WEB_DIR" audit --omit=dev --audit-level=high
    run_check "Svelte type check" npm --prefix "$WEB_DIR" run check
    if grep -q '"lint"' "$WEB_DIR/package.json"; then
        run_check "Frontend lint" npm --prefix "$WEB_DIR" run lint
    fi
    if grep -q '"test"' "$WEB_DIR/package.json"; then
        run_check "Frontend unit tests" npm --prefix "$WEB_DIR" test
    fi
    run_check "Frontend build" npm --prefix "$WEB_DIR" run build
else
    echo ">> No $WEB_DIR/package.json yet, skipping frontend checks"
fi
