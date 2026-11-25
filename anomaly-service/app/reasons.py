"""Human-readable (Vietnamese) reasons for trace-only top factors.

The model works on standardized features, which only data engineers can read. The
backend and admins need a sentence that says what looked unusual, so every factor
carries a `reason` string built here. With a `Signals` view the sentence quotes the
real quantity ("3 yêu cầu trong 5 phút"); without one it falls back to the z-score.
Text only describes how atypical an input is for this request; it is not an
attribution of the score.
"""

import math

WINDOWS = {"120s": "2 phút", "300s": "5 phút", "3600s": "1 giờ", "86400s": "24 giờ"}
NEIGHBOURHOOD = "10 phút"

# feature -> (sentence when z > 0, sentence when z < 0), used when no real quantity helps.
NUMERIC = {
    "log_amount": (
        "Số tiền chuyển lớn hơn bình thường",
        "Số tiền chuyển nhỏ hơn bình thường",
    ),
    "log_seconds_since_previous": (
        "Đã rất lâu tài khoản nguồn mới có yêu cầu chuyển tiền lại",
        "Giao dịch dồn dập: cách yêu cầu trước quá ngắn",
    ),
    "log_src_history_count": (
        "Tài khoản nguồn có nhiều lịch sử giao dịch hơn bình thường",
        "Tài khoản nguồn có rất ít lịch sử giao dịch",
    ),
    "log_amount_to_type_median": (
        "Số tiền gấp nhiều lần mức trung vị các giao dịch cùng loại của tài khoản",
        "Số tiền thấp hơn nhiều so với mức trung vị các giao dịch cùng loại của tài khoản",
    ),
    "log_amount_to_type_mean": (
        "Số tiền gấp nhiều lần mức trung bình các giao dịch cùng loại của tài khoản",
        "Số tiền thấp hơn nhiều so với mức trung bình các giao dịch cùng loại của tài khoản",
    ),
    "log_dst_transfer_attempt_distinct_sources": (
        "Tài khoản nhận vừa nhận tiền từ nhiều nguồn khác nhau (dấu hiệu tài khoản trung gian)",
        "Tài khoản nhận gần đây chưa nhận tiền từ nguồn nào khác",
    ),
    "log_src_transfer_attempt_distinct_sources": (
        "Tài khoản nguồn vừa nhận tiền từ nhiều nguồn khác nhau",
        "Tài khoản nguồn gần đây chưa nhận tiền từ nguồn nào khác",
    ),
    "log_src_received_settled_transfer_amount": (
        "Tài khoản nguồn vừa nhận nhiều tiền rồi chuyển đi",
        "Tài khoản nguồn gần như chưa nhận tiền nào",
    ),
    "log_src_transfer_attempt_count": (
        "Tài khoản nguồn có nhiều yêu cầu chuyển đi trong thời gian ngắn",
        "Tài khoản nguồn gần đây không có yêu cầu chuyển đi nào",
    ),
    "hour_sin": ("Giờ giao dịch bất thường", "Giờ giao dịch bất thường"),
    "hour_cos": ("Giờ giao dịch bất thường", "Giờ giao dịch bất thường"),
    "weekday_sin": ("Ngày trong tuần bất thường", "Ngày trong tuần bất thường"),
    "weekday_cos": ("Ngày trong tuần bất thường", "Ngày trong tuần bất thường"),
}

FLAGS = {
    "new_receiver": "Người nhận mới, tài khoản nguồn chưa từng chuyển cho người này",
    "new_device": "Giao dịch từ thiết bị mới, chưa từng dùng với tài khoản này",
}


def fmt_amount(value):
    return "không rõ" if value is None or math.isnan(value) else f"{value:,.0f}"


def fmt_duration(seconds):
    if seconds < 90:
        return f"{seconds:.0f} giây"
    if seconds < 5400:
        return f"{seconds / 60:.0f} phút"
    if seconds < 129600:
        return f"{seconds / 3600:.0f} giờ"
    return f"{seconds / 86400:.0f} ngày"


