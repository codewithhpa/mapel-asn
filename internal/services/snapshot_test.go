package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

func TestSnapshot_RoundtripStripsRaw(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir, 24*time.Hour)

	summary := &models.AttendanceSummary{
		TotalPegawai:     1,
		TepatWaktuCount:  1,
		ApelSoreCount:    1,
		KepatuhanPercent: 100,
		Logs: []models.DailyAttendanceLog{
			{
				EmpCode:         "A",
				Name:            "Andi",
				Date:            "2026-08-24",
				CheckInStatus:   models.StatusTepatWaktu,
				RawTransactions: []models.BioTimeTransaction{{ID: 1, EmpCode: "A"}},
			},
		},
	}
	store.Save("date:2026-08-24", summary)

	got, ok := store.Load("date:2026-08-24")
	if !ok {
		t.Fatal("snapshot harus bisa dibaca kembali")
	}
	if got.TotalPegawai != 1 || len(got.Logs) != 1 {
		t.Fatalf("isi snapshot rusak: %+v", got)
	}
	if len(got.Logs[0].RawTransactions) != 0 {
		t.Error("RawTransactions harus dibuang agar file ramping")
	}
	if _, err := os.Stat(filepath.Join(dir, "date_2026-08-24.json")); err != nil {
		t.Errorf("nama file harus aman di Windows (tanpa ':'): %v", err)
	}
}

func TestSnapshot_ErrorNeverSaved(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir, 24*time.Hour)
	store.Save("date:2026-08-24", &models.AttendanceSummary{ErrorMessage: "boom"})
	if _, ok := store.Load("date:2026-08-24"); ok {
		t.Fatal("ringkasan error TIDAK BOLEH menjadi snapshot")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("dir harus tetap kosong, got %d file", len(entries))
	}
}

func TestSnapshot_ExpiredRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore(dir, 24*time.Hour)

	payload, _ := json.Marshal(snapshotFile{
		SavedAt: time.Now().Add(-48 * time.Hour),
		Summary: &models.AttendanceSummary{TotalPegawai: 5},
	})
	_ = os.WriteFile(filepath.Join(dir, "date_2026-08-01.json"), payload, 0644)

	if _, ok := store.Load("date:2026-08-01"); ok {
		t.Fatal("snapshot 48 jam (max 24 jam) harus ditolak")
	}
}

// Simulasi cold-start: svc1 sukses (snapshot tertulis) → svc2 fresh dengan BioTime mati total.
func TestSnapshot_ColdRestartServesDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SNAPSHOT_DIR", dir)
	t.Setenv("SNAPSHOT_MAX_AGE_HOURS", "24")

	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 15:35:00"},
	}
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
		{ID: 2, EmpCode: "B", FirstName: "Budi", FullName: "Budi"},
	}
	serverOK, clientOK := setupAlphaMock(t, tx, emps, nil)
	defer serverOK.Close()

	svc1 := NewAttendanceService(clientOK, NewAttendanceEngine(), NewAttendanceCache())
	svc1.SetAuditService(nil)
	first := svc1.GetAttendanceByDate(context.Background(), monday)
	if first.ErrorMessage != "" || first.TotalPegawai != 2 {
		t.Fatalf("setup: want sukses Total=2, got %+v", first)
	}

	var txHits int32
	serverDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "down"})
			return
		}
		atomic.AddInt32(&txHits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"mati total"}`))
	}))
	defer serverDown.Close()

	svc2 := NewAttendanceService(newResilienceClient(t, serverDown), NewAttendanceEngine(), NewAttendanceCache())
	svc2.SetAuditService(nil)
	got := svc2.GetAttendanceByDate(context.Background(), monday)

	if got.ErrorMessage != "" {
		t.Fatalf("cold-start harus sajikan snapshot, bukan error: %s", got.ErrorMessage)
	}
	if got.TotalPegawai != 2 || len(got.Logs) != 2 {
		t.Fatalf("snapshot harus utuh (Total=2, 2 baris): Total=%d Logs=%d", got.TotalPegawai, len(got.Logs))
	}
	if hits := atomic.LoadInt32(&txHits); hits != 1 {
		t.Fatalf("gagal cepat: want 1 hit (lalu negative-cache), got %d", hits)
	}
	// Panggilan kedua: dari memory/negative-cache, tanpa hit baru
	_ = svc2.GetAttendanceByDate(context.Background(), monday)
	if hits := atomic.LoadInt32(&txHits); hits != 1 {
		t.Fatalf("panggilan kedua harus tanpa hit baru, got %d", hits)
	}
}
