# Rencana Sentralisasi BioTime + Monitoring Apel (MAPEL) di Server Kominfo

> Dokumen teknis untuk Dinas Kominfo — 1 server sentral melayani device face recognition di dinas-dinas.
> Versi: 1.0 — 9 Okt 2026

---

## 1. Ringkasan Eksekutif

- **Server Kominfo** menghosting 2 aplikasi: **ZKTeco BioTime 9.5** (sumber data) + **Monitoring Apel / MAPEL-ASN** (viewer).
- **Di dinas hanya ada device** (rencana awal: 4 unit SpeedFace M4 untuk 1 dinas). Tidak perlu PC/server di dinas.
- Pola ini **memungkinkan dan direkomendasikan**: maintenance terpusat, backup satu pintu, penambahan dinas baru tinggal tambah device + area.
- MAPEL **tidak konek langsung ke device**, hanya baca API BioTime via `BIOTIME_BASE_URL`. Jadi MAPEL tetap `http://127.0.0.1:8081` di server Kominfo, tidak berubah walau device tersebar.

```
[Device 1-4 di Dinas] -- push ADMS HTTP :8081 (VPN/Internet) -->
  [Server Kominfo: BioTime :8081 + MAPEL :3000 + DB]
    --> [Browser/Pimpinan/TV Lobby: buka http://SERVER:3000]
```

---

## 2. Topologi & Alur Data

### 2.1. Arah koneksi (penting)

| Jalur | Arah | Protokol | Keterangan |
|---|---|---|---|
| Device → BioTime | keluar dari dinas ke Kominfo | HTTP `GET/POST /iclock/cdata` tiap ±15 detik, port `8081` | Mode **ADMS push**. Bukan pull `4370`. Firewall dinas harus allow outbound ke server. |
| MAPEL → BioTime | lokal di server | `POST /jwt-api-token-auth/` + `GET /iclock/api/transactions/` | JWT auto-refresh. Cache 60 detik + snapshot disk 24 jam. |
| User → MAPEL | browser ke server | HTTP `:3000` | Untuk pimpinan / TV lobby. Bisa reverse-proxy / domain bila perlu. |

### 2.2. Port di server Kominfo yang dipakai

| Port | Layanan | Akses dari |
|---|---|---|
| `8081` | BioTime httpd (pintu masuk device) | Device di dinas (wajib dibuka + whitelist IP dinas) |
| `3000` | MAPEL-ASN (`APP_PORT`) | Browser internal / TV |
| `3306` | MariaDB dashboard | Lokal saja |
| `7496` | PostgreSQL internal BioTime | Lokal saja (audit read-only) |
| `7497` | Django internal BioTime | Lokal saja |

---

## 3. Kebutuhan Jaringan (Kominfo ↔ Dinas)

Device harus bisa me-reach IP server Kominfo. Pilih salah satu, urut prioritas:

1. **VPN / intranet Pemda (disarankan)** — device pakai IP intranet server, paling aman, tanpa expose publik.
2. **IP publik + NAT port-forward `8081`** ke server BioTime — wajib firewall whitelist IP dinas.
3. **Tunnel (WireGuard / Tailscale / ZeroTier)** — bila tidak ada IP publik / intranet belum siap.

Kebutuhan minimum per device sangat kecil (payload tap < 5 KB, polling tiap 15 detik). Latency < 500 ms masih aman. Yang kritis adalah **stabilitas**, bukan bandwidth.

> Jangan expose port `8081` polos ke internet tanpa whitelist. Yang dibuka hanya dari IP dinas.

---

## 4. Kebutuhan Server Kominfo

- OS: Windows (deployment eksisting native, bukan Docker).
- Instalasi: `C:\ZKBioTime` (BioTime 9.5) + folder `mapel-asn` berisi `MAPEL-ASN.exe` + `.env` + `data/`.
- Service `bio-*` (httpd, pgsql, redis, celery `iclock_data_upload`, dll.) = **Automatic**.
- MAPEL auto-start via Task Scheduler (nama task: `MAPEL-ASN`): trigger AtStartup delay 1 menit, user `SYSTEM`, restart 3x tiap 1 menit. Buka inbound TCP `3000` bila diakses lintas PC.
- Backup berkala: folder `data/` (users, audit, snapshots) + database MariaDB + database BioTime (`biotime` di `127.0.0.1:7496`).
- File kredensial: `.env` (jangan disebar) + `C:\ZKBioTime\attsite.ini` bagian `[DATABASE] PASSWORD=` untuk audit read-only.

---

## 5. Setup 4 Device di 1 Dinas

### 5.1. Penempatan fisik

- Semua 4 device = **bidirectional (masuk + pulang)**. MAPEL menentukan pagi/sore dari jam (batas tengah hari `12:00`), bukan dari device.
- Rekomendasi: 2 di jalur masuk / lapangan apel, 2 di jalur keluar / bubaran apel sore. Di akses terbuka lapangan upacara, bukan ruangan tertutup (anti "tap & run" sesuai SOP apel).
- Sediakan UPS + kabel LAN (lebih stabil dari WiFi). Bila WiFi, kunci via DHCP reservation.

### 5.2. Konfigurasi jaringan per device

| Device | Nama saran | IP statis saran |
|---|---|---|
| 1 | `DINASX-MASUK-01` | `192.168.10.11` |
| 2 | `DINASX-MASUK-02` | `192.168.10.12` |
| 3 | `DINASX-KELUAR-01` | `192.168.10.13` |
| 4 | `DINASX-KELUAR-02` | `192.168.10.14` |

