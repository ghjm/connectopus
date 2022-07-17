VERSION_TAG ?= $(shell if VER=`git describe --match "v[0-9]*" --tags 2>/dev/null`; then echo $VER; else echo "v0.0.1"; fi)
VERSION ?= $(VERSION_TAG:v%=%)
LDFLAGS := -ldflags "-X 'github.com/ghjm/connectopus/internal/version.version=$(VERSION)'"
BUILDENV ?= CGO_ENABLED=0

PROGRAMS := connectopus
PLATFORMS := linux:amd64: linux:arm64: windows:amd64:.exe windows:arm64:.exe darwin:amd64: darwin:arm64:
UI_DEP := internal/ui_embed/embed/dist/main.bundle.js
EXTRA_DEPS_connectopus := $(UI_DEP)

.PHONY: all
all: update-version $(PROGRAMS) $(UI_DEP)

.PHONY: update-version
update-version:
	@if [ "$$(cat ui/package.json | jq .version)" != "\"$(VERSION)\"" ]; then cd ui && npm version $(VERSION) --allow-same-version; fi

# go_deps finds all of the non-test/non-generated .go files under the
# current directory, which are in directories reported as being dependencies
# of the given go source file.
define go_deps
$(shell find $$(go list -f '{{.Dir}}' -deps $(1) | grep "^$$PWD") -name '*.go' | grep -v '_test.go$$' | grep -v '_gen.go$$')
endef

define PROGRAM_template
$(2)$(1)$(3): cmd/$(1)/$(1).go Makefile $(PROGRAM_DEPS_$(1))
	$(4) go build -o $(2)$(1)$(3) $(LDFLAGS) cmd/$(1)/$(1).go
endef
$(foreach p,$(PROGRAMS),$(eval PROGRAM_DEPS_$p := $(call go_deps,cmd/$(p)/$(p).go)))
$(foreach p,$(PROGRAMS),$(eval PROGRAM_DEPS_$p += $(EXTRA_DEPS_$p)))
$(foreach p,$(PROGRAMS),$(eval $(call PROGRAM_template,$(p),,,$(BUILDENV))))

define PLATFORM_template
$(foreach p,$(PROGRAMS),$(eval BINFILES += bin/$(p)-$(1)-$(2)$(3))) 
$(foreach p,$(PROGRAMS),$(eval $(call PROGRAM_template,$(p),bin/,-$(1)-$(2)$(3),$(BUILDENV) GOOS=$(1) GOARCH=$(2))))
endef
$(foreach a,$(PLATFORMS),$(eval $(call PLATFORM_template,$(word 1,$(subst :, ,$(a))),$(word 2,$(subst :, ,$(a))),$(word 3,$(subst :, ,$(a))))))

.PHONY: bin
bin: $(BINFILES)

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
	@go test ./... -count=1 -race -parallel=16 -timeout=2m

.PHONY: testloop
testloop:
	@i=1; while echo "-------------------------- $$i" && make test; do i=$$((i+1)); done

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
cap-net-admin: connectopus
	@sudo setcap cap_net_admin+ep ./connectopus

.PHONY: ctun
ctun: connectopus
	@sudo ./connectopus setup-tunnel --config test.yml --id foo

.PHONY: version
version:
	@echo "$(VERSION)"

.PHONY: ui
ui: $(UI_DEP)

$(UI_DEP): ui/package.json ui/package-lock.json ui/*.js $(shell find ui/src -type f)
	@cd ui && make ui

.PHONY: ui-dev
ui-dev:
	@cd ui && npm run dev

bin: $(PROGRAM_DEPS_connectopus) $(EXTRA_DEPS_connectopus)
	@mkdir -p bin
	@touch bin

.PHONY: clean
clean:
	@rm -fv $(PROGRAMS) $(BINFILES) coverage.html
	@find . -name Makefile -and -not -path ./Makefile -and -not -path './ui/node_modules/*' -execdir make clean --no-print-directory \;

.PHONY: distclean
distclean: clean
	@find . -name Makefile -and -not -path ./Makefile -and -not -path './ui/node_modules/*' -execdir make distclean --no-print-directory \;

