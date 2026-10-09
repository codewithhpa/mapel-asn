# Product Requirements Document (PRD)
**Project Name:** MAPEL — Monitoring Apel (BFF Architecture)  
**Document Version:** 1.11 (Integrasi Standar Rumus & Analisa Tingkat Kepatuhan ASN)  
**Status:** Approved for Implementation  

---

## 1. Project Overview & Business Goals
Proyek ini bertujuan untuk membangun sebuah dashboard *reporting* presensi karyawan & ASN berbasis web yang aman, sangat cepat (*blazing fast*), dan responsif (*mobile-first*). Dashboard ini berfungsi sebagai *viewer* publik, display TV lobby/kantor, atau monitoring pimpinan yang mengkonsumsi data dari **API Utama (ZKTeco BioTime 8.5)** secara otomatis tanpa mengekspos rute, kredensial, maupun token API tersebut ke publik.

### Target Pengguna & Use Case
* **Manajemen & Pimpinan (Eksekutif):** Memantau kedisiplinan harian, tingkat kepatuhan apel pagi/sore, serta rekapitulasi kehadiran divisi secara realtime.
* **Lobby / Display TV Kantor:** Menampilkan status kehadiran dan kepatuhan agenda pagi & sore secara transparan dengan fitur auto-refresh.
* **Karyawan / ASN:** Mengecek status kehadiran dan rekaman waktu scan biometrik mandiri.

---

## 2. System Architecture & Token Lifecycle Strategy
Sistem menggunakan pola **Backend-for-Frontend (BFF)** dengan pendekatan **Server-Side Rendering (SSR)**, **In-Memory Caching**, dan **Autonomous JWT Token Management**.

```
[ Browser / Klien ]
       │  (Hanya menerima HTML statis + Assets)
       ▼
[ Go Fiber BFF Server ]
  ├── 1. In-Memory Cache Check (TTL: 30-60 detik)
  │      └── Jika HIT: Langsung render HTML template
  └── 2. Jika MISS:
         ├── A. Periksa Ketersediaan JWT Token di Memory
         │      └── Jika Token Kosong / Expired:
         │             Panggil POST /jwt-api-token-auth/ (Username & Password)
         │             Simpan Token JWT baru ke Memory
         ├── B. Request Log ke GET /iclock/api/transactions/ (Header: Authorization: JWT <token>)
         │      └── Jika 401 Unauthorized -> Refresh Token & Retry Request
         ├── C. Transformasi Raw Punch Logs Berdasarkan "Matriks Aturan Kerja & Cutoff Mutlak"
         ├── D. Simpan hasil agregasi ke Cache (TTL 60 detik)
         ├── E. Render data ke Mazer HTML Template
         └── F. Kirim HTML utuh ke Browser Klien
```

---

## 3. Technology Stack
* **Backend & Server:** Golang (1.21+)
* **Web Framework:** Go Fiber v2 (`github.com/gofiber/fiber/v2`)
* **Template Engine:** Go HTML Template (`github.com/gofiber/template/html/v2`)
* **In-Memory Caching:** Go Fiber Cache middleware / In-memory TTL Map
* **HTTP Client:** `net/http` dengan Connection Pooling & Configurable Timeout
* **Frontend UI:** Mazer Admin Template (Mobile-first, HTML5, Vanilla JS, Bootstrap 5)
* **Visualisasi Data:** ApexCharts / Chart.js (Bawaan Mazer)
* **Tabel Interaktif:** Simple-DataTables (Bawaan Mazer)
* **Environment Config:** `github.com/joho/godotenv`

---

## 4. Tabel Matriks Aturan Kerja & Presensi Resmi (Acuan Dasar Sistem)

Berikut adalah aturan resmi jam kerja, kegiatan apel/senam, batas toleransi scan masuk, serta **Batas Akhir Mutlak (*Hard Cutoff*) Scan Apel Sore / Check-Out** yang diterima sistem:

