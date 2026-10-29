.DEFAULT_GOAL := help
.PHONY: help release release-plan release-check release-build release-test
PYTHON ?= python3
VERSION ?= 0.1.0
REGISTRY ?= ghcr.io/dont-wait
GATV2_DIR ?= $(abspath ../GATv2)
BUILD_NETWORK ?= default
export VERSION REGISTRY GATV2_DIR GATV2_REF BUILD_NETWORK RELEASE_BUILD_ID

help:
	@printf '%s\n' \
	  'make release GATV2_REF=<commit>       Docker checks -> build -> smoke -> push GHCR images' \
	  'make release-plan GATV2_REF=<commit>  Print inputs/image plan; no Docker or push' \
	  'make release-check GATV2_REF=<commit> Run container checks only' \
	  'make release-build GATV2_REF=<commit> Checks/build/smoke; no push' \
	  'make release-test                    Unit tests for release orchestration' \
	  'Registry login required before release. See deploy/release/README.md.'

release:
	$(PYTHON) scripts/release/release.py

release-plan:
	$(PYTHON) scripts/release/release.py --dry-run

release-check:
	$(PYTHON) scripts/release/release.py --check-only

release-build:
	$(PYTHON) scripts/release/release.py --build-only

release-test:
	$(PYTHON) -m unittest discover -s scripts/release -p 'test_*.py' -v
