VERSION_TAG ?= $(shell if VER=`git describe --match "v[0-9]*" --tags 2>/dev/null`; then echo $VER; else echo "v0.0.1"; fi)
VERSION ?= $(VERSION_TAG:v%=%)
LDFLAGS := -ldflags "-X 'github.com/ghjm/connectopus/internal/version.version=$(VERSION)'"

PROGRAMS := connectopus cpctl
UI_DEP := internal/ui_embed/embed/dist/main.bundle.js
NPM_DEP := ui/node_modules/rimraf/rimraf.js
EXTRA_DEPS_connectopus := $(UI_DEP) $(NPM_DEP)

.PHONY: all
all: $(PROGRAMS) $(UI_DEP)

# go_deps finds all of the non-test/non-generated .go files under the
# current directory, which are in directories reported as being dependencies
# of the given go source file.
define go_deps
$(shell find $$(go list -f '{{.Dir}}' -deps $(1) | grep "^$$PWD") -name '*.go' | grep -v '_test.go$$' | grep -v '_gen.go$$')
endef

define PROGRAM_template
$(1): cmd/$(1)/$(1).go Makefile $(PROGRAM_DEPS_$(1))
	go build -o $(1) $(LDFLAGS) $(GCFLAGS) cmd/$(1)/$(1).go
endef
$(foreach p,$(PROGRAMS),$(eval PROGRAM_DEPS_$p := $(call go_deps,cmd/$(p)/$(p).go)))
$(foreach p,$(PROGRAMS),$(eval PROGRAM_DEPS_$p += $(EXTRA_DEPS_$p)))
$(foreach p,$(PROGRAMS),$(eval $(call PROGRAM_template,$(p))))

.PHONY: gen
gen:
	@go generate ./...

.PHONY: lint
lint:
	@golangci-lint run --timeout 5m
	@cd ui && npm run --silent lint

.PHONY: fmt
fmt:
	@go fmt ./...
	@cd ui && npm run --silent format

.PHONY: test
test:
	@go test ./... -count=1 -race

.PHONY: test-root
test-root: connectopus
	@sudo GOPATH=$$HOME/go $$(which go) test ./... -test.run 'TestAsRoot*' -count=1 -race

.PHONY: test-coverage
test-coverage:
	@go test -coverprofile cover.out ./...
	@go tool cover -html=cover.out -o coverage.html
	@rm -f cover.out
	@echo See coverage.html for details

.PHONY: cap-net-admin
cap-net-admin:
	@sudo setcap cap_net_admin+ep ./connectopus

.PHONY: ctun
CTUN_NAME ?= ctun
CTUN_ADDR ?= FD00::1:2
CTUN_NET ?= FD00::0/8
ctun:
	@sudo bash -c ' \
		ip tuntap del dev $(CTUN_NAME) mode tun && \
		ip tuntap add dev $(CTUN_NAME) mode tun user $$SUDO_UID group $$SUDO_GID && \
		ip addr add dev $(CTUN_NAME) $(CTUN_ADDR) && \
		ip link set $(CTUN_NAME) up && \
		ip route add $(CTUN_NET) dev $(CTUN_NAME) src $(CTUN_ADDR) \
		'

.PHONY: version
version:
	@echo "$(VERSION)"

$(NPM_DEP):
	@cd ui && npm ci --legacy-peer-deps

.PHONY: ui
ui: $(UI_DEP)

$(UI_DEP): $(NPM_DEP) ui/package.json ui/package-lock.json ui/*.js $(shell find ui/src -type f)
	@cd ui && npm version --allow-same-version $(VERSION) && npm run build

.PHONY: ui-dev
ui-dev:
	@cd ui && npm run dev

# The prod build target performs a build inside a standard golang build
# container which uses glibc 2.31.  The resulting binaries should be
# broadly compatible across Linux distributions.
PLATFORM ?= linux/amd64
.PHONY: prod
prod:
	@docker run -it --rm --platform $(PLATFORM) -v $$PWD:/work \
		--env BUILD_USER=$$(id -u) --env BUILD_GROUP=$$(id -g)\
		golang:1.18-bullseye bash -c \
		'cd /work && make clean && GCFLAGS="" make && chown $$BUILD_USER:$$BUILD_GROUP $(PROGRAMS)'

# Because we use plugins and therefore cgo, building for arm64 isn't just
# regular go cross-compilation.  To enable arm64 emulation in Fedora x86-64,
# run: "dnf install qemu qemu-user-binfmt qemu-user-static".
.PHONY: prod-arm64
prod-arm64: PLATFORM=linux/arm64
prod-arm64: prod

.PHONY: clean
clean:
	@rm -fv $(PROGRAMS) coverage.html
	@rm -rf internal/ui_embed/embed/dist/*
	@find . -name Makefile -and -not -path ./Makefile -and -not -path './ui/node_modules/*' -execdir make clean --no-print-directory \;

.PHONY: distclean
distclean: clean
	@rm -rf ui/node_modules

