package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/models"

	"github.com/gofiber/fiber/v2"
)

// getCurrentUserName mengambil nama user yang sedang login untuk audit log
func (h *DashboardHandler) getCurrentUserName(c *fiber.Ctx) string {
	if u, ok := c.Locals("currentUser").(*models.User); ok && u != nil {
		return fmt.Sprintf("%s (%s)", u.FullName, u.Username)
	}
	return "Administrator Sistem"
}

// generateNextCode menghitung nomor urut kode berikutnya berbasis prefix (misal: B001, L001, J001)
func generateNextCode(prefix string, existingCodes []string) string {
	prefixUpper := strings.ToUpper(prefix)
	maxNum := 0

	for _, code := range existingCodes {
		trimmed := strings.TrimSpace(code)
		trimmedUpper := strings.ToUpper(trimmed)

		if strings.HasPrefix(trimmedUpper, prefixUpper) {
			numPart := trimmedUpper[len(prefixUpper):]
			if num, err := strconv.Atoi(numPart); err == nil {
				if num > maxNum {
					maxNum = num
				}
			}
		}
	}

	return fmt.Sprintf("%s%03d", prefixUpper, maxNum+1)
}

// ==========================================
// 1. MASTER PEGAWAI (EMPLOYEE) HANDLERS
// ==========================================

func (h *DashboardHandler) RenderMasterPegawai(c *fiber.Ctx) error {
	ctx := context.Background()

	// Tarik data pendukung secara paralel/berurutan
	depts, err := h.biotimeClient.FetchDepartments(ctx)
	if err != nil {
		log.Printf("[MasterPegawai] Gagal menarik departemen: %v\n", err)
	}

	areas, err := h.biotimeClient.FetchAreas(ctx)
	if err != nil {
		log.Printf("[MasterPegawai] Gagal menarik area: %v\n", err)
	}

	positions, err := h.biotimeClient.FetchPositions(ctx)
	if err != nil {
		log.Printf("[MasterPegawai] Gagal menarik posisi: %v\n", err)
	}

	empResp, err := h.biotimeClient.FetchEmployees(ctx, 1, 200, "", 0)
	var employees []models.BioTimeEmployee
	totalCount := 0
	if err == nil && empResp != nil {
		employees = empResp.Data
		totalCount = empResp.Count
	} else {
		log.Printf("[MasterPegawai] Gagal menarik pegawai: %v\n", err)
	}

	// Hitung metrik KPI
	var biometricActiveCount int
	for _, e := range employees {
		if e.VLFace != "" && e.VLFace != "-" {
			biometricActiveCount++
		}
	}

	return c.Render("master/pegawai", fiber.Map{
		"BioTimeVersion":       config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":                "Master Data Pegawai (ASN)",
		"ActiveMenu":           "pegawai",
		"ActiveMenuGroup":      "master",
		"CurrentUser":          c.Locals("currentUser"),
		"Employees":            employees,
		"TotalCount":           totalCount,
		"BiometricActiveCount": biometricActiveCount,
		"Departments":          depts,
		"Areas":                areas,
		"Positions":            positions,
	}, "layouts/main")
}

func (h *DashboardHandler) GetEmployeesJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	search := c.Query("search")
	deptID, _ := strconv.ParseInt(c.Query("department"), 10, 64)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "100"))

	resp, err := h.biotimeClient.FetchEmployees(ctx, page, pageSize, search, deptID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(resp)
}

func (h *DashboardHandler) PostCreateEmployee(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.CreateEmployeeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format request pegawai tidak valid",
		})
	}

	if req.EmpCode == "" || req.FirstName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "NIP / ID Pegawai dan Nama Depan wajib diisi",
		})
	}

	if req.Department <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Pilih Departemen / Unit Kerja yang valid",
		})
	}

	if len(req.Area) == 0 {
		req.Area = []int64{1} // Default area 1 jika tidak dipilih
	}

	created, err := h.biotimeClient.CreateEmployee(ctx, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": fmt.Sprintf("Gagal menyimpan pegawai ke BioTime: %v", err),
		})
	}

	// Catat audit trail
	_ = h.auditService.RecordMasterAction(
		"MASTER_PEGAWAI",
		h.getCurrentUserName(c),
		created.EmpCode,
		created.FullName,
		fmt.Sprintf("Menambahkan Pegawai ASN Baru: %s (NIP: %s)", created.FullName, created.EmpCode),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":  "success",
		"message": "Pegawai berhasil ditambahkan ke sistem BioTime",
		"data":    created,
	})
}

