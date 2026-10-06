package models

import "time"

// Role merepresentasikan entitas hak akses / peran dalam sistem
type Role struct {
	ID          int       `json:"id"`
	RoleCode    string    `json:"role_code"`
	RoleName    string    `json:"role_name"`
	Description string    `json:"description"`
	UserCount   int       `json:"user_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RoleDTO representasi publik role
type RoleDTO struct {
	ID          int    `json:"id"`
	RoleCode    string `json:"role_code"`
	RoleName    string `json:"role_name"`
	Description string `json:"description"`
	UserCount   int    `json:"user_count"`
	CreatedAt   string `json:"created_at"`
}

// ToDTO mengonversi Role ke RoleDTO
func (r *Role) ToDTO() RoleDTO {
	return RoleDTO{
		ID:          r.ID,
		RoleCode:    r.RoleCode,
		RoleName:    r.RoleName,
		Description: r.Description,
		UserCount:   r.UserCount,
		CreatedAt:   r.CreatedAt.Format("02/01/2006 15:04"),
	}
}

// CreateRoleRequest payload untuk tambah role baru
type CreateRoleRequest struct {
	RoleCode    string `json:"role_code"`
	RoleName    string `json:"role_name"`
	Description string `json:"description"`
}

// UpdateRoleRequest payload untuk edit role
type UpdateRoleRequest struct {
	RoleCode    string `json:"role_code"`
	RoleName    string `json:"role_name"`
	Description string `json:"description"`
}