- IP statis se-subnet LAN dinas, gateway/DNS benar agar bisa keluar ke Kominfo.
- Catat SN tiap unit (contoh: `GAT5261100070`) untuk tracing di log.

### 5.3. Konfigurasi ADMS (sama di 4 unit)

1. `COMM → Cloud Server Settings`: Mode `ADMS`, Domain OFF.
2. Server Address = **IP/domain server Kominfo**, Server Port = **`8081`**.
3. `Serial Comm (No Using)`, `Ethernet (0.0.0.0)`, `HTTPS off` = normal bila pakai WiFi. Port `4370` lokal tidak dipakai mode push ini — jangan dikejar.
4. **Sinkron jam (NTP) wajib sama detik** di 4 unit (arahkan ke server Kominfo). Beda 2 menit saja merusak cutoff `07:15:00` / `15:45:00`.
5. Reboot tiap unit setelah ubah setting, tunggu 2–3 menit.

---

## 6. Setup BioTime di Server Kominfo (per Dinas)

1. **Daftarkan 4 SN** di menu Device. Pastikan kolom State hijau + `Last Activity` menitan. Bila `Cmd` menumpuk → `Clear Pending Command`.
2. **Wajib keluar dari Default Area.** BioTime memblokir transfer untuk device di area default (*"Default area device the data will not transfer"*). Buat 1 Area baru, contoh `Dinas X`, masukkan 4 terminal ke situ.
3. **Assign pegawai** dinas tersebut ke Area `Dinas X`.
4. **Sinkron wajah 1x untuk 4 device:** enroll sekali (via BioTime / salah satu device), lalu `Personnel → Transfer to Device` ke 4 SN. Tanpa ini, wajah yang daftar di device 1 tidak dikenali device lain.
5. **Tambah dinas baru = ulangi langkah 2–4** dengan Area baru. Tidak perlu install server lagi.

---

## 7. Cara Kerja MAPEL (tidak perlu diubah)

- Ambil seluruh transaksi hari itu via `GET /iclock/api/transactions/` (pagination otomatis, 1000/page).
- Engine (`ProcessDailyAttendance`): group per `emp_code` → sort kronologis → ambil scan pagi paling awal + scan sore paling awal yang sah di jendela (`15:30–15:45` Sen–Kam, `16:00–16:15` Jum).
- Efeknya: **tap ganda di device berbeda tidak double-count**, tetap 1 sesi pagi + 1 sesi sore.
- Cache 60 detik (`CACHE_TTL_SECONDS`), fallback snapshot disk 24 jam (`data/snapshots/`) saat BioTime/device offline — TV tetap tampil data terakhir + banner offline, bukan error.
- Master pegawai yang nol tap tetap muncul sebagai `Belum Scan` (Alpha) dari join master BioTime.

---

## 8. SOP Saat Internet Dinas Putus

- Tap tertahan di buffer device, otomatis ter-upload saat online (`POST /iclock/cdata` jalan lagi).
- Pemicu manual dari Kominfo: `POST /iclock/api/terminals/upload_transaction/` body `{"terminals":[<id>]}`.
- MAPEL tetap tayang dari snapshot maks 24 jam (`SNAPSHOT_MAX_AGE_HOURS`).

---

## 9. Checklist Serah Terima (per Dinas, 4 Device)

- [ ] 4 SN hijau di BioTime → Device, `Last Activity` menitan, bukan Default Area.
- [ ] `C:\ZKBioTime\apache\logs\access.log` ada `GET /iclock/cdata` tiap ±15 detik + `POST /iclock/cdata` dari 4 SN.
- [ ] `GET /iclock/api/transactions/` (filter hari ini) muncul tap dengan 4 `terminal_alias` berbeda.
- [ ] `GET /iclock/api/terminals/` state OK.
- [ ] Tes scan 1 wajah di tiap device → dashboard tampil 1 baris (bukan 4), jam 4 device sama detik.
- [ ] Transfer template wajah ke 4 device terverifikasi (tes wajah yang sama di 4 unit).
- [ ] `http://SERVER:3000/health` OK setelah reboot server.
- [ ] Backup `data/` + kredensial `.env` tersimpan aman.

---

## 10. Pembagian Tanggung Jawab Usulan

| Item | Kominfo | Dinas |
|---|---|---|
| Server, BioTime, dashboard, backup, firewall, VPN | ✅ | — |
| Area + device + pegawai + sync wajah di BioTime | ✅ | — |
| Listrik/UPS, LAN/WiFi, fisik device, kebersihan sensor | — | ✅ |
| Ikut apel + tap wajah tertib (SOP apel pagi/sore) | — | ✅ |
| Dispensasi (DL/cuti/sakit via kode ekinerja) | — | ✅ (admin kepegawaian) |

---

## Lampiran: Contoh `.env` di Server (tidak berubah)

```env
APP_PORT=3000
BIOTIME_BASE_URL=http://127.0.0.1:8081
BIOTIME_USERNAME=sysdba
BIOTIME_TIMEOUT_SECONDS=5
CACHE_TTL_SECONDS=60
SNAPSHOT_DIR=./data/snapshots
SNAPSHOT_MAX_AGE_HOURS=24
```

> Siap untuk di-share ke Kominfo. Detail implementasi dashboard mengacu ke `docs/PRD.MD` dan operasional harian ke `docs/Guide.MD` di repo ini.
