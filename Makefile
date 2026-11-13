-include .env
export

TEST ?= .*
PACKAGE ?= ./...
# Windows only: testcontainers' auto-detection there intermittently fails ("rootless Docker is not supported on
# Windows"). Elsewhere DOCKER_HOST stays as the environment has it (unset = the default socket); `export` above
# would otherwise send the npipe value to docker and the tests on Linux / macOS.
ifeq ($(OS),Windows_NT)
DOCKER_HOST ?= npipe:////./pipe/docker_engine
endif

# Tests (integration tests start Postgres through testcontainers; -short skips them)
test:
	go test -count=1 ./...

test_short:
	go test -count=1 -short ./...

test_only:
	go test -v -run ^$(TEST)$$ $(PACKAGE) -count=1

test_package:
	go test -v -count=1 $(PACKAGE)

test_coverage:
	go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...

coverage_report_view:
	go tool cover -html=coverage.out

# Race detector, always in Docker (decision 0017): -race needs cgo on amd64 / arm64, which a local Go may not
# have. Debian image (it has gcc) at go.mod's toolchain patch version, bumped with it and the Dockerfile; module
# and build caches in named volumes. Unit tests only (-short): the integration tests would need the Docker socket.
RACE_IMAGE ?= golang:1.26.11
test_race:
	MSYS_NO_PATHCONV=1 docker run --rm -v "$(CURDIR):/src" -w /src \
		-v go_boilerplate_vs_gomod:/go/pkg/mod -v go_boilerplate_vs_gobuild:/root/.cache/go-build \
		-e GOFLAGS=-buildvcs=false $(RACE_IMAGE) go test -race -short -count=1 $(PACKAGE)

# Codegen
SOURCE ?= .
DESTINATION ?= .
PKG_NAME ?= .

mock:
	mockgen -source=$(SOURCE) -destination=$(DESTINATION) -package=$(PKG_NAME)

sqlc:
	sqlc generate

# Migrations
DIRE ?= internal/platform/db/migration
NAME ?= .
SEQ ?= .

migrate_create:
	migrate create -ext sql -dir $(DIRE) -seq $(NAME)

migrate_up:
	migrate -path internal/platform/db/migration -database "$(DB_SOURCE_LOCAL)" -verbose up

migrate_up1:
	migrate -path internal/platform/db/migration -database "$(DB_SOURCE_LOCAL)" -verbose up 1

migrate_down:
	migrate -path internal/platform/db/migration -database "$(DB_SOURCE_LOCAL)" -verbose down

migrate_down1:
	migrate -path internal/platform/db/migration -database "$(DB_SOURCE_LOCAL)" -verbose down 1

migrate_dirty:
	migrate -path internal/platform/db/migration -database "$(DB_SOURCE_LOCAL)" force $(SEQ)

# Security scans (base security; run before a release). LOCAL ONLY: it runs only
# with the confirmation typed on the command line —
#     make sec SEC_PASSWORD=imsureitislocal
# A value from .env or the environment is refused (origin must be "command line"),
# so a .env copied to a server can't unlock it. ENVIRONMENT can't tell a laptop
# from the dev server (both are development), hence the typed phrase. It guards
# against running it by accident; it isn't a secret. A server gains nothing (same
# source) and would pay CPU, outbound calls and .env secrets exported to the tools.
# Every tool runs even when
# an earlier one fails, and the target fails at the end if any did. gosec skips
# generated code (sqlc). A false positive is silenced on its line with a reason:
# `// #nosec G101 -- why` (gosec), `// nosemgrep: <rule-id>` (semgrep).
# Tools: `make sec_install` (gosec, govulncheck, gitleaks — pinned), plus semgrep
# via `pipx install semgrep` (or `pip install --user semgrep`, then put its
# Scripts / bin folder on PATH). .semgrep/ holds our own rules.
SEC_PASSWORD_EXPECTED := imsureitislocal

