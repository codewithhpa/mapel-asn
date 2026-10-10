package services

import (
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

func newTestCalendar() *CalendarService {
	return NewCalendarService(nil) // fallback in-memory (tanpa DB)
}

func TestCalendarService_CreateListDelete(t *testing.T) {
	cal := newTestCalendar()
	created, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeLiburNasional, Name: "Hari Suci Nyepi",
		StartDate: "2026-03-19", EndDate: "2026-03-19",
	}, "Admin Tes")
	if err != nil {
		t.Fatalf("create gagal: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("event salah: %+v", created)
	}

	events, err := cal.ListEvents(2026)
	if err != nil || len(events) != 1 {
		t.Fatalf("list 2026 mau 1 event, dapat %d err=%v", len(events), err)
	}

	// Rentang Idulfitri 21-22 Maret
	if _, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeLiburNasional, Name: "Idulfitri",
		StartDate: "2026-03-21", EndDate: "2026-03-22",
	}, "Admin Tes"); err != nil {
		t.Fatalf("create rentang gagal: %v", err)
	}

	if !cal.IsHolidayDate(time.Date(2026, 3, 21, 0, 0, 0, 0, time.Local)) {
		t.Error("21 Maret harus hari libur")
	}
	if !cal.IsHolidayDate(time.Date(2026, 3, 22, 0, 0, 0, 0, time.Local)) {
		t.Error("22 Maret harus hari libur")
	}
	if cal.IsHolidayDate(time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)) {
		t.Error("24 Maret bukan hari libur")
	}

	holidays := cal.HolidayDatesInRange(
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local))
	if len(holidays) != 3 { // 19, 21, 22
		t.Errorf("rentang Maret harus 3 hari libur, dapat %d", len(holidays))
	}

	if err := cal.DeleteEvent(created.ID); err != nil {
		t.Fatalf("delete gagal: %v", err)
	}
	if cal.IsHolidayDate(time.Date(2026, 3, 19, 0, 0, 0, 0, time.Local)) {
		t.Error("19 Maret harus sudah bukan libur setelah delete")
	}
}

func TestCalendarService_Validation(t *testing.T) {
	cal := newTestCalendar()

	if _, err := cal.CreateEvent(models.CalendarEventRequest{Type: "haha", Name: "X", StartDate: "2026-01-01"}, "a"); err == nil {
		t.Error("tipe tidak valid harus ditolak")
	}
	if _, err := cal.CreateEvent(models.CalendarEventRequest{Type: models.CalendarTypeLiburNasional, Name: "", StartDate: "2026-01-01"}, "a"); err == nil {
		t.Error("nama kosong harus ditolak")
	}
	if _, err := cal.CreateEvent(models.CalendarEventRequest{Type: models.CalendarTypeLiburNasional, Name: "X", StartDate: "2026-01-01", EndDate: "2025-12-31"}, "a"); err == nil {
		t.Error("end sebelum start harus ditolak")
	}
	if _, err := cal.CreateEvent(models.CalendarEventRequest{Type: models.CalendarTypeLiburNasional, Name: "X", StartDate: "01-01-2026"}, "a"); err == nil {
		t.Error("format tanggal salah harus ditolak")
	}
	// End kosong = 1 hari
	ev, err := cal.CreateEvent(models.CalendarEventRequest{Type: models.CalendarTypeLiburNasional, Name: "Satu Hari", StartDate: "2026-08-17"}, "a")
	if err != nil {
		t.Fatalf("end kosong harus jalan: %v", err)
	}
	if ev.EndDate != ev.StartDate {
		t.Errorf("end kosong harus = start, dapat %s", ev.EndDate)
	}
	// jam_kerja_ramadhan: jam wajib format benar
	if _, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeJamKhusus, Name: "Ramadhan", StartDate: "2026-02-18", EndDate: "2026-03-19",
		WorkStartTime: "7:30",
	}, "a"); err == nil {
		t.Error("jam format salah harus ditolak")
	}
	if _, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeJamKhusus, Name: "Ramadhan", StartDate: "2026-02-18", EndDate: "2026-03-19",
		WorkStartTime: "07:30:00", CheckoutTimeMonThu: "14:00:00", CheckoutCutoffMonThu: "14:15:00",
	}, "a"); err != nil {
		t.Errorf("jam valid harus diterima: %v", err)
	}
}

