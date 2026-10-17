package ports

// LoanAppendEvent mô tả một sự kiện cần ghi vào Loan Stream.
type LoanAppendEvent struct {
	EventType string
	Payload   any
}
