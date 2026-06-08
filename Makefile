##
### Environment variables
##

BIN       := flightrecorder
GO        := go

# Build configuration
BUILD_DEBUG ?= 1        # Enable debug builds (1=enabled, 0=disabled)
BUILD_RACE  ?= 1        # Enable race detector (1=enabled, 0=disabled)
BUILD_FLAGS ?=          # Additional Go build flags
BUILD_ENV   ?=          # Additional build environment variables

# Build output directories
BUILD_DIR     := bin
REPORTS_DIR   := reports

##
### Build metadata
##

# Build information (overridable)
BUILD          ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_VERSION  ?= $(shell git describe --always --tags 2>/dev/null || echo "dev")
BUILD_TIMEUTC  ?= $(shell date -u '+%Y/%m/%d-%H:%M:%S')
BUILD_GO_TAGS  ?=

BUILD_META := -X github.com/luma/flightrecorder/env.Build=${BUILD} \
              -X github.com/luma/flightrecorder/env.Branch=${BUILD_BRANCH} \
              -X github.com/luma/flightrecorder/env.Version=${BUILD_VERSION} \
              -X github.com/luma/flightrecorder/env.BuildTimeUTC=${BUILD_TIMEUTC} \
              -X github.com/luma/flightrecorder/env.GoTag=${BUILD_GO_TAGS}

##
### Configure build flags based on environment
##

# Debug mode configuration
ifeq ($(strip $(BUILD_DEBUG)),1)
	# Disable compiler optimizations to help with debugging
	BUILD_FLAGS += -ldflags '${BUILD_META}' -gcflags='all=-N -l'
else
	# Force rebuilding of packages and strip debug info
	BUILD_FLAGS += -a -ldflags '-w -s ${BUILD_META}'
endif

# Race detector configuration
ifeq ($(strip $(BUILD_RACE)),1)
	BUILD_FLAGS += -race
endif

# Add tags
ifneq ($(strip $(BUILD_GO_TAGS)),)
	BUILD_FLAGS += -tags=$(BUILD_GO_TAGS)
endif

# Find all Go source files, excluding vendor
SRC := $(shell find . -name '*.go' | grep -v "^./vendor/")

# Find all embedded files (SQL migrations).
# SPA assets are excluded because their hashed filenames change every build,
# which poisons Make's dependency graph when web-build runs before go build.
EMBEDDED := $(shell find db/schema/migrations -name '*.sql' 2>/dev/null)

##
### Make targets
##

.PHONY: all
all: build test lint

install: all
	install -m 755 $(BUILD_DIR)/$(BIN) ~/projects/bin/$(BIN)

.PHONY: dev-up
dev-up:
	docker compose up -d --force-recreate --wait postgres

.PHONY: dev-down
dev-down:
	docker compose down

.PHONY: dev-reset
dev-reset:
	docker compose down -v
	docker compose up -d --force-recreate --wait postgres

.PHONY: migrate-up
migrate-up:
	$(GO) run . migrate up

.PHONY: serve
serve:
	$(GO) run . serve

.PHONY: generate
generate:
	go generate ./...

.PHONY: build
build: fmt generate web-build $(BUILD_DIR)/$(BIN)

CGO_FLAG := CGO_ENABLED=0
ifeq ($(strip $(BUILD_RACE)),1)
	# Race detector requires CGO
	CGO_FLAG :=
endif

$(BUILD_DIR)/$(BIN): $(SRC) $(EMBEDDED) go.mod go.sum
	@mkdir -p $(BUILD_DIR)
	$(CGO_FLAG) $(BUILD_ENV) go build -v $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BIN)

.PHONY: test
test: go-test
	make -C godot test

.PHONY: go-test
go-test: web-build
	$(BUILD_ENV) go run github.com/onsi/ginkgo/v2/ginkgo \
                $(if $(filter 1,$(BUILD_RACE)),-race,) -r -p \
                --randomize-all --randomize-suites --fail-on-pending --keep-going \
                -ldflags '$(BUILD_META)' \
                -gcflags='all=-N -l' \
                $(if $(BUILD_GO_TAGS),-tags=$(BUILD_GO_TAGS),) \
                ./...

.PHONY: cover
cover: $(REPORTS_DIR)/coverage/coverage.out
	go tool cover -func=$(REPORTS_DIR)/coverage/coverage.out

.PHONY: cover-html
cover-html: $(REPORTS_DIR)/coverage/coverage.out
	go tool cover -html=$(REPORTS_DIR)/coverage/coverage.out

$(REPORTS_DIR)/coverage/coverage.out: $(SRC) $(EMBEDDED) go.mod go.sum
	@mkdir -p $(REPORTS_DIR)/coverage/
	go run github.com/onsi/ginkgo/v2/ginkgo -r -cover -covermode=count \
                -outputdir=$(REPORTS_DIR)/coverage/ -coverprofile=coverage.out.tmp \
                -v -ldflags '$(BUILD_META)' -gcflags='all=-N -l' \
                $(if $(BUILD_GO_TAGS),-tags=$(BUILD_GO_TAGS),) \
                ./...
	@cat $(REPORTS_DIR)/coverage/coverage.out.tmp | grep -v -e "mode: count" > $(REPORTS_DIR)/coverage/coverage.out
	@echo 'mode: count' | cat - $(REPORTS_DIR)/coverage/coverage.out > $(REPORTS_DIR)/coverage/coverage.out.tmp
	@mv $(REPORTS_DIR)/coverage/coverage.out.tmp $(REPORTS_DIR)/coverage/coverage.out
	@find . -name "coverage.out.tmp" -delete

.PHONY: fmt
fmt:
	@go run mvdan.cc/gofumpt@latest -l -w .

.PHONY: lint
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo >&2 "golangci-lint not found. Please install it first."; exit 1; }
	golangci-lint run -v

.PHONY: lint-prod
lint-prod:
	@command -v golangci-lint >/dev/null 2>&1 || { echo >&2 "golangci-lint not found. Please install it first."; exit 1; }
	golangci-lint run

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

## Code generation

sqlc:
	sqlc generate

## Web frontend

.PHONY: web-build
web-build:
	$(MAKE) -C web-vite build
	find api/spa -mindepth 1 ! -name embed.go ! -name .gitkeep -exec rm -rf {} +
	cp -r web-vite/dist/* api/spa/

# Clean generated files
.PHONY: clean-gen
clean-gen:
	find . -path "*/mocks/*.go" -delete
	find . -type d -name mocks -empty -delete

.PHONY: clean
clean: clean-gen
	rm -rf $(BUILD_DIR)/$(BIN) $(REPORTS_DIR)
