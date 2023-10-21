UI_DEP := ../internal/ui_embed/embed/dist/main.bundle.js
NPM_DEP := node_modules/.package_lock.json

UI_SRC := $(shell find src -type f)
$(UI_DEP): $(NPM_DEP) *.js $(UI_SRC)
	@npm version --allow-same-version $(VERSION)
	@npm run build

$(NPM_DEP): package.json package-lock.json
	@npm ci --legacy-peer-deps
	@touch $(NPM_DEP)

.PHONY: ui
ui: $(UI_DEP)


.PHONY: lint
lint:
	@npm run --silent lint

.PHONY: fmt
fmt:
	@npm run --silent format

.PHONY: ui-dev
ui-dev:
	@npm run dev

.PHONY: clean
clean:
	@rm -rfv ../internal/ui_embed/embed/dist/*

.PHONY: distclean
distclean:
	@rm -rfv node_modules