def _fallback(feature, z_score):
    direction = "cao hơn" if z_score > 0 else "thấp hơn"
    return f"Chỉ số {feature} {direction} bình thường {abs(z_score):.1f} độ lệch chuẩn"


def _quantified(feature, z_score, signals):
    """A sentence quoting the real quantity, or None when it cannot be recovered."""
    if signals is None:
        return None
    qty = signals.qty(feature)
    if math.isnan(qty):
        return None
    if feature.startswith("log_src_request_count_"):
        window = WINDOWS.get(feature.rsplit("_", 1)[-1])
        if window:
            return f"Tài khoản nguồn đã gửi {qty:.0f} yêu cầu chuyển tiền trong {window} trước đó"
    if feature == "log_amount":
        direction = "lớn" if z_score > 0 else "nhỏ"
        return f"Số tiền chuyển {fmt_amount(qty)} {direction} hơn mức thông thường"
    if feature == "log_amount_to_type_median":
        return f"Số tiền gấp {qty:.1f} lần mức trung vị 30 ngày của các giao dịch cùng loại"
    if feature == "log_amount_to_type_mean":
        return f"Số tiền gấp {qty:.1f} lần mức trung bình 7 ngày của các giao dịch cùng loại"
    if feature == "log_seconds_since_previous":
        gap = fmt_duration(qty)
        if z_score < 0:
            return f"Giao dịch dồn dập: chỉ cách yêu cầu trước {gap}"
        return f"Cách yêu cầu chuyển tiền trước đó {gap}, lâu hơn thường lệ"
    if feature == "log_src_history_count":
        if z_score < 0:
            return f"Tài khoản nguồn chỉ có {qty:.0f} yêu cầu chuyển tiền trước đó"
        return f"Tài khoản nguồn đã có {qty:.0f} yêu cầu trước đó, nhiều hơn thường lệ"
    if feature == "log_dst_transfer_attempt_distinct_sources":
        return (
            f"Tài khoản nhận đã nhận tiền từ {qty:.0f} nguồn khác nhau trong {NEIGHBOURHOOD} "
            "trước đó"
        )
    if feature == "log_src_transfer_attempt_distinct_sources":
        return (
            f"Tài khoản nguồn đã nhận tiền từ {qty:.0f} nguồn khác nhau trong {NEIGHBOURHOOD} "
            "trước đó"
        )
    if feature == "log_src_received_settled_transfer_amount":
        return f"Tài khoản nguồn vừa nhận tổng {fmt_amount(qty)} trong {NEIGHBOURHOOD} trước đó"
    if feature == "log_src_transfer_attempt_count":
        return (
            f"Tài khoản nguồn có {qty:.0f} yêu cầu chuyển đi trong {NEIGHBOURHOOD} trước đó"
        )
    return None


def describe_factor(factor, signals=None):
    """Return a one-line Vietnamese explanation for a wire factor dict."""
    feature = factor["feature"]
    if "z_score" in factor:
        z_score = factor["z_score"]
        text = _quantified(feature, z_score, signals)
        if text is None and feature.startswith("log_src_request_count_"):
            window = WINDOWS.get(feature.rsplit("_", 1)[-1])
            if window:
                if z_score > 0:
                    return f"Tài khoản nguồn gửi dồn dập yêu cầu chuyển tiền trong {window}"
                return f"Tài khoản nguồn gần như không có yêu cầu chuyển tiền nào trong {window}"
        if text is None:
            pair = NUMERIC.get(feature)
            if pair is None:
                return _fallback(feature, z_score)
            text = pair[0] if z_score > 0 else pair[1]
        return f"{text} (z={z_score:+.1f})"
    if feature in FLAGS:
        return FLAGS[feature]
    if feature == "channel_prior_fraction":
        percent = factor["value"] * 100
        return f"Kênh giao dịch hiếm với tài khoản này (chỉ {percent:.1f}% giao dịch trước đó)"
    return f"{feature} = {factor['value']}"
