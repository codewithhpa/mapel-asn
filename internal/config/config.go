package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadConfig memuat .env dari working directory, lalu fallback ke folder exe.
// Tanpa fallback ini, app yang dijalankan dari folder lain diam-diam kehilangan
// kredensial BioTime dan semua panggilan BioTime gagal 401.
func LoadConfig() {
	if err := godotenv.Load(); err == nil {
		return
	}
	if exe, err := os.Executable(); err == nil {
		if err := godotenv.Load(filepath.Join(filepath.Dir(exe), ".env")); err == nil {
			return
		}
	}
	log.Println("No .env file found, relying on system environment variables")
}

func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
