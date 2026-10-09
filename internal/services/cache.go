package services

import (
	"sync"
	"time"

	"dashboard-presensi/internal/models"
)

// cacheEntry menyimpan satu entri ringkasan beserta waktu kedaluwarsa.
type cacheEntry struct {
	data      *models.AttendanceSummary
	expiresAt time.Time
}

// AttendanceCache menyediakan in-memory cache thread-safe per kunci tanggal/periode.
type AttendanceCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

func NewAttendanceCache() *AttendanceCache {
	return &AttendanceCache{entries: make(map[string]*cacheEntry)}
}

// Get mengambil data dari cache berdasarkan kunci jika belum kedaluwarsa.
func (c *AttendanceCache) Get(key string) (*models.AttendanceSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok || e == nil || e.data == nil {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

// Set menyimpan data ke dalam cache dengan kunci dan durasi TTL tertentu.
func (c *AttendanceCache) Set(key string, data *models.AttendanceSummary, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]*cacheEntry)
	}
	c.entries[key] = &cacheEntry{data: data, expiresAt: time.Now().Add(ttl)}
}

// GetStale mengambil data cache terakhir untuk kunci sebagai fallback saat BioTime offline.
func (c *AttendanceCache) GetStale(key string) *models.AttendanceSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.entries[key]; ok && e != nil {
		return e.data
	}
	return nil
}

// Clear membuang seluruh entri cache (dipanggil setelah dispensasi/aturan berubah
// agar efeknya instan, bukan menunggu TTL).
func (c *AttendanceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}
