"""Local GATv2 bundle inference; no HTTP model service required."""

import logging
import math
import sys
from copy import deepcopy
from pathlib import Path

from .contracts import Prediction, PredictorResult
from .reasons import describe_factor
from .scenarios import detect_scenarios
from .signals import Signals

log = logging.getLogger(__name__)

TOP_FACTOR_COUNT = 3  # at most 3 + len(RISK_FLAGS) + len(RARE_FRACTIONS) entries; server accepts 8
# Not standardized by preprocessing.json, so z-scores can't surface them. Binary
# flags are reported when raised; channel_prior_fraction is reported when rare (low).
RISK_FLAGS = ("new_receiver", "new_device")
RARE_FRACTIONS = ("channel_prior_fraction",)
RARE_FRACTION_MAX = 0.05


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
        captured = {}
        original_inputs = getattr(self.predictor, "inputs", None)
        if original_inputs is not None:
            def capture_inputs(*args, **kwargs):
                captured["inputs"] = original_inputs(*args, **kwargs)
                return captured["inputs"]

            # Reuse the inputs predict() already builds instead of replaying history twice.
            self.predictor.inputs = capture_inputs
        try:
            payload = self.predictor.predict(model_request, model_prefix)
        finally:
            if original_inputs is not None:
                del self.predictor.inputs
        factors, scenarios = self._explain(captured.get("inputs"))
        return PredictorResult(
            prediction=Prediction.from_payload(payload),
            request=request,
            factors=factors,
            scenarios=scenarios,
        )

    def _signals(self, inputs):
        row = inputs["edge_features"][0]
        names = self.predictor.manifest["feature_names"]
        preprocessing = self.predictor.preprocessing
        columns = preprocessing["numeric_indices"]
        return Signals(
            names, row, columns, preprocessing.get("mean"), preprocessing.get("scale")
        )

    def _explain(self, inputs):
        """(factors, scenarios) for one request; never raises, scoring must not depend on it."""
        try:
            signals = self._signals(inputs)
        except (KeyError, IndexError, TypeError, ValueError, AttributeError):
            log.warning("explanation skipped: model inputs have an unexpected shape", exc_info=True)
            return (), ()
        try:
            factors = self._top_factors(inputs, signals)
        except Exception:
            log.warning("top_factors skipped", exc_info=True)
            factors = ()
        try:
            scenarios = tuple(detect_scenarios(signals))
        except Exception:
            log.warning("scenarios skipped", exc_info=True)
            scenarios = ()
        return factors, scenarios

    def _top_factors(self, inputs, signals=None):
        """Inputs that look most unusual for this request, as trace-only notes.

        edge_features are already standardized by preprocessing.json for the numeric
        columns, so a value is a z-score against the training distribution. This
        ranks atypical inputs, not model attribution: a large |z| does not mean the
        feature raised the score. Columns that only hold a placeholder because the
        account has no history (see signals.GATES) are skipped. The result is
        advisory, unsigned, and capped well below the server's MaxTopFactors.
        """
        signals = signals or self._signals(inputs)
        ranked = sorted(
            (
                (signals.z(name), name)
                for name in signals.names
                if signals.is_numeric(name) and not math.isnan(signals.z(name))
            ),
            key=lambda item: -abs(item[0]),
        )
        factors = [
            {"feature": name, "z_score": round(z, 3)} for z, name in ranked[:TOP_FACTOR_COUNT]
        ]
        for name in RISK_FLAGS:
            if signals.flag(name):
                factors.append({"feature": name, "value": 1})
        for name in RARE_FRACTIONS:
            value = signals.raw(name)
            if not math.isnan(value) and value <= RARE_FRACTION_MAX:
                factors.append({"feature": name, "value": round(value, 4)})
        return tuple({**f, "reason": describe_factor(f, signals)} for f in factors)

    def close(self):
        close = getattr(self.history, "close", None)
        if close is not None:
            close()
