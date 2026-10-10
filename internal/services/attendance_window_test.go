package services

import (
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

// TestEngine_CheckinWindowStart_IgnoresEarlyScans membuktikan jendela scan pagi
// (CHECKIN_WINDOW_START) benar-benar dibaca engine: scan sebelum jam jendela
// tidak dihitung sebagai scan masuk apel pagi.
func TestEngine_CheckinWindowStart_IgnoresEarlyScans(t *testing.T) {
	t.Run("scan sebelum jendela diabaikan, scan sah tetap dihitung", func(t *testing.T) {
		e := NewAttendanceEngine()
		e.SetRule(&models.AttendanceRule{
			WorkStartTime:        "07:00:00",
			CheckinToleranceTime: "07:15:00",
			CheckinWindowStart:   "06:15:00",
		})
		s := e.ProcessDailyAttendance([]models.BioTimeTransaction{
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 05:30:00"}, // sebelum jendela
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 07:10:00"}, // scan sah
		}, time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local))
		if len(s.Logs) != 1 {
			t.Fatalf("mau 1 log, dapat %d", len(s.Logs))
		}
		if s.Logs[0].CheckInDisplay != "07:10:00" {
			t.Errorf("scan sah yang harus dipakai, dapat %s", s.Logs[0].CheckInDisplay)
		}
		if s.Logs[0].CheckInStatus != models.StatusTepatWaktu {
			t.Errorf("07:10 harus tepat waktu, dapat %s", s.Logs[0].CheckInStatus)
		}
	})

	t.Run("hanya scan sebelum jendela = belum scan", func(t *testing.T) {
		e := NewAttendanceEngine()
		e.SetRule(&models.AttendanceRule{
			WorkStartTime:        "07:00:00",
			CheckinToleranceTime: "07:15:00",
			CheckinWindowStart:   "06:15:00",
		})
		s := e.ProcessDailyAttendance([]models.BioTimeTransaction{
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 05:30:00"},
		}, time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local))
		if s.Logs[0].CheckInStatus != models.StatusBelumScan {
			t.Errorf("scan sebelum jendela harus diabaikan (Belum Scan), dapat %s", s.Logs[0].CheckInStatus)
		}
	})

	t.Run("aturan jendela bisa diubah admin dan tetap dipatuhi", func(t *testing.T) {
		e := NewAttendanceEngine()
		e.SetRule(&models.AttendanceRule{
			WorkStartTime:        "07:00:00",
			CheckinToleranceTime: "07:15:00",
			CheckinWindowStart:   "06:30:00", // admin perketat jendela
		})
		s := e.ProcessDailyAttendance([]models.BioTimeTransaction{
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 06:20:00"}, // kini sebelum jendela
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 06:35:00"}, // sah
		}, time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local))
		if s.Logs[0].CheckInDisplay != "06:35:00" {
			t.Errorf("scan sah setelah jendela baru yang dipakai, dapat %s", s.Logs[0].CheckInDisplay)
		}
	})

	t.Run("tanpa rule, default 06:15 tetap berlaku", func(t *testing.T) {
		e := NewAttendanceEngine()
		s := e.ProcessDailyAttendance([]models.BioTimeTransaction{
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 06:10:00"}, // sebelum default
			{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-10-13 06:20:00"},
		}, time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local))
		if s.Logs[0].CheckInDisplay != "06:20:00" {
			t.Errorf("default jendela 06:15 harus mengabaikan 06:10, dapat %s", s.Logs[0].CheckInDisplay)
		}
	})
}
