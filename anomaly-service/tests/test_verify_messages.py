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
    assert valid["transfer_id"] in result.stdout
    assert valid["decision"] == "BLOCK"
    assert valid["is_anomalous"] is True
    assert messages["allow"]["decision"] == "ALLOW"
    assert messages["allow"]["is_anomalous"] is False
    assert messages["allow"]["transfer_id"] != valid["transfer_id"]
    assert messages["allow"] == sign_risk_event(messages["allow"], secret)
    assert messages["tampered"]["signature"] != sign_risk_event(messages["tampered"], secret)[
        "signature"
    ]
    assert "signature" not in messages["unsigned"]
    assert messages["wrong-key"]["signature"] != valid["signature"]
    assert "risk_score" not in messages["missing-score"]
