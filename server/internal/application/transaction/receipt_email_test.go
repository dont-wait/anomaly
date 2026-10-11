package transaction

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTransferReceiptBilingualAndBranded(t *testing.T) {
	postedAt := time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)
	subject, text, html, err := RenderTransferReceipt(TransferReceiptData{
		Reference:        "FTABC123",
		SenderName:       "Nguyen Van A",
		SenderAccount:    "012345678901",
		RecipientName:    "Nguyen Van B",
		RecipientAccount: "987654321098",
		RecipientBank:    "ANOMALY",
		Amount:           1250000,
		Fee:              0,
		Currency:         "VND",
		Note:             "Thanh toan don hang",
		PostedAt:         postedAt,
		AuthMethod:       "Email OTP",
	})
	if err != nil {
		t.Fatalf("RenderTransferReceipt() error = %v", err)
	}

	if subject != transferReceiptSubject {
		t.Fatalf("subject = %q, want %q", subject, transferReceiptSubject)
	}
	for _, value := range []string{
		"Giao dịch thành công",
		"Transfer successful",
		"1.250.000 VND",
		"Mã tham chiếu",
		"Reference",
		"Thanh toan don hang",
		"8901",
		"1098",
	} {
		if !strings.Contains(html, value) && !strings.Contains(text, value) {
			t.Fatalf("receipt missing %q", value)
		}
	}
	if strings.Contains(html, "{{") {
		t.Fatal("HTML contains unrendered template action")
	}
	if strings.Contains(html, "012345678901") || strings.Contains(html, "987654321098") {
		t.Fatal("receipt exposes an unmasked account number")
	}
	for _, color := range []string{"#00236f", "#b582b5", "#8b5cf6", "#faf8ff", "#f2f3ff"} {
		if !strings.Contains(html, color) {
			t.Fatalf("HTML missing brand color %q", color)
		}
	}
}

func TestRenderTransferReceiptUsesDefaults(t *testing.T) {
	_, text, html, err := RenderTransferReceipt(TransferReceiptData{
		Amount: 1000,
	})
	if err != nil {
		t.Fatalf("RenderTransferReceipt() error = %v", err)
	}

	for _, value := range []string{"1.000 VND", "—", "Email OTP", "UTC+07:00"} {
		if !strings.Contains(html, value) && !strings.Contains(text, value) {
			t.Fatalf("receipt missing default value %q", value)
		}
	}
}
