package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/models"

	"github.com/go-resty/resty/v2"
)

// BioTimeClient adalah interface untuk berinteraksi dengan API ZKTeco BioTime 8.5
type BioTimeClient struct {
	client   *resty.Client
	baseURL  string
	username string
	password string

	tokenMutex  sync.RWMutex
	cachedToken string
}

// bioTimeout membaca BIOTIME_TIMEOUT_SECONDS (detik, maks 30).
// Sebelumnya timeout di-hardcode 5 detik sehingga .env tidak berpengaruh.
func bioTimeout() time.Duration {
	secs, err := strconv.Atoi(config.GetEnv("BIOTIME_TIMEOUT_SECONDS", "5"))
	if err != nil || secs <= 0 {
		secs = 5
	}
	if secs > 30 {
		secs = 30
	}
	return time.Duration(secs) * time.Second
}

// NewBioTimeClient menginisiasi HTTP Client Resty untuk BioTime 8.5
func NewBioTimeClient() *BioTimeClient {
	baseURL := config.GetEnv("BIOTIME_BASE_URL", "http://127.0.0.1:8081")
	username := config.GetEnv("BIOTIME_USERNAME", "admin")
	password := config.GetEnv("BIOTIME_PASSWORD", "admin")

	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(bioTimeout()).
		SetHeader("Content-Type", "application/json")

	return &BioTimeClient{
		client:   client,
		baseURL:  baseURL,
		username: username,
		password: password,
	}
}

// GetToken mengambil token dari cache atau melakukan login baru ke /jwt-api-token-auth/
func (s *BioTimeClient) GetToken(ctx context.Context) (string, error) {
	s.tokenMutex.RLock()
	if s.cachedToken != "" {
		token := s.cachedToken
		s.tokenMutex.RUnlock()
		return token, nil
	}
	s.tokenMutex.RUnlock()

	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()

	// Double check setelah mendapatkan write lock
	if s.cachedToken != "" {
		return s.cachedToken, nil
	}

	payload := models.LoginRequest{
		Username: s.username,
		Password: s.password,
	}

	var loginResp models.LoginResponse
	resp, err := s.client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&loginResp).
		Post("/jwt-api-token-auth/")

	if err != nil {
		return "", fmt.Errorf("koneksi ke BioTime gagal: %w", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("login BioTime gagal dengan status %d: %s", resp.StatusCode(), resp.String())
	}

	if loginResp.Token == "" {
		return "", fmt.Errorf("token kosong dari respons login BioTime. Body: %s", resp.String())
	}

	s.cachedToken = loginResp.Token
	log.Println("[BioTime] Berhasil mendapatkan token JWT baru")
	return s.cachedToken, nil
}

// InvalidateToken menghapus token dari cache (misal saat terdeteksi 401 Unauthorized)
func (s *BioTimeClient) InvalidateToken() {
	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()
	s.cachedToken = ""
}

// executeRequest menjalankan HTTP request ke BioTime dengan JWT token dan auto retry saat 401
func (s *BioTimeClient) executeRequest(ctx context.Context, method, endpoint string, body any, queryParams map[string]string, result any) (*resty.Response, error) {
	token, err := s.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	req := s.client.R().
		SetContext(ctx).
		SetHeader("Authorization", "JWT "+token)

	if body != nil {
		req.SetBody(body)
	}
	if queryParams != nil {
		req.SetQueryParams(queryParams)
	}
	if result != nil {
		req.SetResult(result)
	}

	resp, err := req.Execute(method, endpoint)
	if err != nil {
		return nil, fmt.Errorf("request ke BioTime (%s %s) gagal: %w", method, endpoint, err)
	}

	// Tangani token kedaluwarsa (401 Unauthorized)
	if resp.StatusCode() == 401 {
		log.Println("[BioTime] Token kedaluwarsa (401). Melakukan auto re-login...")
		s.InvalidateToken()

		newToken, err := s.GetToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("auto re-login gagal: %w", err)
		}

		retryReq := s.client.R().
			SetContext(ctx).
			SetHeader("Authorization", "JWT "+newToken)

		if body != nil {
			retryReq.SetBody(body)
		}
		if queryParams != nil {
			retryReq.SetQueryParams(queryParams)
		}
		if result != nil {
			retryReq.SetResult(result)
		}

		retryResp, err := retryReq.Execute(method, endpoint)
		if err != nil {
			return nil, fmt.Errorf("retry request (%s %s) gagal: %w", method, endpoint, err)
		}
		return retryResp, nil
	}

	return resp, nil
}

