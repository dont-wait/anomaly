"""Scenario rules run against rows built from the real bundle manifest/preprocessing."""

import json
import math
from pathlib import Path

import pytest

from app.predictor import LocalBundlePredictor
from app.scenarios import CODES, MAX_SCENARIOS, SEVERITY_ORDER, detect_scenarios
from app.signals import Signals

BUNDLE = Path(__file__).resolve().parents[1] / "models" / "gatv2-1m-baseline-v1"
MANIFEST = json.loads((BUNDLE / "manifest.json").read_text())
PREPROCESSING = json.loads((BUNDLE / "preprocessing.json").read_text())
NAMES = MANIFEST["feature_names"]
NUMERIC = PREPROCESSING["numeric_indices"]

# A routine transfer: known device and receiver, 1M amount, ordinary history.
TYPICAL_FLAGS = {
    "has_previous": 1, "has_type_completed_history": 1, "type_amount_reference_reliable": 1,
    "has_type_mean_history": 1, "type_mean_reference_reliable": 1, "has_device_history": 1,
    "has_device": 1, "has_authentication_verified": 1, "authentication_verified": 1,
    "channel_prior_fraction": 0.5, "hour_prior_fraction": 0.2,
}
TYPICAL_QTY = {
    "log_amount": 1_000_000, "log_amount_to_type_median": 1, "log_amount_to_type_mean": 1,
    "log_src_history_count": 60, "log_seconds_since_previous": 86_400,
    "log_src_type_completed_median_amount": 1_000_000, "log_src_type_completed_count": 15,
    "log_src_type_completed_mean_amount": 1_000_000, "log_src_type_mean_count": 5,
}


def build(flags=None, qty=None):
    """Rows hold z-scores for numeric columns, so convert real quantities the way export did."""
    flags = {**TYPICAL_FLAGS, **(flags or {})}
    qty = {**TYPICAL_QTY, **(qty or {})}
    row = [0.0] * len(NAMES)
    for name, value in flags.items():
        row[NAMES.index(name)] = float(value)
    for k, index in enumerate(NUMERIC):
        name = NAMES[index]
        raw = math.log1p(qty.get(name, 0))
        row[index] = (raw - PREPROCESSING["mean"][k]) / PREPROCESSING["scale"][k]
    return Signals(NAMES, row, NUMERIC, PREPROCESSING["mean"], PREPROCESSING["scale"])


def codes(signals):
    return [item["code"] for item in detect_scenarios(signals)]


def by_code(signals, code):
    return next(item for item in detect_scenarios(signals) if item["code"] == code)


def test_routine_transfer_has_no_scenario():
    assert detect_scenarios(build()) == []


def test_signals_round_trip_real_quantities():
    signals = build(qty={"log_src_request_count_300s": 4, "log_amount_to_type_median": 8})
    assert signals.qty("log_src_request_count_300s") == pytest.approx(4)
    assert signals.qty("log_amount_to_type_median") == pytest.approx(8)
    assert signals.qty("log_amount") == pytest.approx(1_000_000)


# One input per scenario code; also proves CODES and the rules stay in step.
CASES = {
    "ACCOUNT_TAKEOVER": (
        {"new_device": 1, "new_receiver": 1},
        {"log_amount_to_type_median": 9},
    ),
    "NEW_DEVICE_NEW_RECEIVER": ({"new_device": 1, "new_receiver": 1}, {}),
    "NEW_DEVICE_LARGE_AMOUNT": ({"new_device": 1}, {"log_amount_to_type_median": 9}),
    "UNVERIFIED_LARGE_TRANSFER": (
        {"authentication_verified": 0},
        {"log_amount_to_type_median": 9},
    ),
    "AMOUNT_SPIKE": ({}, {"log_amount_to_type_median": 9}),
    "LARGE_AMOUNT_LOW_HISTORY": (
        {"cold_start": 1, "type_amount_reference_reliable": 0, "type_mean_reference_reliable": 0},
        {"log_amount": 300_000_000, "log_src_history_count": 3},
    ),
    "DORMANT_REACTIVATION": (
        {},
        {"log_seconds_since_previous": 45 * 86_400, "log_amount_to_type_median": 9},
    ),
    "VELOCITY_BURST": ({}, {"log_src_request_count_300s": 4}),
    "MULE_COLLECTOR": ({}, {"log_dst_transfer_attempt_distinct_sources": 3}),
    "PASS_THROUGH": ({}, {"log_src_received_settled_transfer_amount": 800_000}),
    "MULE_RELAY": (
        {},
        {
            "log_src_received_settled_transfer_amount": 900_000,
            "log_src_transfer_attempt_distinct_sources": 3,
        },
    ),
    "HABIT_DEVIATION": ({"hour_prior_fraction": 0.01, "channel_prior_fraction": 0.02}, {}),
}