| Parameter | Hari Senin – Kamis | Hari Jumat | Ketentuan & Validasi Logika Sistem |
| :--- | :--- | :--- | :--- |
| **Agenda Pagi** | **Apel Pagi (07:00 WIB)** | **Senam Pagi (06:30 WIB)** *(Tidak ada apel)* | Banner header menampilkan agenda harian |
| **Jam Masuk Resmi** | **07:00 WIB** | **07:00 WIB** | Jam dasar perhitungan menit keterlambatan |
| **Jendela Scan FR Masuk** | `06:15 – 07:15 WIB` | `06:15 – 07:15 WIB` | Mesin FR aktif merekam scan masuk |
| **Toleransi Keterlambatan** | **15 Menit** (s/d `07:15:00 WIB`) | **15 Menit** (s/d `07:15:00 WIB`) | Scan $\le$ `07:15:00` dihitung Hadir Apel Pagi |
| **Status: Apel Pagi Sah** | **`06:15:00 – 07:15:00 WIB`** | **`06:15:00 – 07:15:00 WIB`** | 🟢 **Hadir Apel Pagi (Badge Hijau)** |
| **Status: Terlambat** | **`> 07:15:00 WIB`** | **`> 07:15:00 WIB`** | 🟡 **Terlambat (Dihitung menit telat dari 07:00)** |
| **Jam Istirahat (*Break*)** | `12:00 – 13:00 WIB` (60 Mnt) | `11:30 – 13:00 WIB` / Auto Deduct | Waktu jeda operasional kantor |
| **Agenda Sore** | **Apel Sore (15:30 WIB)** | **Apel Sore / Pulang (16:00 WIB)** | Jadwal apel penutupan kerja |
| **Jam Buka Scan Pulang** | **15:30 WIB** | **16:00 WIB** | Batas awal diperbolehkan check-out |
| **Batas Akhir Scan (*Cutoff*)**| **`15:45:00 WIB`** *(Terakhir Diterima)*| **`16:15:00 WIB`** *(Terakhir Diterima)*| **Batas mutlak scan apel sore yang sah** |
| **Status: Apel Sore Sah**| **`15:30:00 – 15:45:00 WIB`** | **`16:00:00 – 16:15:00 WIB`** | 🔷 **Hadir Apel Sore (Badge Cyan)** |
| **Status: Pulang Cepat** | Scan sebelum `15:30:00 WIB` | Scan sebelum `16:00:00 WIB` | 🟠 **Pulang Cepat (*Early Leave*)** |
| **Status: Lewat Batas / Ditolak**| Scan setelah `15:45:00 WIB` | Scan setelah `16:15:00 WIB` | 🔴 **Tidak Diakui / Lewat Batas Apel Sore** |
| **Status: Belum Hadir / Alpha**| Tidak ada tap masuk | Tidak ada tap masuk | 🔴 **Belum Hadir / Tanpa Keterangan** |
| **Status: Dinas Luar / Cuti (Ekinerja)** | Surat tugas / izin terkonfirmasi | Surat tugas / izin terkonfirmasi | 🔵 **Kode ekinerja (CS/DL/CM/CB/CAP/CH/CLTN/CBA/TUBEL/DD)** |

---

## 5. Arsitektur Otomasi Presensi, SOP Fisik & Manajemen Dispensasi

Sistem mengadopsi pendekatan **100% Otomatisasi Mesin Biometrik** yang dipadukan dengan **SOP Fisik & Manajemen Dispensasi Khusus**:

### 5.1. Otomasi Penuh Mesin Face Recognition (Zero Manual Burden)
* Seluruh data kehadiran pegawai normal dihitung dan ditampilkan secara **100% otomatis** dari mesin ZKTeco SpeedFace M4 via API BioTime 8.5.
* Tidak ada kewajiban bagi petugas Provos/Kepegawaian untuk memverifikasi pegawai satu per satu setiap hari, sehingga sistem bekerja mandiri tanpa membebani aparatur di lapangan.

### 5.2. Solusi Fisik & Pengaturan Jadwal Mesin (Pencegahan "Tap & Run")
* **Koreksi Jendela Scan Sore:** Mesin Face Recognition diatur di BioTime **baru dibuka menerima scan kepulangan SETELAH apel sore bubar** (Senin–Kamis: `15:40 – 15:55 WIB`, Jumat: `16:10 – 16:25 WIB`). Pegawai wajib mengikuti apel hingga selesai karena sebelum apel bubar mesin belum menerima scan.
* **Penempatan Fisik Mesin:** Mesin ditempatkan di akses keluar-masuk lapangan upacara apel, bukan di dalam ruangan tertutup.

