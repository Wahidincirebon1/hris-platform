# HRIS Platform

HRIS baru untuk kantor, dikembangkan terpisah dari aplikasi lama. Jangan hapus volume atau container aplikasi lama ketika mengerjakan proyek ini.

Blueprint kebutuhan dan arsitektur: [HRIS-PLAN-ARCHITECTURE.md](../Aplikasi-HRIS/docs/HRIS-PLAN-ARCHITECTURE.md).

## Status

Fondasi organisasi, absensi, cuti, kalender perusahaan, KPI, dan laporan sudah tersedia pada stack lokal. Aplikasi belum siap untuk pilot lintas cabang atau penggunaan produksi; prasyaratnya tercantum pada bagian kesiapan pilot:

- React web app responsif dengan absensi diri, pengajuan cuti, persetujuan manager/HR, serta pengaturan cuti dan kalender oleh HR.
- Go API dengan liveness/readiness checks dan migrasi database versioned.
- PostgreSQL privat di jaringan Docker.
- Cabang awal Bandung dan Surabaya; koordinat geofence masih menunggu data resmi HR.
- Docker Compose terpisah, hanya tersedia pada komputer ini melalui `http://127.0.0.1.nip.io:8090` atau `http://localhost:8090`.

Autentikasi, bootstrap superadmin satu kali, pembatasan role/cabang, provisioning akun oleh HR, profil karyawan dan unggah foto, audit, jadwal efektif, absensi geofence, rekap keterlambatan, ledger cuti, kalender perusahaan, KPI, serta ekspor CSV/PDF tersedia pada stack baru. Migrasi `005_kpi.sql` telah diterapkan dan alur KPI telah diuji end-to-end menggunakan data sintetis yang sudah dibersihkan. Migrasi `006_employee_profile_and_default_leave.sql` menambahkan grade/foto profil dan memberikan hak awal 12 hari untuk tahun berjalan di zona waktu Jakarta jika kebijakan tahun tersebut belum ada; tahun berikutnya tetap perlu ditetapkan HR. Migrasi `007_employee_self_service_requests.sql` sudah diterapkan pada database lokal dan menambahkan alur izin/lembur serta persetujuan personal HR oleh superadmin; tabel pengajuan baru masih kosong setelah deployment. Hak cuti tahunan ditetapkan HR per tahun dengan jumlah sama untuk karyawan aktif dan tanpa prorata. Pengajuan cuti satuan hari kerja penuh mengikuti alur karyawan → manager → HR; manager mengajukan langsung ke HR; HR mengajukan ke superadmin. Pengajuan pending dapat ditarik karyawan, sedangkan pembatalan cuti yang disetujui hanya oleh HR dengan alasan. Cuti bersama mengurangi saldo; perubahan ke hari kerja mengembalikan debit dan membuka absensi. Karyawan, manager, dan HR memiliki akses ke data mandiri; pengajuan izin/lembur tercatat dan disetujui menurut role. Persetujuan lembur bukan perhitungan payroll. Koordinat lokasi perangkat tidak disimpan; hasil geofence, jarak, dan akurasi GPS disimpan. Absensi tiap cabang tetap nonaktif sampai HR memasukkan koordinat kantor resmi; radius yang disarankan untuk simulasi awal adalah 50 m, tetapi radius tidak menggantikan koordinat. Alur pengecualian absensi dan payroll belum dibuat. Tidak ada admin/password bawaan.

## Menjalankan lokal

PowerShell dari folder ini:

```powershell
.\scripts\Initialize-Local.ps1
docker compose up --build -d
docker compose ps
```

Buka `http://127.0.0.1.nip.io:8090` atau `http://localhost:8090`. API hanya tersedia melalui reverse proxy; endpoint internal tidak dipublikasikan ke host. Database memakai named volume proyek baru `hris-platform`, terpisah dari volume aplikasi lama.

