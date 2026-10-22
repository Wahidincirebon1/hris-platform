-- Modul ujian KPI: 10 soal pilihan ganda, nilai lulus 70%.
CREATE TABLE kpi_exam_questions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    question TEXT NOT NULL,
    options JSONB NOT NULL,
    correct_index INTEGER NOT NULL CHECK (correct_index >= 0 AND correct_index <= 9),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE kpi_exam_attempts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    answers JSONB NOT NULL,
    correct_count INTEGER NOT NULL CHECK (correct_count >= 0),
    total_count INTEGER NOT NULL CHECK (total_count > 0),
    score_percent NUMERIC(5,2) NOT NULL CHECK (score_percent BETWEEN 0 AND 100),
    passed BOOLEAN NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id)
);

CREATE INDEX idx_kpi_exam_attempts_user ON kpi_exam_attempts (user_id);

-- Seed 10 soal (berlaku untuk semua divisi)
INSERT INTO kpi_exam_questions (question, options, correct_index) VALUES
    ('Apa itu KPI (Key Performance Indicator)?',
     '["Indikator kinerja utama yang mengukur pencapaian tujuan","Daftar hadir karyawan","Form pengajuan cuti","Surat peringatan"]', 0),
    ('Berapa bobot total seluruh target KPI sebaiknya?',
     '["150%","100%","50%","Tidak ada aturan"]', 1),
    ('Kapan target KPI tahunan idealnya disusun?',
     '["Akhir Desember","Awal periode tahun bersama atasan","Setelah tahun berakhir","Saat ada teguran"]', 1),
    ('Apa yang dimaksud target SMART dalam penyusunan KPI?',
     '["Target yang sulit dicapai","Target spesifik, terukur, dapat dicapai, relevan, dan berbatas waktu","Target yang ditentukan acak","Target tanpa batas waktu"]', 1),
    ('Seberapa sering progres KPI sebaiknya diperbarui?',
     '["Sekali seumur hidup","Secara berkala sesuai kesepakatan (misal bulanan)","Tidak perlu diperbarui","Hanya saat pengajuan cuti"]', 1),
    ('Siapa yang memberikan evaluasi akhir kinerja karyawan?',
     '["Rekan kerja sebild","Atasan langsung / manajer","Klien","Diri sendiri saja"]', 1),
    ('Apa tujuan tinjauan kinerja (performance review) tahunan?',
     '["Menaltap karyawan","Menilai pencapaian, memberi umpan balik, dan menyusun pengembangan","Mengganti semua karyawan","Menghapus data absensi"]', 1),
    ('Jika Anda tidak setuju dengan nilai akhir KPI, mekanisme resminya adalah?',
     '["Protes di grup chat","Mengajukan keberatan (appeal) melalui sistem","Mangkir dari kerja","Meminta rekan menilai ulang"]', 1),
    ('Bagaimana absensi memengaruhi penilaian kinerja?',
     '["Tidak ada hubungan","Kedisiplinan hadir adalah bagian dari penilaian kinerja","Absensi hanya untuk dipotong gaji","Terlambat tidak dihitung"]', 1),
    ('Apa langkah pertama saat Anda kesulitan mencapai target KPI?',
     '["Menyembunyikan masalah","Meng komunikasikan hambatan kepada atasan dan meminta arahan","Mengajukan resign","Menyalahkan rekan kerja"]', 1);
