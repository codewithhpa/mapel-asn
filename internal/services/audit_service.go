package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dashboard-presensi/internal/models"
)

// AuditService mengelola pencatatan verifikasi lapangan fisik barisan secara append-only
type AuditService struct {
	mu       sync.RWMutex
	db       *sql.DB
	filePath string
	records  []*models.AuditRecord
}

func NewAuditService(filePath string, db *sql.DB) *AuditService {
	svc := &AuditService{
		db:       db,
		filePath: filePath,
		records:  make([]*models.AuditRecord, 0),
	}
	if db == nil {
		svc.loadFromFile()
	}
	return svc
}

func (s *AuditService) loadFromFile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var loaded []*models.AuditRecord
	if err := json.Unmarshal(data, &loaded); err == nil {
		s.records = loaded
	}
}

func (s *AuditService) saveToFile() error {
	if s.filePath == "" {
		return nil
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// RecordAudit menyimpan aksi dispensasi / verifikasi ke dalam audit trail (append-only)
func (s *AuditService) RecordAudit(req models.AuditVerifyRequest, verifierIP string) (*models.AuditRecord, error) {
	now := time.Now()
	dateStr := req.Date
	if dateStr == "" {
		dateStr = now.Format(time.DateOnly)
	}
	id := fmt.Sprintf("DISP-%s-%s-%d", req.EmpCode, dateStr, now.Unix())

	session := req.Session
	if session == "" {
		session = "Apel Pagi"
	}

	verifier := req.VerifierName
	if verifier == "" {
		verifier = "Petugas Dispensasi"
	}

	record := &models.AuditRecord{
		ID:             id,
		Date:           dateStr,
		EmpCode:        req.EmpCode,
		Name:           req.Name,
		Department:     req.Department,
		Session:        session,
		PhysicalStatus: req.PhysicalStatus,
		Notes:          req.Notes,
		VerifierName:   verifier,
		VerifierIP:     verifierIP,
		CreatedAt:      now,
	}

	if s.db != nil {
		query := `INSERT INTO audit_trail 
			(id, date, emp_code, name, department, session, physical_status, notes, verifier_name, verifier_ip, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := s.db.Exec(query, record.ID, record.Date, record.EmpCode, record.Name, record.Department,
			record.Session, record.PhysicalStatus, record.Notes, record.VerifierName, record.VerifierIP, record.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("gagal mencatat audit ke MariaDB: %w", err)
		}
		return record, nil
	}

	// Fallback in-memory/file
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = append(s.records, record)
	_ = s.saveToFile()

	return record, nil
}

// RecordMasterAction mencatat perubahan pada Master Data (Pegawai, Departemen, Area, Jabatan, Mutasi)
func (s *AuditService) RecordMasterAction(category, actor, empCode, name, notes, clientIP string) error {
	now := time.Now()
	dateStr := now.Format(time.DateOnly)
	id := fmt.Sprintf("MST-%d-%d", now.UnixNano(), now.Unix()%1000)

	record := &models.AuditRecord{
		ID:             id,
		Date:           dateStr,
		EmpCode:        empCode,
		Name:           name,
		Department:     "-",
		Session:        category,
		PhysicalStatus: "Master Data Update",
		Notes:          notes,
		VerifierName:   actor,
		VerifierIP:     clientIP,
		CreatedAt:      now,
	}

	if s.db != nil {
		query := `INSERT INTO audit_trail 
			(id, date, emp_code, name, department, session, physical_status, notes, verifier_name, verifier_ip, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := s.db.Exec(query, record.ID, record.Date, record.EmpCode, record.Name, record.Department,
			record.Session, record.PhysicalStatus, record.Notes, record.VerifierName, record.VerifierIP, record.CreatedAt)
		if err != nil {
			log.Printf("[AuditService] Warning: gagal catat audit ke DB: %v\n", err)
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return s.saveToFile()
}

// DeleteAudit menghapus catatan dispensasi / audit berdasarkan ID
func (s *AuditService) DeleteAudit(id string) error {
	if s.db != nil {
		_, err := s.db.Exec("DELETE FROM audit_trail WHERE id = ?", id)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newRecords := make([]*models.AuditRecord, 0, len(s.records))
	for _, r := range s.records {
		if r.ID != id {
			newRecords = append(newRecords, r)
		}
	}
	s.records = newRecords
	return s.saveToFile()
}

// GetLatestAuditMap mengembalikan map status verifikasi terkini per pegawai untuk tanggal tertentu
func (s *AuditService) GetLatestAuditMap(date string) map[string]*models.AuditRecord {
	result := make(map[string]*models.AuditRecord)

	if s.db != nil {
		rows, err := s.db.Query(`SELECT id, date, emp_code, name, department, session, physical_status, notes, verifier_name, verifier_ip, created_at 
			FROM audit_trail WHERE date = ? ORDER BY created_at ASC`, date)
		if err != nil {
			log.Printf("[AuditService] GetLatestAuditMap error: %v\n", err)
			return result
		}
		defer rows.Close()

		for rows.Next() {
			var rec models.AuditRecord
			if err := rows.Scan(&rec.ID, &rec.Date, &rec.EmpCode, &rec.Name, &rec.Department, &rec.Session,
				&rec.PhysicalStatus, &rec.Notes, &rec.VerifierName, &rec.VerifierIP, &rec.CreatedAt); err == nil {
				key := fmt.Sprintf("%s_%s", rec.EmpCode, rec.Session)
				result[key] = &rec
			}
		}
		return result
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rec := range s.records {
		if rec.Date == date {
			key := fmt.Sprintf("%s_%s", rec.EmpCode, rec.Session)
			result[key] = rec
		}
	}
	return result
}

// GetAllLogs mengembalikan seluruh riwayat audit trail (terbaru di atas)
func (s *AuditService) GetAllLogs() []*models.AuditRecord {
	if s.db != nil {
		rows, err := s.db.Query(`SELECT id, date, emp_code, name, department, session, physical_status, notes, verifier_name, verifier_ip, created_at 
			FROM audit_trail ORDER BY created_at DESC LIMIT 500`)
		if err != nil {
			log.Printf("[AuditService] GetAllLogs error: %v\n", err)
			return []*models.AuditRecord{}
		}
		defer rows.Close()

		var list []*models.AuditRecord
		for rows.Next() {
			var rec models.AuditRecord
			if err := rows.Scan(&rec.ID, &rec.Date, &rec.EmpCode, &rec.Name, &rec.Department, &rec.Session,
				&rec.PhysicalStatus, &rec.Notes, &rec.VerifierName, &rec.VerifierIP, &rec.CreatedAt); err == nil {
				list = append(list, &rec)
			}
		}
		return list
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	n := len(s.records)
	reversed := make([]*models.AuditRecord, n)
	for i := 0; i < n; i++ {
		reversed[i] = s.records[n-1-i]
	}
	return reversed
}
