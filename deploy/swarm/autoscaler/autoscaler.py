import json
import os
import time
import urllib.parse
import urllib.request

import docker


PROMETHEUS_URL = os.getenv("PROMETHEUS_URL", "http://prometheus:9090")
SERVICE_NAME = os.environ["ML_SERVICE_NAME"]
GROUP = os.getenv("KAFKA_CONSUMER_GROUP", "ml-pipeline")
TOPIC = os.getenv("KAFKA_INPUT_TOPIC", "anomaly.bank.events.v1")
MIN_REPLICAS = int(os.getenv("ML_MIN_REPLICAS", "1"))
MAX_REPLICAS = int(os.getenv("ML_MAX_REPLICAS", "4"))
SCALE_UP_LAG = float(os.getenv("ML_SCALE_UP_LAG", "1000"))
SCALE_DOWN_LAG = float(os.getenv("ML_SCALE_DOWN_LAG", "100"))
POLL_SECONDS = int(os.getenv("AUTOSCALER_POLL_SECONDS", "30"))
COOLDOWN_SECONDS = int(os.getenv("AUTOSCALER_COOLDOWN_SECONDS", "120"))


def lag():
    query = (
        'sum(kafka_consumergroup_lag{consumergroup="%s",topic="%s"})'
        % (GROUP, TOPIC)
    )
    url = PROMETHEUS_URL.rstrip("/") + "/api/v1/query?" + urllib.parse.urlencode({"query": query})
    with urllib.request.urlopen(url, timeout=10) as response:
        payload = json.load(response)
    result = payload.get("data", {}).get("result", [])
    if not result:
        return None
    return float(result[0]["value"][1])


def replicas(client):
    service = client.services.get(SERVICE_NAME)
    return service, int(service.attrs["Spec"]["Mode"]["Replicated"]["Replicas"])


def main():
    client = docker.from_env()
    last_scale = 0.0
    while True:
        try:
            current_lag = lag()
            service, current = replicas(client)
            if current_lag is None or time.time() - last_scale < COOLDOWN_SECONDS:
                time.sleep(POLL_SECONDS)
                continue
            desired = current
            if current_lag > SCALE_UP_LAG:
                desired = min(MAX_REPLICAS, current + 1)
            elif current_lag < SCALE_DOWN_LAG:
                desired = max(MIN_REPLICAS, current - 1)
            if desired != current:
                service.scale(desired)
                last_scale = time.time()
                print(f"scaled {SERVICE_NAME}: lag={current_lag:.0f} replicas={current}->{desired}", flush=True)
        except Exception as error:
            print(f"autoscaler check failed: {error}", flush=True)
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    main()