func TestEngine_SkipsHolidayAndAppliesSpecialHours(t *testing.T) {
	cal := newTestCalendar()
	_, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeCutiBersama, Name: "Cuti Bersama Idulfitri",
		StartDate: "2026-03-20", EndDate: "2026-03-20",
	}, "a")
	if err != nil {
		t.Fatalf("setup cuti gagal: %v", err)
	}
	_, err = cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeJamKhusus, Name: "Ramadhan",
		StartDate: "2026-02-18", EndDate: "2026-03-19",
		WorkStartTime: "07:30:00", CheckinToleranceTime: "07:45:00",
		CheckoutTimeMonThu: "14:00:00", CheckoutCutoffMonThu: "14:15:00",
		CheckoutTimeFri: "13:30:00", CheckoutCutoffFri: "13:45:00",
	}, "a")
	if err != nil {
		t.Fatalf("setup jam khusus gagal: %v", err)
	}

	engine := NewAttendanceEngine()
	engine.SetCalendar(cal)

	// 20 Maret (Jumat, cuti bersama) → summary libur, tidak proses tap.
	s := engine.ProcessDailyAttendance([]models.BioTimeTransaction{
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-03-20 07:10:00"},
	}, time.Date(2026, 3, 20, 0, 0, 0, 0, time.Local))
	if len(s.Logs) != 0 || s.HolidayName == "" {
		t.Fatalf("hari cuti bersama harus summary libur kosong, dapat %+v", s)
	}

	// 25 Feb (Rabu, Ramadhan) → scan 07:40 masih tepat waktu (toleransi 07:45),
	// scan sore 13:50 sah (jendela 14:00-14:15? tidak — 13:50 < 14:00 → Pulang Cepat).
	// Buat yang jelas: scan sore 14:05 = sah.
	s2 := engine.ProcessDailyAttendance([]models.BioTimeTransaction{
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-02-25 07:40:00"},
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-02-25 14:05:00"},
	}, time.Date(2026, 2, 25, 0, 0, 0, 0, time.Local))
	if len(s2.Logs) != 1 {
		t.Fatalf("mau 1 log, dapat %d", len(s2.Logs))
	}
	l := s2.Logs[0]
	if l.CheckInStatus != models.StatusTepatWaktu {
		t.Errorf("07:40 dgn toleransi Ramadhan 07:45 harus tepat waktu, dapat %s", l.CheckInStatus)
	}
	if l.CheckOutStatus != models.StatusSahApelSore {
		t.Errorf("14:05 dgn jendela Ramadhan 14:00-14:15 harus sah, dapat %s", l.CheckOutStatus)
	}

	// Tanpa kalender (engine baru): 07:40 = terlambat (default 07:15) — memastikan kalender benar-benar mengubah evaluasi.
	enginePlain := NewAttendanceEngine()
	s3 := enginePlain.ProcessDailyAttendance([]models.BioTimeTransaction{
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-02-25 07:40:00"},
	}, time.Date(2026, 2, 25, 0, 0, 0, 0, time.Local))
	if s3.Logs[0].CheckInStatus != models.StatusTerlambat {
		t.Errorf("tanpa kalender 07:40 harus terlambat, dapat %s", s3.Logs[0].CheckInStatus)
	}

	// Periode: hari libur di-skip (tidak ada log untuk 20 Maret).
	sp := engine.ProcessPeriodAttendance([]models.BioTimeTransaction{
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-03-19 07:10:00"}, // Kamis, Nyepi? tidak — 19 tidak diset libur di test ini; 20 cuti
		{EmpCode: "NIP1", FirstName: "Budi", PunchTime: "2026-03-20 07:10:00"},
	})
	found := false
	for _, lg := range sp.Logs {
		if lg.Date == "2026-03-20" {
			found = true
		}
	}
	if found {
		t.Error("log hari cuti bersama harus di-skip di periode")
	}
}
