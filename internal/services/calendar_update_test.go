package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

// TestSpecialHoursUpdate_TakesEffectAfterInvalidate membuktikan alur nyata:
// jam khusus Ramadhan di-update setelah SE Bupati menyusul -> InvalidateCache ->
// evaluasi berikutnya memakai jam baru (tidak menunggu TTL).
func TestSpecialHoursUpdate_TakesEffectAfterInvalidate(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwt-api-token-auth/":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case "/iclock/api/transactions/":
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "code": 0, "data": []any{
				map[string]any{"emp_code": "NIP1", "first_name": "Budi", "department": "Sekretariat", "punch_time": "2026-02-25 07:40:00"},
			}})
		case "/personnel/api/employees/":
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "data": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mock.Close()
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("BIOTIME_USERNAME", "admin")
	t.Setenv("BIOTIME_PASSWORD", "pw")
	t.Setenv("CACHE_TTL_SECONDS", "3600") // TTL panjang: bukti efek datang dari invalidasi, bukan kedaluwarsa

	cal := NewCalendarService(nil)
	ev, err := cal.CreateEvent(models.CalendarEventRequest{
		Type: models.CalendarTypeJamKhusus, Name: "Ramadhan 1447",
		StartDate: "2026-02-18", EndDate: "2026-03-19",
		CheckinToleranceTime: "07:15:00", // belum ada SE: pakai default dulu
	}, "Tes")
	if err != nil {
		t.Fatalf("setup jam khusus gagal: %v", err)
	}

	svc := NewAttendanceService(NewBioTimeClient(), NewAttendanceEngine(), NewAttendanceCache())
	svc.SetCalendarService(cal)

	date := time.Date(2026, 2, 25, 0, 0, 0, 0, time.Local)
	s1 := svc.GetAttendanceByDate(context.Background(), date)
	if len(s1.Logs) != 1 || s1.Logs[0].CheckInStatus != models.StatusTerlambat {
		t.Fatalf("toleransi 07:15: scan 07:40 harus terlambat, dapat %+v", s1.Logs)
	}

	// SE Bupati turun menyusul: toleransi jadi 07:45 -> update event.
	if _, err := cal.UpdateEvent(ev.ID, models.CalendarEventRequest{
		Type: models.CalendarTypeJamKhusus, Name: "Ramadhan 1447",
		StartDate: "2026-02-18", EndDate: "2026-03-19",
		CheckinToleranceTime: "07:45:00",
	}); err != nil {
		t.Fatalf("update jam gagal: %v", err)
	}

	// Tanpa invalidasi: cache TTL 1 jam masih memegang evaluasi lama.
	s2 := svc.GetAttendanceByDate(context.Background(), date)
	if s2.Logs[0].CheckInStatus != models.StatusTerlambat {
		t.Fatalf("cache seharusnya masih evaluasi lama (terlambat)")
	}

	// Handler memanggil InvalidateCache setelah ubah kalender -> evaluasi baru.
	svc.InvalidateCache()
	s3 := svc.GetAttendanceByDate(context.Background(), date)
	if s3.Logs[0].CheckInStatus != models.StatusTepatWaktu {
		t.Fatalf("toleransi 07:45 + invalidasi: scan 07:40 harus tepat waktu, dapat %s", s3.Logs[0].CheckInStatus)
	}
}
