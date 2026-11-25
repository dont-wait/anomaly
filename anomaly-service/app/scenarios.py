"""Named fraud scenarios recognised from combinations of model inputs.

A scenario is a hand-written rule over several inputs at once, with a stable code
(for filtering/grouping by the backend), a short title, and a one-line reason that
quotes the actual numbers. It tells an investigator *which pattern to look at*.
It is not model attribution: the score comes from the GATv2 model, and a scenario
can fire on a transfer the model scored low, or stay silent on one it scored high.

Thresholds are starting points chosen from the training scale in
preprocessing.json, not tuned on labelled fraud. Adjust them here, and add a test.
Counts are of *prior* requests: the current transfer is not included.
"""

import math

from .reasons import fmt_amount, fmt_duration

MAX_SCENARIOS = 6
SEVERITY_ORDER = {"high": 0, "medium": 1, "low": 2}

# Request-window and neighbourhood sizes baked into the bundle (manifest options).
NEIGHBOURHOOD = "10 phút"
MEDIAN_BASIS = "trung vị 30 ngày"
MEAN_BASIS = "trung bình 7 ngày"

BIG_MULTIPLE = 5.0
HUGE_MULTIPLE = 10.0
BIG_AMOUNT_Z = 2.0
DORMANT_SECONDS = 30 * 86400
PASS_THROUGH_SHARE = 0.5
FULL_PASS_THROUGH_SHARE = 0.9
RARE_HOUR = 0.02
RARE_CHANNEL = 0.05


def _make(code, severity, title, reason, evidence):
    return {
        "code": code,
        "severity": severity,
        "title": title,
        "reason": reason,
        "evidence": list(evidence),
    }


def _amount_reference(s):
    """(multiple, label, feature) of this amount vs the account's usual, or None."""
    for feature, label in (
        ("log_amount_to_type_median", MEDIAN_BASIS),
        ("log_amount_to_type_mean", MEAN_BASIS),
    ):
        multiple = s.qty(feature)
        if not math.isnan(multiple):
            return multiple, label, feature
    return None


def _amount_phrase(s):
    """How big the amount is, in words, or None when it is not unusually large."""
    reference = _amount_reference(s)
    if reference is not None:
        multiple, label, feature = reference
        if multiple >= BIG_MULTIPLE:
            return f"gấp {multiple:.1f} lần mức {label} của tài khoản", feature
        return None
    z = s.z("log_amount")
    if z >= BIG_AMOUNT_Z:
        return f"{fmt_amount(s.qty('log_amount'))} (cao hơn mức thông thường, z={z:+.1f})", (
            "log_amount"
        )
    return None


def _device_is_new(s):
    # new_device is also 1 for an account's very first device, which says nothing.
    return s.flag("new_device") and s.flag("has_device_history")


def _takeover(s):
    amount = _amount_phrase(s)
    if _device_is_new(s) and s.flag("new_receiver") and amount:
        extra = ""
        if s.flag("has_authentication_verified") and not s.flag("authentication_verified"):
            extra = ", phiên chưa được xác thực"
        return _make(
            "ACCOUNT_TAKEOVER", "high", "Nghi chiếm đoạt tài khoản",
            f"Thiết bị mới + người nhận mới + số tiền {amount[0]}{extra}. Đây là mẫu thường "
            "gặp khi kẻ gian đăng nhập từ thiết bị lạ rồi chuyển tiền cho tài khoản của chúng.",
            ["new_device", "new_receiver", amount[1]],
        )
    return None


def _new_device_new_receiver(s):
    if _device_is_new(s) and s.flag("new_receiver") and not _amount_phrase(s):
        return _make(
            "NEW_DEVICE_NEW_RECEIVER", "medium", "Thiết bị mới chuyển cho người nhận mới",
            "Giao dịch từ thiết bị chưa từng dùng với tài khoản này, tới người nhận mà tài "
            "khoản nguồn chưa từng chuyển tiền.",
            ["new_device", "new_receiver"],
        )
    return None