### 5.3. Manajemen Pengecualian & Dispensasi Kedinasan
* Petugas/Admin kepegawaian hanya bertindak menginput data **Pengecualian / Dispensasi Khusus** memakai kode selaras **ekinerja**:
  1. **CS - Cuti Sakit:** Pegawai berhalangan hadir dengan surat keterangan dokter.
  2. **DL - Dinas Luar Daerah:** Pegawai bertugas ke luar daerah dengan Surat Perintah Tugas (SPT).
  3. **CM - Cuti Bersalin:** Cuti melahirkan sesuai hak ASN.
  4. **CB - Cuti Besar:** Cuti besar sesuai hak ASN.
  5. **CAP - Cuti karena Alasan Penting:** Cuti untuk alasan penting yang sah.
  6. **CH - Cuti Tahunan:** Cuti tahunan sesuai hak ASN.
  7. **CLTN - Cuti Diluar Tanggungan Negara:** Cuti di luar tanggungan negara.
  8. **CBA - Cuti Bersama:** Cuti bersama nasional/pemerintah.
  9. **TUBEL - Tugas Belajar:** Pegawai dalam penugasan belajar resmi.
  10. **DD - Dinas Dalam Daerah:** Pegawai bertugas di dalam daerah dengan surat tugas.
  11. **Lainnya (Keterangan Lainnya):** Pengecualian lain yang disahkan petugas; alasan wajib ditulis di kolom catatan dan ikut dihitung sah.
  12. **Dispensasi Lupa Scan Apel Pagi / Sore:** (disembunyikan dari form; catatan lama tetap dibaca sistem).
* Seluruh kode di atas diakui sah oleh sistem (menggugurkan anomali, dihitung hadir pada KPI/kepatuhan). Kategori lama (`Dinas Luar`, `Cuti`, `Izin Sakit`) tetap dibaca untuk kompatibilitas catatan historis.

---

## 6. Functional Requirements (Fitur & Spesifikasi UI)

### 6.1. Smart Header & Context Banner (Day-Aware)
* **Senin – Kamis:** Banner menampilkan `"Senin-Kamis: Masuk 07:00 (Toleransi s/d 07:15) | Apel Pagi 07:00 | Scan Apel Sore: 15:30 s/d 15:45 WIB (Batas Akhir 15:45)"`.
* **Jumat:** Banner menampilkan `"Jumat: Senam Pagi 06:30 | Masuk Toleransi s/d 07:15 | Scan Apel Sore/Pulang: 16:00 s/d 16:15 WIB (Batas Akhir 16:15)"`.
* **Filter Periode / Tanggal:** Pilihan filter (Hari Ini, 7 Hari Terakhir, Bulan Ini, atau Tanggal Spesifik).
* **Auto-Refresh Toggle:** Interval pembaruan otomatis (Mati, 30 detik, 1 menit, 5 menit) untuk display TV monitor.

### 6.2. Metric Cards (Ringkasan Kehadiran)
1. **Total Pegawai Terjadwal:** Total seluruh pegawai terdaftar.
2. **Hadir Apel Pagi:** Jumlah & persentase pegawai scan $\le$ `07:15:00 WIB`.
3. **Terlambat:** Jumlah & persentase pegawai scan $>$ `07:15:00 WIB`.
4. **Hadir Apel Sore:** Pegawai scan pada rentang `15:30 - 15:45` (Senin-Kamis) atau `16:00 - 16:15` (Jumat).
5. **Pulang Cepat:** Jumlah pegawai scan sebelum `15:30` (Senin-Kamis) atau `16:00` (Jumat).
6. **Temuan Anomali (Kabur):** Jumlah pegawai yang ditandai tidak hadir di barisan fisik lapangan.
7. **Tidak Hadir Apel Sore / Lewat Batas:** Pegawai yang tidak scan atau scan setelah cutoff `15:45` (Sen-Kam) / `16:15` (Jum).
8. **Dinas Luar / Cuti:** Pegawai dengan status penugasan dinas luar atau izin resmi.
9. **Belum Hadir / Alpha:** Pegawai tanpa rekaman kehadiran pada hari berjalan.

### 6.3. Visualisasi Grafik (Charts)
* **Chart 1: Distribusi Status Kehadiran Hari Ini (Donut Chart)**
  * Proporsi: Tepat Waktu vs Terlambat vs Anomali Lapangan vs Dinas Luar/Cuti vs Alpha.
* **Chart 2: Kepatuhan Apel Sore (Gauge / Donut Chart)**
  * Persentase pegawai yang sah mengikuti apel sore sebelum batas cutoff.
* **Chart 3: Pola Jam Kedatangan / Arrival Heatmap (Bar Chart)**
  * Pengelompokan: `06:15-06:45`, `06:46-07:00`, `07:01-07:15` (Toleransi), `07:16-07:30`, `>07:30`.
