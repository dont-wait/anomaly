.DEFAULT_GOAL := help
.PHONY: help release release-check release-test
PYTHON ?= python3
VERSION ?= 0.1.0
APP_PLATFORM ?= linux
export VERSION

help:
	@printf '%s\n' \
	  'make release APP_PLATFORM=linux|windows|android   Build user installers; no upload' \
	  'make release-check APP_PLATFORM=...              Validate version/endpoint/signing inputs' \
	  'make release-test                               Test installer orchestration' \
	  'Set VITE_API_ENDPOINT=https://...; see docs/release-apps.md.'

release:
	$(PYTHON) scripts/release/apps.py --platform $(APP_PLATFORM)

release-check:
	$(PYTHON) scripts/release/apps.py --platform $(APP_PLATFORM) --check-only

release-test:
	$(PYTHON) -m unittest discover -s scripts/release -p 'test_apps.py' -v
