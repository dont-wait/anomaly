import sys
import types

import pytest


class FakeHistory:
    def __init__(self):
        self.closed = False
        self.event_ids = []

    def read_request_and_prefix(self, event_id):
        self.event_ids.append(event_id)
        return {"eventId": event_id}, [{"eventId": "earlier"}]

    def close(self):
        self.closed = True


def fake_fraud_pipeline(monkeypatch, *, dynamic_accounts=False):
    calls = {}

    class FakeReplayPredictor:
        account_identity_policy = (
            "distinct-account-nodes-v1" if dynamic_accounts else "bundle-catalog"
        )

        def __init__(self, bundle, *, device="cpu"):
            calls["bundle"] = bundle
            calls["device"] = device
            self.nodes = [
                {"id": identity, "type": "account"}
                for identity in ["acc-uuid-1", "acc-uuid-2", "custom-uuid"]
            ]

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
        account_id_map={
            "known-dest": "custom-uuid",
            "6623a1f89c30111111111111": "acc-uuid-1",
            "6623a1f89c30222222222222": "acc-uuid-2",
        },
    )
    result = predictor.predict("evt-123")

    assert result.request["payload"]["sourceAccountId"] == "6623a1f89c30111111111111"
    model_req = calls["request"]
    assert model_req["payload"]["sourceAccountId"] == "acc-uuid-1"
    assert model_req["payload"]["destinationAccountId"] == "acc-uuid-2"
    assert calls["history"][0]["payload"]["destinationAccountId"] == "custom-uuid"


@pytest.mark.parametrize("identity", ["00000000000000000000002c", "00000000000000000000004e"])
def test_unknown_accounts_are_rejected_before_model_inference(monkeypatch, identity):
    calls = fake_fraud_pipeline(monkeypatch)
    from app.predictor import LocalBundlePredictor, UnsupportedIdentityError

    class History:
        def read_request_and_prefix(self, event_id):
            return {
                "payload": {"sourceAccountId": identity, "destinationAccountId": "acc-uuid-1"}
            }, []

    predictor = LocalBundlePredictor("bundle", History())
    with pytest.raises(UnsupportedIdentityError, match="not supported"):
        predictor.predict("request")
    assert "request" not in calls


@pytest.mark.parametrize(
    "mapping",
    [
        {"a": "acc-uuid-1", "b": "acc-uuid-1"},
        {"a": "__unknown__"},
        {"a": "not-in-bundle"},
        {"acc-uuid-1": "acc-uuid-2"},
    ],
)
def test_invalid_explicit_account_mapping_is_rejected(monkeypatch, mapping):
    fake_fraud_pipeline(monkeypatch)
    from app.predictor import LocalBundlePredictor, UnsupportedIdentityError

    with pytest.raises(UnsupportedIdentityError):
        LocalBundlePredictor("bundle", FakeHistory(), account_id_map=mapping)


def test_explicit_alias_cannot_collapse_with_native_account_in_history(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch)
    from app.predictor import LocalBundlePredictor, UnsupportedIdentityError

    class History:
        def read_request_and_prefix(self, event_id):
            return (
                {"payload": {"sourceAccountId": "alias", "destinationAccountId": "acc-uuid-2"}},
                [{"payload": {"sourceAccountId": "acc-uuid-1"}}],
            )

    predictor = LocalBundlePredictor("bundle", History(), account_id_map={"alias": "acc-uuid-1"})
    with pytest.raises(UnsupportedIdentityError, match="merges distinct"):
        predictor.predict("request")
    assert "request" not in calls


def test_native_bundle_accounts_preserve_request_and_history(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch)
    from app.predictor import LocalBundlePredictor

    request = {"payload": {"sourceAccountId": "acc-uuid-1", "destinationAccountId": "acc-uuid-2"}}
    prefix = [{"payload": {"sourceAccountId": "acc-uuid-2", "destinationAccountId": "acc-uuid-1"}}]

    class History:
        def read_request_and_prefix(self, event_id):
            return request, prefix

    result = LocalBundlePredictor("bundle", History()).predict("request")
    assert calls["request"] == request
    assert calls["history"] == prefix
    assert result.request is request


def test_ip_retained_for_training_but_ip_and_prior_risk_never_enter_bundle(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch)
    from conftest import request_event_bytes

    from app.contracts import build_training_record
    from app.events import select_trigger_event
    from app.predictor import LocalBundlePredictor

    request = {
        "eventId": "request", "occurredAt": "2026-10-19T10:30:00Z",
        "payload": {"sourceAccountId": "acc-uuid-1", "destinationAccountId": "acc-uuid-2",
                    "ip_address": "203.0.113.10", "device_id": "device-1"},
    }
    prefix = [{"payload": {
        "sourceAccountId": "acc-uuid-2", "destinationAccountId": "acc-uuid-1",
        "ip_address": "198.51.100.1", "riskAssessment": {"risk_score": 0.99},
    }}]

    class History:
        def read_request_and_prefix(self, event_id):
            return request, prefix

    result = LocalBundlePredictor("bundle", History()).predict("request")
    assert calls["request"]["payload"] == {
        "sourceAccountId": "acc-uuid-1", "destinationAccountId": "acc-uuid-2",
        "device_id": "device-1",
    }
    assert calls["history"][0]["payload"] == {
        "sourceAccountId": "acc-uuid-2", "destinationAccountId": "acc-uuid-1",
    }
    training = build_training_record(
        select_trigger_event(request_event_bytes()), result.request, result.prediction
    )
    assert training["request"]["payload"]["ip_address"] == "203.0.113.10"
    assert prefix[0]["payload"]["riskAssessment"]["risk_score"] == 0.99


def test_dynamic_account_predictor_receives_distinct_original_objectids(monkeypatch):
    calls = fake_fraud_pipeline(monkeypatch, dynamic_accounts=True)
    from app.predictor import LocalBundlePredictor

    request = {"payload": {
        "sourceAccountId": "6623a1f89c30111111111111",
        "destinationAccountId": "6623a1f89c30222222222222",
    }}

    class History:
        def read_request_and_prefix(self, event_id):
            return request, []

    result = LocalBundlePredictor("bundle", History()).predict("request")
    assert calls["request"] == request
    assert result.request == request
    assert calls["request"]["payload"]["sourceAccountId"] != calls["request"]["payload"][
        "destinationAccountId"
    ]
