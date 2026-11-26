from app.reasons import describe_factor


def test_window_feature_names_the_window():
    text = describe_factor({"feature": "log_src_request_count_300s", "z_score": 4.0})
    assert "5 phút" in text


def test_unknown_feature_still_reads_as_text():
    text = describe_factor({"feature": "mystery", "z_score": -2.0})
    assert text == "Chỉ số mystery thấp hơn bình thường 2.0 độ lệch chuẩn"


def test_every_numeric_reason_fits_server_limit():
    for feature in ("log_amount", "log_seconds_since_previous", "log_amount_to_type_median"):
        for z in (-9.0, 9.0):
            assert len(describe_factor({"feature": feature, "z_score": z})) <= 200