def _new_device_large(s):
    amount = _amount_phrase(s)
    if _device_is_new(s) and not s.flag("new_receiver") and amount:
        return _make(
            "NEW_DEVICE_LARGE_AMOUNT", "medium", "Số tiền lớn từ thiết bị mới",
            f"Thiết bị chưa từng dùng với tài khoản này, số tiền {amount[0]}.",
            ["new_device", amount[1]],
        )
    return None


def _unverified_large(s):
    amount = _amount_phrase(s)
    if s.flag("has_authentication_verified") and not s.flag("authentication_verified") and amount:
        return _make(
            "UNVERIFIED_LARGE_TRANSFER", "medium", "Số tiền lớn nhưng chưa xác thực",
            f"Giao dịch có số tiền {amount[0]} nhưng phiên chưa được xác thực.",
            ["authentication_verified", amount[1]],
        )
    return None


def _amount_spike(s):
    reference = _amount_reference(s)
    if reference is None:
        return None
    multiple, label, feature = reference
    if multiple < BIG_MULTIPLE:
        return None
    severity = "high" if multiple >= HUGE_MULTIPLE else "medium"
    return _make(
        "AMOUNT_SPIKE", severity, "Số tiền tăng đột biến",
        f"Số tiền gấp {multiple:.1f} lần mức {label} các giao dịch cùng loại của tài khoản.",
        [feature],
    )


def _low_history_large(s):
    # Without a reliable per-account baseline, fall back to the global amount scale.
    if s.flag("cold_start") and _amount_reference(s) is None:
        z = s.z("log_amount")
        if z >= BIG_AMOUNT_Z:
            history = s.qty("log_src_history_count")
            seen = "" if math.isnan(history) else f" (chỉ {history:.0f} yêu cầu trước đó)"
            return _make(
                "LARGE_AMOUNT_LOW_HISTORY", "medium", "Tài khoản ít lịch sử chuyển số tiền lớn",
                f"Số tiền {fmt_amount(s.qty('log_amount'))} cao hơn mức thông thường "
                f"(z={z:+.1f}) trong khi tài khoản còn ít lịch sử{seen}, chưa đủ để so với "
                "thói quen riêng.",
                ["cold_start", "log_amount"],
            )
    return None


def _dormant(s):
    gap = s.qty("log_seconds_since_previous")
    amount = _amount_phrase(s)
    if gap >= DORMANT_SECONDS and amount:
        return _make(
            "DORMANT_REACTIVATION", "medium", "Tài khoản lâu không hoạt động bất ngờ chuyển lớn",
            f"Tài khoản không có yêu cầu nào trong {fmt_duration(gap)}, nay chuyển số tiền "
            f"{amount[0]}.",
            ["log_seconds_since_previous", amount[1]],
        )
    return None


def _velocity(s):
    n120, n300, n3600 = (
        s.qty("log_src_request_count_120s"),
        s.qty("log_src_request_count_300s"),
        s.qty("log_src_request_count_3600s"),
    )
    if math.isnan(n120) or math.isnan(n300) or math.isnan(n3600):
        return None
    n120, n300, n3600 = round(n120), round(n300), round(n3600)
    if n120 >= 3 or n300 >= 5 or n3600 >= 15:
        severity = "high"
    elif (n120 >= 1 and n3600 >= 3) or n300 >= 3 or n3600 >= 8:
        severity = "medium"
    else:
        return None
    return _make(
        "VELOCITY_BURST", severity, "Chuyển tiền dồn dập",
        f"Trước giao dịch này, tài khoản nguồn đã gửi {n120} yêu cầu trong 2 phút, {n300} trong "
        f"5 phút và {n3600} trong 1 giờ.",
        [
            "log_src_request_count_120s",
            "log_src_request_count_300s",
            "log_src_request_count_3600s",
        ],
    )


def _collector(s):
    sources = s.qty("log_dst_transfer_attempt_distinct_sources")
    if math.isnan(sources) or round(sources) < 2:
        return None
    sources = round(sources)
    severity = "high" if sources >= 4 else "medium"
    return _make(
        "MULE_COLLECTOR", severity, "Tài khoản nhận gom tiền từ nhiều nguồn",
        f"Tài khoản nhận đã nhận tiền từ {sources} nguồn khác nhau trong {NEIGHBOURHOOD} trước "
        "đó. Đây là dấu hiệu tài khoản trung gian gom tiền.",
        ["log_dst_transfer_attempt_distinct_sources"],
    )


