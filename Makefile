VERSION_TAG ?= $(shell if VER=`git describe --match "v[0-9]*" --tags 2>/dev/null`; then echo $VER; else echo "v0.0.1"; fi)
VERSION ?= $(VERSION_TAG:v%=%)
LDFLAGS := -ldflags "-X 'github.com/ghjm/connectopus/internal/version.version=$(VERSION)'"

PROGRAMS := connectopus
PLUGINS := echo

PLUGIN_TARGETS := $(foreach p,$(PLUGINS),plugins/$(p).so)

all: $(PROGRAMS) plugins

plugins: $(PLUGIN_TARGETS)

define PROGRAM_template
$(1): cmd/$(1).go Makefile $(EXTRA_DEPS_$(1)) $(shell find $(go list -f '{{.Dir}}' -deps cmd/$(1).go | grep "^$PWD") -name '*.go' | grep -v '_test.go$' | grep -v '_gen.go$')
	go build -o $(1) $(LDFLAGS) cmd/$(1).go
endef

$(foreach p,$(PROGRAMS),$(eval $(call PROGRAM_template,$(p))))

define PLUGIN_template
plugins/$(1).so: plugins/$(1)/$(1).go Makefile $(EXTRA_DEPS_$(1)) $(shell find $(go list -f '{{.Dir}}' -deps plugins/$(1)/$(1).go | grep "^$PWD") -name '*.go' | grep -v '_test.go$' | grep -v '_gen.go$')
	go build -buildmode=plugin -o plugins/$(1).so plugins/$(1)/$(1).go
endef

$(foreach p,$(PLUGINS),$(eval $(call PLUGIN_template,$(p))))

lint:
	@golangci-lint run --timeout 5m

fmt:
	@go fmt ./...

test:
	@go test ./... -count=1 -race

test-coverage:
	@go test -coverprofile cover.out ./...
	@go tool cover -html=cover.out -o coverage.html
	@rm -f cover.out
	@echo See coverage.html for details

cap-net-admin:
	@sudo setcap cap_net_admin+ep ./connectopus

version:
	@echo "$(VERSION)"

clean:
	@rm -fv $(PROGRAMS) $(PLUGIN_TARGETS) coverage.html

.PHONY: all lint fmt test test-coverage version clean cap-net-admin plugins

