VERSION_TAG ?= $(shell if VER=`git describe --match "v[0-9]*" --tags 2>/dev/null`; then echo $VER; else echo "v0.0.1"; fi)
VERSION ?= $(VERSION_TAG:v%=%)
LDFLAGS := -ldflags "-X 'github.com/ghjm/connectopus/internal/version.version=$(VERSION)'"
GCFLAGS ?= -gcflags "all=-N -l"
OS ?= $(shell sh -c 'uname 2>/dev/null || echo Unknown')

PROGRAMS := connectopus
ifeq ($(OS),Linux)
PLUGINS := echo
endif

PLUGIN_TARGETS := $(foreach p,$(PLUGINS),plugins/$(p).so)

.PHONY: all
all: $(PROGRAMS) plugins

.PHONY: plugins
plugins: $(PLUGIN_TARGETS)

# go_deps finds all of the non-test/non-generated .go files under the
# current directory, which are in directories reported as being dependencies
# of the given go source file.
define go_deps
$(shell find $$(go list -f '{{.Dir}}' -deps $(1) | grep "^$$PWD") -name '*.go' | grep -v '_test.go$$' | grep -v '_gen.go$$')
endef

define PROGRAM_template
$(1): cmd/$(1).go Makefile $(PROGRAM_DEPS_$(1))
	go build -o $(1) $(LDFLAGS) $(GCFLAGS) cmd/$(1).go
endef
$(foreach p,$(PROGRAMS),$(eval PROGRAM_DEPS_$p := $(call go_deps,cmd/$(p).go)))
$(foreach p,$(PROGRAMS),$(eval $(call PROGRAM_template,$(p))))

define PLUGIN_template
plugins/$(1).so: plugins/$(1)/$(1).go Makefile $(PLUGIN_DEPS_$(1))
	go build -buildmode=plugin -o plugins/$(1).so $(GCFLAGS) plugins/$(1)/$(1).go
endef
$(foreach p,$(PLUGINS),$(eval PLUGIN_DEPS_$p := $(call go_deps,plugins/$(p)/$(p).go)))
$(foreach p,$(PLUGINS),$(eval $(call PLUGIN_template,$(p))))

.PHONY: lint
lint:
	@golangci-lint run --timeout 5m

.PHONY: fmt
fmt:
	@go fmt ./...

.PHONY: test
test:
	@go test ./... -count=1 -race

.PHONY: test-coverage
test-coverage:
	@go test -coverprofile cover.out ./...
	@go tool cover -html=cover.out -o coverage.html
	@rm -f cover.out
	@echo See coverage.html for details

.PHONY: cap-net-admin
cap-net-admin:
	@sudo setcap cap_net_admin+ep ./connectopus

.PHONY: version
version:
	@echo "$(VERSION)"

# The prod build target performs a build inside a standard golang build
# container which uses glibc 2.31.  The resulting binaries should be
# broadly compatible across Linux distributions.
PLATFORM ?= linux/amd64
.PHONY: prod
prod:
	@docker run -it --rm --platform $(PLATFORM) -v $$PWD:/work \
		--env BUILD_USER=$$(id -u) --env BUILD_GROUP=$$(id -g)\
		golang:1.18-bullseye bash -c \
		'cd /work && make clean && GCFLAGS="" make && chown $$BUILD_USER:$$BUILD_GROUP $(PROGRAMS) $(PLUGIN_TARGETS)'

# Because we use plugins and therefore cgo, building for arm64 isn't just
# regular go cross-compilation.  To enable arm64 emulation in Fedora x86-64,
# run: "dnf install qemu qemu-user-binfmt qemu-user-static".
.PHONY: prod-arm64
prod-arm64: PLATFORM=linux/arm64
prod-arm64: prod

.PHONY: clean
clean:
	@rm -fv $(PROGRAMS) $(PLUGIN_TARGETS) coverage.html

