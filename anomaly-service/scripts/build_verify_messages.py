"""Create synthetic Kafka messages to verify the decision contract, not model accuracy."""

import argparse
import json
import os
import sys
import uuid
from pathlib import Path
from types import SimpleNamespace

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app.contracts import Prediction, build_risk_event  # noqa: E402
from app.publisher import sign_risk_event  # noqa: E402


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    key = os.environ.get("KAFKA_RISK_DECISION_SIGNING_KEY", "")
    if not key.strip():
        parser.error("export the same KAFKA_RISK_DECISION_SIGNING_KEY used by the server")
    def decision_message(score):
        transaction_id = str(uuid.uuid4())
        event = SimpleNamespace(
            transaction_id=transaction_id,
            event_id=str(uuid.uuid5(uuid.NAMESPACE_OID, transaction_id + ":TransferCreated")),
        )
        prediction = Prediction(
            transaction_id, score, int(score >= 0.95), 0.95,
            "manual-contract-test-v1", "manual-contract-test-v1",
        )
        return build_risk_event(event, prediction)

    decision = decision_message(0.98)
    valid = sign_risk_event(decision, key)
    missing = dict(valid)
    del missing["risk_score"]
    messages = {
        "valid": valid,
        "allow": sign_risk_event(decision_message(0.08), key),
        "tampered": {**valid, "risk_score": 0.99},
        "unsigned": decision,
        "wrong-key": sign_risk_event(decision, key + "-wrong"),
        "missing-score": missing,
    }
    args.output_dir.mkdir(parents=True, exist_ok=True)
    for name, message in messages.items():
        (args.output_dir / f"{name}.json").write_text(
            json.dumps(message, ensure_ascii=False, separators=(",", ":")) + "\n",
            encoding="utf-8",
        )
    print(f"Synthetic BLOCK transfer_id: {decision['transfer_id']}")
    print(f"Synthetic ALLOW transfer_id: {messages['allow']['transfer_id']}")
    print(f"Messages written to {args.output_dir}")


if __name__ == "__main__":
    main()