* **Chart 4: Rekap Kepatuhan per Bidang / Seksi (Horizontal Bar Chart)**
  * Persentase kedisiplinan dan kepatuhan per bidang/departemen.

### 6.4. Tabel Data Rincian Presensi
* **Kolom Tabel:** NIP / ID (`emp_code`), Nama Pegawai, Bidang/Seksi, Waktu Scan Masuk, Waktu Scan Pulang, Status Pagi (Badge On-Time / Late / Anomali), Status Apel Sore (Badge Sah / Pulang Cepat / Lewat Batas), Verifikasi Lapangan, Keterangan.
* **Fitur Tabel:** Quick Filter Tabs (*Semua, Tepat Waktu, Terlambat, Anomali, Sah Apel Sore, Pulang Cepat, Lewat Batas Apel, Dinas Luar, Alpha*), instant search, dan pagination.

---

## 7. ZKTeco BioTime 8.5 API Specifications & Postman Guide

### 7.1. Modul 1: Autentikasi & JWT Token API
* **HTTP Method:** `POST`
* **URI:** `/jwt-api-token-auth/`
* **Base URL:** `http://{serverIP}:{serverPort}` (Contoh: `http://192.168.218.8:8090`)
* **Content-Type:** `application/json`
* **Request Body:**
  ```json
  {
    "username": "admin",
    "password": "your_password"
  }
  ```
* **Response Body (`200 OK`):**
  ```json
  {
    "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImFkbWluIiwiZXhwIjoxNzI0Mz..."
  }
  ```

---

### 7.2. Modul 2: Transaction API (Log Transaksi Presensi)
* **HTTP Method:** `GET`
* **URI:** `/iclock/api/transactions/`
* **Header Wajib:** 
  * `Content-Type`: `application/json`
  * `Authorization`: `JWT <token_hasil_login>`
* **Query Parameters:** `start_time`, `end_time`, `emp_code`, `terminal_sn`, `page`, `page_size`.

---

### 7.3. Logika Transformasi Data & Validasi Cutoff di BFF Golang
1. **Fetch & Grouping:** Mengambil seluruh transaksi hari ini dari `/iclock/api/transactions/` dan mengelompokkannya per `emp_code`.
2. **Validasi Masuk (Check-In):**
   * Masuk $\le$ `07:15:00` $\rightarrow$ **Tepat Waktu (On-Time)**.
   * Masuk $>$ `07:15:00` $\rightarrow$ **Terlambat (Late)** beserta selisih menit keterlambatan dari `07:00:00`.
3. **Validasi Apel Sore / Kepulangan (Strict Cutoff):**
   * **Senin – Kamis:**
     * Scan `< 15:30:00` $\rightarrow$ **Pulang Cepat (*Early Leave*)**.
     * Scan `15:30:00 – 15:45:00` $\rightarrow$ **Hadir Apel Sore & Pulang Tepat Waktu**.
     * Scan `> 15:45:00` $\rightarrow$ **Lewat Batas Scan Apel Sore / Tidak Diterima Sistem**.
   * **Jumat:**
     * Scan `< 16:00:00` $\rightarrow$ **Pulang Cepat (*Early Leave*)**.
     * Scan `16:00:00 – 16:15:00` $\rightarrow$ **Hadir Apel Sore & Pulang Tepat Waktu**.
     * Scan `> 16:15:00` $\rightarrow$ **Lewat Batas Scan Apel Sore / Tidak Diterima Sistem**.

---

## 8. Environment Configuration (`.env`)

```env
# Server Configuration
PORT=3000
APP_ENV=production          # development | production
APP_NAME="MAPEL"

# ZKTeco BioTime 8.5 API Credentials
BIOTIME_BASE_URL=http://192.168.218.8:8090
BIOTIME_USERNAME=admin
BIOTIME_PASSWORD=your_password_here
BIOTIME_TIMEOUT_SECONDS=5

# Official Attendance Schedule & Cutoff Limits
WORK_START_TIME="07:00:00"          # Jam masuk dasar
CHECKIN_TOLERANCE_TIME="07:15:00"   # Batas akhir toleransi tepat waktu (06:15 - 07:15)
CHECKIN_WINDOW_START="06:15:00"     # Jam buka scan FR pagi

CHECKOUT_TIME_MON_THU="15:30:00"    # Jam buka scan apel sore Senin - Kamis
CHECKOUT_CUTOFF_MON_THU="15:45:00"  # BATAS AKHIR MUTLAK scan apel sore Senin - Kamis

CHECKOUT_TIME_FRI="16:00:00"        # Jam buka scan apel sore Jumat
CHECKOUT_CUTOFF_FRI="16:15:00"      # BATAS AKHIR MUTLAK scan apel sore Jumat

# Caching Configuration
CACHE_TTL_SECONDS=60                # Durasi in-memory cache (detik)
```

