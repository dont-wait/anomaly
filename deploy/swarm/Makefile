SHELL := /bin/sh

STACK ?= ml
ML_SERVICE ?= $(STACK)_ml-service
AUTOSCALER_SERVICE ?= $(STACK)_ml-autoscaler
SCALE_UP_TO ?= 2
SCALE_DOWN_TO ?= 1
TARGET_URL ?= http://localhost:19100/metrics
K6_VUS ?= 10
K6_DURATION ?= 30s

.PHONY: help run-server run-autoscaler test-request scale-up scale-down services logs-autoscaler

help:
	@printf '%s\n' \
	  'make -C deploy/swarm <target>' \
	  '' \
	  '  run-server       Start local API/infrastructure Compose stack' \
	  '  run-autoscaler   Follow the Swarm autoscaler logs' \
	  '  test-request     Run k6 against TARGET_URL' \
	  '  scale-up         Scale ML service to SCALE_UP_TO replicas' \
	  '  scale-down       Scale ML service to SCALE_DOWN_TO replicas' \
	  '  services         Show Swarm services in STACK' \
	  '  logs-autoscaler  Follow autoscaler service logs'

run-server:
	cd ../../server && ./dev-up.sh

run-autoscaler:
	docker service logs -f $(AUTOSCALER_SERVICE)

test-request:
	command -v k6 >/dev/null 2>&1 || { echo 'k6 is required'; exit 1; }
	k6 run --vus $(K6_VUS) --duration $(K6_DURATION) -e TARGET_URL=$(TARGET_URL) k6/request.js

scale-up:
	docker service scale $(ML_SERVICE)=$(SCALE_UP_TO)

scale-down:
	docker service scale $(ML_SERVICE)=$(SCALE_DOWN_TO)

services:
	docker stack services $(STACK)

logs-autoscaler:
	docker service logs --tail 100 $(AUTOSCALER_SERVICE)
