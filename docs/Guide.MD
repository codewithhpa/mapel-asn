# Guide — Operasional Monitoring Apel / MAPEL (Windows Native)

> Cara menjalankan, auto-start sehabis reboot, verifikasi, dan jebakan umum.
> Deploy project ini = **native Windows** (`MAPEL-ASN.exe`), bukan Docker.

---

## 0. Setup Mesin & Troubleshooting (BACA DULU — sumber data)

Dashboard hanya menampilkan apa yang ada di server BioTime. Kalau tap tidak
muncul di dashboard, 99% masalahnya di jalur **mesin → server**, bukan di
dashboard. Fakta di bawah ini hasil bedah langsung instalasi ini (BioTime 9.5
di `C:\ZKBioTime`, SpeedFace M4 SN `GAT5261100070`).

### 0.1. Setup SpeedFace M4 yang terbukti jalan

- IP mesin **statis** se-subnet dengan PC (`192.168.1.9` vs PC `192.168.1.8`).
  Kunci via static di mesin atau DHCP reservation di router.
- COMM → **Cloud Server Settings**: Mode `ADMS`, Domain OFF,
  Server Address = **IP LAN PC BioTime**, Server Port = **`8081`**.
  > Push BioTime 9.5 ini lewat **HTTP port 8081** (`GET/POST /iclock/cdata`
  > tiap ~15 detik) — **BUKAN 4370**. Jangan kejar port 4370.
- Layar `Serial Comm (No Using)`, `Ethernet (0.0.0.0)`, `HTTPS off` = normal
  bila mesin pakai WiFi. `PC Connection → TCP COMM Port 4370` = port pull
  lokal mesin, tidak dipakai mode push ini.
- **WAJIB: keluarkan mesin dari Default Area!** BioTime memblokir transfer
  untuk device di area default (peringatan kuning di kolom State:
  *"Default area device the data will not transfer"*). Pindahkan via aksi
  area di halaman Device (atau buat baru via **New Area**).
- Setelah ubah setting: **reboot mesin**, tunggu 2–3 menit, scan 1 wajah tes.

### 0.2. Kalau tap tidak muncul — urutan cek (5 menit)

1. **BioTime web → Device**: State hijau? `Last Activity` baru (menitan)?
   Bukan di Default Area? `Cmd` menumpuk = perintah basi → **Clear Pending
   Command**.
2. **Log push**: `C:\ZKBioTime\apache\logs\access.log` — cari `SN` mesin:
   - Ada `GET /iclock/cdata` tiap ~15 dtk = mesin hidup & polling ✅
   - Ada `POST /iclock/cdata` = payload tap dikirim ✅ (tidak ada = tidak ada
     yang diupload: verifikasi gagal di mesin / menunggu jadwal transfer)
3. **API**: `GET /iclock/api/transactions/` (filter tanggal hari ini) dan
   `GET /iclock/api/terminals/` (`state`, `last_activity`).
4. **Login API 401** → password berubah: samakan `BIOTIME_PASSWORD` di `.env`
   lalu restart dashboard (token lama di memori mati begitu kedaluwarsa).
5. **Dashboard tampil 4 baris Belum Scan** = normal saat transaksi hari ini 0
   (baris Alpha dari master). Bukan error.
6. **Tap siang (≥ 12:00)** = sesi **sore** → tampil `Belum Scan Pagi +
   Pulang Cepat`, bukan check-in pagi (aturan batas tengah hari di engine).
7. **Picu upload jarak jauh**: `POST /iclock/api/terminals/upload_transaction/`
   body `{"terminals":[<id>]}` (id dari daftar terminals).

### 0.3. Peta arsitektur (catatan lapangan)

- `PC (192.168.1.8)`: `MAPEL-ASN.exe :3000` + MariaDB `:3306` + BioTime
  (`httpd :8081` → Django `:7497`, pgsql `:7496`, redis, celery workers
  `iclock_data_upload`, dsb. — services `bio-*`, semua Automatic).
- `Mesin (192.168.1.9, WiFi)`: push HTTP → `192.168.1.8:8081`.
- Dashboard baca BioTime via JWT (`POST /jwt-api-token-auth/`), cache 60 detik
  + snapshot disk 24 jam (`data/snapshots/`).

### 0.4. Akses database BioTime (Postgres, read-only untuk audit)

- Host `127.0.0.1`, port **`7496`** (bukan 5432), database `biotime`, user `postgres`.
- Tabel utama: `personnel_employee` (master ASN + FK department/position),
  `personnel_resign`, `iclock_transaction`, `iclock_terminal`.
- **Password TIDAK ditulis di sini** — file ini ikut ke-push ke repo.
  Ambil dari server langsung: `C:\ZKBioTime\attsite.ini`, bagian `[DATABASE]`,
  baris `PASSWORD=`.
- Contoh cek cepat (PowerShell, tanpa mengetik password):
  ```powershell
  $env:PGPASSWORD = (Select-String -Path C:\ZKBioTime\attsite.ini -Pattern '^PASSWORD=' |
    Select-Object -First 1).Line.Substring(9)
  & "C:\ZKBioTime\pgsql\bin\psql.exe" -h 127.0.0.1 -p 7496 -U postgres -d biotime `
    -c "SELECT count(*) FROM personnel_employee;"
  ```
- TablePlus/DBeaver/pgAdmin: host `127.0.0.1`, port `7496`, user `postgres`,
  database `biotime`, SSL off, password di-paste dari file di atas.
- Peringatan: ini kredensial backend internal BioTime (tulis = bisa merusak
  sinkronisasi). Untuk audit cukup hak baca; jangan ubah data langsung.

---

## 1. Menjalankan manual

```powershell
cd "C:\Users\HPAiD\Downloads\mapel-asn"
go run ./cmd/app
# atau binary produksi:
go build -o MAPEL-ASN.exe ./cmd/app
.\MAPEL-ASN.exe
```

Buka: `http://localhost:3000` (port ikut `APP_PORT` di `.env`).
Login default: `admin` / `adminkey1234` — **wajib diganti sebelum live**.