func (h *DashboardHandler) PutUpdateEmployee(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID pegawai tidak valid",
		})
	}

	var req models.UpdateEmployeeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format request update tidak valid",
		})
	}

	if len(req.Area) == 0 {
		req.Area = []int64{1}
	}

	updated, err := h.biotimeClient.UpdateEmployee(ctx, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": fmt.Sprintf("Gagal memperbarui pegawai di BioTime: %v", err),
		})
	}

	// Catat audit trail
	_ = h.auditService.RecordMasterAction(
		"MASTER_PEGAWAI",
		h.getCurrentUserName(c),
		updated.EmpCode,
		updated.FullName,
		fmt.Sprintf("Mengubah Data Pegawai: %s (NIP: %s)", updated.FullName, updated.EmpCode),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Data pegawai berhasil diperbarui",
		"data":    updated,
	})
}

func (h *DashboardHandler) DeleteEmployee(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID pegawai tidak valid",
		})
	}

	// Ambil info nama untuk audit log sebelum dihapus
	emp, _ := h.biotimeClient.GetEmployeeByID(ctx, id)
	empName := fmt.Sprintf("ID #%d", id)
	empCode := ""
	if emp != nil {
		empName = emp.FullName
		empCode = emp.EmpCode
	}

	if err := h.biotimeClient.DeleteEmployee(ctx, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": fmt.Sprintf("Gagal menghapus pegawai dari BioTime: %v", err),
		})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_PEGAWAI",
		h.getCurrentUserName(c),
		empCode,
		empName,
		fmt.Sprintf("Menghapus Pegawai dari BioTime: %s (NIP: %s)", empName, empCode),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Pegawai berhasil dihapus dari BioTime",
	})
}

// ==========================================
// 2. MASTER DEPARTEMEN (BAGIAN) HANDLERS
// ==========================================

func (h *DashboardHandler) RenderMasterDepartemen(c *fiber.Ctx) error {
	ctx := context.Background()
	depts, err := h.biotimeClient.FetchDepartments(ctx)
	if err != nil {
		log.Printf("[MasterDept] Error: %v\n", err)
	}

	var deptCodes []string
	for _, d := range depts {
		deptCodes = append(deptCodes, d.DeptCode)
	}
	nextDeptCode := generateNextCode("B", deptCodes)

	return c.Render("master/departemen", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":           "Master Data Departemen (Bagian)",
		"ActiveMenu":      "departemen",
		"ActiveMenuGroup": "master",
		"CurrentUser":     c.Locals("currentUser"),
		"Departments":     depts,
		"NextDeptCode":    nextDeptCode,
	}, "layouts/main")
}

func (h *DashboardHandler) GetDepartmentsJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	depts, err := h.biotimeClient.FetchDepartments(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "data": depts})
}

func (h *DashboardHandler) PostCreateDepartment(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.CreateDepartmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	req.DeptCode = strings.TrimSpace(req.DeptCode)
	req.DeptName = strings.TrimSpace(req.DeptName)

	if req.DeptName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Nama Departemen wajib diisi"})
	}

	if req.DeptCode == "" {
		depts, _ := h.biotimeClient.FetchDepartments(ctx)
		var codes []string
		for _, d := range depts {
			codes = append(codes, d.DeptCode)
		}
		req.DeptCode = generateNextCode("B", codes)
	}

	created, err := h.biotimeClient.CreateDepartment(ctx, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_DEPARTEMEN",
		h.getCurrentUserName(c),
		created.DeptCode,
		created.DeptName,
		fmt.Sprintf("Menambahkan Departemen: %s (Kode: %s)", created.DeptName, created.DeptCode),
		c.IP(),
	)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":  "success",
		"message": "Departemen berhasil ditambahkan",
		"data":    created,
	})
}

