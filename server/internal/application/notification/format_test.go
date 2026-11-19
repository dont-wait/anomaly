package notification

import "testing"

func TestFormatVND(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 1000: "1.000", 1500000: "1.500.000", 12345678901: "12.345.678.901", -2500: "-2.500"} {
		if got := FormatVND(in); got != want {
			t.Fatalf("%d: got %s want %s", in, got, want)
		}
	}
}

func TestLast4(t *testing.T) {
	if Last4("1000000111") != "0111" || Last4("12") != "12" {
		t.Fatal("unexpected Last4")
	}
}
