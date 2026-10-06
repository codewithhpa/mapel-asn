package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"dashboard-presensi/internal/models"

	"github.com/go-resty/resty/v2"
)

func TestBioTimeClient_GetToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwt-api-token-auth/" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.LoginResponse{Token: "test_jwt_token_12345"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &BioTimeClient{
		client:   resty.New().SetBaseURL(server.URL).SetTimeout(2 * time.Second),
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}

	token, err := client.GetToken(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if token != "test_jwt_token_12345" {
		t.Errorf("Expected token 'test_jwt_token_12345', got '%s'", token)
	}

	// Test caching: subsequent call should not make another HTTP request
	cachedToken, err := client.GetToken(context.Background())
	if err != nil {
		t.Fatalf("Expected no error on cached token, got %v", err)
	}
	if cachedToken != token {
		t.Errorf("Expected cached token to match")
	}
}

func TestBioTimeClient_FetchTransactions_WithAutoReLogin(t *testing.T) {
	var loginCount int32
	var transCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/jwt-api-token-auth/" {
			atomic.AddInt32(&loginCount, 1)
			json.NewEncoder(w).Encode(models.LoginResponse{Token: "valid_jwt_token"})
			return
		}

		if r.URL.Path == "/iclock/api/transactions/" {
			count := atomic.AddInt32(&transCount, 1)
			auth := r.Header.Get("Authorization")

			// Simulate token expired on first transaction request
			if count == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"detail": "Signature has expired."}`))
				return
			}

			// Expect second request to succeed with new token
			if auth != "JWT valid_jwt_token" {
				t.Errorf("Expected Authorization 'JWT valid_jwt_token', got '%s'", auth)
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.TransactionResponse{
				Count: 1,
				Code:  0,
				Data: []models.BioTimeTransaction{
					{
						ID:        101,
						EmpCode:   "198801122011011002",
						FirstName: "Budi",
						PunchTime: "2026-08-27 06:58:21",
					},
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &BioTimeClient{
		client:      resty.New().SetBaseURL(server.URL).SetTimeout(2 * time.Second),
		baseURL:     server.URL,
		username:    "admin",
		password:    "password",
		cachedToken: "expired_token", // Start with expired token
	}

	resp, err := client.FetchTransactions(context.Background(), "2026-08-27 00:00:00", "2026-08-27 23:59:59", 1, 20)
	if err != nil {
		t.Fatalf("Expected successful fetch after retry, got: %v", err)
	}

	if resp.Count != 1 || len(resp.Data) != 1 {
		t.Errorf("Expected 1 transaction, got %d", resp.Count)
	}

	if resp.Data[0].EmpCode != "198801122011011002" {
		t.Errorf("Expected EmpCode '198801122011011002', got '%s'", resp.Data[0].EmpCode)
	}

	if atomic.LoadInt32(&loginCount) != 1 {
		t.Errorf("Expected 1 re-login call, got %d", loginCount)
	}
}
