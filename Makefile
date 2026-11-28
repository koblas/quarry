# The documentation site (mkdocs.yml, docs/). Go builds, tests and lint run
# directly or through .claude/scripts/verify.sh; this file holds only what
# needs a tool outside devenv: Zensical, run through uv's uvx.

ZENSICAL_VERSION ?= 0.0.70
ZENSICAL := uvx zensical==$(ZENSICAL_VERSION)

.PHONY: docs
docs: docs-test docs-build ## Check the reference pages, then build the site

.PHONY: docs-build
docs-build: ## Build the documentation site to out/site, failing on broken links (as docs.yml)
	$(ZENSICAL) build --strict --clean

.PHONY: docs-test
docs-test: ## Check the reference pages cover every command, flag and config key
	go test ./docs ./internal/config

.PHONY: docs-serve
docs-serve: ## Preview the documentation site with live reload
	$(ZENSICAL) serve
