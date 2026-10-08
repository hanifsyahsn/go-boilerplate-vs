package dto

import "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"

type User struct {
	ID    int64     `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Role  enum.Role `json:"role"`
}
