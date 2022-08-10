ifeq ($(OS),Windows_NT)
	# choco install -y make git.install grep findutils gnuwin32-coreutils.install
	FIND := "C:\ProgramData\chocolatey\lib\findutils\tools\install\bin\find.exe"
	TOUCH := "C:\Program Files (x86)\GnuWin32\bin\touch.exe"
	MKDIR := "C:\Program Files (x86)\GnuWin32\bin\mkdir.exe"
	RM := "C:\Program Files (x86)\GnuWin32\bin\rm.exe"
	MAKE := "C:\ProgramData\chocolatey\lib\make.exe"
	GREP := "C:\ProgramData\chocolatey\bin\grep.exe"
	GIT := "C:\Program Files\Git\bin\git.exe"
else
	FIND := find
	TOUCH := touch
	MKDIR := mkdir
	MAKE := make
	GREP := grep
	GIT := git
endif


UI_DEP := ../internal/ui_embed/embed/dist/main.bundle.js
NPM_DEP := node_modules/.package_lock.json

$(UI_DEP): $(NPM_DEP) *.js $(shell $(FIND) src -type f)
	@npm version --allow-same-version $(VERSION) && npm run build

$(NPM_DEP): package.json package-lock.json
	@npm ci --legacy-peer-deps
	@$(TOUCH) $(NPM_DEP)

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
	@$(RM) -rf ../internal/ui_embed/embed/dist/*

.PHONY: distclean
distclean:
	@$(RM) -rfv node_modules

