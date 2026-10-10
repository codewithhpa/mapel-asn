package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dashboard-presensi/internal/models"

	"github.com/go-resty/resty/v2"
)

func newDeviceTestClient(server *httptest.Server) *BioTimeClient {
	return &BioTimeClient{
		client:   resty.New().SetBaseURL(server.URL).SetTimeout(2 * time.Second),
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}
}

func deviceStubServer(t *testing.T, bodies map[string][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/jwt-api-token-auth/":
			json.NewEncoder(w).Encode(models.LoginResponse{Token: "dev_token"})
		case r.URL.Path == "/iclock/api/terminals/":
			w.Write([]byte(`{"count":1,"next":null,"previous":null,"msg":"","code":0,"data":[{"id":4,"sn":"GAT5261100070","ip_address":"192.168.1.10","alias":"SpeedFace M4","terminal_name":"SpeedFace M4","state":"1","area":{"id":4,"area_code":"2","area_name":"DISKOMINFO-Server"},"last_activity":"2026-10-09 13:49:39","user_count":9,"fp_count":0,"face_count":6,"palm_count":0,"transaction_count":0,"transfer_interval":1,"is_attendance":1,"area_name":"DISKOMINFO-Server"}]}`))
		case r.URL.Path == "/iclock/api/terminals/upload_transaction/":
			raw, _ := io.ReadAll(r.Body)
			bodies["upload"] = raw
			json.NewEncoder(w).Encode(models.TerminalActionDetail{Detail: "Success."})
		case r.URL.Path == "/iclock/api/terminals/upload_all/":
			raw, _ := io.ReadAll(r.Body)
			bodies["upload_users"] = raw
			json.NewEncoder(w).Encode(models.TerminalActionDetail{Detail: "Success."})
		case r.URL.Path == "/personnel/api/employees/resync_to_device/":
			raw, _ := io.ReadAll(r.Body)
			bodies["resync"] = raw
			json.NewEncoder(w).Encode(models.TerminalActionDetail{Detail: "Success."})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestBioTimeClient_FetchTerminals(t *testing.T) {
	bodies := map[string][]byte{}
	server := deviceStubServer(t, bodies)
	defer server.Close()

	terms, err := newDeviceTestClient(server).FetchTerminals(context.Background())
	if err != nil {
		t.Fatalf("fetch gagal: %v", err)
	}
	if len(terms) != 1 || terms[0].SN != "GAT5261100070" {
		t.Fatalf("isi terminal salah: %+v", terms)
	}
	if !terms[0].IsOnline() || terms[0].DisplayName() != "SpeedFace M4" {
		t.Fatalf("helper salah: %+v", terms[0])
	}
}

func TestBioTimeClient_UploadTransactions(t *testing.T) {
	bodies := map[string][]byte{}
	server := deviceStubServer(t, bodies)
	defer server.Close()

	if err := newDeviceTestClient(server).UploadTransactions(context.Background(), []int64{4}); err != nil {
		t.Fatalf("upload gagal: %v", err)
	}
	var got struct {
		Terminals []int64 `json:"terminals"`
	}
	if err := json.Unmarshal(bodies["upload"], &got); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(got.Terminals) != 1 || got.Terminals[0] != 4 {
		t.Fatalf("body salah: %s", bodies["upload"])
	}
}

func TestBioTimeClient_ResyncEmployeesToDevice(t *testing.T) {
	bodies := map[string][]byte{}
	server := deviceStubServer(t, bodies)
	defer server.Close()

	if err := newDeviceTestClient(server).ResyncEmployeesToDevice(context.Background(), []int64{11, 22}); err != nil {
		t.Fatalf("resync gagal: %v", err)
	}
	var got struct {
		Employees []int64 `json:"employees"`
	}
	if err := json.Unmarshal(bodies["resync"], &got); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(got.Employees) != 2 || got.Employees[0] != 11 || got.Employees[1] != 22 {
		t.Fatalf("body salah: %s", bodies["resync"])
	}
}

func TestBioTimeClient_UploadUsersFromDevice(t *testing.T) {
	bodies := map[string][]byte{}
	server := deviceStubServer(t, bodies)
	defer server.Close()

	if err := newDeviceTestClient(server).UploadUsersFromDevice(context.Background(), []int64{4}); err != nil {
		t.Fatalf("upload users gagal: %v", err)
	}
	var got struct {
		Terminals []int64 `json:"terminals"`
	}
	if err := json.Unmarshal(bodies["upload_users"], &got); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(got.Terminals) != 1 || got.Terminals[0] != 4 {
		t.Fatalf("body salah: %s", bodies["upload_users"])
	}
}
