package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

func TestIsExcusedLeaveStatus(t *testing.T) {
	sah := []string{
		"Dinas Luar (DL)", "Cuti", "Izin Sakit", "Lainnya",
		"CS - Cuti Sakit", "DL - Dinas Luar Daerah", "CM - Cuti Bersalin",
		"CB - Cuti Besar", "CAP - Cuti karena Alasan Penting", "CH - Cuti Tahunan",
		"CLTN - Cuti Diluar Tanggungan Negara", "CBA - Cuti Bersama",
		"TUBEL - Tugas Belajar", "DD - Dinas Dalam Daerah",
	}
	for _, s := range sah {
		if !models.IsExcusedLeaveStatus(s) {
			t.Errorf("want sah: %q", s)
		}
	}
	bukan := []string{"Terlambat", "Belum Scan", "Hadir Apel Pagi", "Pulang Cepat", "", "DL", "Cuti Bersama"}
	for _, s := range bukan {
		if models.IsExcusedLeaveStatus(s) {
			t.Errorf("want bukan sah: %q", s)
		}
	}
	if len(models.ExcusedSessions()) != 14 {
		t.Errorf("want 14 sesi (3 legacy + 10 ekinerja + Lainnya), got %d", len(models.ExcusedSessions()))
	}
}

func setupEnrichService(t *testing.T, tx []models.BioTimeTransaction, emps []models.BioTimeEmployee, audits []models.AuditVerifyRequest) (*AttendanceService, context.Context) {
	t.Helper()
	server, client := setupAlphaMock(t, tx, emps, nil)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	auditSvc := NewAuditService(filepath.Join(dir, "audit_ekin.json"), nil)
	for _, a := range audits {
		if _, err := auditSvc.RecordAudit(a, "127.0.0.1"); err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
	}
	svc := NewAttendanceService(client, NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(auditSvc)
	return svc, context.Background()
}

func TestEnrich_EkinerjaDL_CountsAsSah(t *testing.T) {
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 15:35:00"},
	}
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
		{ID: 2, EmpCode: "B", FirstName: "Budi", FullName: "Budi"},
	}
	svc, ctx := setupEnrichService(t, tx, emps, []models.AuditVerifyRequest{
		{EmpCode: "B", Name: "Budi", Department: "Umum", Date: "2026-08-24", Session: "DL", PhysicalStatus: "DL - Dinas Luar Daerah", Notes: "SPT 123"},
	})

	summary := svc.GetAttendanceByDate(ctx, monday)
	var budi *models.DailyAttendanceLog
	for i := range summary.Logs {
		if summary.Logs[i].EmpCode == "B" {
			budi = &summary.Logs[i]
		}
	}
	if budi == nil {
		t.Fatal("log Budi (Alpha + dispensasi DL) hilang")
	}
	if budi.CheckInStatus != "DL - Dinas Luar Daerah" || budi.CheckOutStatus != "DL - Dinas Luar Daerah" {
		t.Errorf("Budi harus berstatus DL ekinerja, got %q / %q", budi.CheckInStatus, budi.CheckOutStatus)
	}
	if budi.IsAnomaly {
		t.Error("Budi berdispensasi tidak boleh anomali")
	}
	if summary.TepatWaktuCount != 2 || summary.ApelSoreCount != 2 {
		t.Errorf("Tepat=%d ApelSore=%d, want 2/2", summary.TepatWaktuCount, summary.ApelSoreCount)
	}
	if summary.KepatuhanPercent < 99.9 || summary.KepatuhanPercent > 100.1 {
		t.Errorf("Kepatuhan want 100, got %.2f", summary.KepatuhanPercent)
	}
}

func TestEnrich_LegacyCuti_StillWorks(t *testing.T) {
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
	}
	svc, ctx := setupEnrichService(t, nil, emps, []models.AuditVerifyRequest{
		{EmpCode: "A", Name: "Andi", Date: "2026-08-24", Session: "Cuti", PhysicalStatus: "Cuti", Notes: "Cuti tahunan"},
	})

	summary := svc.GetAttendanceByDate(ctx, monday)
	if len(summary.Logs) != 1 {
		t.Fatalf("want 1 log, got %d", len(summary.Logs))
	}
	if summary.Logs[0].CheckInStatus != "Cuti" {
		t.Errorf("catatan legacy Cuti harus tetap jadi %q, got %q", "Cuti", summary.Logs[0].CheckInStatus)
	}
}

func TestEnrich_Lainnya_CountsAsSah(t *testing.T) {
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
	}
	svc, ctx := setupEnrichService(t, nil, emps, []models.AuditVerifyRequest{
		{EmpCode: "A", Name: "Andi", Date: "2026-08-24", Session: "Lainnya", PhysicalStatus: "Lainnya", Notes: "Tugas mendesak pimpinan"},
	})

	summary := svc.GetAttendanceByDate(ctx, monday)
	if len(summary.Logs) != 1 {
		t.Fatalf("want 1 log, got %d", len(summary.Logs))
	}
	if summary.Logs[0].CheckInStatus != "Lainnya" || summary.Logs[0].CheckOutStatus != "Lainnya" {
		t.Errorf("dispensasi Lainnya harus disahkan, got %q / %q", summary.Logs[0].CheckInStatus, summary.Logs[0].CheckOutStatus)
	}
	if summary.TepatWaktuCount != 1 || summary.ApelSoreCount != 1 {
		t.Errorf("Tepat=%d ApelSore=%d, want 1/1", summary.TepatWaktuCount, summary.ApelSoreCount)
	}
}
