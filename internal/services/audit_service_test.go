package services

import (
	"os"
	"path/filepath"
	"testing"

	"dashboard-presensi/internal/models"
)

func TestAuditService_RecordAndRetrieve(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_audit.json")

	svc := NewAuditService(filePath, nil)

	req1 := models.AuditVerifyRequest{
		EmpCode:        "199304152018022001",
		Name:           "Desy Rahmawati",
		Department:     "Sekda",
		Session:        "Apel Pagi",
		PhysicalStatus: models.AuditStatusHadirBarisan,
		Notes:          "Berada di barisan peleton 1",
		VerifierName:   "Bripka Supriadi (Provos)",
	}

	rec1, err := svc.RecordAudit(req1, "127.0.0.1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rec1.PhysicalStatus != models.AuditStatusHadirBarisan {
		t.Errorf("Expected status '%s', got '%s'", models.AuditStatusHadirBarisan, rec1.PhysicalStatus)
	}

	req2 := models.AuditVerifyRequest{
		EmpCode:        "198207092008011005",
		Name:           "Ahmad Fadillah",
		Department:     "Inspektorat",
		Session:        "Apel Pagi",
		PhysicalStatus: models.AuditStatusAnomaliTapRun,
		Notes:          "Scan jam 06:20 tapi kabur tidak ikut barisan",
		VerifierName:   "Bripka Supriadi (Provos)",
	}

	_, err = svc.RecordAudit(req2, "127.0.0.1")
	if err != nil {
		t.Fatalf("Expected no error on req2, got %v", err)
	}

	// Verify persistence: create new service instance pointing to same file
	svcReloaded := NewAuditService(filePath, nil)
	logs := svcReloaded.GetAllLogs()
	if len(logs) != 2 {
		t.Fatalf("Expected 2 logs after reload, got %d", len(logs))
	}

	// Latest should be first in GetAllLogs
	if logs[0].EmpCode != "198207092008011005" {
		t.Errorf("Expected latest log to be Ahmad, got %s", logs[0].EmpCode)
	}

	// Test GetLatestAuditMap
	dateStr := rec1.Date
	auditMap := svcReloaded.GetLatestAuditMap(dateStr)
	desyKey := "199304152018022001_Apel Pagi"
	if auditMap[desyKey] == nil || auditMap[desyKey].PhysicalStatus != models.AuditStatusHadirBarisan {
		t.Errorf("Expected Desy audit map to have status Hadir di Barisan")
	}

	_ = os.Remove(filePath)
}
