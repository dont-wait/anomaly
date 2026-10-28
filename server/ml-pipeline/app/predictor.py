"""Local GATv2 bundle inference; no HTTP model service required."""
import hashlib
import sys
from copy import deepcopy
from pathlib import Path

from .contracts import Prediction, PredictorResult


class LocalBundlePredictor:
    def __init__(self, bundle_path, history, *, gatv2_src_path=None, device="cpu", account_id_map=None):
        if not bundle_path:
            raise ValueError("model bundle path is required")
        if history is None:
            raise ValueError("history reader is required")
        if gatv2_src_path:
            source = Path(gatv2_src_path).expanduser().resolve()
            if not source.is_dir():
                raise ValueError(f"GATv2 source path does not exist: {source}")
            if str(source) not in sys.path:
                sys.path.insert(0, str(source))
        try:
            from fraud_pipeline.inference.predictor import ReplayPredictor
        except ImportError as error:
            raise RuntimeError(
                "fraud_pipeline is not importable; set GATV2_SRC_PATH or install the GATv2 package"
            ) from error
        self.history = history
        self.predictor = ReplayPredictor(bundle_path, device=device)
        self.account_id_map = dict(account_id_map) if account_id_map else {}
        self.account_node_ids = [
            n["id"] for n in getattr(self.predictor, "nodes", [])
            if n.get("type") == "account" and n.get("id") != "__unknown__"
        ]
        self.known_node_ids = set(getattr(self.predictor, "mapping", {}).keys())

    def _map_account_id(self, account_id):
        if not account_id or not isinstance(account_id, str):
            return account_id
        if account_id in self.known_node_ids:
            return account_id
        if account_id in self.account_id_map:
            return self.account_id_map[account_id]
        if self.account_node_ids:
            idx = int(hashlib.sha256(account_id.encode("utf-8")).hexdigest(), 16) % len(self.account_node_ids)
            return self.account_node_ids[idx]
        return account_id

    def _map_event(self, event):
        if not isinstance(event, dict) or "payload" not in event:
            return event
        payload = event.get("payload")
        if not isinstance(payload, dict):
            return event
        src = payload.get("sourceAccountId")
        dst = payload.get("destinationAccountId")
        if not src and not dst:
            return event
        mapped = deepcopy(event)
        if src:
            mapped["payload"]["sourceAccountId"] = self._map_account_id(src)
        if dst:
            mapped["payload"]["destinationAccountId"] = self._map_account_id(dst)
        return mapped

    def predict(self, event_id):
        request, prefix = self.history.read_request_and_prefix(event_id)
        model_request = self._map_event(request)
        model_prefix = [self._map_event(e) for e in prefix]
        payload = self.predictor.predict(model_request, model_prefix)
        return PredictorResult(prediction=Prediction.from_payload(payload), request=request)

    def close(self):
        close = getattr(self.history, "close", None)
        if close is not None:
            close()
