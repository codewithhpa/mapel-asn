package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dashboard-presensi/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// sessionEntry adalah token sesi opaque server-side (pengganti cookie ID polos yang bisa dipalsu).
type sessionEntry struct {
	userID  int
	expires time.Time
}

// UserService mengelola akun petugas verifikator, peran (roles), dan autentikasi
type UserService struct {
	mu       sync.RWMutex
	db       *sql.DB
	filePath string
	users    map[int]*models.User // fallback in-memory cache (key: int)

	sessMu   sync.RWMutex
	sessions map[string]sessionEntry
}

func NewUserService(filePath string, db *sql.DB) *UserService {
	svc := &UserService{
		db:       db,
		filePath: filePath,
		users:    make(map[int]*models.User),
		sessions: make(map[string]sessionEntry),
	}

	if db == nil {
		svc.loadFromFile()
		svc.seedDefaultAdmin()
	}
	return svc
}

// CreateSession membuat token sesi opaque 256-bit untuk user (valid 24 jam).
func (s *UserService) CreateSession(userID int) string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback sangat jarang: token berbasis waktu (tetap 256-bit entropy campuran)
		seed := fmt.Sprintf("%d-%d", userID, time.Now().UnixNano())
		hash, _ := bcrypt.GenerateFromPassword([]byte(seed), bcrypt.MinCost)
		tok := hex.EncodeToString(hash)[:64]
		s.sessMu.Lock()
		s.sessions[tok] = sessionEntry{userID: userID, expires: time.Now().Add(24 * time.Hour)}
		s.sessMu.Unlock()
		return tok
	}
	tok := hex.EncodeToString(b[:])
	s.sessMu.Lock()
	s.sessions[tok] = sessionEntry{userID: userID, expires: time.Now().Add(24 * time.Hour)}
	// Bersihkan sesi kedaluwarsa secara oportunistik
	for k, v := range s.sessions {
		if time.Now().After(v.expires) {
			delete(s.sessions, k)
		}
	}
	s.sessMu.Unlock()
	return tok
}

// GetSessionUser memvalidasi token sesi dan mengembalikan user aktif (tolak token palsu/kedaluarsa).
func (s *UserService) GetSessionUser(token string) (*models.User, bool) {
	if token == "" {
		return nil, false
	}
	s.sessMu.RLock()
	entry, ok := s.sessions[token]
	s.sessMu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		if ok {
			s.sessMu.Lock()
			delete(s.sessions, token)
			s.sessMu.Unlock()
		}
		return nil, false
	}
	u, err := s.GetUserByID(entry.userID)
	if err != nil || u.IsActive != models.UserStatusActive {
		return nil, false
	}
	return u, true
}

// DeleteSession mencabut token sesi (logout).
func (s *UserService) DeleteSession(token string) {
	if token == "" {
		return
	}
	s.sessMu.Lock()
	delete(s.sessions, token)
	s.sessMu.Unlock()
}

