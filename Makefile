-include .env
export

TEST ?= .*
PACKAGE ?= ./...
DOCKER_HOST ?= npipe:////./pipe/docker_engine

# Tests (integration tests start Postgres through testcontainers; -short skips them)
test:
	DOCKER_HOST=$(DOCKER_HOST) go test -count=1 ./...

test_short:
	go test -count=1 -short ./...

test_only:
	DOCKER_HOST=$(DOCKER_HOST) go test -v -run ^$(TEST)$$ $(PACKAGE) -count=1

test_package:
	DOCKER_HOST=$(DOCKER_HOST) go test -v -count=1 $(PACKAGE)

test_coverage:
	DOCKER_HOST=$(DOCKER_HOST) go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...

coverage_report_view:
	go tool cover -html=coverage.out

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

# Run
run:
	go run cmd/api/main.go

# EC keys for the JWTs (paths match EC_PRIVATE_KEY_PATH / EC_PUBLIC_KEY_PATH)
ec_private:
	openssl ecparam -name prime256v1 -genkey -noout -out internal/platform/config/ec-private.pem

ec_public:
	openssl ec -in internal/platform/config/ec-private.pem -pubout -out internal/platform/config/ec-public.pem

.PHONY: sec sec_install test test_short test_only test_package test_coverage coverage_report_view mock sqlc migrate_create migrate_up migrate_up1 migrate_down migrate_down1 migrate_dirty run ec_private ec_public
