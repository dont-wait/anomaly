import json
import os
import subprocess
import sys
from pathlib import Path

from app.publisher import sign_risk_event


def test_verify_script_builds_signed_and_invalid_messages_without_exposing_key(tmp_path):
    secret = "test-verify-only-secret"
    script = Path(__file__).resolve().parents[1] / "scripts/build_verify_messages.py"
    result = subprocess.run(
        [sys.executable, str(script), "--output-dir", str(tmp_path)],
        env={**os.environ, "KAFKA_RISK_DECISION_SIGNING_KEY": secret},
        capture_output=True, text=True, check=True,
    )
    assert secret not in result.stdout + result.stderr
    messages = {}
    for path in tmp_path.glob("*.json"):
        raw = path.read_text()
        assert secret not in raw
        messages[path.stem] = json.loads(raw)
    assert set(messages) == {"valid", "allow", "tampered", "unsigned", "wrong-key", "missing-score"}
    valid = messages["valid"]
    assert valid == sign_risk_event(valid, secret)
    assert valid["transaction_id"] in result.stdout
    assert valid["predicted_label"] == 1
    assert messages["allow"]["predicted_label"] == 0
    assert messages["allow"]["transaction_id"] != valid["transaction_id"]
    assert messages["allow"] == sign_risk_event(messages["allow"], secret)
    assert messages["tampered"]["signature"] != sign_risk_event(messages["tampered"], secret)[
        "signature"
    ]
    assert "signature" not in messages["unsigned"]
    assert messages["wrong-key"]["signature"] != valid["signature"]
    assert "risk_score" not in messages["missing-score"]
