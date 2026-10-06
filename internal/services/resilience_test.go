package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dashboard-presensi/internal/models"

	"github.com/go-resty/resty/v2"
)

func newResilienceClient(t *testing.T, server *httptest.Server) *BioTimeClient {
	t.Helper()
	return &BioTimeClient{
		client:   resty.New().SetBaseURL(server.URL).SetTimeout(5 * time.Second),
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}
}

// Error BioTime harus di-negative-cache: 2 panggilan berurutan = 1 hit server.
func TestNegativeCache_ErrorCachedFast(t *testing.T) {
	var txHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "neg"})
			return
		}
		if r.URL.Path == "/iclock/api/transactions/" {
			atomic.AddInt32(&txHits, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := NewAttendanceService(newResilienceClient(t, server), NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(nil)
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)

	first := svc.GetAttendanceByDate(context.Background(), monday)
	second := svc.GetAttendanceByDate(context.Background(), monday)

	if first.ErrorMessage == "" || second.ErrorMessage == "" {
		t.Fatal("keduanya harus membawa ErrorMessage BioTime")
	}
	if got := atomic.LoadInt32(&txHits); got != 1 {
		t.Fatalf("error harus di-cache: want 1 hit server, got %d", got)
	}
}

// 8 pemanggil concurrent saat cache-miss = 1 fetch BioTime (single-flight).
func TestSingleflight_CoalescesConcurrent(t *testing.T) {
	var txHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "sf"})
			return
		}
		if r.URL.Path == "/iclock/api/transactions/" {
			atomic.AddInt32(&txHits, 1)
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.TransactionResponse{Count: 0, Code: 0, Data: []models.BioTimeTransaction{}})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count":0,"code":0,"data":[]}`))
	}))
	defer server.Close()

	svc := NewAttendanceService(newResilienceClient(t, server), NewAttendanceEngine(), NewAttendanceCache())
	svc.SetAuditService(nil)
	monday, _ := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)

	const n = 8
	results := make([]*models.AttendanceSummary, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = svc.GetAttendanceByDate(context.Background(), monday)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&txHits); got != 1 {
		t.Fatalf("single-flight: want 1 hit server, got %d", got)
	}
	for i, r := range results {
		if r == nil {
			t.Fatalf("hasil %d nil (bocor dari single-flight)", i)
		}
		if r != results[0] {
			t.Fatalf("hasil %d harus pointer ringkasan yang sama", i)
		}
	}
}

// Timeout harus ikut BIOTIME_TIMEOUT_SECONDS dari env, bukan hardcode.
func TestBioTimeout_FromEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // lebih lama dari timeout uji
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "slow"})
	}))
	defer server.Close()

	t.Setenv("BIOTIME_BASE_URL", server.URL)
	t.Setenv("BIOTIME_TIMEOUT_SECONDS", "1")
	client := NewBioTimeClient()

	start := time.Now()
	_, err := client.GetToken(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("harus timeout, malah sukses")
	}
	if elapsed > 4*time.Second {
		t.Fatalf("timeout env diabaikan: butuh %v untuk gagal", elapsed)
	}
}