def test_every_code_is_covered_by_a_case():
    assert set(CASES) == set(CODES)


@pytest.mark.parametrize("code", CODES)
def test_each_scenario_fires_on_its_pattern(code):
    flags, qty = CASES[code]
    assert code in codes(build(flags, qty))


@pytest.mark.parametrize("code", CODES)
def test_scenarios_match_the_server_wire_contract(code):
    flags, qty = CASES[code]
    item = by_code(build(flags, qty), code)
    assert set(item) == {"code", "severity", "title", "reason", "evidence"}
    assert item["severity"] in SEVERITY_ORDER
    assert 0 < len(item["title"]) <= 80
    assert 0 < len(item["reason"]) <= 300
    assert 0 < len(item["evidence"]) <= 8
    for feature in item["evidence"]:
        assert feature in NAMES and len(feature) <= 64


def test_takeover_quotes_the_numbers_and_does_not_double_report_device_rules():
    signals = build(
        {"new_device": 1, "new_receiver": 1, "authentication_verified": 0},
        {"log_amount_to_type_median": 9},
    )
    found = codes(signals)
    assert "NEW_DEVICE_NEW_RECEIVER" not in found and "NEW_DEVICE_LARGE_AMOUNT" not in found
    reason = by_code(signals, "ACCOUNT_TAKEOVER")["reason"]
    assert "gấp 9.0 lần" in reason and "chưa được xác thực" in reason


def test_first_device_is_not_treated_as_new():
    # new_device is 1 for an account's very first device; with no history it means nothing.
    signals = build({"new_device": 1, "has_device_history": 0, "new_receiver": 1})
    assert not {"ACCOUNT_TAKEOVER", "NEW_DEVICE_NEW_RECEIVER"} & set(codes(signals))


def test_unreliable_baseline_never_triggers_amount_spike():
    signals = build(
        {"type_amount_reference_reliable": 0, "type_mean_reference_reliable": 0},
        {"log_amount_to_type_median": 50},
    )
    assert "AMOUNT_SPIKE" not in codes(signals)


@pytest.mark.parametrize(
    ("qty", "severity"),
    [
        ({"log_src_request_count_300s": 2}, None),
        ({"log_src_request_count_120s": 1, "log_src_request_count_300s": 1}, None),
        # Third request of a 15-second burst: two prior requests within 2 minutes.
        (
            {
                "log_src_request_count_120s": 2,
                "log_src_request_count_300s": 2,
                "log_src_request_count_3600s": 2,
            },
            "medium",
        ),
        ({"log_src_request_count_300s": 3}, "medium"),
        ({"log_src_request_count_120s": 1, "log_src_request_count_3600s": 3}, "medium"),
        ({"log_src_request_count_120s": 3}, "high"),
        ({"log_src_request_count_3600s": 15}, "high"),
    ],
)
def test_velocity_tiers(qty, severity):
    found = [i for i in detect_scenarios(build(qty=qty)) if i["code"] == "VELOCITY_BURST"]
    assert (found[0]["severity"] if found else None) == severity


def test_velocity_reason_quotes_counts():
    qty = {
        "log_src_request_count_120s": 1,
        "log_src_request_count_300s": 3,
        "log_src_request_count_3600s": 6,
    }
    reason = by_code(build(qty=qty), "VELOCITY_BURST")["reason"]
    assert "1 yêu cầu trong 2 phút, 3 trong 5 phút và 6 trong 1 giờ" in reason


def test_collector_needs_two_other_sources_and_escalates():
    one = build(qty={"log_dst_transfer_attempt_distinct_sources": 1})
    assert "MULE_COLLECTOR" not in codes(one)
    two = build(qty={"log_dst_transfer_attempt_distinct_sources": 2})
    four = build(qty={"log_dst_transfer_attempt_distinct_sources": 4})
    assert by_code(two, "MULE_COLLECTOR")["severity"] == "medium"
    assert by_code(four, "MULE_COLLECTOR")["severity"] == "high"