func (h *DashboardHandler) PutUpdateDepartment(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	var req models.UpdateDepartmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	updated, err := h.biotimeClient.UpdateDepartment(ctx, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_DEPARTEMEN",
		h.getCurrentUserName(c),
		"",
		updated.DeptName,
		fmt.Sprintf("Mengubah Departemen ID #%d: %s", id, updated.DeptName),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Departemen berhasil diperbarui", "data": updated})
}

func (h *DashboardHandler) DeleteDepartment(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	if err := h.biotimeClient.DeleteDepartment(ctx, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_DEPARTEMEN",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Menghapus Departemen ID #%d", id),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Departemen berhasil dihapus"})
}

// ==========================================
// 3. MASTER AREA (LOKASI KERJA) HANDLERS
// ==========================================

func (h *DashboardHandler) RenderMasterArea(c *fiber.Ctx) error {
	ctx := context.Background()
	areas, err := h.biotimeClient.FetchAreas(ctx)
	if err != nil {
		log.Printf("[MasterArea] Error: %v\n", err)
	}

	var areaCodes []string
	for _, a := range areas {
		areaCodes = append(areaCodes, a.AreaCode)
	}
	nextAreaCode := generateNextCode("L", areaCodes)

	return c.Render("master/area", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":           "Master Data Area (Lokasi Kerja)",
		"ActiveMenu":      "area",
		"ActiveMenuGroup": "master",
		"CurrentUser":     c.Locals("currentUser"),
		"Areas":           areas,
		"NextAreaCode":    nextAreaCode,
	}, "layouts/main")
}

func (h *DashboardHandler) GetAreasJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	areas, err := h.biotimeClient.FetchAreas(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "data": areas})
}

func (h *DashboardHandler) PostCreateArea(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.CreateAreaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	req.AreaCode = strings.TrimSpace(req.AreaCode)
	req.AreaName = strings.TrimSpace(req.AreaName)

	if req.AreaName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Nama Area wajib diisi"})
	}

	if req.AreaCode == "" {
		areas, _ := h.biotimeClient.FetchAreas(ctx)
		var codes []string
		for _, a := range areas {
			codes = append(codes, a.AreaCode)
		}
		req.AreaCode = generateNextCode("L", codes)
	}

	created, err := h.biotimeClient.CreateArea(ctx, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_AREA",
		h.getCurrentUserName(c),
		created.AreaCode,
		created.AreaName,
		fmt.Sprintf("Menambahkan Area Kerja: %s (Kode: %s)", created.AreaName, created.AreaCode),
		c.IP(),
	)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"status": "success", "message": "Area kerja berhasil ditambahkan", "data": created})
}

func (h *DashboardHandler) PutUpdateArea(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	var req models.UpdateAreaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	updated, err := h.biotimeClient.UpdateArea(ctx, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_AREA",
		h.getCurrentUserName(c),
		"",
		updated.AreaName,
		fmt.Sprintf("Mengubah Area ID #%d: %s", id, updated.AreaName),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Area berhasil diperbarui", "data": updated})
}

func (h *DashboardHandler) DeleteArea(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	if err := h.biotimeClient.DeleteArea(ctx, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_AREA",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Menghapus Area ID #%d", id),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Area berhasil dihapus"})
}

// ==========================================
// 4. MASTER JABATAN (POSITION) HANDLERS
// ==========================================

func (h *DashboardHandler) RenderMasterJabatan(c *fiber.Ctx) error {
	ctx := context.Background()
	positions, err := h.biotimeClient.FetchPositions(ctx)
	if err != nil {
		log.Printf("[MasterJabatan] Error: %v\n", err)
	}

	var posCodes []string
	for _, p := range positions {
		posCodes = append(posCodes, p.PositionCode)
	}
	nextPosCode := generateNextCode("J", posCodes)

	return c.Render("master/jabatan", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":           "Master Data Jabatan (Position)",
		"ActiveMenu":      "jabatan",
		"ActiveMenuGroup": "master",
		"CurrentUser":     c.Locals("currentUser"),
		"Positions":       positions,
		"NextPosCode":     nextPosCode,
	}, "layouts/main")
}

func (h *DashboardHandler) GetPositionsJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	positions, err := h.biotimeClient.FetchPositions(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "data": positions})
}

