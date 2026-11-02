package savings

import "testing"

func TestSavingsTermsValidate(t *testing.T) {
	valid := TermsSnapshot{
		Version: "savings-standard-v1", Title: "Điều kiện gửi tiền",
		Introduction: "Điều kiện được lưu trong cơ sở dữ liệu.",
		Sections:     []TermsSection{{Title: "Kỳ hạn", Body: "Đáo hạn theo hợp đồng."}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	valid.Sections[0].Body = " "
	if err := valid.Validate(); err == nil {
		t.Fatal("empty terms section must fail closed")
	}
}