`127.0.0.1.nip.io` adalah alias DNS sementara yang diarahkan ke loopback komputer yang membuka URL; resolusi nama bergantung pada layanan DNS publik nip.io. Port web tetap **hanya terikat ke `127.0.0.1:8090`**, jadi alias sementara ini hanya untuk browser pada komputer yang menjalankan Docker dan bukan domain kantor atau akses untuk komputer Bandung/Surabaya lainnya. Gunakan `localhost` jika alias tidak tersedia. Jangan ubah binding menjadi `0.0.0.0`, membuka firewall, atau membagikan URL ini sebagai akses pilot jarak jauh. Pilot lintas cabang memerlukan domain yang dikuasai perusahaan dan staging terpisah dengan HTTPS, pengelolaan secret, backup, monitoring, dan review keamanan.

## Bootstrap akun awal dan provisioning HR

1. Jalankan stack lokal terlebih dahulu.
2. Jalankan `.\scripts\Bootstrap-Superadmin.ps1` dan masukkan email, password unik minimal 16 karakter, serta cabang. Password tidak disimpan di `.env` dan tidak ditampilkan.
3. Login di aplikasi; akun awal wajib mengganti password.
4. Sebagai superadmin, buat akun HR melalui form provisioning dan pilih role HR.
5. HR kemudian memverifikasi data karyawan sebelum membuat akun employee, manager, atau finance. Password sementara wajib diubah saat login pertama.

Bootstrap hanya dapat berhasil jika belum ada superadmin. Tidak ada pendaftaran publik.

## Struktur

- `backend/cmd/api`: server HTTP.
- `backend/internal/database/migrations`: migrasi skema berurutan dan embedded ke binary.
- `frontend`: React dan Nginx reverse proxy.
- `scripts`: utilitas lokal.

## Data dan kebijakan

Jadwal awal berlaku Senin–Jumat pukul 08.30–18.00 WIB, istirahat 12.00–13.00. Keterlambatan dimulai setelah 08.30 tanpa masa tenggang, dicatat dalam menit penuh, dan tidak memotong gaji otomatis. HR dapat mengubah jadwal/radius dengan tanggal efektif serta mengoreksi waktu absensi dengan alasan yang masuk audit. Akurasi GPS harus 30 meter atau lebih baik. Radius awal geofence 50 meter.

Database pengembangan tidak boleh diisi data produksi. Sebelum memakai absensi, HR perlu memasukkan koordinat resmi Bandung dan Surabaya; tanpa koordinat cabang, tombol absensi resmi tetap nonaktif (data dummy tidak digunakan untuk melewati geofence). Hak cuti awal 12 hari berlaku untuk tahun kalender Jakarta saat migrasi dan dapat diubah HR; HR tetap perlu memeriksa jumlah tahun berjalan serta menetapkan hak cuti tahun berikutnya, kemudian mengisi kalender hari libur/cuti bersama. Kebijakan KPI telah disepakati dan dicatat di bawah; retensi catatan dan proses undangan/notifikasi masih perlu ditetapkan.

### KPI dan laporan

Kebijakan KPI yang disepakati: siklus tahunan bergerak satu arah `planning → open → closed`; manager menyusun target dan bobot, HR menyetujui/mengunci atau meminta revisi, lalu bobot indikator wajib tepat 100% dengan maksimal dua angka desimal. Progres dapat dicatat selama target dikunci dan siklus terbuka. Setelah siklus ditutup, manager menilai dengan skala 0–100 dan HR melakukan tinjauan akhir. Karyawan dapat mengajukan satu keberatan paling lambat lima hari kerja setelah finalisasi; HR memutuskan untuk mempertahankan atau menyesuaikan nilai. Laporan CSV dan PDF mencakup karyawan, absensi, transaksi cuti, KPI, dan koreksi absensi, dengan akses dibatasi berdasarkan role/cakupan. Dashboard HR/superadmin memisahkan kalender/hak cuti, KPI, persetujuan, absensi, jadwal/lokasi, direktori, dan provisioning akun; karyawan memiliki navigasi terpisah untuk absensi, cuti, KPI, dan profil.

Migrasi KPI `005_kpi.sql` sudah diterapkan pada database lokal HRIS baru dan alurnya telah diverifikasi end-to-end. Saat menyiapkan environment staging/produksi yang terpisah, pastikan migrasi berjalan dan verifikasi di environment tersebut juga dilakukan sebelum digunakan.

### Layanan mandiri karyawan

