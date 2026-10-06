package services

import (
	"path/filepath"
	"testing"

	"dashboard-presensi/internal/models"
)

func TestUserService_FullLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_users.json")

	svc := NewUserService(filePath, nil)

	// 1. Test auto seed admin
	admin, err := svc.Authenticate("admin", "adminkey1234")
	if err != nil {
		t.Fatalf("Expected admin to authenticate successfully, got %v", err)
	}
	if admin.Username != "admin" || admin.Role != models.RoleAdmin {
		t.Errorf("Expected role Administrator, got %s", admin.Role)
	}

	// 2. Test wrong password
	_, err = svc.Authenticate("admin", "wrongpassword")
	if err == nil {
		t.Error("Expected error for wrong password, got nil")
	}

	// 3. Test Create User
	req := models.CreateUserRequest{
		Username: "provos1",
		Password: "password123",
		FullName: "Bripka Supriadi",
		NIP:      "19850510 201001 1 003",
		Role:     models.RoleProvos,
	}
	newUser, err := svc.CreateUser(req)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test login with new user
	authUser, err := svc.Authenticate("provos1", "password123")
	if err != nil {
		t.Fatalf("Expected provos1 to authenticate, got %v", err)
	}
	if authUser.FullName != "Bripka Supriadi" {
		t.Errorf("Expected name Bripka Supriadi, got %s", authUser.FullName)
	}

	// 4. Test Update User
	updateReq := models.UpdateUserRequest{
		FullName: "Bripka Supriadi, S.H.",
		NIP:      authUser.NIP,
		Role:     authUser.Role,
		IsActive: models.UserStatusActive,
	}
	updatedDTO, err := svc.UpdateUser(newUser.ID, updateReq)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}
	if updatedDTO.FullName != "Bripka Supriadi, S.H." {
		t.Errorf("Expected updated name, got %s", updatedDTO.FullName)
	}

	// 5. Test Delete Protection on Admin
	err = svc.DeleteUser(admin.ID)
	if err == nil {
		t.Error("Expected deleting admin to fail, got nil")
	}

	// 6. Test Delete custom user
	err = svc.DeleteUser(newUser.ID)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}
}
