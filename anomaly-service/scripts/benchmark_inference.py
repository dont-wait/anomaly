"""Benchmark local GATv2 bundle inference with a synthetic request."""

import argparse
import math
import statistics
import sys
import time
from pathlib import Path

SERVICE_ROOT = Path(__file__).resolve().parents[1]
if str(SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(SERVICE_ROOT))

from app.predictor import LocalBundlePredictor  # noqa: E402


def _request():
    return {
        "eventId": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
        "transactionId": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "eventType": "TransferCreated",
        "schemaVersion": 1,
        "sequence": 1,
        "occurredAt": "2026-10-29T00:00:00Z",
        "payload": {
            "sourceAccountId": "6623a1f89c30111111111111",
            "destinationAccountId": "6623a1f89c30222222222222",
            "amount": 1_000_000,
            "fee": 0,
            "currency": "VND",
            "channel": "mobile",
            "status": "awaiting_otp",
            "device_id": "synthetic-device",
            "platform": "android",
            "authentication_verified": False,
            "ip_address": "203.0.113.10",
        },
    }


class SyntheticHistory:
    def __init__(self, request):
        self.request = request

    def read_request_and_prefix(self, event_id):
        return self.request, []


def _percentile(samples, percentile):
    ordered = sorted(samples)
    index = max(0, math.ceil(percentile * len(ordered)) - 1)
    return ordered[index]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--iterations", type=int, default=200)
    parser.add_argument("--warmup", type=int, default=20)
    parser.add_argument("--device", default="cpu")
    parser.add_argument(
        "--bundle",
        default=str(Path(__file__).resolve().parents[1] / "models/gatv2-1m-baseline-v1"),
    )
    parser.add_argument("--gatv2-src", default="../../GATv2/src")
    args = parser.parse_args()
    if args.iterations < 1 or args.warmup < 0:
        parser.error("--iterations must be positive and --warmup must be nonnegative")

    request = _request()
    predictor = LocalBundlePredictor(
        args.bundle,
        SyntheticHistory(request),
        gatv2_src_path=args.gatv2_src,
        device=args.device,
    )
    try:
        for _ in range(args.warmup):
            predictor.predict(request["eventId"])

        samples = []
        for _ in range(args.iterations):
            started = time.perf_counter()
            result = predictor.predict(request["eventId"])
            samples.append(time.perf_counter() - started)

        total_seconds = sum(samples)
        print(f"device: {args.device}")
        print(f"bundle: {Path(args.bundle).resolve()}")
        print("scope: LocalBundlePredictor.predict; synthetic request, empty history")
        print(f"iterations: {args.iterations} (warmup: {args.warmup})")
        print(f"risk_score: {result.prediction.risk_score:.6f}")
        print(f"mean_ms: {statistics.fmean(samples) * 1000:.3f}")
        print(f"p50_ms: {_percentile(samples, 0.50) * 1000:.3f}")
        print(f"p95_ms: {_percentile(samples, 0.95) * 1000:.3f}")
        print(f"p99_ms: {_percentile(samples, 0.99) * 1000:.3f}")
        print(f"throughput_inferences_per_second: {args.iterations / total_seconds:.2f}")
    finally:
        predictor.close()


if __name__ == "__main__":
    main()
