package main

import (
	"log"
	"os"
	"path/filepath"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/database"
	"dashboard-presensi/internal/handlers"
	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
)

func main() {
	// Self-healing working directory: bila ./web/views tidak ketemu dari CWD
	// (misal dijalankan scheduler tanpa "Start in"), pindah ke folder exe
	// asalkan di sana ada web/views. Cek ganda ini penting karena `go run`
	// menaruh binary di folder TEMP — chdir membabi-buta justru merusak path.
	if _, err := os.Stat(filepath.Join("web", "views")); os.IsNotExist(err) {
		if exe, err := os.Executable(); err == nil {
			if dir := filepath.Dir(exe); dir != "" {
				if _, err := os.Stat(filepath.Join(dir, "web", "views")); err == nil {
					_ = os.Chdir(dir)
				}
			}
		}
	}

	// Load .env file
	config.LoadConfig()

	// Initialize MariaDB
	db, err := database.InitDB()
	if err != nil {
		log.Printf("[Database Warning] %v - falling back to JSON storage\n", err)
	}

	// Initialize Core Services
	biotimeClient := services.NewBioTimeClient()
	attendanceEngine := services.NewAttendanceEngine()
	attendanceCache := services.NewAttendanceCache()
	attendanceService := services.NewAttendanceService(biotimeClient, attendanceEngine, attendanceCache)
	auditService := services.NewAuditService("./data/audit_trail.json", db)
	userService := services.NewUserService("./data/users.json", db)
	ruleService := services.NewRuleService(db)
	attendanceService.SetRuleService(ruleService)
	attendanceService.SetAuditService(auditService)

	// Setup template engine with auto-reload for local development
	engine := html.New("./web/views", ".html")
	engine.Reload(true)
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("mul", func(a, b int) int {
		return a * b
	})
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)

	// Initialize Fiber app (Network "tcp" = dual-stack IPv4+IPv6 agar
	// http://localhost:3000/ tidak nyangkut di ::1; default Fiber tcp4 hanya IPv4)
	app := fiber.New(fiber.Config{
		Views:   engine,
		Network: "tcp",
	})

	// Middleware
	app.Use(logger.New())
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "SAMEORIGIN")
		c.Set("X-XSS-Protection", "1; mode=block")
		return c.Next()
	})

	// Serve Static Files (CSS, JS, Images from Mazer)
	app.Static("/assets", "./web/static/assets")

	// Register Routes
	handlers.SetupRoutes(app, attendanceService, auditService, userService, ruleService)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "mapel-asn-bff",
		})
	})

	// Start server
	port := config.GetEnv("APP_PORT", "3000")
	log.Printf("Server starting on http://localhost:%s\n", port)
	log.Fatal(app.Listen(":" + port))
}
