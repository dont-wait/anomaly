"""Opt-in real weights test; no DB, broker, HTTP, or training involved."""

import os
from pathlib import Path

import pytest

from app.predictor import LocalBundlePredictor


@pytest.mark.skipif(
    os.getenv("RUN_GATV2_BUNDLE_TEST") != "1",
    reason="set RUN_GATV2_BUNDLE_TEST=1 with PyTorch/NumPy/PyG available",
)
def test_real_bundle_scores_distinct_new_objectids_and_preserves_ip():
    source = os.getenv("GATV2_SRC_PATH", "../../GATv2/src")
    bundle = Path(__file__).resolve().parents[1] / "models/gatv2-1m-baseline-v1"
    request = {
        "eventId": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
        "transactionId": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "eventType": "TransferCreated", "schemaVersion": 1, "sequence": 1,
        "occurredAt": "2026-10-29T00:00:00Z",
        "payload": {
            "sourceAccountId": "6623a1f89c30111111111111",
            "destinationAccountId": "6623a1f89c30222222222222",
            "amount": 1000000, "fee": 0, "currency": "VND", "channel": "mobile",
            "status": "awaiting_otp", "device_id": "synthetic-device",
            "platform": "android", "authentication_verified": False,
            "ip_address": "203.0.113.10",
        },
    }

    class History:
        def read_request_and_prefix(self, event_id):
            return request, []

    predictor = LocalBundlePredictor(bundle, History(), gatv2_src_path=source)
    try:
        assert predictor.supports_dynamic_accounts
        context = predictor.predictor.inputs(predictor._map_event(request), [])
        source_node, destination_node = context["target_edge_index"][:, 0]
        assert source_node != 0 and destination_node != 0 and source_node != destination_node
        result = predictor.predict(request["eventId"])
        assert result.prediction.transaction_id == request["transactionId"]
        assert 0 <= result.prediction.risk_score <= 1
        assert result.request["payload"]["ip_address"] == "203.0.113.10"
    finally:
        predictor.close()
