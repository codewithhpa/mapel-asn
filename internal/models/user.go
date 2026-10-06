package models

import "time"

// Status Aktif User (1 = Active, 2 = Inactive)
const (
	UserStatusActive   = 1
	UserStatusInactive = 2
)

// Role Petugas
const (
	RoleAdmin  = "Administrator"
	RoleProvos = "Petugas Verifikasi"
	RolePiket  = "Petugas Piket"
)

// User merepresentasikan entitas akun petugas verifikator / admin
type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	FullName     string    `json:"full_name"`
	NIP          string    `json:"nip"`
	RoleID       int       `json:"role_id"`
	Role         string    `json:"role"`
	IsActive     int       `json:"is_active"` // 1: Active, 2: Inactive
	CreatedAt    time.Time `json:"created_at"`
}

// UserDTO adalah representasi publik aman dari User (tanpa hash password)
type UserDTO struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	NIP       string `json:"nip"`
	RoleID    int    `json:"role_id"`
	Role      string `json:"role"`
	IsActive  int    `json:"is_active"` // 1: Active, 2: Inactive
	CreatedAt string `json:"created_at"`
}

// ToDTO mengonversi User ke UserDTO
func (u *User) ToDTO() UserDTO {
	return UserDTO{
		ID:        u.ID,
		Username:  u.Username,
		FullName:  u.FullName,
		NIP:       u.NIP,
		RoleID:    u.RoleID,
		Role:      u.Role,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt.Format("02/01/2006 15:04"),
	}
}

// UserLoginRequest payload untuk request login
type UserLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateUserRequest payload untuk tambah akun petugas baru
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	NIP      string `json:"nip"`
	RoleID   int    `json:"role_id,omitempty"`
	Role     string `json:"role"`
}

// UpdateUserRequest payload untuk edit akun petugas
type UpdateUserRequest struct {
	FullName string `json:"full_name"`
	NIP      string `json:"nip"`
	RoleID   int    `json:"role_id,omitempty"`
	Role     string `json:"role"`
	Password string `json:"password,omitempty"` // Opsional, jika diisi berarti ganti password
	IsActive int    `json:"is_active"`          // 1: Active, 2: Inactive
}