func (h *DashboardHandler) PostCreatePosition(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.CreatePositionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	req.PositionCode = strings.TrimSpace(req.PositionCode)
	req.PositionName = strings.TrimSpace(req.PositionName)

	if req.PositionName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Nama Jabatan wajib diisi"})
	}

	if req.PositionCode == "" {
		positions, _ := h.biotimeClient.FetchPositions(ctx)
		var codes []string
		for _, p := range positions {
			codes = append(codes, p.PositionCode)
		}
		req.PositionCode = generateNextCode("J", codes)
	}

	created, err := h.biotimeClient.CreatePosition(ctx, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_JABATAN",
		h.getCurrentUserName(c),
		created.PositionCode,
		created.PositionName,
		fmt.Sprintf("Menambahkan Jabatan: %s (Kode: %s)", created.PositionName, created.PositionCode),
		c.IP(),
	)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"status": "success", "message": "Jabatan berhasil ditambahkan", "data": created})
}

func (h *DashboardHandler) PutUpdatePosition(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	var req models.UpdatePositionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	updated, err := h.biotimeClient.UpdatePosition(ctx, id, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_JABATAN",
		h.getCurrentUserName(c),
		"",
		updated.PositionName,
		fmt.Sprintf("Mengubah Jabatan ID #%d: %s", id, updated.PositionName),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Jabatan berhasil diperbarui", "data": updated})
}

func (h *DashboardHandler) DeletePosition(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	if err := h.biotimeClient.DeletePosition(ctx, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MASTER_JABATAN",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Menghapus Jabatan ID #%d", id),
		c.IP(),
	)

	return c.JSON(fiber.Map{"status": "success", "message": "Jabatan berhasil dihapus"})
}

// ==========================================
// 5. MASTER MUTASI / RESIGN ASN HANDLERS
// ==========================================

func (h *DashboardHandler) RenderMasterMutasi(c *fiber.Ctx) error {
	ctx := context.Background()
	resigns, err := h.biotimeClient.FetchResigns(ctx)
	if err != nil {
		log.Printf("[MasterMutasi] Error: %v\n", err)
	}

	empResp, err := h.biotimeClient.FetchEmployees(ctx, 1, 200, "", 0)
	var employees []models.BioTimeEmployee
	if err == nil && empResp != nil {
		employees = empResp.Data
	}

	today := time.Now().Format("2006-01-02")

	return c.Render("master/mutasi", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":           "Master Data Mutasi & Resign ASN",
		"ActiveMenu":      "mutasi",
		"ActiveMenuGroup": "master",
		"CurrentUser":     c.Locals("currentUser"),
		"Resigns":         resigns,
		"Employees":       employees,
		"TodayDate":       today,
	}, "layouts/main")
}

func (h *DashboardHandler) GetResignsJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	resigns, err := h.biotimeClient.FetchResigns(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "data": resigns})
}

func (h *DashboardHandler) PostCreateResign(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.CreateResignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format request tidak valid"})
	}

	if req.Employee <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Pilih Pegawai yang akan dimutasi"})
	}
	if req.ResignDate == "" {
		req.ResignDate = time.Now().Format("2006-01-02")
	}
	if req.ResignType <= 0 {
		req.ResignType = 1
	}
	req.DisableAtt = true

	created, err := h.biotimeClient.CreateResign(ctx, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MUTASI_RESIGN",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Mencatat Mutasi/Resign Pegawai ID #%d (Alasan: %s, Efektif: %s)", req.Employee, req.Reason, req.ResignDate),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"status": "success", "message": "Mutasi pegawai berhasil dicatat, presensi dinonaktifkan", "data": created})
}

func (h *DashboardHandler) DeleteResign(c *fiber.Ctx) error {
	ctx := context.Background()
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID tidak valid"})
	}

	if err := h.biotimeClient.DeleteResign(ctx, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MUTASI_RESIGN",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Menghapus Catatan Mutasi ID #%d", id),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{"status": "success", "message": "Catatan mutasi berhasil dihapus"})
}

func (h *DashboardHandler) PostReinstatementResign(c *fiber.Ctx) error {
	ctx := context.Background()
	var req models.ReinstatementRequest
	if err := c.BodyParser(&req); err != nil || len(req.Resigns) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Daftar ID mutasi tidak valid"})
	}

	if err := h.biotimeClient.ReinstatementResign(ctx, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	_ = h.auditService.RecordMasterAction(
		"MUTASI_RESIGN",
		h.getCurrentUserName(c),
		"",
		"",
		fmt.Sprintf("Pemulihan / Pengaktifan Kembali ASN dari status Mutasi: %v", req.Resigns),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{"status": "success", "message": "Pegawai berhasil dipulihkan & presensi diaktifkan kembali"})
}
