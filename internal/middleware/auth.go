package middleware

import (
	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
)

// RequireAuth melindungi rute agar hanya bisa diakses oleh petugas yang sudah login
func RequireAuth(userSvc *services.UserService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies("auth_session")
		if token == "" {
			if len(c.Path()) >= 5 && c.Path()[:5] == "/api/" {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"status":  "error",
					"message": "Sesi autentikasi telah berakhir. Silakan login kembali.",
				})
			}
			return c.Redirect("/login")
		}

		user, ok := userSvc.GetSessionUser(token)
		if !ok || user.IsActive != models.UserStatusActive {
			c.ClearCookie("auth_session")
			if len(c.Path()) >= 5 && c.Path()[:5] == "/api/" {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"status":  "error",
					"message": "Sesi tidak valid atau akun telah dinonaktifkan.",
				})
			}
			return c.Redirect("/login")
		}

		c.Locals("currentUser", user)
		return c.Next()
	}
}

// OptionalAuth membaca user jika ada session cookie aktif tanpa memblokir akses publik
func OptionalAuth(userSvc *services.UserService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies("auth_session"); token != "" {
			if user, ok := userSvc.GetSessionUser(token); ok && user.IsActive == models.UserStatusActive {
				c.Locals("currentUser", user)
			}
		}
		return c.Next()
	}
}

// RequireAdmin melindungi rute agar HANYA bisa diakses oleh Administrator (RoleID = 1)
func RequireAdmin(userSvc *services.UserService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies("auth_session")
		if token == "" {
			if len(c.Path()) >= 5 && c.Path()[:5] == "/api/" {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"status":  "error",
					"message": "Sesi autentikasi telah berakhir. Silakan login kembali.",
				})
			}
			return c.Redirect("/login")
		}

		user, ok := userSvc.GetSessionUser(token)
		if !ok || user.IsActive != models.UserStatusActive {
			c.ClearCookie("auth_session")
			if len(c.Path()) >= 5 && c.Path()[:5] == "/api/" {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"status":  "error",
					"message": "Sesi tidak valid atau akun telah dinonaktifkan.",
				})
			}
			return c.Redirect("/login")
		}

		// Validasi Hak Akses Khusus Administrator (RoleID = 1)
		if user.RoleID != 1 && user.Role != models.RoleAdmin && user.Role != "admin" {
			if len(c.Path()) >= 5 && c.Path()[:5] == "/api/" {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"status":  "error",
					"message": "Akses ditolak. Fitur ini hanya dapat diakses oleh Administrator.",
				})
			}
			// Redirect non-admin (seperti Verifikator) ke /dispensasi
			return c.Redirect("/dispensasi")
		}

		c.Locals("currentUser", user)
		return c.Next()
	}
}