GOSEC_VERSION ?= v2.30.0
GOVULNCHECK_VERSION ?= v1.9.0
GITLEAKS_VERSION ?= v8.30.1

sec:
	@if [ "$(origin SEC_PASSWORD)" != "command line" ] || [ "$(SEC_PASSWORD)" != "$(SEC_PASSWORD_EXPECTED)" ]; then \
		echo "make sec runs only on a developer's machine. Confirm by typing: make sec SEC_PASSWORD=<phrase from the Makefile>"; \
		echo "(a SEC_PASSWORD from .env or the environment doesn't count). Not run."; \
		exit 1; \
	fi
	@status=0; \
	echo "== gosec"; gosec -quiet -exclude-generated -severity medium ./... || status=1; \
	echo "== semgrep"; semgrep scan --config p/golang --config .semgrep/ --error --metrics=off || status=1; \
	echo "== govulncheck"; govulncheck ./... || status=1; \
	echo "== gitleaks"; gitleaks git -v --redact || status=1; \
	if [ $$status -ne 0 ]; then echo "== sec: FAILED (see above)"; else echo "== sec: all clean"; fi; \
	exit $$status

sec_install:
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

# Lint (static analysis + formatting). Runs anywhere, no guard: no secrets, no outbound calls.
# staticcheck runs on its own (staticcheck.conf) and inside golangci-lint (.golangci.yml, same checks);
# both run even when the first fails, and the target fails at the end if either did. A finding is fixed,
# or a false positive silenced on its line with a reason: `//nolint:<linter> // why` (golangci-lint),
# `//lint:ignore <check> why` on the line before (staticcheck). `make fmt` applies gofmt + goimports.
# Tools: `make lint_install` (pinned; golangci-lint must be built with a Go >= go.mod's).
STATICCHECK_VERSION ?= v0.8.2
GOLANGCI_LINT_VERSION ?= v2.15.0

lint:
	@status=0; \
	echo "== staticcheck"; staticcheck ./... || status=1; \
	echo "== golangci-lint"; golangci-lint run ./... || status=1; \
	if [ $$status -ne 0 ]; then echo "== lint: FAILED (see above)"; else echo "== lint: all clean"; fi; \
	exit $$status

fmt:
	golangci-lint fmt ./...

# After coding — the standard, for humans and AI alike (decision 0014): gofmt, go vet, lint, go test, in that
# order. Every step runs; the target fails at the end if any did. `make check` runs the full test suite
# (Docker); `make check_short` only the unit tests, for quick iterations — `make check` before calling it done.
CHECK_TEST_FLAGS ?=

check:
	@status=0; \
	echo "== gofmt"; out=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$out" ]; then echo "$$out"; echo "not gofmt-ed: run make fmt"; status=1; fi; \
	echo "== go vet"; go vet ./... || status=1; \
	echo "== lint"; "$(MAKE)" --no-print-directory lint || status=1; \
	echo "== go test $(CHECK_TEST_FLAGS)"; go test -count=1 $(CHECK_TEST_FLAGS) ./... || status=1; \
	if [ $$status -ne 0 ]; then echo "== check: FAILED (see above)"; else echo "== check: all clean"; fi; \
	exit $$status

check_short:
	@"$(MAKE)" --no-print-directory check CHECK_TEST_FLAGS=-short

lint_install:
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Run
run:
	go run cmd/api/main.go

# EC keys for the JWTs (paths match EC_PRIVATE_KEY_PATH / EC_PUBLIC_KEY_PATH)
ec_private:
	openssl ecparam -name prime256v1 -genkey -noout -out internal/platform/config/ec-private.pem

ec_public:
	openssl ec -in internal/platform/config/ec-private.pem -pubout -out internal/platform/config/ec-public.pem

.PHONY: check check_short lint fmt lint_install sec sec_install test test_short test_race test_only test_package test_coverage coverage_report_view mock sqlc migrate_create migrate_up migrate_up1 migrate_down migrate_down1 migrate_dirty run ec_private ec_public