Karyawan, manager/atasan, dan HR dapat melihat absensi, profil, riwayat cuti, serta membuat pengajuan cuti, izin, dan lembur untuk dirinya sendiri. Pengajuan karyawan mengikuti atasan langsung → HR; pengajuan manager/atasan langsung ke HR; pengajuan pribadi HR ditinjau superadmin. Keputusan dan alasan penolakan dicatat. Izin dapat berupa satu hari/rentang hari atau jam tertentu pada satu hari; lembur meminta tanggal dan jam mulai/selesai pada hari yang sama. Catatan ini adalah permintaan dan persetujuan administratif saja, bukan persetujuan payroll/perhitungan upah. Finance tidak mendapat akses ke layanan mandiri personal dalam cakupan ini. Migrasi `007_employee_self_service_requests.sql` menyiapkan tabel dan tahapan persetujuan tersebut.

## Kesiapan pilot terbatas

Mode yang dipilih saat ini adalah lokakarya lokal dengan data sintetis. Ini masih persiapan/uji internal, **belum pilot karyawan** dan bukan produksi. Stack HRIS lokal sehat di port 8090 dan alias sementara `http://127.0.0.1.nip.io:8090` telah diuji pada komputer Docker. Uji API sintetis mencakup manager/karyawan Bandung dan Surabaya, alur cuti dan KPI sampai keberatan, pembatasan laporan, ekspor CSV/PDF, serta check-in yang tetap ditolak tanpa geofence resmi. Backup lokal juga berhasil dipulihkan dan diverifikasi pada container tanpa jaringan. Akun demo khusus untuk lokakarya sekarang tersedia seperti dicantumkan di bawah; data uji transaksi sebelumnya sudah dibersihkan.

### Contoh penanggung jawab (dummy, bukan penunjukan resmi)

Nama, surel, kanal, dan jam di tabel ini hanya contoh fiktif untuk simulasi pembagian tanggung jawab. Akhiran `example.invalid` sengaja tidak dapat menerima surel. Tidak ada akun yang dibuat dari roster ini dan tidak ada orang yang diberi kewenangan nyata. Ganti dengan personel serta kanal yang disetujui sebelum pilot karyawan.

| Peran contoh | Nama dummy | Cabang/cakupan | Tanggung jawab simulasi |
|---|---|---|---|
| HR lead | Rina Contoh | Semua cabang | Memeriksa kebijakan/data HR, memimpin UAT dan keputusan go/no-go |
| Penanggung jawab teknis | Bima Simulasi | Lingkungan aplikasi | Memeriksa health, backup/restore, triase dan eskalasi teknis |
| Manager perwakilan | Dimas Sampel | Bandung | Menguji alur persetujuan dan KPI sebagai manager |
| Karyawan perwakilan | Sari Uji | Bandung | Menguji pengajuan cuti, progres KPI dan laporan pribadi |
| Manager perwakilan | Nadia Demo | Surabaya | Menguji alur persetujuan dan KPI sebagai manager |
| Karyawan perwakilan | Budi Contoh | Surabaya | Menguji pengajuan cuti, progres KPI dan laporan pribadi |

Kanal contoh `#pilot-hris-dummy` dan jam contoh Senin–Jumat 08.30–18.00 WIB hanya untuk melengkapi template simulasi; bukan kanal atau jadwal dukungan aktif. Roster ini bukan penunjukan resmi dan tidak digunakan untuk membuat akun.

### Akun demo lokakarya lokal

Akun berikut adalah data sintetis aktif pada database HRIS lokal, bukan akun karyawan nyata atau akun pilot. Semua password sementara dibuat acak dan harus diganti saat login pertama; password tidak disimpan di repository atau dokumentasi. Login melalui `http://127.0.0.1.nip.io:8090` atau `http://localhost:8090` hanya pada komputer Docker ini.

| Peran | Email login | Cabang |
|---|---|---|
| Superadmin demo | `demo.admin@hris.test` | Bandung |
| HR demo | `demo.hr@hris.test` | Bandung |
| Manager demo | `demo.manager.bdg@hris.test` | Bandung |
| Karyawan demo | `demo.employee.bdg@hris.test` | Bandung |
| Manager demo | `demo.manager.sby@hris.test` | Surabaya |
| Karyawan demo | `demo.employee.sby@hris.test` | Surabaya |

