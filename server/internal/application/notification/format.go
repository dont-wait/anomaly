package notification

import (
	"strconv"
	"strings"
)

// FormatVND groups thousands with dots: 1500000 -> "1.500.000".
func FormatVND(amount int64) string {
	digits := strconv.FormatInt(amount, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var out []byte
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, digits[i])
	}
	return sign + string(out)
}

// Last4 returns the trailing 4 characters of an account number for masked
// display ("•••• 1234"); shorter values are returned as is.
func Last4(accountNo string) string {
	runes := []rune(accountNo)
	if len(runes) <= 4 {
		return accountNo
	}
	return string(runes[len(runes)-4:])
}
