package services

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dashboard-presensi/internal/models"
)

// snapshotFile adalah amplop persistensi: ringkasan + kapan disimpan (untuk max-age).
type snapshotFile struct {
	SavedAt time.Time                 `json:"saved_at"`
	Summary *models.AttendanceSummary `json:"summary"`
}

// SnapshotStore menyimpan ringkasan last-good ke disk (satu JSON per cache-key).
// Sengaja independen dari MariaDB: tetap berfungsi walau DB ikut mati, dan
// membuat dashboard selamat dari restart process (cold start → sajikan snapshot).
type SnapshotStore struct {
	mu     sync.RWMutex
	dir    string
	maxAge time.Duration

	pruneMu   sync.Mutex
	lastPrune time.Time
}

func NewSnapshotStore(dir string, maxAge time.Duration) *SnapshotStore {
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}
	return &SnapshotStore{dir: dir, maxAge: maxAge}
}

// keyToFile memetakan cache-key ("date:..", "period:..:..") ke nama file aman
// (Windows melarang ':' pada nama file).
func (s *SnapshotStore) keyToFile(key string) string {
	var b strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String() + ".json"
}

// Save menyimpan ringkasan sukses. Ringkasan error TIDAK PERNAH disimpan
// (agar halaman error tidak jadi "last-good" palsu).
func (s *SnapshotStore) Save(key string, summary *models.AttendanceSummary) {
	if summary == nil || summary.ErrorMessage != "" {
		return
	}
	// Rampingkan: RawTransactions tidak dipakai view mana pun (cek: hanya ditulis, tak dibaca).
	slim := *summary
	slim.Logs = make([]models.DailyAttendanceLog, len(summary.Logs))
	for i, l := range summary.Logs {
		l.RawTransactions = nil
		slim.Logs[i] = l
	}
	payload, err := json.Marshal(snapshotFile{SavedAt: time.Now(), Summary: &slim})
	if err != nil {
		log.Printf("[Snapshot] marshal %s gagal: %v\n", key, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		log.Printf("[Snapshot] mkdir %s gagal: %v\n", s.dir, err)
		return
	}
	target := filepath.Join(s.dir, s.keyToFile(key))
	tmp, err := os.CreateTemp(s.dir, "snap-*.tmp")
	if err != nil {
		log.Printf("[Snapshot] tmp %s gagal: %v\n", key, err)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		log.Printf("[Snapshot] rename %s gagal: %v\n", key, err)
		return
	}
	s.pruneLocked()
}

// Load membaca snapshot bila ada dan belum kedaluwarsa.
func (s *SnapshotStore) Load(key string) (*models.AttendanceSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(s.dir, s.keyToFile(key)))
	if err != nil {
		return nil, false
	}
	var payload snapshotFile
	if err := json.Unmarshal(data, &payload); err != nil || payload.Summary == nil {
		return nil, false
	}
	if time.Since(payload.SavedAt) > s.maxAge {
		return nil, false
	}
	return payload.Summary, true
}

// pruneLocked menghapus file snapshot lebih tua dari maxAge (dibatasi 1x/jam).
func (s *SnapshotStore) pruneLocked() {
	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()
	if time.Since(s.lastPrune) < time.Hour {
		return
	}
	s.lastPrune = time.Now()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > s.maxAge {
			_ = os.Remove(filepath.Join(s.dir, name))
		}
	}
}