Karyawan demo berada di bawah manager pada cabang yang sama. Siklus KPI 2026 berstatus terbuka untuk latihan; belum ada target KPI atau transaksi cuti demo. HR perlu menetapkan hak cuti sebelum pengajuan cuti dapat diuji. Geofence tetap belum dikonfigurasi sehingga check-in tidak dapat digunakan.

Gerbang sebelum HR mengundang peserta:

1. **Lingkungan:** tentukan apakah pilot tatap muka pada komputer lokal atau melalui staging terpisah. Untuk penggunaan dari Bandung dan Surabaya, siapkan staging yang disetujui, TLS/HTTPS, akses jaringan terbatas, secret tersimpan aman, serta backup dan monitoring. Stack lokal port 8090 tidak dapat dijadikan layanan antar-cabang.
2. **Pemilik dan peserta:** tunjuk HR lead, administrator/penanggung jawab teknis, atasan, dan peserta karyawan dari kedua cabang. Tetapkan kanal dukungan, jam dukungan, serta cara melaporkan masalah tanpa menyertakan password, NIK, atau data sensitif pada tiket.
3. **Konfigurasi HR:** verifikasi koordinat dan radius kantor resmi per cabang sebelum mengaktifkan absensi; tetapkan hak cuti tahun berjalan, kalender nasional/cuti bersama, dan data jadwal efektif. Tentukan kebijakan retensi catatan dan cara aktivasi/pengiriman kredensial. Jangan mengirim password lewat kanal yang tidak disetujui.
4. **Uji penerimaan:** gunakan akun/data sintetis atau data minimum yang diizinkan. HR dan perwakilan manager/karyawan memeriksa login dan pergantian password, akses role/cabang, absensi dan koreksi, alur cuti dan saldo, KPI/keberatan, laporan, serta ekspor. Catat hasil, pemilik tindak lanjut, dan persetujuan HR sebelum memperluas penggunaan.
5. **Pemulihan dan insiden:** buat backup sebelum perubahan/pilot, jalankan uji restore terisolasi, dan dokumentasikan siapa yang berwenang menghentikan layanan, mengisolasi akses, menghubungi pemilik teknis, serta memulihkan layanan. Setelah restore, validasi health, versi migrasi, dan data inti bersama HR.
6. **Go/no-go:** jangan lanjut ke produksi sebelum penerimaan HR, review keamanan, backup/restore teruji, pemantauan, retensi, domain/TLS, dan prosedur dukungan disetujui. Payroll otomatis tetap di luar lingkup.

Status blocker saat ini: contoh penanggung jawab di atas belum disahkan atau ditunjuk sebagai personel; staging perusahaan/domain HTTPS belum tersedia; koordinat geofence resmi belum diberikan dalam format angka; metode aktivasi akun dan notifikasi serta kebijakan retensi masih menunggu keputusan HR. Karena itu, jangan aktifkan absensi geofence atau gunakan data nyata. Untuk lokakarya lokal, gunakan agenda: login dan ganti password; pastikan check-in tetap ditolak tanpa geofence resmi; alur pengajuan/persetujuan cuti; alur target/progres/penilaian/keberatan KPI; lalu tinjau laporan dan cara meminta dukungan. Catat temuan tanpa password atau data pribadi, tetapkan penanggung jawab dan status retest, lalu minta persetujuan HR.

### Backup dan uji restore lokal

Jalankan dari PowerShell; backup default disimpan di `%LOCALAPPDATA%\HRIS-Platform\Backups`, di luar repository. Arsip backup **tidak dienkripsi oleh script** dan dapat memuat data pribadi; batasi akses perangkat dan gunakan media terenkripsi untuk salinan lain.

```powershell
.\scripts\Backup-Local.ps1
.\scripts\Test-BackupRestore-Local.ps1 -BackupPath "$env:LOCALAPPDATA\HRIS-Platform\Backups\<nama-file>.dump"
```

Uji restore berjalan pada container sementara dengan jaringan dinonaktifkan dan tanpa volume persisten; uji ini tidak mengubah database aktif. Restore ke database/volume aktif adalah tindakan operasional terpisah yang memerlukan persetujuan eksplisit dan pemeriksaan target sebelum dijalankan.