Mode TV / display lobby (tanpa sidebar & menu, selalu data hari ini):
- `http://localhost:3000/tv` — dashboard (jam besar, hero % kepatuhan, tren, tabel)
- `http://localhost:3000/tv/presensi` — tabel presensi fullscreen
- Auto-refresh default 1 menit (pilihan tersimpan per browser). Bookmark salah
  satu URL di browser TV. Saat BioTime mati, TV menampilkan banner offline di
  atas data terakhir, bukan halaman error.
- Jalan pintas operator: menu sidebar **Mode TV** (tampil setelah login,
  membuka `/tv` di tab baru) untuk setup pertama kali display lobby.

Catatan: sejak v1.x, `MAPEL-ASN.exe` otomatis pindah working directory ke foldernya
sendiri saat start (`main.go`), jadi aman dijalankan dari folder mana pun.

### 1a. Rebuild tiap ada revisi kode

Urutan wajib (jangan dibalik — exe yang sedang jalan dikunci Windows):

```powershell
cd "C:\Users\HPAiD\Downloads\mapel-asn"
taskkill /F /IM MAPEL-ASN.exe 2>$null
go build -o MAPEL-ASN.exe ./cmd/app
.\MAPEL-ASN.exe
```

Ikon tidak perlu diurus lagi (`cmd/app/resource.syso` otomatis kebawa tiap
build; sumbernya `logo.ico`). Kalau jalan via Task Scheduler: hentikan
task-nya dulu sebelum rebuild.

---

## 2. Auto-start sehabis reboot (Task Scheduler — rekomendasi)

Jalankan **PowerShell as Administrator**:

```powershell
$dir = "C:\Users\HPAiD\Downloads\mapel-asn"
go build -o "$dir\MAPEL-ASN.exe" ./cmd/app   # jalankan dari folder project

$action = New-ScheduledTaskAction -Execute "$dir\MAPEL-ASN.exe" -WorkingDirectory $dir
$trigger = New-ScheduledTaskTrigger -AtStartup
$trigger.Delay = "PT1M"   # jeda 1 menit: beri napas untuk MariaDB & network
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 `
  -RestartInterval (New-TimeSpan -Minutes 1) -StartWhenAvailable
Register-ScheduledTask -TaskName "MAPEL-ASN" `
  -Action $action -Trigger $trigger -Settings $settings `
  -User "SYSTEM" -RunLevel Highest -Force
```

Artinya: tiap PC menyala → tunggu 1 menit → jalankan `MAPEL-ASN.exe` sebagai
SYSTEM (tidak butuh user login — cocok untuk display TV/lobby) → kalau crash,
coba lagi 3x tiap 1 menit.

> `schtasks.exe` bawaan **tidak punya** opsi working directory — karena itu
> pakai `Register-ScheduledTask` (PowerShell) seperti di atas, atau bungkus
> `.bat` yang `cd /d` dulu ke folder project.

### Alternatif: Windows Service (NSSM)

`sc.exe` tidak bisa langsung (exe Go bukan service-aware). Pakai NSSM:

1. `nssm install "MAPEL-ASN"`
2. Path: `...\MAPEL-ASN.exe`, **Startup directory: folder project (wajib!)**
3. Startup type: Automatic (Delayed Start)

---

## 3. Verifikasi

```powershell
# Tanpa reboot: jalankan task manual lalu cek health
Start-ScheduledTask -TaskName "MAPEL-ASN"
Start-Sleep -Seconds 5
Invoke-WebRequest http://localhost:3000/health -UseBasicParsing |
  Select-Object StatusCode, Content

# Pastikan tidak ada instance ganda (port bentrok)
Get-NetTCPConnection -LocalPort 3000 -State Listen

# Uji final: reboot PC, lalu cek lagi /health
```

---

## 4. Jebakan umum

| Gejala | Penyebab | Solusi |
|---|---|---|
| `template not found` / `.env not found` | Working directory salah | Wajib set WorkingDirectory/“Start in” ke folder project (kode sudah self-healing, tapi scheduler tetap diset benar) |
| Port 3000 bentrok | App lama masih hidup | `taskkill /F /IM MAPEL-ASN.exe` dulu sebelum tes |
| Dashboard 500 / lambat ~5 dtk | BioTime `:8081` mati/lemot | Cek mesin & network; turunkan `BIOTIME_TIMEOUT_SECONDS` ke `2`–`3`; snapshot `data/snapshots/` menutupinya maks 24 jam (`SNAPSHOT_MAX_AGE_HOURS`) |
| PC lain tidak bisa buka | Windows Firewall | Allow saat prompt pertama, atau buka inbound TCP 3000 |
| Data kosong setelah deploy | Cache & snapshot masih dingin | Tunggu 1 fetch sukses; snapshot tersimpan otomatis di `data/snapshots/` |

---

## 5. File & folder penting saat operasional

- `MAPEL-ASN.exe` — binary produksi (hasil `go build`, di-ignore git).
- `.env` — kredensial + port + timeout + snapshot (di-ignore git, **jangan disebar**).
- `data/` — `users.json`, `audit_trail.json`, `snapshots/` (fallback saat BioTime mati). **Backup folder ini berkala.**
- `docs/PRD.MD` — acuan aturan & fitur.
