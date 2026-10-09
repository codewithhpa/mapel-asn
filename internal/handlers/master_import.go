package handlers

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
)

// GetEmployeeTemplate mengunduh template .xlsx import pegawai (ref live dari BioTime).
func (h *DashboardHandler) GetEmployeeTemplate(c *fiber.Ctx) error {
	ctx := context.Background()

	depts, _ := h.biotimeClient.FetchDepartments(ctx)
	positions, _ := h.biotimeClient.FetchPositions(ctx)
	areas, _ := h.biotimeClient.FetchAreas(ctx)

	data, err := services.BuildEmployeeTemplate(depts, positions, areas)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": fmt.Sprintf("Gagal membuat template: %v", err),
		})
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", `attachment; filename="Template_Import_Pegawai_MAPEL.xlsx"`)
	return c.Send(data)
}

// PostImportEmployees memproses upload .xlsx dan membuat pegawai massal ke BioTime.
func (h *DashboardHandler) PostImportEmployees(c *fiber.Ctx) error {
	ctx := context.Background()

	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": "Pilih file Excel (.xlsx) dulu",
		})
	}
	name := strings.ToLower(fh.Filename)
	if !strings.HasSuffix(name, ".xlsx") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": "Format harus .xlsx sesuai template resmi",
		})
	}
	if fh.Size > 4<<20 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": "File maksimal 4 MB",
		})
	}

	src, err := fh.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": "File tidak bisa dibaca",
		})
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, (4<<20)+1))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": "File tidak bisa dibaca",
		})
	}

	rows, err := services.ParseEmployeeImportXLSX(raw)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status": "error", "message": err.Error(),
		})
	}

	depts, _ := h.biotimeClient.FetchDepartments(ctx)
	positions, _ := h.biotimeClient.FetchPositions(ctx)
	areas, _ := h.biotimeClient.FetchAreas(ctx)
	existing := h.collectExistingEmpCodes(ctx)

	pending, results := services.ValidateImportRows(rows, depts, positions, areas, existing)

	sukses, duplikat, gagal := 0, 0, 0
	for _, r := range results {
		if r.Status == "duplikat" {
			duplikat++
		} else {
			gagal++
		}
	}
	for _, p := range pending {
		req := p.Req
		created, err := h.biotimeClient.CreateEmployee(ctx, &req)
		fullName := strings.TrimSpace(p.Row.FirstName + " " + p.Row.LastName)
		if err != nil {
			gagal++
			results = append(results, services.ImportRowResult{
				RowNum: p.Row.RowNum, EmpCode: p.Row.EmpCode, FullName: fullName,
				Status: "gagal", Message: fmt.Sprintf("BioTime menolak: %v", err),
			})
			continue
		}
		sukses++
		results = append(results, services.ImportRowResult{
			RowNum: p.Row.RowNum, EmpCode: created.EmpCode, FullName: created.FullName,
			Status: "sukses", Message: "Terdaftar di BioTime",
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].RowNum < results[j].RowNum })

	_ = h.auditService.RecordMasterAction(
		"MASTER_PEGAWAI",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Import Excel pegawai (%s): %d sukses, %d duplikat, %d gagal dari %d baris",
			fh.Filename, sukses, duplikat, gagal, len(rows)),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Import selesai: %d sukses, %d duplikat dilewati, %d gagal", sukses, duplikat, gagal),
		"summary": fiber.Map{"total": len(rows), "sukses": sukses, "duplikat": duplikat, "gagal": gagal},
		"results": results,
	})
}

// collectExistingEmpCodes mengambil seluruh emp_code di BioTime untuk deteksi duplikat.
func (h *DashboardHandler) collectExistingEmpCodes(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	const pageSize = 500
	for page := 1; page <= 20; page++ {
		resp, err := h.biotimeClient.FetchEmployees(ctx, page, pageSize, "", 0)
		if err != nil || resp == nil || len(resp.Data) == 0 {
			break
		}
		for _, e := range resp.Data {
			if c := strings.ToUpper(strings.TrimSpace(e.EmpCode)); c != "" {
				out[c] = true
			}
		}
		if len(resp.Data) < pageSize {
			break
		}
	}
	return out
}
