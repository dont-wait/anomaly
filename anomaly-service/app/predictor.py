"""Local GATv2 bundle inference; no HTTP model service required."""

import sys
from copy import deepcopy
from pathlib import Path

from .contracts import Prediction, PredictorResult


class UnsupportedIdentityError(ValueError):
    """The bundle cannot preserve the supplied account identities."""


class LocalBundlePredictor:
    def __init__(
        self, bundle_path, history, *, gatv2_src_path=None, device="cpu", account_id_map=None
    ):
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
        self.supports_dynamic_accounts = (
            getattr(self.predictor, "account_identity_policy", None) == "distinct-account-nodes-v1"
        )
        self.account_id_map = dict(account_id_map) if account_id_map else {}
        self.account_node_ids = [
            n["id"]
            for n in getattr(self.predictor, "nodes", [])
            if n.get("type") == "account" and n.get("id") != "__unknown__"
        ]
        self.known_account_ids = set(self.account_node_ids)
        targets = list(self.account_id_map.values())
        if any(not isinstance(key, str) or not key for key in self.account_id_map):
            raise UnsupportedIdentityError("account mapping keys must be nonempty strings")
        if any(
            not isinstance(target, str) or target not in self.known_account_ids
            for target in targets
        ):
            raise UnsupportedIdentityError("account mapping targets must be known bundle accounts")
        if any(
            key in self.known_account_ids and key != value
            for key, value in self.account_id_map.items()
        ):
            raise UnsupportedIdentityError("native bundle account identities cannot be remapped")
        if len(set(targets)) != len(targets):
            raise UnsupportedIdentityError("account mapping must preserve distinct identities")

    def _map_account_id(self, account_id):
        if not account_id or not isinstance(account_id, str):
            return account_id
        if account_id in self.known_account_ids:
            return account_id
        if account_id in self.account_id_map:
            return self.account_id_map[account_id]
        if self.supports_dynamic_accounts and account_id != "__unknown__":
            return account_id
        raise UnsupportedIdentityError(
            "account is not supported by this bundle; use a compatible bundle or an explicit "
            "identity-preserving account mapping"
        )

    def _map_event(self, event):
        if not isinstance(event, dict) or "payload" not in event:
            return event
        payload = event.get("payload")
        if not isinstance(payload, dict):
            return event
        src = payload.get("sourceAccountId")
        dst = payload.get("destinationAccountId")
        mapped = deepcopy(event)
        # Preserve IP in the original request/training record. The frozen bundle
        # has no IP features and its input validator rejects additional fields.
        mapped["payload"].pop("ip_address", None)
        # Server terminal events may contain a prior model output. Never feed
        # that output back into causal history as a feature/label.
        mapped["payload"].pop("riskAssessment", None)
        if src:
            mapped["payload"]["sourceAccountId"] = self._map_account_id(src)
        if dst:
            mapped["payload"]["destinationAccountId"] = self._map_account_id(dst)
        return mapped

    def predict(self, event_id):
        request, prefix = self.history.read_request_and_prefix(event_id)
        model_request = self._map_event(request)
        model_prefix = [self._map_event(e) for e in prefix]
        # An explicit alias and a native bundle ID must not collapse two accounts
        # appearing together in this inference history.
        identities = {}
        for original, mapped in zip([request, *prefix], [model_request, *model_prefix]):
            for key in ("sourceAccountId", "destinationAccountId"):
                source = original.get("payload", {}).get(key)
                target = mapped.get("payload", {}).get(key)
                if source and target:
                    previous = identities.setdefault(target, source)
                    if previous != source:
                        raise UnsupportedIdentityError("account mapping merges distinct identities")
        payload = self.predictor.predict(model_request, model_prefix)
        return PredictorResult(prediction=Prediction.from_payload(payload), request=request)

    def close(self):
        close = getattr(self.history, "close", None)
        if close is not None:
            close()
