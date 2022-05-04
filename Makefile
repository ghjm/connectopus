VERSION_TAG ?= $(shell if VER=`git describe --match "v[0-9]*" --tags 2>/dev/null`; then echo $VER; else echo "v0.0.1"; fi)
VERSION ?= $(VERSION_TAG:v%=%)
LDFLAGS := -ldflags "-X 'github.com/ghjm/connectopus/internal/version.version=$(VERSION)'"

PROGRAMS := connectopus

all: $(PROGRAMS)

define PROGRAM_template
$(1): cmd/$(1).go Makefile $(EXTRA_DEPS_$(1)) $(shell find $(go list -f '{{.Dir}}' -deps cmd/$(1).go | grep "^$PWD") -name '*.go' | grep -v '_test.go$' | grep -v '_gen.go$')
	CGO_ENABLED=0 go build -o $(1) $(LDFLAGS) cmd/$(1).go
endef

$(foreach prog,$(PROGRAMS),$(eval $(call PROGRAM_template,$(prog))))

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

version:
	@echo "$(VERSION)"

clean:
	@rm -fv $(PROGRAMS) coverage.html

.PHONY: all lint fmt test test-coverage version clean