---

## 9. Non-Functional Requirements (NFR)
1. **Performance:** 
   * Server Response Time (TTFB): < 100ms (Cache Hit) dan < 500ms (Cache Miss).
   * In-memory cache 60 detik untuk mencegah overload pada server BioTime.
2. **Security:**
   * Contextual Auto-Escaping dengan `html/template` Golang untuk mencegah XSS.
   * Security Headers: `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `X-XSS-Protection: 1; mode=block`.
3. **Resilience & Auto Recovery:**
   * Auto re-login jika token expired (menangkap kode 401 dan melakukan auth ulang secara transparan).
   * Fallback ke cache data terakhir jika server BioTime sempat mengalami timeout.
   * Halaman Custom 404 (Not Found) & 500 (Internal Server Error) berbasis template Mazer.
4. **Browser Compatibility:**
   * Kompatibel dengan semua browser modern (Chrome, Edge, Firefox, Safari) di perangkat Mobile, Tablet, Desktop, dan Smart TV.

---

## 10. Directory Structure Blueprint

```text
/dashboard-presensi
├── main.go                     # Inisialisasi Fiber, routing, cache middleware, & graceful shutdown
├── go.mod                      # Go module dependencies
├── go.sum
├── .env.example                # Template variabel lingkungan
├── .env                        # Konfigurasi aktif (di-ignore oleh git)
├── /config                     # Parser konfigurasi & schedule constants
│   └── config.go
├── /controllers                # Handler HTTP & SSR controller
│   └── dashboard_controller.go
├── /services                   # Logika konsumsi API BioTime & pengolahan data
│   ├── biotime_client.go       # HTTP Client, JWT Auto-Auth, & error interceptor
│   ├── attendance_service.go   # Pengolahan raw logs -> Matriks & Cutoff -> ViewModel
│   └── cache_service.go        # In-memory TTL Cache
├── /models                     # Struct definisi data JSON BioTime & ViewModel
│   ├── biotime.go              # Struct response login & transactions BioTime
│   └── dashboard_view.go       # Struct data untuk Mazer HTML Template
├── /public                     # File statis Mazer (CSS, JS, Fonts, Images)
│   └── assets/
│       ├── css/
│       ├── js/
│       ├── images/
│       └── vendors/
└── /views                      # Template Go HTML
    ├── index.html              # Halaman utama dashboard presensi
    ├── partials/               # Potongan komponen UI (header, cards, chart)
    │   ├── header.html
    │   ├── footer.html
    │   └── metric_cards.html
    └── errors/                 # Halaman error khusus Mazer
        ├── 404.html
        └── 500.html