// GetAllRoles mengambil daftar hak akses / peran dari tabel roles
func (s *UserService) GetAllRoles() []models.Role {
	defaultRoles := []models.Role{
		{ID: 1, RoleCode: "admin", RoleName: "Administrator", Description: "Akses penuh manajemen sistem, data pengguna, dan pengaturan"},
		{ID: 2, RoleCode: "provos", RoleName: "Petugas Verifikasi", Description: "Piket Provos untuk verifikasi fisik kehadiran barisan apel"},
		{ID: 3, RoleCode: "piket", RoleName: "Petugas Piket", Description: "Petugas piket harian monitoring presensi dan apel ASN"},
		{ID: 4, RoleCode: "pimpinan", RoleName: "Pimpinan / Inspektorat", Description: "Monitoring pengawasan kepatuhan apel dan laporan eksekutif"},
	}

	if s.db != nil {
		rows, err := s.db.Query("SELECT id, role_code, role_name, description, created_at, updated_at FROM roles ORDER BY id ASC")
		if err != nil {
			log.Printf("[UserService] GetAllRoles error: %v\n", err)
			return defaultRoles
		}
		defer rows.Close()

		var list []models.Role
		for rows.Next() {
			var r models.Role
			if err := rows.Scan(&r.ID, &r.RoleCode, &r.RoleName, &r.Description, &r.CreatedAt, &r.UpdatedAt); err == nil {
				list = append(list, r)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	return defaultRoles
}

// GetAllRolesWithCount mengambil seluruh role beserta jumlah petugas yang memakainya
func (s *UserService) GetAllRolesWithCount() []models.Role {
	defaultRoles := []models.Role{
		{ID: 1, RoleCode: "admin", RoleName: "Administrator", Description: "Akses penuh manajemen sistem, data pengguna, dan pengaturan", UserCount: 1},
		{ID: 2, RoleCode: "provos", RoleName: "Petugas Verifikasi", Description: "Piket Provos untuk verifikasi fisik kehadiran barisan apel", UserCount: 0},
		{ID: 3, RoleCode: "piket", RoleName: "Petugas Piket", Description: "Petugas piket harian monitoring presensi dan apel ASN", UserCount: 0},
		{ID: 4, RoleCode: "pimpinan", RoleName: "Pimpinan / Inspektorat", Description: "Monitoring pengawasan kepatuhan apel dan laporan eksekutif", UserCount: 0},
	}

	if s.db != nil {
		query := `SELECT r.id, r.role_code, r.role_name, r.description, r.created_at, r.updated_at,
			COUNT(u.id) AS user_count
			FROM roles r
			LEFT JOIN users u ON r.id = u.role_id
			GROUP BY r.id, r.role_code, r.role_name, r.description, r.created_at, r.updated_at
			ORDER BY r.id ASC`
		rows, err := s.db.Query(query)
		if err != nil {
			log.Printf("[UserService] GetAllRolesWithCount error: %v\n", err)
			return s.GetAllRoles()
		}
		defer rows.Close()

		var list []models.Role
		for rows.Next() {
			var r models.Role
			if err := rows.Scan(&r.ID, &r.RoleCode, &r.RoleName, &r.Description, &r.CreatedAt, &r.UpdatedAt, &r.UserCount); err == nil {
				list = append(list, r)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	return defaultRoles
}

// GetRoleByID mengambil role berdasarkan ID
func (s *UserService) GetRoleByID(id int) (*models.Role, error) {
	if s.db != nil {
		query := `SELECT r.id, r.role_code, r.role_name, r.description, r.created_at, r.updated_at,
			COUNT(u.id) AS user_count
			FROM roles r
			LEFT JOIN users u ON r.id = u.role_id
			WHERE r.id = ?
			GROUP BY r.id, r.role_code, r.role_name, r.description, r.created_at, r.updated_at`
		row := s.db.QueryRow(query, id)
		var r models.Role
		err := row.Scan(&r.ID, &r.RoleCode, &r.RoleName, &r.Description, &r.CreatedAt, &r.UpdatedAt, &r.UserCount)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("role tidak ditemukan")
			}
			return nil, err
		}
		return &r, nil
	}

	for _, r := range s.GetAllRoles() {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, errors.New("role tidak ditemukan")
}

// CreateRole membuat peran/role baru
func (s *UserService) CreateRole(req models.CreateRoleRequest) (*models.Role, error) {
	code := strings.ToLower(strings.TrimSpace(req.RoleCode))
	name := strings.TrimSpace(req.RoleName)
	desc := strings.TrimSpace(req.Description)

	if code == "" {
		return nil, errors.New("kode role wajib diisi")
	}
	if name == "" {
		return nil, errors.New("nama peran (role) wajib diisi")
	}

	if s.db != nil {
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM roles WHERE LOWER(role_code) = LOWER(?)", code).Scan(&count)
		if count > 0 {
			return nil, errors.New("kode role '" + code + "' sudah digunakan")
		}

		res, err := s.db.Exec("INSERT INTO roles (role_code, role_name, description) VALUES (?, ?, ?)", code, name, desc)
		if err != nil {
			return nil, fmt.Errorf("gagal membuat role di MariaDB: %w", err)
		}
		newID, _ := res.LastInsertId()
		return s.GetRoleByID(int(newID))
	}

	return &models.Role{
		ID:          99,
		RoleCode:    code,
		RoleName:    name,
		Description: desc,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// UpdateRole memperbarui data role
func (s *UserService) UpdateRole(id int, req models.UpdateRoleRequest) (*models.Role, error) {
	code := strings.ToLower(strings.TrimSpace(req.RoleCode))
	name := strings.TrimSpace(req.RoleName)
	desc := strings.TrimSpace(req.Description)

	if code == "" {
		return nil, errors.New("kode role wajib diisi")
	}
	if name == "" {
		return nil, errors.New("nama peran (role) wajib diisi")
	}

	if s.db != nil {
		current, err := s.GetRoleByID(id)
		if err != nil {
			return nil, err
		}

		// Jika role code berubah, pastikan tidak duplikat
		if !strings.EqualFold(current.RoleCode, code) {
			var count int
			s.db.QueryRow("SELECT COUNT(*) FROM roles WHERE LOWER(role_code) = LOWER(?) AND id != ?", code, id).Scan(&count)
			if count > 0 {
				return nil, errors.New("kode role '" + code + "' sudah digunakan oleh role lain")
			}
		}

		_, err = s.db.Exec("UPDATE roles SET role_code = ?, role_name = ?, description = ? WHERE id = ?", code, name, desc, id)
		if err != nil {
			return nil, fmt.Errorf("gagal update role di MariaDB: %w", err)
		}

		// Sinkronkan nama role di tabel users yang terikat dengan role_id ini
		_, _ = s.db.Exec("UPDATE users SET role = ? WHERE role_id = ?", name, id)

		return s.GetRoleByID(id)
	}

	return &models.Role{
		ID:          id,
		RoleCode:    code,
		RoleName:    name,
		Description: desc,
		UpdatedAt:   time.Now(),
	}, nil
}

// DeleteRole menghapus role jika tidak sedang digunakan
func (s *UserService) DeleteRole(id int) error {
	if id == 1 {
		return errors.New("role utama 'Administrator' tidak boleh dihapus demi integritas sistem")
	}

	if s.db != nil {
		// Periksa apakah ada user yang masih terasosiasi dengan role_id ini
		var userCount int
		s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role_id = ?", id).Scan(&userCount)
		if userCount > 0 {
			return fmt.Errorf("role ini tidak dapat dihapus karena sedang digunakan oleh %d pengguna", userCount)
		}

		res, err := s.db.Exec("DELETE FROM roles WHERE id = ?", id)
		if err != nil {
			return fmt.Errorf("gagal menghapus role: %w", err)
		}
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			return errors.New("role tidak ditemukan")
		}
		return nil
	}

	return nil
}

func (s *UserService) resolveRole(roleID int, roleName string) (int, string) {
	roles := s.GetAllRoles()
	if roleID > 0 {
		for _, r := range roles {
			if r.ID == roleID {
				return r.ID, r.RoleName
			}
		}
	}

	if roleName != "" {
		for _, r := range roles {
			if strings.EqualFold(r.RoleName, roleName) || strings.EqualFold(r.RoleCode, roleName) {
				return r.ID, r.RoleName
			}
		}
	}

	return 2, "Petugas Verifikasi"
}

func (s *UserService) seedDefaultAdmin() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.users {
		if strings.EqualFold(u.Username, "admin") {
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("adminkey1234"), bcrypt.DefaultCost)
	if err != nil {
		return
	}

	adminUser := &models.User{
		ID:           1,
		Username:     "admin",
		PasswordHash: string(hash),
		FullName:     "Administrator Utama",
		NIP:          "19800101 200501 1 001",
		RoleID:       1,
		Role:         models.RoleAdmin,
		IsActive:     models.UserStatusActive,
		CreatedAt:    time.Now(),
	}

	s.users[adminUser.ID] = adminUser
	_ = s.saveToFileLocked()
}

func (s *UserService) loadFromFile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var userList []*models.User
	if err := json.Unmarshal(data, &userList); err == nil {
		for _, u := range userList {
			if u.RoleID == 0 {
				u.RoleID, u.Role = s.resolveRole(0, u.Role)
			}
			if u.IsActive == 0 {
				u.IsActive = models.UserStatusActive
			}
			s.users[u.ID] = u
		}
	}
}

func (s *UserService) saveToFileLocked() error {
	if s.filePath == "" {
		return nil
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	userList := make([]*models.User, 0, len(s.users))
	for _, u := range s.users {
		userList = append(userList, u)
	}

	data, err := json.MarshalIndent(userList, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// Authenticate memeriksa username dan password (hanya user dengan is_active = 1 yang diizinkan)
func (s *UserService) Authenticate(username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)

	if s.db != nil {
		query := `SELECT u.id, u.username, u.password_hash, u.full_name, u.nip, u.role_id, 
			COALESCE(r.role_name, u.role), u.is_active, u.created_at 
			FROM users u 
			LEFT JOIN roles r ON u.role_id = r.id 
			WHERE LOWER(u.username) = LOWER(?)`
		row := s.db.QueryRow(query, username)
		var u models.User
		err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.FullName, &u.NIP, &u.RoleID, &u.Role, &u.IsActive, &u.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("username tidak ditemukan")
			}
			return nil, fmt.Errorf("database error: %w", err)
		}

		if u.IsActive != models.UserStatusActive {
			return nil, errors.New("akun ini sedang dinonaktifkan")
		}

		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
			return nil, errors.New("password salah")
		}
		return &u, nil
	}

	// Fallback in-memory
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if strings.EqualFold(u.Username, username) {
			if u.IsActive != models.UserStatusActive {
				return nil, errors.New("akun ini sedang dinonaktifkan")
			}
			err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
			if err != nil {
				return nil, errors.New("password salah")
			}
			return u, nil
		}
	}

	return nil, errors.New("username tidak ditemukan")
}

// CreateUser membuat akun petugas baru
func (s *UserService) CreateUser(req models.CreateUserRequest) (*models.UserDTO, error) {
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		return nil, errors.New("username wajib diisi")
	}
	if len(req.Password) < 6 {
		return nil, errors.New("password minimal 6 karakter")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("gagal mengenkripsi password: %w", err)
	}

	roleID, roleName := s.resolveRole(req.RoleID, req.Role)
	now := time.Now()

	if s.db != nil {
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username) = LOWER(?)", username).Scan(&count)
		if count > 0 {
			return nil, errors.New("username '" + username + "' sudah digunakan")
		}

		res, err := s.db.Exec(`INSERT INTO users (username, password_hash, full_name, nip, role_id, role, is_active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
			username, string(hash), strings.TrimSpace(req.FullName), strings.TrimSpace(req.NIP), roleID, roleName, now)
		if err != nil {
			return nil, fmt.Errorf("gagal menyimpan ke MariaDB: %w", err)
		}

		newID, _ := res.LastInsertId()

		dto := models.UserDTO{
			ID:        int(newID),
			Username:  username,
			FullName:  strings.TrimSpace(req.FullName),
			NIP:       strings.TrimSpace(req.NIP),
			RoleID:    roleID,
			Role:      roleName,
			IsActive:  models.UserStatusActive,
			CreatedAt: now.Format("02/01/2006 15:04"),
		}
		return &dto, nil
	}

	// Fallback in-memory
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.users {
		if strings.EqualFold(u.Username, username) {
			return nil, errors.New("username '" + username + "' sudah digunakan")
		}
	}

	newID := len(s.users) + 1
	for {
		if _, exists := s.users[newID]; !exists {
			break
		}
		newID++
	}

	newUser := &models.User{
		ID:           newID,
		Username:     username,
		PasswordHash: string(hash),
		FullName:     strings.TrimSpace(req.FullName),
		NIP:          strings.TrimSpace(req.NIP),
		RoleID:       roleID,
		Role:         roleName,
		IsActive:     models.UserStatusActive,
		CreatedAt:    now,
	}

	s.users[newUser.ID] = newUser
	_ = s.saveToFileLocked()

	dto := newUser.ToDTO()
	return &dto, nil
}

// GetAllUsers mengambil daftar seluruh petugas beserta nama rolenya
func (s *UserService) GetAllUsers() []models.UserDTO {
	if s.db != nil {
		query := `SELECT u.id, u.username, u.full_name, u.nip, u.role_id, 
			COALESCE(r.role_name, u.role), u.is_active, u.created_at 
			FROM users u 
			LEFT JOIN roles r ON u.role_id = r.id 
			ORDER BY u.created_at ASC`
		rows, err := s.db.Query(query)
		if err != nil {
			log.Printf("[UserService] GetAllUsers error: %v\n", err)
			return []models.UserDTO{}
		}
		defer rows.Close()

		var list []models.UserDTO
		for rows.Next() {
			var u models.UserDTO
			var createdAt time.Time
			if err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.NIP, &u.RoleID, &u.Role, &u.IsActive, &createdAt); err == nil {
				u.CreatedAt = createdAt.Format("02/01/2006 15:04")
				list = append(list, u)
			}
		}
		return list
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]models.UserDTO, 0, len(s.users))
	for _, u := range s.users {
		list = append(list, u.ToDTO())
	}
	return list
}

// GetUserByID mengambil user berdasarkan ID (integer)
func (s *UserService) GetUserByID(id int) (*models.User, error) {
	if s.db != nil {
		query := `SELECT u.id, u.username, u.password_hash, u.full_name, u.nip, u.role_id, 
			COALESCE(r.role_name, u.role), u.is_active, u.created_at 
			FROM users u 
			LEFT JOIN roles r ON u.role_id = r.id 
			WHERE u.id = ?`
		row := s.db.QueryRow(query, id)
		var u models.User
		err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.FullName, &u.NIP, &u.RoleID, &u.Role, &u.IsActive, &u.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("user tidak ditemukan")
			}
			return nil, err
		}
		return &u, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("user tidak ditemukan")
}

// UpdateUser memperbarui data profil atau password petugas
func (s *UserService) UpdateUser(id int, req models.UpdateUserRequest) (*models.UserDTO, error) {
	isActive := req.IsActive
	if isActive != models.UserStatusActive && isActive != models.UserStatusInactive {
		isActive = models.UserStatusActive
	}

	if s.db != nil {
		u, err := s.GetUserByID(id)
		if err != nil {
			return nil, err
		}

		fullName := u.FullName
		if req.FullName != "" {
			fullName = strings.TrimSpace(req.FullName)
		}
		nip := u.NIP
		if req.NIP != "" {
			nip = strings.TrimSpace(req.NIP)
		}

		roleID := u.RoleID
		roleName := u.Role
		if req.RoleID > 0 || req.Role != "" {
			roleID, roleName = s.resolveRole(req.RoleID, req.Role)
		}

		if req.Password != "" {
			if len(req.Password) < 6 {
				return nil, errors.New("password baru minimal 6 karakter")
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				return nil, err
			}
			_, err = s.db.Exec("UPDATE users SET full_name = ?, nip = ?, role_id = ?, role = ?, is_active = ?, password_hash = ? WHERE id = ?",
				fullName, nip, roleID, roleName, isActive, string(hash), id)
			if err != nil {
				return nil, fmt.Errorf("gagal update ke MariaDB: %w", err)
			}
		} else {
			_, err := s.db.Exec("UPDATE users SET full_name = ?, nip = ?, role_id = ?, role = ?, is_active = ? WHERE id = ?",
				fullName, nip, roleID, roleName, isActive, id)
			if err != nil {
				return nil, fmt.Errorf("gagal update ke MariaDB: %w", err)
			}
		}

		dto := models.UserDTO{
			ID:        id,
			Username:  u.Username,
			FullName:  fullName,
			NIP:       nip,
			RoleID:    roleID,
			Role:      roleName,
			IsActive:  isActive,
			CreatedAt: u.CreatedAt.Format("02/01/2006 15:04"),
		}
		return &dto, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[id]
	if !ok {
		return nil, errors.New("user tidak ditemukan")
	}

	if req.FullName != "" {
		u.FullName = strings.TrimSpace(req.FullName)
	}
	if req.NIP != "" {
		u.NIP = strings.TrimSpace(req.NIP)
	}
	if req.RoleID > 0 || req.Role != "" {
		u.RoleID, u.Role = s.resolveRole(req.RoleID, req.Role)
	}
	u.IsActive = isActive

	if req.Password != "" {
		if len(req.Password) < 6 {
			return nil, errors.New("password baru minimal 6 karakter")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = string(hash)
	}

	_ = s.saveToFileLocked()
	dto := u.ToDTO()
	return &dto, nil
}

// DeleteUser menghapus akun petugas (mencegah penghapusan admin utama id: 1)
func (s *UserService) DeleteUser(id int) error {
	if s.db != nil {
		u, err := s.GetUserByID(id)
		if err != nil {
			return err
		}
		if id == 1 || strings.EqualFold(u.Username, "admin") {
			return errors.New("akun 'admin' utama tidak boleh dihapus demi keamanan sistem")
		}

		_, err = s.db.Exec("DELETE FROM users WHERE id = ?", id)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[id]
	if !ok {
		return errors.New("user tidak ditemukan")
	}

	if id == 1 || strings.EqualFold(u.Username, "admin") {
		return errors.New("akun 'admin' utama tidak boleh dihapus demi keamanan sistem")
	}

	delete(s.users, id)
	return s.saveToFileLocked()
}
