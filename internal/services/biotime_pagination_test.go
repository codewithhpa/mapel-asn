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

func TestFetchAllTransactions_Pagination(t *testing.T) {
	all := []models.BioTimeTransaction{
		{ID: 1, EmpCode: "A", PunchTime: "2026-08-01 07:00:00"},
		{ID: 2, EmpCode: "B", PunchTime: "2026-08-01 07:01:00"},
		{ID: 3, EmpCode: "C", PunchTime: "2026-08-01 07:02:00"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(models.LoginResponse{Token: "pg"})
			return
		}
		if r.URL.Path == "/iclock/api/transactions/" {
			page := r.URL.Query().Get("page")
			w.WriteHeader(http.StatusOK)
			switch page {
			case "1":
				_ = json.NewEncoder(w).Encode(models.TransactionResponse{Count: 3, Code: 0, Data: all[:2]})
			case "2":
				_ = json.NewEncoder(w).Encode(models.TransactionResponse{Count: 3, Code: 0, Data: all[2:]})
			default:
				_ = json.NewEncoder(w).Encode(models.TransactionResponse{Count: 3, Code: 0, Data: []models.BioTimeTransaction{}})
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &BioTimeClient{
		client:   resty.New().SetBaseURL(server.URL).SetTimeout(2 * time.Second),
		baseURL:  server.URL,
		username: "a",
		password: "b",
	}
	got, err := client.FetchAllTransactions(context.Background(), "2026-08-01 00:00:00", "2026-08-31 23:59:59", 2, 5)
	if err != nil {
		t.Fatalf("FetchAll error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 records across pages, got %d", len(got))
	}
	if got[2].EmpCode != "C" {
		t.Errorf("page 2 data hilang, got %+v", got[2])
	}
}
