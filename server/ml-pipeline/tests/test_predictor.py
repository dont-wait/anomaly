import sys
import types


class FakeHistory:
    def __init__(self):
        self.closed = False
        self.event_ids = []

    def read_request_and_prefix(self, event_id):
        self.event_ids.append(event_id)
        return {"eventId": event_id}, [{"eventId": "earlier"}]

    def close(self):
        self.closed = True


def fake_fraud_pipeline(monkeypatch):
    calls = {}

    class FakeReplayPredictor:
        def __init__(self, bundle, *, device="cpu"):
            calls["bundle"] = bundle
            calls["device"] = device

        def predict(self, request, history):
            calls["request"] = request
            calls["history"] = history
            return {
                "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "risk_score": 0.9,
                "predicted_label": 1,
                "threshold": 0.5,
                "model_version": "test-model",
                "feature_version": "test-features",
            }

    package = types.ModuleType("fraud_pipeline")
    package.__path__ = []
    inference = types.ModuleType("fraud_pipeline.inference")
    inference.__path__ = []
    predictor = types.ModuleType("fraud_pipeline.inference.predictor")
    predictor.ReplayPredictor = FakeReplayPredictor
    monkeypatch.setitem(sys.modules, "fraud_pipeline", package)
    monkeypatch.setitem(sys.modules, "fraud_pipeline.inference", inference)
    monkeypatch.setitem(sys.modules, "fraud_pipeline.inference.predictor", predictor)
    return calls


def test_local_bundle_predictor_uses_history_and_bundle(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch)
    history = FakeHistory()
    from app.predictor import LocalBundlePredictor

    predictor = LocalBundlePredictor("/models/bundle", history, device="cuda")
    result = predictor.predict("event-1")
    predictor.close()

    assert calls["bundle"] == "/models/bundle"
    assert calls["device"] == "cuda"
    assert calls["request"] == {"eventId": "event-1"}
    assert result.request == {"eventId": "event-1"}
    assert result.prediction.transaction_id == "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    assert history.event_ids == ["event-1"]
    assert history.closed is True


def test_local_bundle_predictor_rejects_missing_source_dir(monkeypatch, tmp_path):
    fake_fraud_pipeline(monkeypatch)
    from app.predictor import LocalBundlePredictor

    missing = tmp_path / "missing"
    try:
        LocalBundlePredictor("/models/bundle", FakeHistory(), gatv2_src_path=str(missing))
    except ValueError as error:
        assert "source path" in str(error)
    else:
        raise AssertionError("missing GATv2 source directory was accepted")


def test_local_bundle_predictor_maps_account_ids(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch)

    class HistoryWithAccounts:
        def read_request_and_prefix(self, event_id):
            req = {
                "eventId": event_id,
                "payload": {
                    "sourceAccountId": "6623a1f89c30111111111111",
                    "destinationAccountId": "6623a1f89c30222222222222",
                },
            }
            prefix = [
                {
                    "eventId": "prev",
                    "payload": {
                        "sourceAccountId": "6623a1f89c30111111111111",
                        "destinationAccountId": "known-dest",
                    },
                }
            ]
            return req, prefix

    from app.predictor import LocalBundlePredictor

    predictor = LocalBundlePredictor(
        "/models/bundle",
        HistoryWithAccounts(),
        account_id_map={"known-dest": "custom-uuid"},
    )
    predictor.account_node_ids = ["acc-uuid-1", "acc-uuid-2"]
    result = predictor.predict("evt-123")

    assert result.request["payload"]["sourceAccountId"] == "6623a1f89c30111111111111"
    model_req = calls["request"]
    assert model_req["payload"]["sourceAccountId"] in ["acc-uuid-1", "acc-uuid-2"]
    assert calls["history"][0]["payload"]["destinationAccountId"] == "custom-uuid"