```

---

## 11. Implementation Roadmap & Acceptance Criteria

### Tahapan Eksekusi:
* [x] **Phase 1: Environment & Project Setup**
  * Inisialisasi Go Module, instalasi dependency (Fiber, HTML template, godotenv).
  * Setup struktur folder dan asset Mazer di `public/assets`.
* [ ] **Phase 2: UI Slicing & Modular Templates**
  * Membagi template Mazer ke dalam `views/partials/` dan halaman `index.html`.
  * Menyiapkan slot chart ApexCharts (Donut, Kepatuhan Apel Sore, Arrival Heatmap) dan tabel interaktif dengan dukungan flag anomali.
* [ ] **Phase 3: Service Layer & BioTime API Integration**
  * Implementasi HTTP client dengan **Auto JWT Login** (`POST /jwt-api-token-auth/`) dan interceptor 401.
  * Implementasi fetch log transaksi (`GET /iclock/api/transactions/`).
  * Implementasi logika validasi **Strict Cutoff Apel Sore** dan deteksi status presensi.
  * Implementasi in-memory cache dengan TTL (60 detik).
  * Mock data provider untuk pengujian tanpa server BioTime live.
* [ ] **Phase 4: Data Binding & SSR Rendering**
  * Menghubungkan controller dengan service layer.
  * Passing data metrik, data chart (JSON serialized), dan daftar pegawai ke template.
* [ ] **Phase 5: Error Handling, Polish & Testing**
  * Implementasi error handler 404/500 dan fallback state saat BioTime offline.
  * Uji responsivitas pada layar mobile, desktop, dan smart TV.
  * Optimasi build binary (`go build -ldflags="-s -w"`).

### Kriteria Penerimaan (Acceptance Criteria):
1. **Validasi Cutoff Apel Sore Mutlak:** Scan hanya sah diterima pada rentang `15:30 – 15:45` (Senin-Kamis) dan `16:00 – 16:15` (Jumat).
2. Scan masuk $\le$ `07:15:00 WIB` terhitung **Tepat Waktu**, dan scan $>$ `07:15:00 WIB` terhitung **Terlambat**.
3. Sistem mengakomodasi penandaan **Anomali Lapangan (Tap Tanpa Hadir Apel)**.
4. BFF backend dapat login secara otomatis ke BioTime via JWT dan me-refresh token saat expired.
5. Tampilan UI Mazer terlihat rapi, modern, cepat, serta responsif di berbagai perangkat.

---

## 12. Standarisasi Status Presensi & Apel Resmi (Format A - Dual-Badge Matrix)

Standarisasi resmi status presensi dan apel yang disepakati untuk Dashboard Utama, Presensi Hari Ini, dan Laporan Rekapitulasi:

### 12.1. Tampilan Dashboard Utama (`/` - Log Presensi Face Recognition)
Kolom **Status Presensi & Apel** menggunakan pendekatan **Format A (Modular Dual-Badge)** sehingga pimpinan/Kepala Dinas dapat memantau kehadiran apel secara instan:

| Kondisi Presensi Riil | Status di Dashboard Utama | Warna & Gaya Badge | Makna Kedinasan |
| :--- | :---: | :---: | :--- |
| **Pagi: Scan $\le$ 07:15:00 WIB** | **`Hadir Apel Pagi`** | `badge bg-success` | Otomatis dari mesin: ASN sah mengikuti apel pagi. |
| **Pagi: Scan $>$ 07:15:00 WIB** | **`Terlambat (+Xm)`** | `badge bg-warning text-dark` | Otomatis dari mesin: terlambat dan tidak ikut apel pagi. |
| **Pagi & Sore: Tidak scan** | **`Tidak Hadir`** | `badge bg-danger text-white` | Tidak ada rekaman kehadiran sama sekali (Alpha). |
| **Pagi belum scan (tapi sore ada)** | **`Belum Scan Pagi`** | `badge bg-secondary` | Hanya hadir sore hari, pagi tidak ada scan. |
| **Sore: Scan di jam apel (15:30-15:45 / 16:00-16:15)** | **`Hadir Apel Sore`** | `badge bg-info text-dark ms-1` | Badge kedua muncul berdampingan di samping status pagi. |
| **Sore: Scan sebelum jam buka apel** | **`Pulang Cepat`** | `badge bg-light-warning text-warning ms-1` | Scan mendahului jam apel kepulangan resmi. |
| **Sore: Scan lewat batas cutoff mutlak** | **`Lewat Batas Waktu`** | `badge bg-danger ms-1` | Scan setelah batas cutoff apel sore (Ditolak sistem). |
| **Temuan Khusus / Ketidaksesuaian** | **`Temuan`** | `badge bg-danger ms-1` | Terdapat pelanggaran disiplin/anomali. |

*Tampilan visual di mata Kepala Dinas:*
* Pagi hari: `[Hadir Apel Pagi]` 🟢
* Sore hari (lengkap): `[Hadir Apel Pagi]` 🟢 `[Hadir Apel Sore]` 🔷
* Pagi terlambat tapi apel sore hadir: `[Terlambat (+10m)]` 🟡 `[Hadir Apel Sore]` 🔷

### 12.2. Tampilan Menu Presensi Hari Ini (`/presensi`)
1. **Kolom Status Masuk:**
   * `Hadir Apel Pagi` 🟢 (Scan masuk $\le$ 07:15:00 WIB)
   * `Terlambat (+Xm)` 🟡 (Scan masuk $>$ 07:15:00 WIB)
   * `Belum Scan` ⚪ (Belum ada rekaman pagi)
2. **Kolom Status Apel Sore:**
   * `Hadir Apel Sore` 🔷 (Scan di jendela apel sore resmi)
   * `Pulang Cepat` 🟡 (Scan sebelum jam apel sore)
   * `Lewat Batas Waktu` 🔴 (Scan lewat batas cutoff)
   * `Belum Scan` ⚪ (Belum ada rekaman sore)
3. **Kolom Status Verifikasi (Administrasi Akhir):**
   * `Terverifikasi Otomatis` 🟢 (Scan pagi & sore lengkap dan sah tanpa anomali)
   * `Hadir Pagi` 🔵 (Baru scan masuk pagi, jam kerja masih berlangsung)
   * `Hanya Apel Sore` 🔷 (Hanya ada rekaman apel sore)
   * `Dispensasi Resmi` 🔵 (Memiliki surat penugasan resmi: kode ekinerja CS/DL/CM/CB/CAP/CH/CLTN/CBA/TUBEL/DD, atau Lupa Scan)
   * `Perlu Konfirmasi` 🟠 (Terdapat anomali mesin yang membutuhkan klarifikasi)
   * `Tidak Hadir` 🔴 (Pagi dan sore kosong tanpa keterangan)

---

## 13. Standar Rumus & Logika Agregasi Tingkat Kepatuhan ASN

### 13.1. Landasan & Kebijakan 2 Sesi Apel Harian
Dalam tata kelola kedinasan pemerintah (khususnya *Dinas PUPR Kabupaten Barito Utara*), kehadiran kerja harian dibagi menjadi dua instrumen pengawasan utama:
1. **Presensi Pagi / Jam Masuk Kerja:**
   - **Senin s.d. Kamis (Apel Pagi):** Pukul 06:15 – 07:15 WIB (Maksimal toleransi apel s/d 07:15:00 WIB).
   - **Jumat (Presensi Pagi & Senam):** Pukul 06:15 – 07:15 WIB.
     > **Aturan Khusus Jumat Pagi:** Pada hari Jumat terdapat agenda senam/olahraga bersama. Keikutsertaan senam **TIDAK berpengaruh** terhadap penilaian sistem. Indikator kepatuhan murni diukur dari **keberhasilan Scan Pagi di mesin**. Pegawai bebas memilih ikut atau tidak ikut senam; yang wajib adalah melakukan scan pagi.
2. **Apel Sore / Jam Pulang Kerja:**
   - **Senin s.d. Kamis:** Pukul 15:30 – 15:45 WIB.
   - **Jumat:** Pukul 16:00 – 16:15 WIB (atau sesuai jam kepulangan dinas).
     > **Aturan Khusus Jumat Sore:** Apel Sore pada hari Jumat adalah **WAJIB dan MUTLAK dihitung**. Pegawai yang hanya scan pagi tetapi tidak mengikuti/tidak scan apel sore di hari Jumat tetap dihitung tidak patuh pada sesi kedua (mengurangi persentase kepatuhan).

Setiap ASN yang aktif bekerja diwajibkan memenuhi **2 sesi pencatatan per hari kerja** (Pagi dan Sore).

---

### 13.2. Studi Kasus Harian (Kamis, 10 September 2026)

#### Data Lapangan:
* **Jumlah Pegawai Aktif:** 50 Pegawai
* **Total Transaksi Mesin (Raw Log):** 90 log scan sah
  * **Absen Pagi (Scan Masuk):** 50 Pegawai (100% hadir apel pagi)
  * **Absen Sore (Scan Pulang):** 40 Pegawai (10 orang tidak melakukan scan sore)

#### Perhitungan & Analisa:
1. **Target Kuota Sesi Wajib Instansi:**
   $$\text{Target Sesi} = 50 \text{ Pegawai} \times 2 \text{ Sesi} = 100 \text{ Sesi}$$
2. **Realisasi Sesi Terpenuhi:**
   $$\text{Realisasi Sesi} = 50 \text{ (Pagi)} + 40 \text{ (Sore)} = 90 \text{ Sesi}$$
3. **Tingkat Kepatuhan Apel Harian:**
   $$\text{Tingkat Kepatuhan Harian} = \frac{90}{100} \times 100\% = \mathbf{90\%}$$

#### Evaluasi Kasus:
* Angka **90%** mencerminkan kondisi lapangan yang sesungguhnya.
* Pengurang sebesar **10%** berasal dari **10 pegawai yang tidak mengikuti atau tidak melakukan scan apel sore** (indikasi: pulang mendahului waktu, tidak ikut barisan sore, atau lupa scan biometrik).

---

### 13.3. Analisa Berjenjang (Hierarki Periode)

#### A. Skala Mingguan (5 Hari Kerja: Senin s.d. Jumat)
Misalkan dalam 1 minggu terdapat 5 hari kerja efektif:
* **Target Kuota 1 Minggu (50 Pegawai):**
  $$50 \text{ Pegawai} \times 5 \text{ Hari} \times 2 \text{ Sesi} = \mathbf{500 \text{ Sesi Apel}}$$

* **Contoh Kasus Realisasi:**
  * Total Scan Pagi terkumpul = 240 sesi (rata-rata 48 orang/hari)
  * Total Scan Sore terkumpul = 200 sesi (rata-rata 40 orang/hari)
  * **Total Realisasi:** $240 + 200 = 440 \text{ Sesi}$

$$\text{Tingkat Kepatuhan Mingguan} = \frac{440}{500} \times 100\% = \mathbf{88\%}$$

---

#### B. Skala Bulanan (Contoh: 22 Hari Kerja Efektif)
Jumlah hari kerja bulanan dihitung hanya hari kerja aktif (Senin–Jumat di luar tanggal merah dan cuti bersama). Misal terdapat **22 hari kerja**:
* **Target Kuota 1 Bulan (50 Pegawai):**
  $$50 \text{ Pegawai} \times 22 \text{ Hari} \times 2 \text{ Sesi} = \mathbf{2.200 \text{ Sesi Apel}}$$

* **Contoh Kasus Realisasi:**
  * Dari target 2.200 sesi, total sesi sah yang terlaksana adalah **1.870 sesi**.

$$\text{Tingkat Kepatuhan Bulanan} = \frac{1.870}{2.200} \times 100\% = \mathbf{85\%}$$

---

#### C. Skala Tahunan (Rata-rata 250 Hari Kerja Efektif)
Dalam 1 tahun kalender dinas, rata-rata terdapat **250 hari kerja efektif**:
* **Target Kuota 1 Tahun (50 Pegawai):**
  $$50 \text{ Pegawai} \times 250 \text{ Hari} \times 2 \text{ Sesi} = \mathbf{25.000 \text{ Sesi Apel}}$$

* **Contoh Kasus Realisasi:**
  * Total akumulasi log sah sepanjang tahun = **21.500 sesi**.

$$\text{Tingkat Kepatuhan Tahunan} = \frac{21.500}{25.000} \times 100\% = \mathbf{86\%}$$

---

### 13.4. Rumus Baku Universal (Kedinasan)

Secara matematis, rumus baku untuk periode apapun ($T$) dinyatakan sebagai:

$$\mathbf{\text{Tingkat Kepatuhan Apel}} = \frac{\sum \text{Sesi Hadir Apel Pagi (Sah)} + \sum \text{Sesi Hadir Apel Sore (Sah)}}{\text{Jumlah Pegawai Aktif} \times \text{Jumlah Hari Kerja Efektif} \times 2} \times 100\%$$

---

### 13.5. Tata Kelola Pengecualian (*Dispensasi & Integrasi e-Kinerja*)

Dalam sistem pemerintahan, tidak semua ketidakhadiran di mesin merupakan pelanggaran disiplin. Mekanisme perlakuan terhadap status khusus dan kode dispensasi e-Kinerja (lihat juga Seksi 5.3) dalam rumus kepatuhan adalah sebagai berikut:

| Status Kehadiran / Kode Dispensasi | Perlakuan dalam Rumus | Keterangan & Dasar Kebijakan |
| :--- | :---: | :--- |
| **Hadir Scan Mesin** | Diakui Sah ($+1$ Sesi) | Sesuai rekaman biometrik BioTime pada jendela waktu resmi |
| **Dinas Luar (DL / DD)** | Diakui Sah ($+1$ Sesi) | Disahkan via Surat Perintah Tugas (SPT) resmi |
| **Cuti Resmi (CH/CB/CAP/CBA/CM/TUBEL)** | Diakui Sah / Dikecualikan | Hak cuti ASN sesuai regulasi BKD (tidak mengurangi hak kepatuhan) |
| **Cuti Sakit (CS)** | Diakui Sah ($+1$ Sesi) | Dilengkapi surat keterangan dokter yang sah |
| **Disahkan Barisan** | Diakui Sah ($+1$ Sesi) | Hadir fisik di barisan lapangan apel namun gagal scan mesin |
| **Tanpa Keterangan (Alpha/TK)** | Dihitung $0$ | Mengurangi persentase kepatuhan instansi |

*Dokumen ini disusun sebagai acuan teknis dan fungsional MAPEL (Monitoring Apel) Dinas Pekerjaan Umum dan Penataan Ruang Kabupaten Barito Utara.*
