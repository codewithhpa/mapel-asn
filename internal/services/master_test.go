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

func setupMockBioTimeServer(t *testing.T) (*httptest.Server, *BioTimeClient) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Auth
		if r.URL.Path == "/jwt-api-token-auth/" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.LoginResponse{Token: "mock_jwt_token"})
			return
		}

		// Check Auth Header
		auth := r.Header.Get("Authorization")
		if auth != "JWT mock_jwt_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// 1. Employees
		if r.URL.Path == "/personnel/api/employees/" {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.EmployeeListResponse{
					Count: 1,
					Data: []models.BioTimeEmployee{
						{
							ID:        10,
							EmpCode:   "19850101",
							FirstName: "Budi",
						},
					},
				})
				return
			} else if r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(models.BioTimeEmployee{
					ID:        11,
					EmpCode:   "19900202",
					FirstName: "Siti",
				})
				return
			}
		}

		if r.URL.Path == "/personnel/api/employees/11/" {
			if r.Method == "PATCH" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.BioTimeEmployee{
					ID:        11,
					EmpCode:   "19900202",
					FirstName: "Siti Updated",
				})
				return
			} else if r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// 2. Departments
		if r.URL.Path == "/personnel/api/departments/" {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.DepartmentListResponse{
					Count: 1,
					Data: []models.BioTimeDepartment{
						{ID: 1, DeptCode: "1", DeptName: "PRODUCTION"},
					},
				})
				return
			} else if r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(models.BioTimeDepartment{
					ID: 2, DeptCode: "2", DeptName: "DISKOMINFO",
				})
				return
			}
		}

		// 3. Areas
		if r.URL.Path == "/personnel/api/areas/" {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.AreaListResponse{
					Count: 1,
					Data: []models.BioTimeArea{
						{ID: 1, AreaCode: "1", AreaName: "Gedung A"},
					},
				})
				return
			}
		}

		// 4. Positions
		if r.URL.Path == "/personnel/api/positions/" {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.PositionListResponse{
					Count: 1,
					Data: []models.BioTimePosition{
						{ID: 1, PositionCode: "1", PositionName: "Staff"},
					},
				})
				return
			}
		}

		// 5. Resigns
		if r.URL.Path == "/personnel/api/resigns/" {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(models.ResignListResponse{
					Count: 1,
					Data: []models.BioTimeResign{
						{ID: 1, ResignDate: "2026-08-30", ResignType: 1, DisableAtt: true},
					},
				})
				return
			} else if r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(models.BioTimeResign{
					ID: 2, ResignDate: "2026-08-30", ResignType: 1, DisableAtt: true,
				})
				return
			}
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

func TestBioTimeMasterCRUD(t *testing.T) {
	server, client := setupMockBioTimeServer(t)
	defer server.Close()

	ctx := context.Background()

	// 1. Employees
	emps, err := client.FetchEmployees(ctx, 1, 10, "", 0)
	if err != nil || len(emps.Data) == 0 {
		t.Fatalf("FetchEmployees failed: %v", err)
	}
	if emps.Data[0].EmpCode != "19850101" {
		t.Errorf("Expected emp_code 19850101, got %s", emps.Data[0].EmpCode)
	}

	createdEmp, err := client.CreateEmployee(ctx, &models.CreateEmployeeRequest{
		EmpCode:   "19900202",
		FirstName: "Siti",
	})
	if err != nil || createdEmp.ID != 11 {
		t.Fatalf("CreateEmployee failed: %v", err)
	}

	updatedEmp, err := client.UpdateEmployee(ctx, 11, &models.UpdateEmployeeRequest{
		FirstName: "Siti Updated",
	})
	if err != nil || updatedEmp.FirstName != "Siti Updated" {
		t.Fatalf("UpdateEmployee failed: %v", err)
	}

	err = client.DeleteEmployee(ctx, 11)
	if err != nil {
		t.Fatalf("DeleteEmployee failed: %v", err)
	}

	// 2. Departments
	depts, err := client.FetchDepartments(ctx)
	if err != nil || len(depts) == 0 {
		t.Fatalf("FetchDepartments failed: %v", err)
	}

	createdDept, err := client.CreateDepartment(ctx, &models.CreateDepartmentRequest{
		DeptCode: "2",
		DeptName: "DISKOMINFO",
	})
	if err != nil || createdDept.ID != 2 {
		t.Fatalf("CreateDepartment failed: %v", err)
	}

	// 3. Areas
	areas, err := client.FetchAreas(ctx)
	if err != nil || len(areas) == 0 {
		t.Fatalf("FetchAreas failed: %v", err)
	}

	// 4. Positions
	positions, err := client.FetchPositions(ctx)
	if err != nil || len(positions) == 0 {
		t.Fatalf("FetchPositions failed: %v", err)
	}

	// 5. Resigns
	resigns, err := client.FetchResigns(ctx)
	if err != nil || len(resigns) == 0 {
		t.Fatalf("FetchResigns failed: %v", err)
	}

	createdResign, err := client.CreateResign(ctx, &models.CreateResignRequest{
		Employee:   10,
		ResignDate: "2026-08-30",
		ResignType: 1,
		DisableAtt: true,
	})
	if err != nil || createdResign.ID != 2 {
		t.Fatalf("CreateResign failed: %v", err)
	}
}
