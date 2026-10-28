"""Local GATv2 bundle inference; no HTTP model service required."""
import sys
from pathlib import Path

from .contracts import Prediction, PredictorResult


class LocalBundlePredictor:
    def __init__(self, bundle_path, history, *, gatv2_src_path=None, device="cpu"):
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

    def predict(self, event_id):
        request, prefix = self.history.read_request_and_prefix(event_id)
        payload = self.predictor.predict(request, prefix)
        return PredictorResult(prediction=Prediction.from_payload(payload), request=request)

    def close(self):
        close = getattr(self.history, "close", None)
        if close is not None:
            close()
