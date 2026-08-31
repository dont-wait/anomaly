package auth

import "time"

// Claims là thông tin định danh user được mang theo trong JWT.
// Thuộc tầng domain vì cả application (use case) lẫn infrastructure
// (token service) đều cần reference cùng một shape.
type Claims struct {
	UserID    string
	Username  string
	IsVerify  bool
	ExpiresAt time.Time
}
