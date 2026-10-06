package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/models"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

// InitDB menginisialisasi koneksi MariaDB dan melakukan auto-migrasi skema tabel
func InitDB() (*sql.DB, error) {
	host := config.GetEnv("DB_HOST", "127.0.0.1")
	port := config.GetEnv("DB_PORT", "3306")
	user := config.GetEnv("DB_USER", "hpa")
	pass := config.GetEnv("DB_PASSWORD", "HPATrigger110977!!!@10143023")
	name := config.GetEnv("DB_NAME", "dashboard_presensi")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, pass, host, port, name)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka koneksi database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("gagal terhubung ke MariaDB (%s:%s): %w", host, port, err)
	}

	log.Printf("[MariaDB] Berhasil terhubung ke database '%s' di %s:%s\n", name, host, port)

	DB = db

	// Jalankan migrasi tabel
	if err := migrateTables(db); err != nil {
		return nil, fmt.Errorf("migrasi database gagal: %w", err)
	}

	// Migrasi data dari file JSON lama jika ada
	migrateExistingJSONData(db)

	return db, nil
}

func migrateTables(db *sql.DB) error {
	// 1. Tabel roles (Hak Akses & Peran Sistem)
	roleTableSQL := `
	CREATE TABLE IF NOT EXISTS roles (
		id INT AUTO_INCREMENT PRIMARY KEY,
		role_code VARCHAR(32) UNIQUE NOT NULL,
		role_name VARCHAR(64) NOT NULL,
		description VARCHAR(255) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := db.Exec(roleTableSQL); err != nil {
		return fmt.Errorf("gagal membuat tabel roles: %w", err)
	}

	// Seed default roles jika belum ada
	seedRolesSQL := `
	INSERT IGNORE INTO roles (id, role_code, role_name, description) VALUES
	(1, 'admin', 'Administrator', 'Akses penuh manajemen sistem, pengguna, dan konfigurasi'),
	(2, 'provos', 'Petugas Verifikasi', 'Piket Provos untuk verifikasi fisik kehadiran barisan apel'),
	(3, 'piket', 'Petugas Piket', 'Petugas piket harian monitoring presensi dan apel ASN'),
	(4, 'pimpinan', 'Pimpinan / Inspektorat', 'Monitoring pengawasan kepatuhan apel dan laporan eksekutif');`
	if _, err := db.Exec(seedRolesSQL); err != nil {
		log.Printf("[MariaDB Warning] Seed roles: %v\n", err)
	}

	// 2. Tabel users (Petugas Provos / Admin)
	userTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(64) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		full_name VARCHAR(128) NOT NULL,
		nip VARCHAR(64) NOT NULL DEFAULT '',
		role_id INT NOT NULL DEFAULT 2,
		role VARCHAR(32) NOT NULL DEFAULT 'Petugas Verifikasi',
		is_active TINYINT NOT NULL DEFAULT 1 COMMENT '1: Active, 2: Inactive',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_role_id (role_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := db.Exec(userTableSQL); err != nil {
		return fmt.Errorf("gagal membuat tabel users: %w", err)
	}

	// 3. Tabel audit_trail (Audit Barisan ASN)
	auditTableSQL := `
	CREATE TABLE IF NOT EXISTS audit_trail (
		id VARCHAR(64) PRIMARY KEY,
		date VARCHAR(10) NOT NULL,
		emp_code VARCHAR(32) NOT NULL,
		name VARCHAR(128) NOT NULL,
		department VARCHAR(128) NOT NULL DEFAULT '',
		session VARCHAR(64) NOT NULL,
		physical_status VARCHAR(64) NOT NULL,
		notes TEXT,
		verifier_name VARCHAR(128) NOT NULL,
		verifier_ip VARCHAR(64) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_date (date),
		INDEX idx_emp_code (emp_code)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := db.Exec(auditTableSQL); err != nil {
		return fmt.Errorf("gagal membuat tabel audit_trail: %w", err)
	}

	// 4. Tabel attendance_rules (Aturan Batas Jam Kerja & Toleransi ASN)
	ruleTableSQL := `
	CREATE TABLE IF NOT EXISTS attendance_rules (
		id INT AUTO_INCREMENT PRIMARY KEY COMMENT 'Primary Key identifier unik aturan jam kerja',
		rule_name VARCHAR(64) NOT NULL DEFAULT 'Reguler' COMMENT 'Nama atau label aturan jadwal jam kerja (e.g. Reguler, Ramadhan)',
		work_start_time VARCHAR(8) NOT NULL DEFAULT '07:00:00' COMMENT 'Jam masuk resmi dasar acuan hitung keterlambatan (HH:MM:SS)',
		checkin_tolerance_time VARCHAR(8) NOT NULL DEFAULT '07:15:00' COMMENT 'Batas akhir toleransi scan masuk apel pagi tepat waktu (HH:MM:SS)',
		checkin_window_start VARCHAR(8) NOT NULL DEFAULT '06:15:00' COMMENT 'Waktu awal scan pagi mulai diterima mesin FR (HH:MM:SS)',
		checkout_time_mon_thu VARCHAR(8) NOT NULL DEFAULT '15:30:00' COMMENT 'Waktu buka scan kepulangan sore hari Senin-Kamis (HH:MM:SS)',
		checkout_cutoff_mon_thu VARCHAR(8) NOT NULL DEFAULT '15:45:00' COMMENT 'Batas akhir mutlak scan pulang Senin-Kamis, lewat dari ini anomali (HH:MM:SS)',
		checkout_time_fri VARCHAR(8) NOT NULL DEFAULT '16:00:00' COMMENT 'Waktu buka scan kepulangan sore khusus hari Jumat (HH:MM:SS)',
		checkout_cutoff_fri VARCHAR(8) NOT NULL DEFAULT '16:15:00' COMMENT 'Batas akhir mutlak scan pulang Jumat, lewat dari ini anomali (HH:MM:SS)',
		is_ramadhan_mode TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'Indikator mode puasa / Ramadhan (0: Nonaktif, 1: Aktif)',
		is_active TINYINT(1) NOT NULL DEFAULT 1 COMMENT 'Status keberlakuan aturan (1: Aktif digunakan sistem, 0: Arsip)',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Waktu pembuatan data aturan jadwal',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Waktu terakhir pembaruan data aturan jadwal'
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := db.Exec(ruleTableSQL); err != nil {
		return fmt.Errorf("gagal membuat tabel attendance_rules: %w", err)
	}

	// Seed default row jika masih kosong
	var ruleCount int
	db.QueryRow("SELECT COUNT(*) FROM attendance_rules").Scan(&ruleCount)
	if ruleCount == 0 {
		seedRuleSQL := `
		INSERT INTO attendance_rules (id, rule_name, work_start_time, checkin_tolerance_time, checkin_window_start, 
			checkout_time_mon_thu, checkout_cutoff_mon_thu, checkout_time_fri, checkout_cutoff_fri, is_ramadhan_mode, is_active)
		VALUES (1, 'Jam Kerja Normal ASN (SOP Resmi)', '07:00:00', '07:15:00', '06:15:00', 
			'15:30:00', '15:45:00', '16:00:00', '16:15:00', 0, 1);`
		if _, err := db.Exec(seedRuleSQL); err != nil {
			log.Printf("[MariaDB Warning] Seed attendance_rules: %v\n", err)
		}
	}

	log.Println("[MariaDB] Skema tabel 'roles', 'users', 'audit_trail', dan 'attendance_rules' berhasil diverifikasi/dibuat.")
	return nil
}

func migrateExistingJSONData(db *sql.DB) {
	// 1. Cek apakah tabel users masih kosong
	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if userCount == 0 {
		// Cek file data/users.json
		if data, err := os.ReadFile("data/users.json"); err == nil {
			var jsonUsers []models.User
			if json.Unmarshal(data, &jsonUsers) == nil && len(jsonUsers) > 0 {
				for _, u := range jsonUsers {
					db.Exec(`INSERT IGNORE INTO users (id, username, password_hash, full_name, nip, role, is_active, created_at)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						u.ID, u.Username, u.PasswordHash, u.FullName, u.NIP, u.Role, u.IsActive, u.CreatedAt)
				}
				log.Printf("[MariaDB] Sukses migrasi %d user dari data/users.json ke tabel 'users'\n", len(jsonUsers))
			}
		}

		// Jika masih kosong (belum ada users.json), seed default admin
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
		if userCount == 0 {
			hash, _ := bcrypt.GenerateFromPassword([]byte("adminkey1234"), bcrypt.DefaultCost)
			db.Exec(`INSERT INTO users (id, username, password_hash, full_name, nip, role_id, role, is_active, created_at)
				VALUES (1, 'admin', ?, 'Administrator', '19800101 200501 1 001', 1, 'Administrator', 1, NOW())`,
				string(hash))
			log.Println("[MariaDB] Seed akun admin default ('admin' / 'adminkey1234') berhasil.")
		}
	}

	// 2. Cek apakah tabel audit_trail masih kosong
	var auditCount int
	db.QueryRow("SELECT COUNT(*) FROM audit_trail").Scan(&auditCount)
	if auditCount == 0 {
		if data, err := os.ReadFile("data/audit_trail.json"); err == nil {
			var jsonAudits []models.AuditRecord
			if json.Unmarshal(data, &jsonAudits) == nil && len(jsonAudits) > 0 {
				for _, a := range jsonAudits {
					db.Exec(`INSERT IGNORE INTO audit_trail (id, date, emp_code, name, department, session, physical_status, notes, verifier_name, verifier_ip, created_at)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						a.ID, a.Date, a.EmpCode, a.Name, a.Department, a.Session, a.PhysicalStatus, a.Notes, a.VerifierName, a.VerifierIP, a.CreatedAt)
				}
				log.Printf("[MariaDB] Sukses migrasi %d rekaman audit dari data/audit_trail.json ke tabel 'audit_trail'\n", len(jsonAudits))
			}
		}
	}
}