def _received_share(s):
    received, amount = s.qty("log_src_received_settled_transfer_amount"), s.qty("log_amount")
    if math.isnan(received) or math.isnan(amount) or amount <= 0:
        return None
    return received, amount, received / amount


def _pass_through(s):
    share = _received_share(s)
    if share is None or share[2] < PASS_THROUGH_SHARE:
        return None
    received, amount, ratio = share
    severity = "high" if ratio >= FULL_PASS_THROUGH_SHARE else "medium"
    return _make(
        "PASS_THROUGH", severity, "Nhận tiền rồi chuyển đi ngay",
        f"Tài khoản nguồn vừa nhận {fmt_amount(received)} trong {NEIGHBOURHOOD} trước và đang "
        f"chuyển đi {fmt_amount(amount)} ({ratio:.0%} số vừa nhận).",
        ["log_src_received_settled_transfer_amount", "log_amount"],
    )


def _relay(s):
    share = _received_share(s)
    sources = s.qty("log_src_transfer_attempt_distinct_sources")
    if share is None or share[2] < PASS_THROUGH_SHARE:
        return None
    if math.isnan(sources) or round(sources) < 2:
        return None
    return _make(
        "MULE_RELAY", "high", "Gom tiền từ nhiều nguồn rồi chuyển tiếp",
        f"Tài khoản nguồn vừa nhận tiền từ {round(sources)} nguồn khác nhau trong "
        f"{NEIGHBOURHOOD} trước, rồi chuyển đi {fmt_amount(share[1])} ({share[2]:.0%} số vừa "
        "nhận). Dấu hiệu tài khoản trung chuyển.",
        ["log_src_transfer_attempt_distinct_sources", "log_src_received_settled_transfer_amount"],
    )


def _habit(s):
    if s.flag("cold_start"):
        return None  # fractions from a handful of requests are not a habit yet
    hour, channel = s.raw("hour_prior_fraction"), s.raw("channel_prior_fraction")
    if math.isnan(hour) or math.isnan(channel):
        return None
    if hour > RARE_HOUR or channel > RARE_CHANNEL:
        return None
    severity = "medium" if s.flag("new_receiver") else "low"
    return _make(
        "HABIT_DEVIATION", severity, "Khác thói quen giao dịch",
        f"Chỉ {hour:.1%} giao dịch trước đó của tài khoản diễn ra vào giờ này và {channel:.1%} "
        "qua kênh này"
        + (", lại chuyển cho người nhận mới." if s.flag("new_receiver") else "."),
        ["hour_prior_fraction", "channel_prior_fraction"],
    )


RULES = (
    _takeover,
    _new_device_new_receiver,
    _new_device_large,
    _unverified_large,
    _amount_spike,
    _low_history_large,
    _dormant,
    _velocity,
    _collector,
    _pass_through,
    _relay,
    _habit,
)

CODES = (
    "ACCOUNT_TAKEOVER",
    "NEW_DEVICE_NEW_RECEIVER",
    "NEW_DEVICE_LARGE_AMOUNT",
    "UNVERIFIED_LARGE_TRANSFER",
    "AMOUNT_SPIKE",
    "LARGE_AMOUNT_LOW_HISTORY",
    "DORMANT_REACTIVATION",
    "VELOCITY_BURST",
    "MULE_COLLECTOR",
    "PASS_THROUGH",
    "MULE_RELAY",
    "HABIT_DEVIATION",
)


def detect_scenarios(signals):
    """Scenarios that match this request, most severe first, at most MAX_SCENARIOS."""
    found = [scenario for rule in RULES if (scenario := rule(signals)) is not None]
    found.sort(key=lambda item: SEVERITY_ORDER[item["severity"]])  # stable: rule order kept
    return found[:MAX_SCENARIOS]
