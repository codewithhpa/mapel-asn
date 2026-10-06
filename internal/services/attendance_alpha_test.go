package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dashboard-presensi/internal/models"

	"github.com/go-resty/resty/v2"
)

func setupAlphaMock(t *testing.T, txData []models.BioTimeTransaction, emps []models.BioTimeEmployee, resigns []models.BioTimeResign) (*httptest.Server, *BioTimeClient) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwt-api-token-auth/":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "alpha_mock"})
			return
		case "/iclock/api/transactions/":
			if r.Header.Get("Authorization") != "JWT alpha_mock" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.TransactionResponse{Count: len(txData), Code: 0, Data: txData})
			return
		case "/personnel/api/employees/":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.EmployeeListResponse{Count: len(emps), Data: emps})
			return
		case "/personnel/api/resigns/":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.ResignListResponse{Count: len(resigns), Data: resigns})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	client := &BioTimeClient{
		client:   resty.New().SetBaseURL(server.URL).SetTimeout(2 * time.Second),
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}
	return server, client
}

func TestAlphaDaily_JoinMaster(t *testing.T) {
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 15:35:00"},
	}
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
		{ID: 2, EmpCode: "B", FirstName: "Budi", FullName: "Budi"},
		{ID: 3, EmpCode: "C", FirstName: "Cici", FullName: "Cici"},
	}
	server, client := setupAlphaMock(t, tx, emps, nil)
	defer server.Close()

	svc := NewAttendanceService(client, NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(nil)
	summary := svc.GetAttendanceByDate(context.Background(), monday)

	if summary.TotalPegawai != 3 {
		t.Fatalf("TotalPegawai want 3 (master), got %d", summary.TotalPegawai)
	}
	if len(summary.Logs) != 3 {
		t.Fatalf("Logs want 3 (1 hadir + 2 alpha), got %d", len(summary.Logs))
	}
	alpha := 0
	for _, l := range summary.Logs {
		if l.CheckInStatus == models.StatusBelumScan && l.CheckOutStatus == models.StatusBelumScan {
			alpha++
		}
	}
	if alpha != 2 {
		t.Errorf("Alpha want 2, got %d", alpha)
	}
	// Kepatuhan = (1 pagi sah + 1 sore sah) / (3*2) = 33.33%
	if summary.KepatuhanPercent < 33.3 || summary.KepatuhanPercent > 33.4 {
		t.Errorf("Kepatuhan want ~33.33, got %.2f", summary.KepatuhanPercent)
	}
}

func TestAlphaDaily_ResignExcluded(t *testing.T) {
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
	}
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
		{ID: 2, EmpCode: "B", FirstName: "Budi", FullName: "Budi"},
		{ID: 3, EmpCode: "C", FirstName: "Cici", FullName: "Cici"},
	}
	resigns := []models.BioTimeResign{
		{ID: 9, Employee: &models.ResignEmpInfo{EmpCode: "C", FirstName: "Cici"}},
	}
	server, client := setupAlphaMock(t, tx, emps, resigns)
	defer server.Close()

	svc := NewAttendanceService(client, NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(nil)
	summary := svc.GetAttendanceByDate(context.Background(), monday)

	if summary.TotalPegawai != 2 {
		t.Fatalf("TotalPegawai want 2 (C resign), got %d", summary.TotalPegawai)
	}
	for _, l := range summary.Logs {
		if l.EmpCode == "C" {
			t.Fatalf("Resigned C tidak boleh muncul sebagai Alpha")
		}
	}
}

func TestAlphaPeriod_MultiDay(t *testing.T) {
	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "Andi", PunchTime: "2026-08-24 15:35:00"},
	}
	emps := []models.BioTimeEmployee{
		{ID: 1, EmpCode: "A", FirstName: "Andi", FullName: "Andi"},
		{ID: 2, EmpCode: "B", FirstName: "Budi", FullName: "Budi"},
	}
	server, client := setupAlphaMock(t, tx, emps, nil)
	defer server.Close()

	svc := NewAttendanceService(client, NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(nil)
	// Senin-Selasa: Senin A hadir + B alpha, Selasa A+B alpha = 4 baris
	summary := svc.GetAttendanceForPeriod(context.Background(), "2026-08-24", "2026-08-25")

	if summary.TotalPegawai != 2 {
		t.Fatalf("TotalPegawai want 2, got %d", summary.TotalPegawai)
	}
	if len(summary.Logs) != 4 {
		t.Fatalf("Logs want 4 (2 hari x 2 pegawai), got %d", len(summary.Logs))
	}
}