def test_pass_through_compares_received_with_sent():
    small = build(qty={"log_src_received_settled_transfer_amount": 100_000})
    assert "PASS_THROUGH" not in codes(small)
    half = build(qty={"log_src_received_settled_transfer_amount": 600_000})
    assert by_code(half, "PASS_THROUGH")["severity"] == "medium"
    nearly_all = build(qty={"log_src_received_settled_transfer_amount": 1_000_000})
    item = by_code(nearly_all, "PASS_THROUGH")
    assert item["severity"] == "high" and "100%" in item["reason"]


def test_habit_deviation_ignored_while_cold_start():
    flags = {"hour_prior_fraction": 0.0, "channel_prior_fraction": 0.0, "cold_start": 1}
    assert "HABIT_DEVIATION" not in codes(build(flags))


def test_habit_deviation_is_medium_with_a_new_receiver():
    flags = {"hour_prior_fraction": 0.0, "channel_prior_fraction": 0.0, "new_receiver": 1}
    assert by_code(build(flags), "HABIT_DEVIATION")["severity"] == "medium"


def test_most_severe_first_and_capped():
    flags = {
        "new_device": 1, "new_receiver": 1, "authentication_verified": 0,
        "hour_prior_fraction": 0.0, "channel_prior_fraction": 0.0,
    }
    qty = {
        "log_amount_to_type_median": 12,
        "log_src_request_count_120s": 3,
        "log_dst_transfer_attempt_distinct_sources": 4,
        "log_src_received_settled_transfer_amount": 1_000_000,
        "log_src_transfer_attempt_distinct_sources": 3,
        "log_seconds_since_previous": 45 * 86_400,
    }
    found = detect_scenarios(build(flags, qty))
    assert len(found) == MAX_SCENARIOS
    ranks = [SEVERITY_ORDER[item["severity"]] for item in found]
    assert ranks == sorted(ranks)


def test_missing_preprocessing_scale_disables_quantity_rules_instead_of_guessing():
    row = [0.0] * len(NAMES)
    signals = Signals(NAMES, row, NUMERIC)  # no mean/scale
    assert math.isnan(signals.qty("log_amount"))
    assert detect_scenarios(signals) == []


# --- factor gating and wording -------------------------------------------------------


def predictor_for(names, numeric, mean=None, scale=None):
    fake = type("P", (), {
        "manifest": {"feature_names": names},
        "preprocessing": {"numeric_indices": numeric, "mean": mean, "scale": scale},
    })()
    predictor = object.__new__(LocalBundlePredictor)
    predictor.predictor = fake
    return predictor


def test_first_ever_transfer_is_not_called_a_burst_or_a_rare_channel():
    names = ["log_seconds_since_previous", "has_previous", "channel_prior_fraction"]
    predictor = predictor_for(names, [0])
    # No history: seconds-since-previous is encoded 0 (z far below mean), fraction is 0.
    factors = predictor._top_factors({"edge_features": [[-4.9, 0.0, 0.0]]})
    assert factors == ()


def test_factor_reasons_quote_real_quantities():
    predictor = object.__new__(LocalBundlePredictor)
    predictor.predictor = type("P", (), {
        "manifest": {"feature_names": NAMES},
        "preprocessing": PREPROCESSING,
    })()
    signals = build(qty={"log_src_request_count_300s": 4, "log_amount_to_type_median": 8})
    factors = predictor._top_factors({"edge_features": [signals._row]}, signals)
    texts = {f["feature"]: f["reason"] for f in factors}
    assert "4 yêu cầu chuyển tiền trong 5 phút" in texts["log_src_request_count_300s"]
    assert "gấp 8.0 lần mức trung vị 30 ngày" in texts["log_amount_to_type_median"]
    assert all(len(text) <= 200 for text in texts.values())


def test_explain_never_raises_on_bad_inputs():
    predictor = predictor_for(["log_amount"], [0])
    assert predictor._explain(None) == ((), ())
    assert predictor._explain({"edge_features": []}) == ((), ())
