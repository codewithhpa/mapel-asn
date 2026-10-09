package services

import (
	"testing"

	"dashboard-presensi/internal/models"
)

// Urutan periode harus deterministik: tanggal terbaru di atas, lalu NIP menaik.
func TestProcessPeriodAttendance_DateDescEmpAsc(t *testing.T) {
	tx := []models.BioTimeTransaction{
		{EmpCode: "B", FirstName: "Budi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "B", FirstName: "Budi", PunchTime: "2026-08-25 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-25 07:00:00"},
	}
	e := NewAttendanceEngine()
	s := e.ProcessPeriodAttendance(tx)

	if len(s.Logs) != 4 {
		t.Fatalf("want 4 logs, got %d", len(s.Logs))
	}
	want := [][2]string{
		{"2026-08-25", "A"}, {"2026-08-25", "B"},
		{"2026-08-24", "A"}, {"2026-08-24", "B"},
	}
	for i, w := range want {
		if s.Logs[i].Date != w[0] || s.Logs[i].EmpCode != w[1] {
			t.Errorf("pos %d: want %v, got %s/%s", i, w, s.Logs[i].Date, s.Logs[i].EmpCode)
		}
	}
}
