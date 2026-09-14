.DEFAULT_GOAL := help

.PHONY: help start

help:
	@printf '%s\n' \
	  'Chạy trong kyc-service/: make <lệnh>' \
	  '' \
	  '  start  Khởi động KYC server bằng Nix development shell'

start:
	nix develop .\#default --command bash -c 'set -e; python -m venv .venv; .venv/bin/python -m pip install -e .; APP_ENV=development VISION_PIPELINE_BACKEND=stub ALLOW_STUB_VISION_PIPELINE=true exec .venv/bin/python -m uvicorn app.main:app --reload --host 0.0.0.0 --port 8090'
