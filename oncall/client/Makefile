PATH := $(PATH):$(VANADIUM_ROOT)/environment/cout/node/bin
PATH := bin:node_modules/.bin:$(PATH)
SHELL := /bin/bash -euo pipefail

js_files := $(shell find browser -name "*.js")
css_files := $(shell find stylesheets -name "*.css")
host ?= 0.0.0.0
port ?= 8088

.DEFAULT_GOAL := all

.PHONY: all
all: public/bundle.js public/bundle.css
	@true  # silences `watch make`

.DELETE_ON_ERROR:
public/bundle.js: browser/index.js $(js_files) node_modules
	browserify --debug $< 1> $@

.DELETE_ON_ERROR:
public/bundle.css: stylesheets/index.css $(css_files) node_modules
	bin/compile-css $< 1> $@

node_modules: package.json
	@npm prune
	@npm install
	@touch $@

.PHONY: start
start: all
	http-server public -p $(port) -a $(host) -d --cors

.PHONY: clean
clean:
	@$(RM) -rf public/bundle.*
	@$(RM) -rf node_modules
	@$(RM) -rf npm-debug.log

.PHONY: lint
lint:
	@jshint .
