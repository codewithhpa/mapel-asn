package services

import (
	"context"
	"log"
	"sync"
	"time"

	"dashboard-presensi/internal/models"
)

// EmployeeDirectory adalah mirror cache ringan (opsi ringan) untuk master pegawai BioTime.
// CRUD tetap ke alat; cache ini hanya untuk join Alpha agar dashboard tidak menghantam
// /personnel/api/employees/ di setiap render + tetap tampil saat alat timeout (stale).
type EmployeeDirectory struct {
	mu        sync.RWMutex
	employees []models.BioTimeEmployee
	byCode    map[string]*models.BioTimeEmployee
	resigned  map[string]bool
	expiresAt time.Time
	ttl       time.Duration
}

func NewEmployeeDirectory(ttl time.Duration) *EmployeeDirectory {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &EmployeeDirectory{
		byCode:   make(map[string]*models.BioTimeEmployee),
		resigned: make(map[string]bool),
		ttl:      ttl,
	}
}

// GetActiveEmployees mengembalikan mirror master aktif (tanpa resign), pakai cache bila segar.
func (d *EmployeeDirectory) GetActiveEmployees(ctx context.Context, client *BioTimeClient) []models.BioTimeEmployee {
	d.mu.RLock()
	if time.Now().Before(d.expiresAt) && len(d.employees) > 0 {
		out := d.employees
		d.mu.RUnlock()
		return out
	}
	d.mu.RUnlock()

	fetched, resignedSet := d.fetchFromBioTime(ctx, client)
	if len(fetched) == 0 {
		// Fallback stale saat alat offline: tetap tampilkan data terakhir (Alpha tetap jalan)
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.employees
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.employees = fetched
	d.byCode = make(map[string]*models.BioTimeEmployee, len(fetched))
	for i := range fetched {
		e := &fetched[i]
		d.byCode[e.EmpCode] = e
	}
	d.resigned = resignedSet
	d.expiresAt = time.Now().Add(d.ttl)
	return d.employees
}

// Invalidate menghapus cache mirror (dipanggil setelah CRUD master pegawai/mutasi ke alat).
func (d *EmployeeDirectory) Invalidate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.expiresAt = time.Time{}
}

// fetchFromBioTime menarik seluruh halaman employees + resign sebagai filter nonaktif.
func (d *EmployeeDirectory) fetchFromBioTime(ctx context.Context, client *BioTimeClient) ([]models.BioTimeEmployee, map[string]bool) {
	const pageSize = 200
	const maxPages = 25

	var all []models.BioTimeEmployee
	for page := 1; page <= maxPages; page++ {
		resp, err := client.FetchEmployees(ctx, page, pageSize, "", 0)
		if err != nil {
			log.Printf("[EmployeeDirectory] FetchEmployees page %d gagal: %v\n", page, err)
			break
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

	resignedSet := make(map[string]bool)
	if resigns, err := client.FetchResigns(ctx); err == nil {
		for _, r := range resigns {
			if r.Employee != nil && r.Employee.EmpCode != "" {
				resignedSet[r.Employee.EmpCode] = true
			}
		}
	} else {
		log.Printf("[EmployeeDirectory] FetchResigns gagal (diabaikan): %v\n", err)
	}

	if len(resignedSet) == 0 {
		return all, resignedSet
	}
	active := all[:0]
	for _, e := range all {
		if !resignedSet[e.EmpCode] {
			active = append(active, e)
		}
	}
	return active, resignedSet
}
