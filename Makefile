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

# Run
run:
	go run cmd/api/main.go

# EC keys for the JWTs (paths match EC_PRIVATE_KEY_PATH / EC_PUBLIC_KEY_PATH)
ec_private:
	openssl ecparam -name prime256v1 -genkey -noout -out internal/platform/config/ec-private.pem

ec_public:
	openssl ec -in internal/platform/config/ec-private.pem -pubout -out internal/platform/config/ec-public.pem

.PHONY: test test_short test_only test_package test_coverage coverage_report_view mock sqlc migrate_create migrate_up migrate_up1 migrate_down migrate_down1 migrate_dirty run ec_private ec_public