// FetchTransactions menarik log transaksi presensi dari /iclock/api/transactions/
func (s *BioTimeClient) FetchTransactions(ctx context.Context, startTime, endTime string, page, pageSize int) (*models.TransactionResponse, error) {
	queryParams := map[string]string{
		"page":      fmt.Sprintf("%d", page),
		"page_size": fmt.Sprintf("%d", pageSize),
	}
	if startTime != "" {
		queryParams["start_time"] = startTime
	}
	if endTime != "" {
		queryParams["end_time"] = endTime
	}

	var transResp models.TransactionResponse
	resp, err := s.executeRequest(ctx, "GET", "/iclock/api/transactions/", nil, queryParams, &transResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch transactions gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return &transResp, nil
}

// FetchAllTransactions menarik seluruh halaman log transaksi (anti-truncation).
// Sebelumnya hanya page 1 (500/1000) sehingga rekap bulanan/tahunan >1000 tap terpotong diam-diam.
func (s *BioTimeClient) FetchAllTransactions(ctx context.Context, startTime, endTime string, pageSize, maxPages int) ([]models.BioTimeTransaction, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	if maxPages <= 0 {
		maxPages = 1
	}
	var all []models.BioTimeTransaction
	for page := 1; page <= maxPages; page++ {
		resp, err := s.FetchTransactions(ctx, startTime, endTime, page, pageSize)
		if err != nil {
			if len(all) > 0 {
				log.Printf("[BioTime] FetchAllTransactions page %d gagal, pakai %d data parsial: %v\n", page, len(all), err)
				break
			}
			return nil, err
		}
		if resp == nil || len(resp.Data) == 0 {
			break
		}
		all = append(all, resp.Data...)
		if len(resp.Data) < pageSize {
			break
		}
		if resp.Count > 0 && len(all) >= resp.Count {
			break
		}
	}
	if len(all) == 0 {
		return []models.BioTimeTransaction{}, nil
	}
	return all, nil
}

// ==========================================
// 1. Master Pegawai (Employee) CRUD
// ==========================================

func (s *BioTimeClient) FetchEmployees(ctx context.Context, page, pageSize int, search string, deptID int64) (*models.EmployeeListResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	queryParams := map[string]string{
		"page":      fmt.Sprintf("%d", page),
		"page_size": fmt.Sprintf("%d", pageSize),
	}
	if search != "" {
		queryParams["emp_code"] = search
	}
	if deptID > 0 {
		queryParams["department"] = fmt.Sprintf("%d", deptID)
	}

	var empResp models.EmployeeListResponse
	resp, err := s.executeRequest(ctx, "GET", "/personnel/api/employees/", nil, queryParams, &empResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch employees gagal status %d: %s", resp.StatusCode(), resp.String())
	}

	// Normalisasi helper nama, departemen, dan area untuk setiap pegawai
	for i := range empResp.Data {
		empResp.Data[i].ParseHelpers()
	}

	return &empResp, nil
}

func (s *BioTimeClient) GetEmployeeByID(ctx context.Context, id int64) (*models.BioTimeEmployee, error) {
	var emp models.BioTimeEmployee
	resp, err := s.executeRequest(ctx, "GET", fmt.Sprintf("/personnel/api/employees/%d/", id), nil, nil, &emp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("get employee %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	emp.ParseHelpers()
	return &emp, nil
}

func (s *BioTimeClient) CreateEmployee(ctx context.Context, req *models.CreateEmployeeRequest) (*models.BioTimeEmployee, error) {
	var created models.BioTimeEmployee
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/employees/", req, nil, &created)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 201 && resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create employee gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	created.ParseHelpers()
	return &created, nil
}

func (s *BioTimeClient) UpdateEmployee(ctx context.Context, id int64, req *models.UpdateEmployeeRequest) (*models.BioTimeEmployee, error) {
	var updated models.BioTimeEmployee
	resp, err := s.executeRequest(ctx, "PATCH", fmt.Sprintf("/personnel/api/employees/%d/", id), req, nil, &updated)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("update employee %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	updated.ParseHelpers()
	return &updated, nil
}

func (s *BioTimeClient) DeleteEmployee(ctx context.Context, id int64) error {
	resp, err := s.executeRequest(ctx, "DELETE", fmt.Sprintf("/personnel/api/employees/%d/", id), nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 204 && resp.StatusCode() != 200 {
		return fmt.Errorf("delete employee %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	if resp.StatusCode() == 200 && len(resp.Body()) > 0 {
		return errors.New(parseBioTimeDeleteError(resp.Body()))
	}
	return nil
}

// ==========================================
// 2. Master Departemen (Bagian) CRUD
// ==========================================

func (s *BioTimeClient) FetchDepartments(ctx context.Context) ([]models.BioTimeDepartment, error) {
	queryParams := map[string]string{
		"page_size": "200",
	}
	var deptResp models.DepartmentListResponse
	resp, err := s.executeRequest(ctx, "GET", "/personnel/api/departments/", nil, queryParams, &deptResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch departments gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return deptResp.Data, nil
}

func (s *BioTimeClient) CreateDepartment(ctx context.Context, req *models.CreateDepartmentRequest) (*models.BioTimeDepartment, error) {
	var created models.BioTimeDepartment
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/departments/", req, nil, &created)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 201 && resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create department gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func (s *BioTimeClient) UpdateDepartment(ctx context.Context, id int64, req *models.UpdateDepartmentRequest) (*models.BioTimeDepartment, error) {
	var updated models.BioTimeDepartment
	resp, err := s.executeRequest(ctx, "PATCH", fmt.Sprintf("/personnel/api/departments/%d/", id), req, nil, &updated)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("update department %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	return &updated, nil
}

func (s *BioTimeClient) DeleteDepartment(ctx context.Context, id int64) error {
	resp, err := s.executeRequest(ctx, "DELETE", fmt.Sprintf("/personnel/api/departments/%d/", id), nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 204 && resp.StatusCode() != 200 {
		return fmt.Errorf("delete department %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	if resp.StatusCode() == 200 && len(resp.Body()) > 0 {
		return errors.New(parseBioTimeDeleteError(resp.Body()))
	}
	return nil
}

// ==========================================
// 3. Master Area (Lokasi Kerja) CRUD
// ==========================================

func (s *BioTimeClient) FetchAreas(ctx context.Context) ([]models.BioTimeArea, error) {
	queryParams := map[string]string{
		"page_size": "200",
	}
	var areaResp models.AreaListResponse
	resp, err := s.executeRequest(ctx, "GET", "/personnel/api/areas/", nil, queryParams, &areaResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch areas gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return areaResp.Data, nil
}

func (s *BioTimeClient) CreateArea(ctx context.Context, req *models.CreateAreaRequest) (*models.BioTimeArea, error) {
	var created models.BioTimeArea
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/areas/", req, nil, &created)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 201 && resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create area gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func (s *BioTimeClient) UpdateArea(ctx context.Context, id int64, req *models.UpdateAreaRequest) (*models.BioTimeArea, error) {
	var updated models.BioTimeArea
	resp, err := s.executeRequest(ctx, "PATCH", fmt.Sprintf("/personnel/api/areas/%d/", id), req, nil, &updated)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("update area %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	return &updated, nil
}

func (s *BioTimeClient) DeleteArea(ctx context.Context, id int64) error {
	resp, err := s.executeRequest(ctx, "DELETE", fmt.Sprintf("/personnel/api/areas/%d/", id), nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 204 && resp.StatusCode() != 200 {
		return fmt.Errorf("delete area %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	if resp.StatusCode() == 200 && len(resp.Body()) > 0 {
		return errors.New(parseBioTimeDeleteError(resp.Body()))
	}
	return nil
}

// ==========================================
// 4. Master Jabatan (Position) CRUD
// ==========================================

func (s *BioTimeClient) FetchPositions(ctx context.Context) ([]models.BioTimePosition, error) {
	queryParams := map[string]string{
		"page_size": "200",
	}
	var posResp models.PositionListResponse
	resp, err := s.executeRequest(ctx, "GET", "/personnel/api/positions/", nil, queryParams, &posResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch positions gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return posResp.Data, nil
}

func (s *BioTimeClient) CreatePosition(ctx context.Context, req *models.CreatePositionRequest) (*models.BioTimePosition, error) {
	var created models.BioTimePosition
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/positions/", req, nil, &created)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 201 && resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create position gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func (s *BioTimeClient) UpdatePosition(ctx context.Context, id int64, req *models.UpdatePositionRequest) (*models.BioTimePosition, error) {
	var updated models.BioTimePosition
	resp, err := s.executeRequest(ctx, "PATCH", fmt.Sprintf("/personnel/api/positions/%d/", id), req, nil, &updated)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("update position %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	return &updated, nil
}

func (s *BioTimeClient) DeletePosition(ctx context.Context, id int64) error {
	resp, err := s.executeRequest(ctx, "DELETE", fmt.Sprintf("/personnel/api/positions/%d/", id), nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 204 && resp.StatusCode() != 200 {
		return fmt.Errorf("delete position %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	if resp.StatusCode() == 200 && len(resp.Body()) > 0 {
		return errors.New(parseBioTimeDeleteError(resp.Body()))
	}
	return nil
}

// ==========================================
// 5. Master Mutasi / Resign ASN
// ==========================================

func (s *BioTimeClient) FetchResigns(ctx context.Context) ([]models.BioTimeResign, error) {
	queryParams := map[string]string{
		"page_size": "200",
	}
	var resignResp models.ResignListResponse
	resp, err := s.executeRequest(ctx, "GET", "/personnel/api/resigns/", nil, queryParams, &resignResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch resigns gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return resignResp.Data, nil
}

func (s *BioTimeClient) CreateResign(ctx context.Context, req *models.CreateResignRequest) (*models.BioTimeResign, error) {
	var created models.BioTimeResign
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/resigns/", req, nil, &created)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 201 && resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create resign gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func (s *BioTimeClient) DeleteResign(ctx context.Context, id int64) error {
	resp, err := s.executeRequest(ctx, "DELETE", fmt.Sprintf("/personnel/api/resigns/%d/", id), nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 204 && resp.StatusCode() != 200 {
		return fmt.Errorf("delete resign %d gagal status %d: %s", id, resp.StatusCode(), resp.String())
	}
	if resp.StatusCode() == 200 && len(resp.Body()) > 0 {
		return errors.New(parseBioTimeDeleteError(resp.Body()))
	}
	return nil
}

func (s *BioTimeClient) ReinstatementResign(ctx context.Context, req *models.ReinstatementRequest) error {
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/resigns/reinstatement/", req, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		return fmt.Errorf("reinstatement gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// parseBioTimeDeleteError menerjemahkan error raw bahasa Inggris dari BioTime menjadi bahasa Indonesia yang ramah pengguna
func parseBioTimeDeleteError(body []byte) string {
	raw := string(body)

	// Hapus tanda kutip jika itu adalah string JSON murni
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}

	// Mapping error umum ZKTeco menggunakan pencarian substring yang lebih aman
	if strings.Contains(raw, "The operation you selected is not working properly") {
		return "Gagal menghapus data: Data sedang digunakan atau terhubung dengan Data Lain dan Device."
	}
	if strings.Contains(raw, "used by") || strings.Contains(raw, "in use") {
		return "Gagal menghapus data: Data ini masih digunakan di tempat lain."
	}

	// Fallback ke pesan aslinya
	return "Ditolak sistem BioTime: " + raw
}
