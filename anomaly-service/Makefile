.DEFAULT_GOAL := help

PYTHON ?= python3
VENV ?= .venv
PIP := $(VENV)/bin/python -m pip
RUFF := $(shell command -v ruff 2>/dev/null || printf '%s' '$(VENV)/bin/ruff')

.PHONY: help install start test lint check

help:
	@printf '%s\n' \
	  'Chạy trong anomaly-service/: make <lệnh>' \
	  '' \
	  '  install  Tạo .venv và cài dependencies (kèm dev tools)' \
	  '  start    Chạy ML worker với biến môi trường từ .env' \
	  '  test     Chạy pytest' \
	  '  lint     Chạy ruff' \
	  '  check    Chạy lint rồi test'

install:
	@if [ ! -x $(VENV)/bin/python ]; then rm -rf $(VENV); $(PYTHON) -m venv $(VENV); fi
	$(PIP) install --upgrade pip
	$(PIP) install -e '.[dev]'

start:
	@test -x $(VENV)/bin/python || $(MAKE) install
	@test -f .env || { echo 'Thiếu .env. Chạy: cp .env.example .env'; exit 1; }
	@set -a; . ./.env; set +a; exec $(VENV)/bin/python -m app

test:
	@test -x $(VENV)/bin/python || $(MAKE) install
	$(VENV)/bin/python -m pytest -q

lint:
	@test -x $(VENV)/bin/python || $(MAKE) install
	@test -x $(RUFF) || $(MAKE) install
	$(RUFF) check app tests scripts

check: lint test
