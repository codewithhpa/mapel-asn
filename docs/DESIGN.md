# DESIGN.md — MAPEL Monitoring Apel

Arah visual produk ini. Filter antislop berlaku di atasnya; yang tertulis di sini
adalah identitas (bukan selera default), ditulis pemilik produk.

## Identitas & dial

- Produk: dashboard kedinasan (Dinas PUPR Barito Utara) + display TV lobby. Bahasa: Indonesia.
- Motif identitas: lencana kedinasan (logo Pemda, BerAKHLAK), angka tabular tegas, badge status apel.
- Dial antislop: **ENERGY 1 / RHYTHM 1 / MOTION 1** — situs dinas yang tenang:
  tidak ada animasi kecuali transisi hover dan animasi data chart.

## Token terkunci

- Aksen tunggal: `#435ebe` (satu-satunya warna aksi/primer di semua halaman).
- Netral: slate `#0f172a/#64748b/#e2e8f0`, kartu gelap `#1e1e2d`.
- Status = makna kedinasan, bukan dekorasi: hijau Hadir, kuning Terlambat (teks
  gelap `#7a4a00` agar lolos AA — R-25), biru/info apel sore, merah anomali,
  biru primer dispensasi resmi.
- Tipe: Plus Jakarta Sans + angka tabular monospace untuk jam/statistik.
- Radius: kartu 16px, badge 6px, pil hanya bottom-nav. Tidak dicampur.

## Job per layar (R-05/R-31: satu baris alasan per keputusan besar)

- `/` Dashboard: **jawab "bagaimana kepatuhan hari ini?"** — hero Kepatuhan %
  (R-14: satu metrik utama pimpinan), 4 kartu pendukung, tren, tabel.
- `/presensi`: **jawab "siapa, jam berapa, status apa?"** — tabel rinci + filter periode.
- `/dispensasi`: **kerja petugas** — form pengesahan + riwayat (satu-satunya
  layar tulis; sisanya baca).
- `/aturan-presensi`: **sumber kebenaran jadwal** — matriks + simulator + form
  admin (satu-satunya layar ubah konfigurasi).
- `/master/*`, `/petugas`, `/roles`: CRUD admin BioTime/pengguna.
- Elevasi (R-12): diam = flat ber-border; lift hanya di card berisi kontrol
  interaktif (filter/tabel/form). Dot header = indikator koneksi real, statis.
