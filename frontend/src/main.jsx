import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import { api, canUseSelfService, formatWib, jakartaDate, shiftDate, timeInputWib } from './shared.js';
import { ExamResultPanel, ExamScheduleEditor, KPIExam, KPIExamResults } from './exam-panels.jsx';
import { EmployeeEditor } from './employee-editor.jsx';

try {
  const storedTheme = window.localStorage.getItem('hris-theme');
  if (storedTheme === 'dark' || storedTheme === 'light') document.documentElement.dataset.theme = storedTheme;
} catch { /* penyimpanan opsional */ }

const emptyEmployee = {
  email: '',
  temporary_password: '',
  employee_number: '',
  full_name: '',
  birth_date: '',
  nik: '',
  phone: '',
  address: '',
  branch_id: '',
  department_id: '',
  position_title: '',
  job_grade: '',
  manager_id: '',
  role: 'employee',
};

function Login({ onLogin }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(event) {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      const user = await api('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });
      onLogin(user);
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-layout">
        <aside className="auth-visual">
          <a className="auth-brand" href="/" aria-label="Human Resources Information System">
            <span className="brand-mark">H</span>
                <span>Human Resources <small>Information System</small></span>
          </a>
          <div className="auth-story">
            <div className="auth-kicker"><span /> PEOPLE OPERATIONS</div>
            <h1>Kerja lebih teratur, tim lebih bertumbuh.</h1>
            <p>Kelola perjalanan karyawan, kehadiran, cuti, dan kinerja dalam satu ruang kerja.</p>
          </div>
          <div className="auth-highlights" aria-label="Fitur HRIS">
            <div><span className="auth-highlight-icon">01</span><span><strong>Terhubung</strong><small>Profil dan tim dalam satu tempat</small></span></div>
            <div><span className="auth-highlight-icon">02</span><span><strong>Terarah</strong><small>Alur kerja yang jelas dan terukur</small></span></div>
            <div><span className="auth-highlight-icon">03</span><span><strong>Terkendali</strong><small>Akses sesuai peran dan cabang</small></span></div>
          </div>
          <div className="auth-visual-footer">Ruang kerja yang lebih baik dimulai dari sini.</div>
        </aside>
        <section className="auth-panel">
          <div className="auth-panel-inner">
            <div className="auth-mobile-brand">
              <span className="brand-mark">H</span><span>Human Resources <small>Information System</small></span>
            </div>
            <div className="eyebrow">SELAMAT DATANG KEMBALI</div>
            <h2>Masuk ke akun Anda</h2>
            <p className="intro">Gunakan akun yang telah disiapkan oleh HR untuk melanjutkan.</p>
            {error && <div className="alert error" role="alert">{error}</div>}
            <form onSubmit={submit} className="form-stack">
              <label>
                Email
                <input
                  autoComplete="username"
                  type="email"
                  placeholder="nama@perusahaan.co.id"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                />
              </label>
              <label>
                Password
                <input
                  autoComplete="current-password"
                  type="password"
                  placeholder="Masukkan password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
              </label>
              <button className="primary auth-submit" disabled={busy}>
                {busy ? 'Memeriksa...' : 'Masuk ke aplikasi'}
                {!busy && <span aria-hidden="true">→</span>}
              </button>
            </form>
            <div className="auth-security-note"><span aria-hidden="true">◆</span> Sesi aman dan akses dibatasi sesuai peran.</div>
            <footer>Akses lokal pengembangan · Belum dibuka untuk internet publik</footer>
          </div>
        </section>
      </section>
    </main>
  );
}

function ChangePassword({ onChanged, onLogout }) {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(event) {
    event.preventDefault();
    setError('');
    if (newPassword !== confirmPassword) {
      setError('Konfirmasi password tidak sama.');
      return;
    }
    setBusy(true);
    try {
      await api('/api/v1/auth/password', {
        method: 'PUT',
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      });
      onChanged();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-layout">
        <aside className="auth-visual">
          <a className="auth-brand" href="/" aria-label="Human Resources Information System">
            <span className="brand-mark">H</span>
            <span>Human Resources <small>Information System</small></span>
          </a>
          <div className="auth-story">
            <div className="auth-kicker"><span /> PERLINDUNGAN AKUN</div>
            <h1>Keamanan akun dimulai dari Anda.</h1>
            <p>Gunakan password unik yang hanya Anda ketahui untuk menjaga akses tetap aman.</p>
          </div>
          <div className="auth-highlights">
            <div><span className="auth-highlight-icon">✓</span><span><strong>Langkah wajib</strong><small>Password sementara hanya berlaku saat pertama masuk</small></span></div>
            <div><span className="auth-highlight-icon">12+</span><span><strong>Minimal 12 karakter</strong><small>Pilih kombinasi yang kuat dan tidak digunakan ulang</small></span></div>
          </div>
          <div className="auth-visual-footer">Ruang kerja yang lebih baik dimulai dari sini.</div>
        </aside>
        <section className="auth-panel">
          <div className="auth-panel-inner">
            <div className="auth-mobile-brand">
              <span className="brand-mark">H</span><span>Human Resources <small>Information System</small></span>
            </div>
            <div className="eyebrow">PENGAMANAN AKUN</div>
            <h2>Buat password baru</h2>
            <p className="intro">Untuk melindungi akun, ganti password sementara sebelum melanjutkan.</p>
            {error && <div className="alert error" role="alert">{error}</div>}
            <form onSubmit={submit} className="form-stack">
              <label>Password sementara<input type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /></label>
              <label>Password baru (12–72 byte)<input type="password" autoComplete="new-password" minLength={12} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required /></label>
              <label>Ulangi password baru<input type="password" autoComplete="new-password" minLength={12} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required /></label>
              <button className="primary auth-submit" disabled={busy}>{busy ? 'Menyimpan...' : 'Simpan password baru'} {!busy && <span aria-hidden="true">→</span>}</button>
              <button type="button" className="secondary" onClick={onLogout}>Keluar</button>
            </form>
            <div className="auth-security-note"><span aria-hidden="true">◆</span> Jangan bagikan password kepada siapa pun.</div>
          </div>
        </section>
      </section>
    </main>
  );
}

function EmployeeProvisioning({ branches, branchID, employees, onCreated, isSuperadmin }) {
  const [form, setForm] = useState({ ...emptyEmployee, branch_id: branchID });
  const [departments, setDepartments] = useState([]);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setForm((current) => ({ ...current, branch_id: branchID, department_id: '', manager_id: '' }));
  }, [branchID]);

  useEffect(() => {
    if (!form.branch_id) {
      setDepartments([]);
      return;
    }
    api(`/api/v1/departments?branch_id=${encodeURIComponent(form.branch_id)}`)
      .then(setDepartments)
      .catch((requestError) => setError(requestError.message));
  }, [form.branch_id]);

  const managers = employees.filter(
    (employee) => employee.role === 'manager' && employee.active && employee.branch_id === form.branch_id,
  );

  function update(field, value) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  async function submit(event) {
    event.preventDefault();
    setError('');
    setNotice('');
    setBusy(true);
    try {
      const payload = {
        ...form,
        birth_date: form.birth_date || null,
        nik: form.nik || null,
        phone: form.phone || null,
        address: form.address || null,
        department_id: form.department_id || null,
        position_title: form.position_title || null,
        manager_id: form.manager_id || null,
      };
      const created = await api('/api/v1/employees', { method: 'POST', body: JSON.stringify(payload) });
      setForm({ ...emptyEmployee, branch_id: branchID });
      setNotice(`Akun karyawan dibuat dengan nomor karyawan ${created.employee_number}. Sampaikan password sementara melalui kanal yang aman.`);
      await onCreated();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div>
          <div className="eyebrow">PROVISIONING HR</div>
          <h2>Buat akun karyawan</h2>
        </div>
      </div>
      <p className="muted">HR memverifikasi identitas terlebih dahulu. Akun baru wajib mengganti password sementara saat login pertama.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      <form className="employee-form" onSubmit={submit}>
        <label>Nama lengkap<input value={form.full_name} onChange={(event) => update('full_name', event.target.value)} required maxLength={150} /></label>
        <label>Tanggal lahir<input type="date" value={form.birth_date} onChange={(event) => update('birth_date', event.target.value)} max={jakartaDate()} /></label>
        <label>NIK (16 digit) — dibiarkan kosong untuk dibuat otomatis<input value={form.nik} onChange={(event) => update('nik', event.target.value)} maxLength={64} placeholder="Otomatis" /></label>
        <label>Divisi / departemen
          <select value={form.department_id} onChange={(event) => update('department_id', event.target.value)}>
            <option value="">Belum ditentukan</option>
            {departments.map((department) => <option key={department.id} value={department.id}>{department.name}</option>)}
          </select>
        </label>
        <label>Grade
          <select value={form.job_grade} onChange={(event) => update('job_grade', event.target.value)}>
            <option value="">Belum ditentukan</option>
            {Array.from({ length: 10 }, (_, index) => index + 1).map((grade) => (
              <option key={grade} value={String(grade)}>Grade {grade}</option>
            ))}
          </select>
        </label>
        <label>Posisi / jabatan<input value={form.position_title} onChange={(event) => update('position_title', event.target.value)} maxLength={100} /></label>
        <label>Email kerja<input type="email" autoComplete="email" value={form.email} onChange={(event) => update('email', event.target.value)} required /></label>
        <label>Nomor telepon<input type="tel" autoComplete="tel" value={form.phone} onChange={(event) => update('phone', event.target.value)} maxLength={40} /></label>
        <label>Alamat<input value={form.address} onChange={(event) => update('address', event.target.value)} maxLength={500} /></label>
        <label>Cabang
          <select value={form.branch_id} onChange={(event) => update('branch_id', event.target.value)} required>
            <option value="">Pilih cabang</option>
            {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
          </select>
        </label>
        <label>Status kepegawaian (role akses)
          <select value={form.role} onChange={(event) => update('role', event.target.value)}>
            <option value="employee">Karyawan</option>
            <option value="manager">Atasan / manajer</option>
            <option value="finance">Finance</option>
            {isSuperadmin && <option value="hr">HR</option>}
          </select>
        </label>
        <label>Atasan langsung
          <select value={form.manager_id} onChange={(event) => update('manager_id', event.target.value)}>
            <option value="">Belum ditentukan</option>
            {managers.map((manager) => <option key={manager.user_id} value={manager.user_id}>{manager.full_name}</option>)}
          </select>
        </label>
        <label>Nomor karyawan (otomatis)<input value={form.employee_number} onChange={(event) => update('employee_number', event.target.value)} maxLength={64} placeholder="Dibuat otomatis saat akun disimpan" /></label>
        <label>Password sementara<input type="password" autoComplete="new-password" value={form.temporary_password} onChange={(event) => update('temporary_password', event.target.value)} minLength={12} required /></label>
        <div className="form-actions">
          <button className="primary" disabled={busy}>{busy ? 'Membuat akun...' : 'Buat akun'}</button>
        </div>
      </form>
    </section>
  );
}

function AttendancePanel() {
  const [today, setToday] = useState(null);
  const [history, setHistory] = useState([]);
  const [yearSummary, setYearSummary] = useState({ days_present: 0, late_arrivals: 0, late_minutes: 0 });
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const [loading, setLoading] = useState(true);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const currentDate = jakartaDate();
      const yearStart = `${currentDate.slice(0, 4)}-01-01`;
      const [overview, records] = await Promise.all([
        api('/api/v1/attendance/today'),
        api(`/api/v1/attendance/me?from=${shiftDate(currentDate, -6)}&to=${currentDate}`),
      ]);
      setToday(overview);
      setHistory(records);
      setYearSummary(overview.year_to_date || { days_present: 0, late_arrivals: 0, late_minutes: 0 });
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  function submitAttendance(action) {
    setError('');
    setNotice('');
    if (!navigator.geolocation) {
      setError('Browser ini tidak mendukung lokasi. Gunakan browser ponsel atau ajukan koreksi kepada HR.');
      return;
    }
    setBusy(action);
    navigator.geolocation.getCurrentPosition(async (position) => {
      try {
        await api(`/api/v1/attendance/${action}`, {
          method: 'POST',
          body: JSON.stringify({
            latitude: position.coords.latitude,
            longitude: position.coords.longitude,
            accuracy_meters: position.coords.accuracy,
          }),
        });
        setNotice(action === 'check-in' ? 'Absen masuk berhasil dicatat.' : 'Absen pulang berhasil dicatat.');
        await loadData();
      } catch (requestError) {
        setError(requestError.message);
      } finally {
        setBusy('');
      }
    }, (locationError) => {
      const messages = {
        1: 'Izin lokasi ditolak. Izinkan akses lokasi atau ajukan koreksi kepada HR.',
        2: 'Lokasi tidak tersedia. Periksa GPS dan coba lagi.',
        3: 'Pembacaan GPS melewati batas waktu. Coba lagi di area dengan sinyal lebih baik.',
      };
      setError(messages[locationError.code] || 'Lokasi tidak dapat dibaca. Coba lagi atau ajukan koreksi kepada HR.');
      setBusy('');
    }, { enableHighAccuracy: true, maximumAge: 0, timeout: 20000 });
  }

  const record = today?.record;
  const canCheckIn = today?.is_workday && today?.geofence_configured && !record;
  const canCheckOut = today?.is_workday && today?.geofence_configured && record && !record.check_out_at;

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">ABSENSI SAYA</div><h2>Masuk dan pulang</h2></div>
        {today && <span className="role-pill">{formatWib(`${today.date}T00:00:00+07:00`, { dateStyle: 'medium' })} · WIB</span>}
      </div>
      <p className="muted">Jadwal {today?.schedule?.work_start?.slice(0, 5) || '08:30'}–{today?.schedule?.work_end?.slice(0, 5) || '18:00'} WIB, Senin–Jumat. Lokasi hanya dipakai saat menekan tombol absensi; koordinat mentah tidak disimpan.</p>
      {error && <div className="alert error" role="alert">{error} <button type="button" className="secondary compact" onClick={loadData}>Coba muat ulang</button></div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {loading ? <p className="muted">Memuat status absensi...</p> : (
        <>
          {!today?.is_workday && <div className="alert info" role="status">Hari ini bukan hari kerja. Absensi tidak dibuka.</div>}
          {today?.is_workday && !today?.geofence_configured && (
            <div className="alert info" role="status">Koordinat kantor cabang Anda belum dikonfigurasi HR. Absensi lokasi dinonaktifkan sampai konfigurasi tersedia.</div>
          )}
          <div className="attendance-action-row">
            <button type="button" className="primary" disabled={!canCheckIn || Boolean(busy)} onClick={() => submitAttendance('check-in')}>
              {busy === 'check-in' ? 'Membaca lokasi...' : !today ? 'Status belum tersedia' : !today.is_workday ? 'Hari ini bukan hari kerja' : !today.geofence_configured ? 'Nonaktif — kantor belum diatur' : record ? 'Sudah absen masuk' : 'Absen masuk'}
            </button>
            <button type="button" className="secondary" disabled={!canCheckOut || Boolean(busy)} onClick={() => submitAttendance('check-out')}>
              {busy === 'check-out' ? 'Membaca lokasi...' : !today ? 'Status belum tersedia' : !today.is_workday ? 'Hari ini bukan hari kerja' : !today.geofence_configured ? 'Nonaktif — kantor belum diatur' : record?.check_out_at ? 'Sudah absen pulang' : 'Absen pulang'}
            </button>
            {record?.check_out_at && <span className="active-pill is-active">Absensi hari ini lengkap</span>}
          </div>
          <div className="attendance-today-grid">
            <div><span>Hari hadir · tahun berjalan</span><strong>{yearSummary.days_present} hari</strong></div>
            <div><span>Jumlah terlambat</span><strong>{yearSummary.late_arrivals} kali</strong></div>
            <div><span>Akumulasi keterlambatan</span><strong>{yearSummary.late_minutes} menit</strong></div>
          </div>
          {record && (
            <div className="attendance-today-grid">
              <div><span>Masuk</span><strong>{formatWib(record.check_in_at, { hour: '2-digit', minute: '2-digit' })} WIB</strong></div>
              <div><span>Status</span><strong>{record.late ? `Terlambat${record.late_minutes ? ` ${record.late_minutes} menit` : ' kurang dari 1 menit'}` : 'Tepat waktu'}</strong></div>
              <div><span>Pulang</span><strong>{record.check_out_at ? `${formatWib(record.check_out_at, { hour: '2-digit', minute: '2-digit' })} WIB` : 'Belum absen'}</strong></div>
            </div>
          )}
        </>
      )}
      <div className="table-wrap attendance-table">
        <table>
          <thead><tr><th>Tanggal</th><th>Masuk</th><th>Status</th><th>Pulang</th></tr></thead>
          <tbody>
            {history.length === 0
              ? <tr><td colSpan="4" className="empty">Belum ada catatan absensi pada periode ini.</td></tr>
              : history.map((item) => (
                <tr key={item.id}>
                  <td>{formatWib(`${item.date}T00:00:00+07:00`, { dateStyle: 'medium' })}</td>
                  <td>{formatWib(item.check_in_at, { hour: '2-digit', minute: '2-digit' })}</td>
                  <td>{item.late ? `Terlambat ${item.late_minutes || '<1'} menit` : 'Tepat waktu'}</td>
                  <td>{formatWib(item.check_out_at, { hour: '2-digit', minute: '2-digit' })}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function AttendanceOverview({ user, branches, branchID, onBranchChange }) {
  const isHR = ['hr', 'superadmin'].includes(user.role);
  const [overview, setOverview] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const loadOverview = useCallback(async () => {
    setBusy(true);
    setError('');
    const query = new URLSearchParams();
    if (isHR && branchID) query.set('branch_id', branchID);
    try {
      setOverview(await api(`/api/v1/attendance/overview${query.size ? `?${query.toString()}` : ''}`));
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }, [branchID, isHR]);

  useEffect(() => { loadOverview(); }, [loadOverview]);

  const employees = overview?.employees || [];
  const checkedIn = employees.filter((employee) => employee.check_in_at);
  const stillAtWork = checkedIn.filter((employee) => !employee.check_out_at);
  const totalLateMinutes = employees.reduce((total, employee) => total + employee.late_minutes_ytd, 0);
  const lateArrivals = employees.reduce((total, employee) => total + employee.late_arrivals_ytd, 0);

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">PANTAU KEHADIRAN</div><h2>Absensi seluruh karyawan</h2></div>
        {isHR && <label className="inline-filter">Cabang
          <select value={branchID} onChange={(event) => onBranchChange(event.target.value)}>
            <option value="">Semua cabang</option>
            {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
          </select>
        </label>}
      </div>
      <p className="muted">Status check-in hari ini dan akumulasi keterlambatan sejak awal tahun. Data lokasi mentah tidak ditampilkan atau disimpan.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      <div className="summary-grid attendance-overview-stats">
        <article className="summary-card"><span>Sudah absen masuk hari ini</span><strong>{busy && !overview ? '—' : checkedIn.length}</strong><small>dari {employees.length} karyawan aktif</small></article>
        <article className="summary-card"><span>Sedang bekerja</span><strong>{busy && !overview ? '—' : stillAtWork.length}</strong><small>Belum mencatat waktu pulang</small></article>
        <article className="summary-card"><span>Terlambat · tahun berjalan</span><strong>{busy && !overview ? '—' : `${totalLateMinutes} menit`}</strong><small>{lateArrivals} kejadian terlambat</small></article>
      </div>
      <div className="section-heading">
        <span className="role-pill">{overview ? formatWib(`${overview.date}T00:00:00+07:00`, { dateStyle: 'full' }) : 'Memuat tanggal…'} · WIB</span>
        <button type="button" className="secondary compact" disabled={busy} onClick={loadOverview}>{busy ? 'Memperbarui…' : 'Perbarui status'}</button>
      </div>
      <div className="table-wrap attendance-table">
        <table>
          <thead><tr><th>Karyawan</th><th>Cabang</th><th>Masuk hari ini</th><th>Pulang</th><th>Status hari ini</th><th>Terlambat · tahun ini</th></tr></thead>
          <tbody>
            {!employees.length
              ? <tr><td colSpan="6" className="empty">{busy ? 'Memuat daftar kehadiran…' : 'Belum ada karyawan aktif pada cakupan ini.'}</td></tr>
              : employees.map((employee) => (
                <tr key={employee.user_id}>
                  <td><strong>{employee.full_name}</strong><small className="subtext">{employee.employee_number}</small></td>
                  <td>{employee.branch_name}</td>
                  <td>{formatWib(employee.check_in_at, { hour: '2-digit', minute: '2-digit' })}</td>
                  <td>{formatWib(employee.check_out_at, { hour: '2-digit', minute: '2-digit' })}</td>
                  <td><span className={`active-pill ${employee.check_out_at ? 'is-active' : employee.check_in_at ? 'is-active' : 'is-inactive'}`}>
                    {employee.check_out_at ? 'Lengkap' : employee.check_in_at ? (employee.late_today ? `Terlambat ${employee.late_minutes_today} menit` : 'Sudah masuk') : 'Belum check-in'}
                  </span></td>
                  <td>{employee.late_minutes_ytd} menit <small className="subtext">{employee.late_arrivals_ytd} kali · {employee.days_present_ytd} hari hadir</small></td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function EmployeeProfile({ profilePath, photoPath, canUpload, canEdit, onEdit, onClose }) {
  const [profile, setProfile] = useState(null);
  const [file, setFile] = useState(null);
  const [preview, setPreview] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const fileInputRef = useRef(null);

  const loadProfile = useCallback(async () => {
    setError('');
    try {
      setProfile(await api(profilePath));
    } catch (requestError) {
      setError(requestError.message);
    }
  }, [profilePath]);

  useEffect(() => { loadProfile(); }, [loadProfile]);
  useEffect(() => {
    if (!file) {
      setPreview('');
      return undefined;
    }
    const objectURL = URL.createObjectURL(file);
    setPreview(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [file]);

  async function uploadPhoto(selected) {
    if (!selected) return;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      const body = new FormData();
      body.append('photo', selected);
      const response = await fetch(photoPath, { method: 'POST', credentials: 'same-origin', body });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(result.error || `Upload foto gagal (${response.status}).`);
      setFile(null);
      setNotice('Foto profil berhasil diperbarui.');
      await loadProfile();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  function choosePhoto(event) {
    const selected = event.target.files?.[0] || null;
    event.target.value = '';
    if (!selected) return;
    if (!['image/jpeg', 'image/png'].includes(selected.type) || selected.size > 5 * 1024 * 1024) {
      setError('Foto harus JPG/PNG dengan ukuran maksimal 5 MB.');
      return;
    }
    setError('');
    setFile(selected);
    uploadPhoto(selected);
  }

  const photoURL = preview || (profile?.profile_photo_url
    ? `${profile.profile_photo_url}?updated=${encodeURIComponent(profile.photo_updated_at || '')}`
    : '');

  return (
    <section className="panel employee-profile-panel">
      <div className="section-heading">
        <div><div className="eyebrow">PROFIL KARYAWAN</div><h2>{profile?.full_name || 'Detail karyawan'}</h2></div>
        <div className="profile-heading-actions">
          {canEdit && profile && <button type="button" className="secondary compact" onClick={() => onEdit?.(profile.user_id)}>Edit data</button>}
          {onClose && <button type="button" className="secondary compact" onClick={onClose}>Tutup profil</button>}
        </div>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {!profile ? <p className="muted">{error ? 'Profil belum berhasil dimuat.' : 'Memuat profil…'} <button type="button" className="secondary compact" onClick={loadProfile}>Muat ulang profil</button></p> : (
        <>
          <div className="profile-hero">
            <div className="profile-avatar-wrap">
              <button
                type="button"
                className={`employee-avatar profile-avatar-button ${canUpload ? 'is-editable' : ''}`}
                onClick={() => canUpload && fileInputRef.current?.click()}
                disabled={!canUpload || busy}
                title={canUpload ? 'Klik untuk mengganti foto profil' : undefined}
                aria-label={canUpload ? 'Ganti foto profil' : `Foto ${profile.full_name}`}
              >
                {photoURL ? <img src={photoURL} alt={`Foto ${profile.full_name}`} /> : <span>{profile.full_name.split(/\s+/).slice(0, 2).map((part) => part[0]).join('').toUpperCase()}</span>}
                {canUpload && (
                  <span className="avatar-overlay" aria-hidden="true">
                    {busy ? 'Mengunggah…' : 'Ganti foto profil'}
                  </span>
                )}
              </button>
              {canUpload && (
                <input
                  ref={fileInputRef}
                  className="visually-hidden-input"
                  type="file"
                  accept="image/jpeg,image/png"
                  onChange={choosePhoto}
                  aria-label="Pilih foto profil"
                />
              )}
            </div>
            <div className="profile-hero-copy">
              <span className="role-pill">{profile.role}</span>
              <h3>{profile.full_name}</h3>
              <p>{profile.position_title || 'Posisi belum ditentukan'} · {profile.department || 'Divisi belum ditentukan'}</p>
              <p className="subtext">{profile.employee_number} · {profile.branch_name}</p>
            </div>
          </div>
          <div className="profile-grid employee-profile-grid">
            <div><span>Nama lengkap</span><strong>{profile.full_name}</strong></div>
            <div><span>Tanggal lahir</span><strong>{profile.birth_date ? formatWib(`${profile.birth_date}T00:00:00+07:00`, { dateStyle: 'long' }) : 'Belum diisi'}</strong></div>
            <div><span>NIK</span><strong>{profile.nik || 'Belum diisi'}</strong></div>
            <div><span>Divisi</span><strong>{profile.department || 'Belum ditentukan'}</strong></div>
            <div><span>Grade</span><strong>{profile.job_grade ? `Grade ${profile.job_grade}` : 'Belum ditentukan'}</strong></div>
            <div><span>Posisi / jabatan</span><strong>{profile.position_title || 'Belum ditentukan'}</strong></div>
            <div><span>Email</span><strong>{profile.email}</strong></div>
            <div><span>Nomor telepon</span><strong>{profile.phone || 'Belum diisi'}</strong></div>
            <div><span>Alamat</span><strong>{profile.address || 'Belum diisi'}</strong></div>
            <div><span>Status karyawan</span><strong>{profile.employment_status}</strong></div>
            <div><span>Cabang</span><strong>{profile.branch_name}</strong></div>
            <div><span>Atasan langsung</span><strong>{profile.manager_name || 'Belum ditetapkan'}</strong></div>
            <div><span>Role akses</span><strong>{profile.role}</strong></div>
            <div><span>Tanggal mulai kerja</span><strong>{profile.hired_on || 'Belum diisi'}</strong></div>
          </div>
          <p className="muted profile-privacy-note">Informasi pribadi hanya ditampilkan kepada pemilik profil dan HR berwenang.</p>
        </>
      )}
    </section>
  );
}

const leaveKindLabels = {
  public_holiday: 'Libur nasional/perusahaan',
  collective_leave: 'Cuti bersama',
  company_workday: 'Hari kerja khusus',
};

const leaveStatusLabels = {
  pending_manager: 'Menunggu manager',
  pending_hr: 'Menunggu HR',
  pending_superadmin: 'Menunggu superadmin',
  approved: 'Disetujui',
  rejected: 'Ditolak',
  cancelled: 'Dibatalkan',
};

function LeavePanel({ user }) {
  const currentYear = Number(jakartaDate().slice(0, 4));
  const [year, setYear] = useState(currentYear);
  const [data, setData] = useState(null);
  const [queue, setQueue] = useState([]);
  const [form, setForm] = useState({ start_date: '', end_date: '', reason: '' });
  const [notes, setNotes] = useState({});
  const [cancelReasons, setCancelReasons] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const canApprove = ['manager', 'hr', 'superadmin'].includes(user.role);
  const canRequest = canUseSelfService(user.role);
  const canCancelApproved = user.role === 'hr';

  const loadData = useCallback(async () => {
    setError('');
    try {
      const [mine, approvals] = await Promise.all([
        canRequest ? api(`/api/v1/leave/me?year=${year}`) : Promise.resolve(null),
        canApprove ? api('/api/v1/leave/requests') : Promise.resolve([]),
      ]);
      if (mine) setData(mine);
      setQueue(approvals);
    } catch (requestError) {
      setError(requestError.message);
    }
  }, [canApprove, canRequest, year]);

  useEffect(() => { loadData(); }, [loadData]);

  async function submitRequest(event) {
    event.preventDefault();
    setBusy('request');
    setError('');
    setNotice('');
    try {
      await api('/api/v1/leave/requests', { method: 'POST', body: JSON.stringify(form) });
      setForm({ start_date: '', end_date: '', reason: '' });
      const destination = user.role === 'hr' ? 'superadmin' : user.role === 'manager' ? 'HR' : 'manager';
      setNotice(`Pengajuan cuti berhasil dikirim ke ${destination}.`);
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function withdraw(requestID) {
    setBusy(requestID);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/leave/requests/${requestID}/withdraw`, { method: 'POST' });
      setNotice('Pengajuan cuti berhasil ditarik.');
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function decide(request, decision) {
    const decisionStage = request.status === 'pending_manager' ? 'manager' :
      request.status === 'pending_superadmin' ? 'superadmin' : 'hr';
    const path = `/api/v1/leave/requests/${request.id}/${decisionStage}-decision`;
    setBusy(request.id);
    setError('');
    setNotice('');
    try {
      await api(path, {
        method: 'POST',
        body: JSON.stringify({ decision, note: notes[request.id] || '' }),
      });
      setNotice(decision === 'approve' ? 'Keputusan persetujuan berhasil disimpan.' : 'Pengajuan berhasil ditolak.');
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function cancelApproved(request) {
    setBusy(request.id);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/leave/requests/${request.id}/cancel`, {
        method: 'POST',
        body: JSON.stringify({ reason: cancelReasons[request.id] || '' }),
      });
      setNotice('Cuti disetujui berhasil dibatalkan dan saldonya dikembalikan.');
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">CUTI & KALENDER</div><h2>Saldo dan pengajuan cuti</h2></div>
        <label className="inline-filter">Tahun saldo
          <select value={year} onChange={(event) => setYear(Number(event.target.value))}>
            {[currentYear - 1, currentYear, currentYear + 1].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {data?.balance && (
        <div className="attendance-today-grid leave-balance-grid">
          <div><span>Hak tahunan</span><strong>{data.balance.policy_configured ? `${data.balance.annual_days} hari` : 'Belum ditetapkan'}</strong></div>
          <div><span>Saldo tersedia</span><strong>{data.balance.balance_days} hari</strong></div>
          <div><span>Terpakai bersih</span><strong>{data.balance.used_days} hari</strong></div>
        </div>
      )}
      {canRequest && !data?.balance?.policy_configured && <div className="alert info">Hak cuti untuk tahun ini belum ditetapkan HR; pengajuan belum dapat dibuat.</div>}
      {canRequest && (
        <>
          <form className="employee-form leave-request-form" onSubmit={submitRequest}>
            <label>Tanggal mulai<input type="date" value={form.start_date} onChange={(event) => setForm({ ...form, start_date: event.target.value })} required /></label>
            <label>Tanggal selesai<input type="date" value={form.end_date} onChange={(event) => setForm({ ...form, end_date: event.target.value })} required /></label>
            <label className="leave-reason">Alasan cuti<textarea value={form.reason} onChange={(event) => setForm({ ...form, reason: event.target.value })} minLength={5} maxLength={1000} rows="2" required /></label>
            <div className="form-actions"><button className="primary" disabled={Boolean(busy) || !data?.balance?.policy_configured}>{busy === 'request' ? 'Mengirim...' : 'Ajukan cuti'}</button></div>
          </form>
          <p className="muted leave-help">Pengajuan menggunakan hari kerja penuh; akhir pekan, libur perusahaan, dan cuti bersama tidak dihitung sebagai hari cuti yang diminta.</p>
        </>
      )}
      {canRequest && <div className="table-wrap">
        <table>
          <thead><tr><th>Periode</th><th>Hari</th><th>Status</th><th>Catatan</th><th>Aksi</th></tr></thead>
          <tbody>
            {!data?.requests?.length
              ? <tr><td colSpan="5" className="empty">Belum ada pengajuan cuti.</td></tr>
              : data.requests.map((request) => (
                <tr key={request.id}>
                  <td>{request.start_date} – {request.end_date}<span className="subtext">{request.reason}</span></td>
                  <td>{request.day_count}</td>
                  <td><span className="role-pill">{leaveStatusLabels[request.status] || request.status}</span></td>
                  <td>{request.manager_note || request.hr_note || request.superadmin_note || '—'}</td>
                  <td>{['pending_manager', 'pending_hr', 'pending_superadmin'].includes(request.status) && <button className="secondary compact" type="button" disabled={Boolean(busy)} onClick={() => withdraw(request.id)}>Tarik</button>}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>}
      {canApprove && (
        <div className="leave-approvals">
          <div className="section-heading"><div><div className="eyebrow">{user.role === 'manager' ? 'TINJAUAN ATASAN' : user.role === 'superadmin' ? 'TINJAUAN SUPERADMIN' : 'TINJAUAN HR'}</div><h2>Antrean persetujuan</h2></div></div>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Karyawan</th><th>Periode</th><th>Hari</th><th>Alasan</th><th>Keputusan</th></tr></thead>
              <tbody>
                {!queue.length
                  ? <tr><td colSpan="5" className="empty">Tidak ada pengajuan yang menunggu keputusan Anda.</td></tr>
                  : queue.map((request) => (
                    <tr key={request.id}>
                      <td><strong>{request.employee_name}</strong><span className="subtext">{request.employee_number} · {request.branch_name}</span></td>
                      <td>{request.start_date} – {request.end_date}</td>
                      <td>{request.day_count}</td>
                      <td>{request.reason}</td>
                      <td>
                        {request.status === 'approved' ? (
                          <div className="leave-decision-form">
                            <textarea aria-label="Alasan pembatalan" placeholder="Alasan pembatalan HR (wajib)" minLength={5} maxLength={500} rows="2" value={cancelReasons[request.id] || ''} onChange={(event) => setCancelReasons({ ...cancelReasons, [request.id]: event.target.value })} />
                            {canCancelApproved && <button type="button" className="secondary compact" disabled={Boolean(busy)} onClick={() => cancelApproved(request)}>Batalkan & kembalikan saldo</button>}
                          </div>
                        ) : (
                          <div className="leave-decision-form">
                            <textarea aria-label="Catatan keputusan" placeholder="Catatan / alasan penolakan" maxLength={500} rows="2" value={notes[request.id] || ''} onChange={(event) => setNotes({ ...notes, [request.id]: event.target.value })} />
                            <div className="attendance-action-row">
                              {request.status !== 'approved' && <button type="button" className="primary compact" disabled={Boolean(busy)} onClick={() => decide(request, 'approve')}>Setujui</button>}
                              {request.status !== 'approved' && <button type="button" className="secondary compact" disabled={Boolean(busy)} onClick={() => decide(request, 'reject')}>Tolak</button>}
                            </div>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

const employeeRequestLabels = {
  overtime: 'Lembur',
  permission: 'Izin',
};

function EmployeeRequestsPanel({ user }) {
  const [requests, setRequests] = useState([]);
  const [approvals, setApprovals] = useState([]);
  const [form, setForm] = useState({
    request_type: 'overtime',
    start_date: '',
    end_date: '',
    start_time: '',
    end_time: '',
    reason: '',
  });
  const [notes, setNotes] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const canRequest = canUseSelfService(user.role);
  const canApprove = ['manager', 'hr', 'superadmin'].includes(user.role);
  const requiresTime = form.request_type === 'overtime';

  const loadData = useCallback(async () => {
    setError('');
    try {
      const [mine, queue] = await Promise.all([
        canRequest ? api('/api/v1/employee-requests/me') : Promise.resolve([]),
        canApprove ? api('/api/v1/employee-requests/approvals') : Promise.resolve([]),
      ]);
      setRequests(mine);
      setApprovals(queue);
    } catch (requestError) {
      setError(requestError.message);
    }
  }, [canApprove, canRequest]);

  useEffect(() => { loadData(); }, [loadData]);

  async function submitRequest(event) {
    event.preventDefault();
    setBusy('submit');
    setError('');
    setNotice('');
    const payload = {
      ...form,
      end_date: form.end_date || form.start_date,
      start_time: requiresTime ? form.start_time : form.start_time || '',
      end_time: requiresTime ? form.end_time : form.end_time || '',
    };
    try {
      const result = await api('/api/v1/employee-requests', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
      const destination = result.status === 'pending_manager' ? 'atasan langsung' :
        result.status === 'pending_hr' ? 'HR' : 'superadmin';
      setNotice(`Pengajuan ${employeeRequestLabels[form.request_type].toLowerCase()} dikirim ke ${destination}.`);
      setForm({ request_type: form.request_type, start_date: '', end_date: '', start_time: '', end_time: '', reason: '' });
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function withdrawRequest(request) {
    setBusy(request.id);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/employee-requests/${request.id}/withdraw`, { method: 'POST' });
      setNotice('Pengajuan berhasil ditarik.');
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function decideRequest(request, decision) {
    const stage = request.status === 'pending_manager' ? 'manager' :
      request.status === 'pending_hr' ? 'hr' : 'superadmin';
    setBusy(request.id);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/employee-requests/${request.id}/${stage}-decision`, {
        method: 'POST',
        body: JSON.stringify({ decision, note: notes[request.id] || '' }),
      });
      setNotice(decision === 'approve' ? 'Persetujuan berhasil dicatat.' : 'Pengajuan ditolak.');
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">LAYANAN MANDIRI</div><h2>Izin dan lembur</h2></div>
        <span className="role-pill">Tanpa perhitungan payroll</span>
      </div>
      <p className="muted">Pengajuan dan keputusan tercatat untuk ditinjau. Pengajuan karyawan melewati atasan lalu HR; pengajuan manager langsung ke HR; pengajuan HR ditinjau superadmin.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}

      {canRequest && (
        <form className="employee-form leave-request-form" onSubmit={submitRequest}>
          <label>Jenis pengajuan
            <select value={form.request_type} onChange={(event) => setForm({
              ...form,
              request_type: event.target.value,
              start_time: event.target.value === 'overtime' ? form.start_time : '',
              end_time: event.target.value === 'overtime' ? form.end_time : '',
            })}>
              <option value="overtime">Lembur</option>
              <option value="permission">Izin</option>
            </select>
          </label>
          <label>Tanggal mulai<input type="date" value={form.start_date} onChange={(event) => setForm({ ...form, start_date: event.target.value, end_date: requiresTime ? event.target.value : form.end_date })} required /></label>
          {!requiresTime && <label>Tanggal selesai<input type="date" value={form.end_date} onChange={(event) => setForm({ ...form, end_date: event.target.value })} /></label>}
          <label>Jam mulai<input type="time" value={form.start_time} onChange={(event) => setForm({ ...form, start_time: event.target.value })} required={requiresTime} /></label>
          <label>Jam selesai<input type="time" value={form.end_time} onChange={(event) => setForm({ ...form, end_time: event.target.value })} required={requiresTime} /></label>
          <label className="leave-reason">Alasan / keterangan<textarea value={form.reason} onChange={(event) => setForm({ ...form, reason: event.target.value })} minLength={5} maxLength={1000} rows="2" required /></label>
          <div className="form-actions"><button className="primary" disabled={Boolean(busy)}>{busy === 'submit' ? 'Mengirim...' : `Ajukan ${employeeRequestLabels[form.request_type].toLowerCase()}`}</button></div>
        </form>
      )}

      {canRequest && (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Jenis / tanggal</th><th>Waktu</th><th>Alasan</th><th>Status</th><th>Catatan approver</th><th>Aksi</th></tr></thead>
            <tbody>
              {!requests.length
                ? <tr><td colSpan="6" className="empty">Belum ada pengajuan izin atau lembur.</td></tr>
                : requests.map((request) => (
                  <tr key={request.id}>
                    <td>{employeeRequestLabels[request.request_type]}<span className="subtext">{request.start_date}{request.end_date !== request.start_date ? ` – ${request.end_date}` : ''}</span></td>
                    <td>{request.start_time ? `${request.start_time}–${request.end_time} WIB` : 'Sehari penuh'}</td>
                    <td>{request.reason}</td>
                    <td><span className="role-pill">{leaveStatusLabels[request.status] || request.status}</span></td>
                    <td>{request.manager_note || request.hr_note || request.superadmin_note || '—'}</td>
                    <td>{request.status.startsWith('pending_') && <button className="secondary compact" type="button" disabled={Boolean(busy)} onClick={() => withdrawRequest(request)}>Tarik</button>}</td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}

      {canApprove && (
        <div className="leave-approvals">
          <div className="section-heading"><div><div className="eyebrow">TINJAUAN PENGAJUAN</div><h2>Antrean izin dan lembur</h2></div></div>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Pemohon</th><th>Jenis / periode</th><th>Waktu</th><th>Alasan</th><th>Keputusan</th></tr></thead>
              <tbody>
                {!approvals.length
                  ? <tr><td colSpan="5" className="empty">Tidak ada pengajuan yang menunggu keputusan Anda.</td></tr>
                  : approvals.map((request) => (
                    <tr key={request.id}>
                      <td><strong>{request.employee_name}</strong><span className="subtext">{request.employee_number} · {request.branch_name}</span></td>
                      <td>{employeeRequestLabels[request.request_type]}<span className="subtext">{request.start_date}{request.end_date !== request.start_date ? ` – ${request.end_date}` : ''}</span></td>
                      <td>{request.start_time ? `${request.start_time}–${request.end_time}` : 'Sehari penuh'}</td>
                      <td>{request.reason}</td>
                      <td><div className="leave-decision-form">
                        <textarea aria-label="Catatan keputusan izin/lembur" placeholder="Catatan; alasan penolakan wajib" maxLength={500} rows="2" value={notes[request.id] || ''} onChange={(event) => setNotes({ ...notes, [request.id]: event.target.value })} />
                        <div className="attendance-action-row">
                          <button type="button" className="primary compact" disabled={Boolean(busy)} onClick={() => decideRequest(request, 'approve')}>Setujui</button>
                          <button type="button" className="secondary compact" disabled={Boolean(busy)} onClick={() => decideRequest(request, 'reject')}>Tolak</button>
                        </div>
                      </div></td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

function LeaveSettings() {
  const currentYear = Number(jakartaDate().slice(0, 4));
  const [year, setYear] = useState(currentYear);
  const [settings, setSettings] = useState(null);
  const [entitlement, setEntitlement] = useState({ annual_days: '', reason: '' });
  const [calendarForm, setCalendarForm] = useState({ date: '', name: '', kind: 'public_holiday', reason: '' });
  const [calendarEdits, setCalendarEdits] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');

  const loadSettings = useCallback(async () => {
    setError('');
    try {
      const data = await api(`/api/v1/leave/settings?year=${year}`);
      setSettings(data);
      setEntitlement((current) => current.reason ? current : {
        annual_days: data.policy_configured ? data.annual_days : 12,
        reason: data.policy_configured ? data.reason : '',
      });
    } catch (requestError) {
      setError(requestError.message);
    }
  }, [year]);

  useEffect(() => { loadSettings(); }, [loadSettings]);

  async function saveEntitlement(event) {
    event.preventDefault();
    setBusy('entitlement');
    setError('');
    setNotice('');
    try {
      const result = await api('/api/v1/leave/entitlements', {
        method: 'PUT',
        body: JSON.stringify({ year, annual_days: Number(entitlement.annual_days), reason: entitlement.reason }),
      });
      setNotice(`Hak cuti disimpan dan diselaraskan untuk ${result.employees_updated} karyawan aktif.`);
      setEntitlement((current) => ({ ...current, reason: '' }));
      await loadSettings();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function createCalendar(event) {
    event.preventDefault();
    setBusy('calendar');
    setError('');
    setNotice('');
    try {
      await api('/api/v1/leave/calendar', { method: 'POST', body: JSON.stringify(calendarForm) });
      setNotice('Tanggal berhasil ditambahkan ke kalender perusahaan.');
      setCalendarForm({ date: '', name: '', kind: 'public_holiday', reason: '' });
      await loadSettings();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function updateCalendar(item) {
    const edit = calendarEdits[item.id] || {};
    setBusy(item.id);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/leave/calendar/${item.id}`, {
        method: 'PATCH',
        body: JSON.stringify({
          date: item.date,
          name: edit.name ?? item.name,
          kind: edit.kind ?? item.kind,
          reason: edit.reason || '',
        }),
      });
      setNotice(edit.kind === 'company_workday' && item.kind === 'collective_leave'
        ? 'Hari diubah menjadi hari kerja; debit cuti bersama telah dikembalikan ke saldo.'
        : 'Kalender perusahaan berhasil diperbarui.');
      setCalendarEdits((current) => ({ ...current, [item.id]: { ...current[item.id], reason: '' } }));
      await loadSettings();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">ADMINISTRASI CUTI</div><h2>Hak tahunan & kalender kantor</h2></div>
        <label className="inline-filter">Tahun
          <select value={year} onChange={(event) => { setYear(Number(event.target.value)); setEntitlement({ annual_days: '', reason: '' }); }}>
            {[currentYear - 1, currentYear, currentYear + 1].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      <form className="employee-form" onSubmit={saveEntitlement}>
        <label>Hak cuti tahunan bersama (hari)<input type="number" min="0" max="60" value={entitlement.annual_days} onChange={(event) => setEntitlement({ ...entitlement, annual_days: event.target.value })} required /></label>
        <label>Alasan penetapan/perubahan<input value={entitlement.reason} onChange={(event) => setEntitlement({ ...entitlement, reason: event.target.value })} minLength={5} maxLength={500} required /></label>
        <div className="form-actions"><button className="primary" disabled={Boolean(busy)}>{busy === 'entitlement' ? 'Menyimpan...' : 'Tetapkan hak cuti'}</button></div>
      </form>
      <p className="muted leave-help">Usulan awal 12 hari per tahun. HR dapat menetapkan jumlah yang sama untuk seluruh karyawan; perubahan hak dan cuti bersama akan dicatat dalam ledger saldo.</p>
      <form className="employee-form leave-calendar-form" onSubmit={createCalendar}>
        <label>Tanggal<input type="date" value={calendarForm.date} onChange={(event) => setCalendarForm({ ...calendarForm, date: event.target.value })} required /></label>
        <label>Nama hari<input value={calendarForm.name} onChange={(event) => setCalendarForm({ ...calendarForm, name: event.target.value })} minLength={2} maxLength={120} required /></label>
        <label>Jenis
          <select value={calendarForm.kind} onChange={(event) => setCalendarForm({ ...calendarForm, kind: event.target.value })}>
            {Object.entries(leaveKindLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label>Alasan / referensi<input value={calendarForm.reason} onChange={(event) => setCalendarForm({ ...calendarForm, reason: event.target.value })} minLength={5} maxLength={500} required /></label>
        <div className="form-actions"><button className="primary" disabled={Boolean(busy)}>{busy === 'calendar' ? 'Menambahkan...' : 'Tambah kalender'}</button></div>
      </form>
      <p className="muted leave-help">Cuti bersama mengurangi saldo satu hari kerja. Ubah menjadi “Hari kerja khusus” untuk membuka absensi dan mengembalikan debit yang sama. Libur selain cuti bersama hanya menutup absensi.</p>
      <div className="table-wrap">
        <table>
          <thead><tr><th>Tanggal</th><th>Nama</th><th>Jenis</th><th>Alasan perubahan</th><th>Aksi</th></tr></thead>
          <tbody>
            {!settings?.calendar?.length
              ? <tr><td colSpan="5" className="empty">Belum ada entri kalender pada tahun ini.</td></tr>
              : settings.calendar.map((item) => {
                const edit = calendarEdits[item.id] || {};
                return (
                  <tr key={item.id}>
                    <td>{item.date}</td>
                    <td><input aria-label="Nama kalender" value={edit.name ?? item.name} onChange={(event) => setCalendarEdits({ ...calendarEdits, [item.id]: { ...edit, name: event.target.value } })} /></td>
                    <td><select aria-label="Jenis kalender" value={edit.kind ?? item.kind} onChange={(event) => setCalendarEdits({ ...calendarEdits, [item.id]: { ...edit, kind: event.target.value } })}>{Object.entries(leaveKindLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></td>
                    <td><input aria-label="Alasan perubahan kalender" placeholder="Wajib saat menyimpan" value={edit.reason || ''} onChange={(event) => setCalendarEdits({ ...calendarEdits, [item.id]: { ...edit, reason: event.target.value } })} /></td>
                    <td><button type="button" className="secondary compact" disabled={Boolean(busy)} onClick={() => updateCalendar(item)}>Simpan</button></td>
                  </tr>
                );
              })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

const kpiStatusLabels = {
  planning: 'Perencanaan',
  open: 'Berjalan',
  closed: 'Ditutup',
  draft: 'Draf manager',
  pending_hr: 'Menunggu persetujuan target HR',
  approved: 'Target dikunci',
  pending_hr_final: 'Menunggu tinjauan akhir HR',
  finalized: 'Final',
};

function KPIWorkspace({ user, employees, branches, branchID, onBranchChange, view = 'all' }) {
  const currentYear = Number(jakartaDate().slice(0, 4));
  const [year, setYear] = useState(currentYear);
  const [cycle, setCycle] = useState(null);
  const [cycles, setCycles] = useState([]);
  const [mySet, setMySet] = useState(null);
  const [sets, setSets] = useState([]);
  const [selectedEmployee, setSelectedEmployee] = useState('');
  const [cycleForm, setCycleForm] = useState({ name: `${currentYear} KPI`, status: 'planning' });
  const [goals, setGoals] = useState([
    { title: '', description: '', target_value: '', weight: '50' },
    { title: '', description: '', target_value: '', weight: '50' },
  ]);
  const [progressForms, setProgressForms] = useState({});
  const [evaluationForms, setEvaluationForms] = useState({});
  const [hrNotes, setHrNotes] = useState({});
  const [appealNotes, setAppealNotes] = useState({});
  const [adjustedScores, setAdjustedScores] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const isHR = ['hr', 'superadmin'].includes(user.role);
  const isManager = user.role === 'manager';
  const isEmployee = user.role === 'employee';
  const canViewReport = isHR || isManager;

  const loadData = useCallback(async () => {
    setError('');
    try {
      const requests = [];
      if (user.role !== 'finance') {
        requests.push(api(`/api/v1/kpi/me?year=${year}`));
      } else {
        requests.push(Promise.resolve(null));
      }
      requests.push(canViewReport
        ? api(`/api/v1/kpi/report?year=${year}${isHR && branchID ? `&branch_id=${encodeURIComponent(branchID)}` : ''}`)
        : Promise.resolve([]));
      requests.push(canViewReport ? api('/api/v1/kpi/cycles') : Promise.resolve([]));
      const [mine, report, cycleRows] = await Promise.all(requests);
      setMySet(mine?.goal_set || null);
      setCycle(mine?.cycle || cycleRows.find((item) => item.year === year) || null);
      setSets(report);
      setCycles(cycleRows);
      const selectedCycle = cycleRows.find((item) => item.year === year);
      if (selectedCycle) {
        setCycleForm({ name: selectedCycle.name, status: selectedCycle.status });
      } else {
        setCycleForm({ name: `${year} KPI`, status: 'planning' });
      }
      if (!selectedEmployee && employees.length) setSelectedEmployee(employees[0].user_id);
    } catch (requestError) {
      setError(requestError.message);
    }
  }, [branchID, canViewReport, employees, isHR, selectedEmployee, user.role, year]);

  useEffect(() => { loadData(); }, [loadData]);

  useEffect(() => {
    const draft = sets.find((item) => item.user_id === selectedEmployee && item.status === 'draft');
    if (draft?.goals?.length) {
      setGoals(draft.goals.map((goal) => ({
        title: goal.title,
        description: goal.description,
        target_value: goal.target_value,
        weight: String(goal.weight),
      })));
    }
  }, [selectedEmployee, sets]);

  async function perform(action, successMessage) {
    setBusy(action.key);
    setError('');
    setNotice('');
    try {
      await action.run();
      setNotice(successMessage);
      await loadData();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function saveCycle(event) {
    event.preventDefault();
    await perform({
      key: 'cycle',
      run: () => api('/api/v1/kpi/cycles', {
        method: 'PUT',
        body: JSON.stringify({ year, ...cycleForm }),
      }),
    }, `Periode KPI ${year} berhasil disimpan.`);
  }

  async function saveTargets(event) {
    event.preventDefault();
    const employee = employees.find((item) => item.user_id === selectedEmployee);
    if (!employee) {
      setError('Pilih karyawan yang menjadi bawahan langsung Anda.');
      return;
    }
    const existing = sets.find((item) => item.user_id === employee.user_id && item.status === 'draft');
    await perform({
      key: 'targets',
      run: () => api(`/api/v1/kpi/employees/${employee.user_id}/targets`, {
        method: existing ? 'PUT' : 'POST',
        body: JSON.stringify({
          year,
          goals: goals.map((goal) => ({ ...goal, weight: Number(goal.weight) })),
        }),
      }),
    }, `Draf target KPI untuk ${employee.full_name} berhasil disimpan.`);
  }

  const csvQuery = `year=${year}${isHR && branchID ? `&branch_id=${encodeURIComponent(branchID)}` : ''}`;
  const selectedManagerEmployee = employees.find((item) => item.user_id === selectedEmployee);
  const showReview = view !== 'approvals';
  const showApprovals = view !== 'review';

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">KINERJA TAHUNAN</div><h2>{view === 'approvals' ? 'Persetujuan target, hasil, dan keberatan' : 'KPI & tinjauan kinerja'}</h2></div>
        <div className="kpi-filters">
          {isHR && <label>Cabang
            <select value={branchID} onChange={(event) => onBranchChange(event.target.value)}>
              <option value="">Semua cabang</option>
              {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
            </select>
          </label>}
          <label>Tahun
            <select value={year} onChange={(event) => setYear(Number(event.target.value))}>
              {[...new Set([currentYear - 1, currentYear, currentYear + 1, ...cycles.map((item) => item.year)])].sort((a, b) => b - a).map((value) => <option key={value} value={value}>{value}</option>)}
            </select>
          </label>
        </div>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {cycle && <div className="alert info">Periode {cycle.name} · {kpiStatusLabels[cycle.status] || cycle.status}. Skala 0–100; seluruh bobot target harus berjumlah 100%.</div>}
      {!cycle && <div className="alert info">Periode KPI tahun {year} belum dibuat HR.</div>}

      {showReview && isHR && (
        <form className="employee-form kpi-cycle-form" onSubmit={saveCycle}>
          <label>Nama periode<input value={cycleForm.name} onChange={(event) => setCycleForm({ ...cycleForm, name: event.target.value })} minLength={3} maxLength={120} required /></label>
          <label>Status periode
            <select value={cycleForm.status} onChange={(event) => setCycleForm({ ...cycleForm, status: event.target.value })}>
              <option value="planning">Perencanaan</option>
              <option value="open">Buka target/progres</option>
              <option value="closed">Tutup untuk penilaian manager</option>
            </select>
          </label>
          <div className="form-actions"><button className="primary" disabled={Boolean(busy)}>{busy === 'cycle' ? 'Menyimpan...' : 'Simpan periode'}</button></div>
        </form>
      )}

      {showReview && isManager && cycle?.status === 'open' && employees.length > 0 && (
        <form className="kpi-target-builder" onSubmit={saveTargets}>
          <div className="eyebrow">PENYUSUNAN TARGET MANAGER</div>
          <h3>Target KPI karyawan</h3>
          <label>Karyawan
            <select value={selectedEmployee} onChange={(event) => setSelectedEmployee(event.target.value)} required>
              {employees.map((employee) => <option key={employee.user_id} value={employee.user_id}>{employee.full_name} · {employee.employee_number}</option>)}
            </select>
          </label>
          {selectedManagerEmployee && <p className="muted">Target hanya dapat disusun untuk bawahan langsung. Simpan sebagai draf, lalu ajukan ke HR untuk dikunci.</p>}
          {goals.map((goal, index) => (
            <div className="kpi-goal-editor" key={index}>
              <div className="section-heading"><strong>Indikator {index + 1}</strong>{goals.length > 1 && <button type="button" className="secondary compact" onClick={() => setGoals(goals.filter((_, goalIndex) => goalIndex !== index))}>Hapus</button>}</div>
              <label>Nama indikator<input value={goal.title} onChange={(event) => setGoals(goals.map((item, goalIndex) => goalIndex === index ? { ...item, title: event.target.value } : item))} minLength={3} maxLength={160} required /></label>
              <label>Deskripsi<input value={goal.description} onChange={(event) => setGoals(goals.map((item, goalIndex) => goalIndex === index ? { ...item, description: event.target.value } : item))} minLength={5} maxLength={1000} required /></label>
              <label>Target terukur<input value={goal.target_value} onChange={(event) => setGoals(goals.map((item, goalIndex) => goalIndex === index ? { ...item, target_value: event.target.value } : item))} maxLength={200} placeholder="Contoh: 12 laporan / 95%" required /></label>
              <label>Bobot (%)<input type="number" min="0.01" max="100" step="0.01" value={goal.weight} onChange={(event) => setGoals(goals.map((item, goalIndex) => goalIndex === index ? { ...item, weight: event.target.value } : item))} required /></label>
            </div>
          ))}
          <div className="kpi-builder-actions">
            <span>Total bobot: {goals.reduce((total, goal) => total + (Number(goal.weight) || 0), 0).toFixed(2)}%</span>
            <button type="button" className="secondary" disabled={goals.length >= 30} onClick={() => setGoals([...goals, { title: '', description: '', target_value: '', weight: '0' }])}>Tambah indikator</button>
            <button className="primary" disabled={Boolean(busy)}>{busy === 'targets' ? 'Menyimpan...' : 'Simpan draf target'}</button>
          </div>
        </form>
      )}

      {showReview && user.role !== 'finance' && mySet && (
        <div className="kpi-self-view">
          <div className="section-heading"><div><div className="eyebrow">TARGET SAYA</div><h3>{mySet.employee_name} · {kpiStatusLabels[mySet.status] || mySet.status}</h3></div>
            {mySet.final_score !== undefined && mySet.final_score !== null && <span className="role-pill">Nilai akhir {Number(mySet.final_score).toFixed(2)} / 100</span>}
          </div>
          {mySet.goals.map((goal) => (
            <article className="kpi-goal-card" key={goal.id}>
              <div className="section-heading"><div><h3>{goal.title}</h3><p className="muted">{goal.description}</p></div><span className="role-pill">Target: {goal.target_value} · Bobot {goal.weight}%</span></div>
              {goal.manager_score !== undefined && goal.manager_score !== null && <p><strong>Nilai manager:</strong> {Number(goal.manager_score).toFixed(2)} / 100 {goal.manager_comment && `· ${goal.manager_comment}`}</p>}
              <div className="table-wrap"><table><thead><tr><th>Progres</th><th>Catatan</th><th>Oleh</th><th>Waktu WIB</th></tr></thead><tbody>
                {!goal.progress_updates.length ? <tr><td colSpan="4" className="empty">Belum ada progres.</td></tr> : goal.progress_updates.map((item) => <tr key={item.id}><td>{item.current_value}{item.progress_percent !== undefined && item.progress_percent !== null ? ` · ${item.progress_percent}%` : ''}</td><td>{item.note}</td><td>{item.entered_by_name}</td><td>{formatWib(item.created_at, { dateStyle: 'medium', timeStyle: 'short' })}</td></tr>)}
              </tbody></table></div>
              {cycle?.status === 'open' && mySet.status === 'approved' && (
                <form className="kpi-progress-form" onSubmit={(event) => {
                  event.preventDefault();
                  const form = progressForms[goal.id] || {};
                  perform({ key: goal.id, run: () => api(`/api/v1/kpi/goals/${goal.id}/progress`, { method: 'POST', body: JSON.stringify(form) }) }, 'Progres KPI berhasil dicatat.');
                }}>
                  <label>Nilai terkini<input value={progressForms[goal.id]?.current_value || ''} onChange={(event) => setProgressForms({ ...progressForms, [goal.id]: { ...progressForms[goal.id], current_value: event.target.value } })} maxLength={200} required /></label>
                  <label>Progres (%)<input type="number" min="0" max="100" step="0.01" value={progressForms[goal.id]?.progress_percent || ''} onChange={(event) => setProgressForms({ ...progressForms, [goal.id]: { ...progressForms[goal.id], progress_percent: event.target.value === '' ? null : Number(event.target.value) } })} /></label>
                  <label>Catatan<input value={progressForms[goal.id]?.note || ''} onChange={(event) => setProgressForms({ ...progressForms, [goal.id]: { ...progressForms[goal.id], note: event.target.value } })} minLength={5} maxLength={1000} required /></label>
                  <button className="secondary compact" disabled={Boolean(busy)}>{busy === goal.id ? 'Menyimpan...' : 'Catat progres'}</button>
                </form>
              )}
            </article>
          ))}
          {mySet.status === 'finalized' && (
            <div className="kpi-appeal-box">
              <p><strong>Tinjauan HR:</strong> {mySet.final_review_note || '—'} · Keberatan sampai {mySet.appeal_deadline}</p>
              {mySet.appeal && <p>Status keberatan: {mySet.appeal.status === 'pending' ? 'Menunggu tinjauan HR' : mySet.appeal.decision === 'adjusted' ? 'Nilai disesuaikan HR' : 'Nilai dipertahankan'}{mySet.appeal.decision_note && ` · ${mySet.appeal.decision_note}`}</p>}
              {!mySet.appeal && mySet.appeal_deadline >= jakartaDate() && (
                <form className="kpi-progress-form" onSubmit={(event) => {
                  event.preventDefault();
                  perform({ key: 'appeal', run: () => api(`/api/v1/kpi/sets/${mySet.id}/appeal`, { method: 'POST', body: JSON.stringify({ reason: appealNotes[mySet.id] || '' }) }) }, 'Keberatan KPI dikirim ke HR.');
                }}>
                  <label>Ajukan keberatan dalam lima hari kerja<textarea value={appealNotes[mySet.id] || ''} onChange={(event) => setAppealNotes({ ...appealNotes, [mySet.id]: event.target.value })} minLength={5} maxLength={1000} rows="3" required /></label>
                  <button className="secondary" disabled={Boolean(busy)}>Kirim keberatan</button>
                </form>
              )}
            </div>
          )}
        </div>
      )}

      {showApprovals && canViewReport && (
        <>
          <div className="section-heading kpi-report-heading"><div><div className="eyebrow">{isHR ? 'TINJAUAN HR' : 'TIM LANGSUNG'}</div><h3>{isHR ? 'Persetujuan target, hasil, dan keberatan' : 'Target dan evaluasi bawahan langsung'}</h3></div></div>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Karyawan</th><th>Status</th><th>Indikator / progres</th><th>Nilai</th><th>Keputusan</th></tr></thead>
              <tbody>
                {!sets.length ? <tr><td colSpan="5" className="empty">Belum ada target KPI untuk tahun ini.</td></tr> : sets.map((item) => (
                  <tr key={item.id}>
                    <td><strong>{item.employee_name}</strong><span className="subtext">{item.employee_number} · {item.branch_name}</span></td>
                    <td><span className="role-pill">{kpiStatusLabels[item.status] || item.status}</span></td>
                    <td><div className="kpi-report-goals">{item.goals.map((goal) => <div key={goal.id}><strong>{goal.title}</strong><span>{goal.weight}% · {goal.progress_updates[0]?.current_value || 'Belum ada progres'}</span></div>)}</div></td>
                    <td>{item.final_score !== undefined && item.final_score !== null ? `${Number(item.final_score).toFixed(2)} / 100` : item.goals.some((goal) => goal.manager_score !== undefined && goal.manager_score !== null) ? item.goals.map((goal) => `${goal.title}: ${goal.manager_score ?? '—'}`).join('; ') : '—'}</td>
                    <td>
                      {isHR && item.status === 'pending_hr' && <button className="primary compact" disabled={Boolean(busy)} onClick={() => perform({ key: item.id, run: () => api(`/api/v1/kpi/sets/${item.id}/approve-targets`, { method: 'POST' }) }, 'Target KPI disetujui dan dikunci.')}>Setujui & kunci</button>}
                      {isManager && item.status === 'draft' && <button className="secondary compact" disabled={Boolean(busy)} onClick={() => perform({ key: item.id, run: () => api(`/api/v1/kpi/sets/${item.id}/submit`, { method: 'POST' }) }, 'Target diajukan ke HR.')}>Ajukan ke HR</button>}
                      {isManager && item.status === 'approved' && cycle?.status === 'closed' && (
                        <form className="kpi-evaluation-form" onSubmit={(event) => {
                          event.preventDefault();
                          const form = evaluationForms[item.id] || {};
                          perform({
                            key: item.id,
                            run: () => api(`/api/v1/kpi/sets/${item.id}/evaluation`, {
                              method: 'POST',
                              body: JSON.stringify({
                                comment: form.comment || '',
                                scores: item.goals.map((goal) => ({
                                  goal_id: goal.id,
                                  score: Number(form.scores?.[goal.id]),
                                  comment: form.comments?.[goal.id] || '',
                                })),
                              }),
                            }),
                          }, 'Penilaian manager dikirim ke HR.');
                        }}>
                          {item.goals.map((goal) => <label key={goal.id}>{goal.title} · {goal.weight}%
                            <input type="number" min="0" max="100" step="0.01" value={evaluationForms[item.id]?.scores?.[goal.id] ?? ''} onChange={(event) => setEvaluationForms({
                              ...evaluationForms,
                              [item.id]: { ...evaluationForms[item.id], scores: { ...evaluationForms[item.id]?.scores, [goal.id]: event.target.value } },
                            })} required />
                            <textarea aria-label={`Komentar ${goal.title}`} placeholder="Catatan indikator (opsional)" maxLength={1000} rows="1" value={evaluationForms[item.id]?.comments?.[goal.id] || ''} onChange={(event) => setEvaluationForms({
                              ...evaluationForms,
                              [item.id]: { ...evaluationForms[item.id], comments: { ...evaluationForms[item.id]?.comments, [goal.id]: event.target.value } },
                            })} />
                          </label>)}
                          <label>Ringkasan evaluasi manager<textarea minLength={5} maxLength={2000} rows="2" value={evaluationForms[item.id]?.comment || ''} onChange={(event) => setEvaluationForms({ ...evaluationForms, [item.id]: { ...evaluationForms[item.id], comment: event.target.value } })} required /></label>
                          <button className="primary compact" disabled={Boolean(busy)}>Kirim penilaian</button>
                        </form>
                      )}
                      {isHR && item.status === 'pending_hr_final' && (
                        <div className="kpi-decision-form">
                          <textarea aria-label="Catatan tinjauan akhir HR" minLength={5} maxLength={2000} rows="2" placeholder="Catatan HR wajib" value={hrNotes[item.id] || ''} onChange={(event) => setHrNotes({ ...hrNotes, [item.id]: event.target.value })} />
                          <button className="primary compact" disabled={Boolean(busy)} onClick={() => perform({ key: item.id, run: () => api(`/api/v1/kpi/sets/${item.id}/finalize`, { method: 'POST', body: JSON.stringify({ note: hrNotes[item.id] || '' }) }) }, 'Hasil KPI difinalisasi; karyawan memiliki lima hari kerja untuk mengajukan keberatan.')}>Finalisasi</button>
                        </div>
                      )}
                      {isHR && item.appeal?.status === 'pending' && (
                        <div className="kpi-decision-form">
                          <span className="subtext">Keberatan: {item.appeal.reason}</span>
                          <input type="number" min="0" max="100" step="0.01" placeholder="Nilai pengganti jika disesuaikan" value={adjustedScores[item.id] || ''} onChange={(event) => setAdjustedScores({ ...adjustedScores, [item.id]: event.target.value })} />
                          <textarea aria-label="Catatan keputusan keberatan" minLength={5} maxLength={2000} rows="2" placeholder="Alasan keputusan HR" value={appealNotes[item.id] || ''} onChange={(event) => setAppealNotes({ ...appealNotes, [item.id]: event.target.value })} />
                          <div className="attendance-action-row">
                            <button className="secondary compact" disabled={Boolean(busy)} onClick={() => perform({ key: item.id, run: () => api(`/api/v1/kpi/sets/${item.id}/appeal/decision`, { method: 'POST', body: JSON.stringify({ decision: 'upheld', note: appealNotes[item.id] || '' }) }) }, 'Keberatan ditinjau; nilai dipertahankan.')}>Pertahankan nilai</button>
                            <button className="primary compact" disabled={Boolean(busy)} onClick={() => perform({ key: item.id, run: () => api(`/api/v1/kpi/sets/${item.id}/appeal/decision`, { method: 'POST', body: JSON.stringify({ decision: 'adjusted', note: appealNotes[item.id] || '', adjusted_score: Number(adjustedScores[item.id]) }) }) }, 'Keberatan ditinjau; nilai akhir disesuaikan.')}>Sesuaikan nilai</button>
                          </div>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {showApprovals && canViewReport && (
        <div className="report-downloads">
          <span className="eyebrow">EKSPOR LAPORAN · WIB</span>
          <a className="secondary compact" href={`/api/v1/reports/kpi.csv?${csvQuery}`}>Unduh KPI · CSV</a>
          <a className="secondary compact" href={`/api/v1/reports/kpi.pdf?${csvQuery}`}>Unduh KPI · PDF</a>
          {isHR && <a className="secondary compact" href={`/api/v1/reports/leave.csv?year=${year}`}>Transaksi cuti · CSV</a>}
          {isHR && <a className="secondary compact" href={`/api/v1/reports/leave.pdf?year=${year}`}>Transaksi cuti · PDF</a>}
          {isHR && <a className="secondary compact" href={`/api/v1/reports/employees.csv?branch_id=${encodeURIComponent(branchID)}`}>Daftar karyawan · CSV</a>}
          {isHR && <a className="secondary compact" href={`/api/v1/reports/employees.pdf?branch_id=${encodeURIComponent(branchID)}`}>Daftar karyawan · PDF</a>}
        </div>
      )}
    </section>
  );
}


function AttendanceSettings() {
  const [settings, setSettings] = useState(null);
  const [schedule, setSchedule] = useState(null);
  const [locations, setLocations] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');

  const loadSettings = useCallback(async () => {
    setError('');
    try {
      const data = await api('/api/v1/attendance/settings');
      setSettings(data);
      setSchedule((current) => current || {
        effective_from: shiftDate(jakartaDate(), 1),
        work_start: data.schedule.work_start.slice(0, 5),
        work_end: data.schedule.work_end.slice(0, 5),
        break_start: data.schedule.break_start.slice(0, 5),
        break_end: data.schedule.break_end.slice(0, 5),
      });
      setLocations((current) => {
        const next = { ...current };
        data.branches.forEach((branch) => {
          if (!next[branch.id]) {
            next[branch.id] = {
              effective_from: shiftDate(jakartaDate(), branch.configured ? 1 : 0),
              latitude: branch.latitude ?? '',
              longitude: branch.longitude ?? '',
              radius_meters: branch.radius_meters ?? 50,
            };
          }
        });
        return next;
      });
    } catch (requestError) {
      setError(requestError.message);
    }
  }, []);

  useEffect(() => { loadSettings(); }, [loadSettings]);

  function updateSchedule(field, value) {
    setSchedule((current) => ({ ...current, [field]: value }));
  }

  function updateLocation(branchID, field, value) {
    setLocations((current) => ({
      ...current,
      [branchID]: { ...current[branchID], [field]: value },
    }));
  }

  async function saveSchedule(event) {
    event.preventDefault();
    setBusy('schedule');
    setError('');
    setNotice('');
    try {
      await api('/api/v1/attendance/schedule', {
        method: 'PUT',
        body: JSON.stringify(schedule),
      });
      setSchedule((current) => ({ ...current, effective_from: shiftDate(current.effective_from, 1) }));
      setNotice('Jadwal baru tersimpan dan akan berlaku sesuai tanggal efektif.');
      await loadSettings();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  async function saveLocation(event, branch) {
    event.preventDefault();
    setBusy(branch.id);
    setError('');
    setNotice('');
    try {
      const form = locations[branch.id];
      await api(`/api/v1/branches/${branch.id}/geofence`, {
        method: 'PUT',
        body: {
          ...form,
          latitude: Number(form.latitude),
          longitude: Number(form.longitude),
          radius_meters: Number(form.radius_meters),
        },
      });
      setLocations((current) => ({
        ...current,
        [branch.id]: { ...current[branch.id], effective_from: shiftDate(current[branch.id].effective_from, 1) },
      }));
      setNotice(`Lokasi geofence ${branch.name} tersimpan sesuai tanggal efektif.`);
      await loadSettings();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">PENGATURAN HR</div><h2>Jadwal dan lokasi kantor</h2></div>
      </div>
      <p className="muted">Perubahan menggunakan tanggal efektif dan tidak mengubah jadwal yang sudah melekat pada catatan absensi. Sistem menolak akurasi GPS di atas 30 meter. Pastikan koordinat berasal dari lokasi kantor resmi.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {!settings || !schedule ? <p className="muted">Memuat pengaturan...</p> : (
        <>
          <form className="employee-form" onSubmit={saveSchedule}>
            <label>Tanggal efektif jadwal<input type="date" value={schedule.effective_from} onChange={(event) => updateSchedule('effective_from', event.target.value)} required /></label>
            <label>Jam masuk<input type="time" value={schedule.work_start} onChange={(event) => updateSchedule('work_start', event.target.value)} required /></label>
            <label>Mulai istirahat<input type="time" value={schedule.break_start} onChange={(event) => updateSchedule('break_start', event.target.value)} required /></label>
            <label>Selesai istirahat<input type="time" value={schedule.break_end} onChange={(event) => updateSchedule('break_end', event.target.value)} required /></label>
            <label>Jam pulang<input type="time" value={schedule.work_end} onChange={(event) => updateSchedule('work_end', event.target.value)} required /></label>
            <div className="form-actions"><button className="primary" disabled={Boolean(busy)}>{busy === 'schedule' ? 'Menyimpan...' : 'Simpan jadwal baru'}</button></div>
          </form>
          <p className="muted">Berlaku untuk seluruh kantor, WIB, Senin–Jumat. Keterlambatan dicatat dalam menit penuh; tidak ada potongan gaji otomatis.</p>
          <div className="location-settings-grid">
            {settings.branches.map((branch) => {
              const form = locations[branch.id];
              if (!form) return null;
              return (
                <form key={branch.id} className="location-card" onSubmit={(event) => saveLocation(event, branch)}>
                  <div className="section-heading">
                    <div><div className="eyebrow">{branch.code}</div><h3>{branch.name}</h3></div>
                    <span className={`active-pill ${branch.configured ? 'is-active' : 'is-inactive'}`}>{branch.configured ? 'Geofence aktif' : 'Belum dikonfigurasi'}</span>
                  </div>
                  <label>Latitude<input type="number" step="any" min="-90" max="90" value={form.latitude} onChange={(event) => updateLocation(branch.id, 'latitude', event.target.value)} required /></label>
                  <label>Longitude<input type="number" step="any" min="-180" max="180" value={form.longitude} onChange={(event) => updateLocation(branch.id, 'longitude', event.target.value)} required /></label>
                  <label>Radius (25–2.000 meter)<input type="number" min="25" max="2000" value={form.radius_meters} onChange={(event) => updateLocation(branch.id, 'radius_meters', event.target.value)} required /></label>
                  <label>Tanggal efektif<input type="date" value={form.effective_from} onChange={(event) => updateLocation(branch.id, 'effective_from', event.target.value)} required /></label>
                  <button className="primary" disabled={Boolean(busy)}>{busy === branch.id ? 'Menyimpan...' : 'Simpan geofence'}</button>
                </form>
              );
            })}
          </div>
        </>
      )}
    </section>
  );
}

function AttendanceReport({ user, branches, branchID, onBranchChange }) {
  const isHR = user.role === 'hr' || user.role === 'superadmin';
  const [from, setFrom] = useState(shiftDate(jakartaDate(), -6));
  const [to, setTo] = useState(jakartaDate());
  const [rows, setRows] = useState([]);
  const [corrections, setCorrections] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);

  const loadReport = useCallback(async () => {
    setBusy(true);
    setError('');
    const query = new URLSearchParams({ from, to });
    if (isHR && branchID) query.set('branch_id', branchID);
    try {
      setRows(await api(`/api/v1/attendance?${query.toString()}`));
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }, [branchID, from, isHR, to]);

  useEffect(() => { loadReport(); }, [loadReport]);
  const totalLateMinutes = rows.reduce((total, record) => total + record.late_minutes, 0);
  const lateRecords = rows.filter((record) => record.late).length;

  function updateCorrection(record, field, value) {
    setCorrections((current) => ({
      ...current,
      [record.id]: {
        check_in_time: current[record.id]?.check_in_time ?? timeInputWib(record.check_in_at),
        check_out_time: current[record.id]?.check_out_time ?? timeInputWib(record.check_out_at),
        reason: current[record.id]?.reason ?? '',
        [field]: value,
      },
    }));
  }

  async function saveCorrection(event, record) {
    event.preventDefault();
    const values = corrections[record.id] || {};
    const payload = { reason: values.reason || '' };
    const oldCheckIn = timeInputWib(record.check_in_at);
    const oldCheckOut = timeInputWib(record.check_out_at);
    if (values.check_in_time && values.check_in_time !== oldCheckIn) payload.check_in_time = values.check_in_time;
    if (values.check_out_time && values.check_out_time !== oldCheckOut) payload.check_out_time = values.check_out_time;
    if (!payload.check_in_time && !payload.check_out_time) {
      setError('Ubah waktu masuk atau pulang sebelum menyimpan koreksi.');
      return;
    }
    setError('');
    setNotice('');
    setBusy(true);
    try {
      await api(`/api/v1/attendance/${record.id}`, { method: 'PATCH', body: JSON.stringify(payload) });
      setNotice(`Koreksi ${record.full_name} tersimpan dan masuk audit.`);
      setCorrections((current) => {
        const next = { ...current };
        delete next[record.id];
        return next;
      });
      await loadReport();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">{isHR ? 'REKAP HR' : 'REKAP TIM'}</div><h2>Absensi dan keterlambatan</h2></div>
        {isHR && <label className="inline-filter">Cabang
          <select value={branchID} onChange={(event) => onBranchChange(event.target.value)}>
            <option value="">Semua cabang</option>
            {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
          </select>
        </label>}
      </div>
      <p className="muted">Laporan hanya mencatat waktu dan keterlambatan. HR mengelola penghitungan potongan secara manual; sistem tidak memotong gaji.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      <div className="report-filters">
        <label>Dari<input type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></label>
        <label>Sampai<input type="date" value={to} onChange={(event) => setTo(event.target.value)} /></label>
        <button type="button" className="secondary" disabled={busy} onClick={loadReport}>Perbarui rekap</button>
        <a className="secondary compact" href={`/api/v1/reports/attendance.csv?from=${from}&to=${to}${isHR && branchID ? `&branch_id=${encodeURIComponent(branchID)}` : ''}`}>Absensi · CSV</a>
        <a className="secondary compact" href={`/api/v1/reports/attendance.pdf?from=${from}&to=${to}${isHR && branchID ? `&branch_id=${encodeURIComponent(branchID)}` : ''}`}>Absensi · PDF</a>
        {isHR && <a className="secondary compact" href={`/api/v1/reports/audit.csv?from=${from}&to=${to}`}>Koreksi · CSV</a>}
        {isHR && <a className="secondary compact" href={`/api/v1/reports/audit.pdf?from=${from}&to=${to}`}>Koreksi · PDF</a>}
      </div>
      <div className="attendance-today-grid report-summary-grid">
        <div><span>Catatan absensi pada periode</span><strong>{busy ? 'Memuat…' : rows.length}</strong></div>
        <div><span>Kejadian terlambat</span><strong>{busy ? 'Memuat…' : lateRecords}</strong></div>
        <div><span>Total menit terlambat</span><strong>{busy ? 'Memuat…' : totalLateMinutes}</strong></div>
      </div>
      <div className="table-wrap attendance-table">
        <table>
          <thead><tr><th>Tanggal</th><th>Karyawan</th><th>Cabang</th><th>Masuk</th><th>Status</th><th>Pulang</th>{isHR && <th>Koreksi</th>}</tr></thead>
          <tbody>
            {busy && rows.length === 0 ? <tr><td colSpan={isHR ? '7' : '6'} className="empty">Memuat rekap...</td></tr>
              : rows.length === 0 ? <tr><td colSpan={isHR ? '7' : '6'} className="empty">Belum ada catatan untuk periode ini.</td></tr>
                : rows.map((record) => (
                  <tr key={record.id}>
                    <td>{formatWib(`${record.date}T00:00:00+07:00`, { dateStyle: 'medium' })}</td>
                    <td><strong>{record.full_name}</strong><small className="subtext">{record.employee_number}</small></td>
                    <td>{record.branch_name}</td>
                    <td>{formatWib(record.check_in_at, { hour: '2-digit', minute: '2-digit' })}</td>
                    <td>{record.late ? `Terlambat ${record.late_minutes || '<1'} menit` : 'Tepat waktu'}</td>
                    <td>{formatWib(record.check_out_at, { hour: '2-digit', minute: '2-digit' })}</td>
                    {isHR && <td>
                      <details className="correction-details">
                        <summary>Koreksi jam</summary>
                        <form className="correction-form" onSubmit={(event) => saveCorrection(event, record)}>
                          <label>Masuk<input type="time" value={corrections[record.id]?.check_in_time ?? timeInputWib(record.check_in_at)} onChange={(event) => updateCorrection(record, 'check_in_time', event.target.value)} /></label>
                          <label>Pulang<input type="time" value={corrections[record.id]?.check_out_time ?? timeInputWib(record.check_out_at)} onChange={(event) => updateCorrection(record, 'check_out_time', event.target.value)} /></label>
                          <label>Alasan wajib<input value={corrections[record.id]?.reason ?? ''} onChange={(event) => updateCorrection(record, 'reason', event.target.value)} minLength="5" maxLength="500" required /></label>
                          <button className="secondary compact" disabled={busy}>Simpan koreksi</button>
                        </form>
                      </details>
                    </td>}
                  </tr>
                ))}
          </tbody>
        </table>
      </div>
      {isHR && <p className="muted">Setiap koreksi HR memerlukan alasan dan dicatat dengan nilai sebelum/sesudah di audit.</p>}
    </section>
  );
}

function ThemeToggle() {
  const [theme, setTheme] = useState(() => document.documentElement.dataset.theme || 'light');
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try { window.localStorage.setItem('hris-theme', theme); } catch { /* penyimpanan opsional */ }
  }, [theme]);
  return (
    <button
      type="button"
      className="theme-toggle"
      onClick={() => setTheme((current) => (current === 'dark' ? 'light' : 'dark'))}
      title={theme === 'dark' ? 'Beralih ke tema terang' : 'Beralih ke tema gelap'}
      aria-label={theme === 'dark' ? 'Beralih ke tema terang' : 'Beralih ke tema gelap'}
    >
      <span aria-hidden="true">{theme === 'dark' ? '☀' : '☾'}</span>
      <span className="theme-toggle-label">{theme === 'dark' ? 'Terang' : 'Gelap'}</span>
    </button>
  );
}

function LiveClock({ branchName }) {
  const [now, setNow] = useState(new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  return (
    <div className="live-clock" title="Waktu Indonesia Barat">
      <span className="live-clock-dot" aria-hidden="true" />
      <div className="live-clock-copy">
        <strong>{formatWib(now, { hour: '2-digit', minute: '2-digit', second: '2-digit' })} WIB</strong>
        <span>{formatWib(now, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}</span>
      </div>
      {branchName && <span className="live-clock-branch">{branchName}</span>}
    </div>
  );
}

function Dashboard({ user, onLogout }) {
  const [branches, setBranches] = useState([]);
  const [employees, setEmployees] = useState([]);
  const isHR = user.role === 'hr' || user.role === 'superadmin';
  const [branchFilter, setBranchFilter] = useState(isHR ? '' : user.branch_id);
  const [route, setRoute] = useState(() => window.location.hash.slice(1) || '/home');
  const [selectedProfile, setSelectedProfile] = useState(null);
  const [editTarget, setEditTarget] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);

  const adminPages = [
    { path: '/home', label: 'Ringkasan', eyebrow: 'IKHTISAR ORGANISASI', icon: '⌂' },
    { path: '/admin/provisioning', label: 'Tambah karyawan', eyebrow: 'AKSES & PROVISIONING', icon: '+' },
    { path: '/admin/employees', label: 'Direktori karyawan', eyebrow: 'DIREKTORI ORGANISASI', icon: '◍' },
    { path: '/admin/attendance', label: 'Absensi & keterlambatan', eyebrow: 'KEHADIRAN', icon: '◷' },
    { path: '/admin/my-attendance', label: 'Absensi saya', eyebrow: 'KEHADIRAN PRIBADI', icon: '✓' },
    { path: '/admin/leave', label: user.role === 'hr' ? 'Cuti & kalender' : 'Hak tahunan & kalender', eyebrow: 'CUTI & KALENDER', icon: '☰' },
    { path: '/admin/requests', label: user.role === 'hr' ? 'Izin & lembur saya' : 'Persetujuan izin & lembur', eyebrow: 'LAYANAN MANDIRI', icon: '✉' },
    { path: '/admin/performance', label: 'KPI & tinjauan kinerja', eyebrow: 'KINERJA TAHUNAN', icon: '▤' },
    { path: '/admin/exam', label: 'Ujian KPI', eyebrow: 'UJIAN PEMAHAMAN', icon: '✎' },
    { path: '/admin/exam-results', label: 'Nilai KPI', eyebrow: 'HASIL UJIAN', icon: '★' },
    { path: '/admin/exam-schedule', label: 'Jadwal ujian', eyebrow: 'PENGATURAN UJIAN', icon: '◔' },
    { path: '/admin/approvals', label: 'Persetujuan KPI', eyebrow: 'ANTREAN TINJAUAN', icon: '⇄' },
    { path: '/admin/schedule', label: 'Jadwal & lokasi kantor', eyebrow: 'PENGATURAN HR', icon: '◎' },
    { path: '/admin/profile', label: 'Profil saya', eyebrow: 'DATA PRIBADI', icon: '☺' },
  ];
  const employeePages = [
    { path: '/home', label: 'Beranda', eyebrow: 'RINGKASAN SAYA', icon: '⌂' },
    ...(canUseSelfService(user.role) ? [
      { path: '/attendance', label: 'Absensi saya', eyebrow: 'KEHADIRAN', icon: '✓' },
      { path: '/leave', label: 'Cuti dan saldo', eyebrow: 'CUTI TAHUNAN', icon: '☰' },
      { path: '/requests', label: 'Izin dan lembur', eyebrow: 'LAYANAN MANDIRI', icon: '✉' },
    ] : []),
    ...(user.role !== 'finance' ? [
      { path: '/exam', label: 'Ujian KPI', eyebrow: 'UJIAN PEMAHAMAN', icon: '✎' },
      { path: '/exam-results', label: 'Nilai KPI', eyebrow: 'HASIL UJIAN', icon: '★' },
      { path: '/performance', label: 'KPI dan kinerja', eyebrow: 'KINERJA TAHUNAN', icon: '▤' },
    ] : []),
    ...(user.role === 'manager' ? [
      { path: '/approvals', label: 'Tinjauan kinerja tim', eyebrow: 'TINJAUAN TIM', icon: '⇄' },
      { path: '/team', label: 'Tim saya', eyebrow: 'STRUKTUR TIM', icon: '◍' },
    ] : []),
    { path: '/profile', label: 'Profil saya', eyebrow: 'DATA KARYAWAN', icon: '☺' },
  ];
  const navItems = isHR ? adminPages : employeePages;
  const activeRoute = navItems.some((page) => page.path === route) ? route : '/home';
  const activePage = navItems.find((page) => page.path === activeRoute) || navItems[0];

  const loadData = useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [branchRows, employeeRows] = await Promise.all([
        api('/api/v1/branches'),
        !isHR && user.role !== 'manager'
          ? Promise.resolve([])
          : api(`/api/v1/employees${isHR && branchFilter ? `?branch_id=${encodeURIComponent(branchFilter)}` : ''}`),
      ]);
      setBranches(branchRows);
      setEmployees(employeeRows);
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }, [branchFilter, isHR, user.role]);

  useEffect(() => { loadData(); }, [loadData]);
  useEffect(() => {
    const updateRoute = () => setRoute(window.location.hash.slice(1) || '/home');
    window.addEventListener('hashchange', updateRoute);
    return () => window.removeEventListener('hashchange', updateRoute);
  }, []);
  useEffect(() => { window.scrollTo({ top: 0, behavior: 'smooth' }); }, [activeRoute]);

  async function logout() {
    try {
      await api('/api/v1/auth/logout', { method: 'POST' });
    } finally {
      onLogout();
    }
  }

  const branchName = branches.find((branch) => branch.id === user.branch_id)?.name || user.branch_name;
  const pageSummary = {
    '/home': isHR ? 'Ringkasan kehadiran dan aktivitas organisasi.' : `Selamat datang kembali, ${user.full_name}.`,
    '/admin/leave': 'Kelola hak cuti tahunan bersama, permintaan, dan kalender kantor.',
    '/admin/my-attendance': 'Lihat catatan absensi pribadi dan status masuk/pulang Anda.',
    '/admin/requests': 'Ajukan dan lacak izin atau lembur; tinjau antrean sesuai kewenangan.',
    '/admin/profile': 'Periksa informasi profil pribadi dan perbarui foto Anda.',
    '/admin/performance': 'Susun target, catat progres, dan tinjau kinerja sepanjang tahun.',
    '/admin/exam': 'Kerjakan ujian KPI sesuai jadwal yang ditetapkan HR untuk divisi Anda.',
    '/admin/exam-results': 'Pantau hasil ujian karyawan dan (khusus superadmin) bank soal beserta kunci jawaban.',
    '/admin/exam-schedule': 'Tentukan sendiri waktu mulai dan selesai ujian KPI setiap divisi.',
    '/admin/approvals': 'Tinjau dan putuskan target, hasil akhir, serta keberatan KPI.',
    '/admin/attendance': 'Pantau status masuk/pulang harian dan rekap keterlambatan.',
    '/admin/schedule': 'Atur jam kerja dan titik kantor yang menjadi dasar validasi absensi.',
    '/admin/employees': 'Telusuri direktori karyawan dan informasi profil yang berwenang.',
    '/admin/provisioning': 'Buat akun baru setelah data karyawan diverifikasi HR.',
    '/attendance': 'Catat jam masuk dan pulang, lalu pantau akumulasi keterlambatan.',
    '/leave': 'Lihat hak cuti, saldo yang tersedia, dan status pengajuan.',
    '/requests': 'Ajukan izin atau lembur dan pantau status serta catatan persetujuannya.',
    '/performance': 'Lihat target, progres, dan hasil tinjauan kinerja Anda.',
    '/exam': 'Kerjakan ujian KPI divisi Anda saat jadwal dibuka HR (10 soal, nilai lulus 70).',
    '/exam-results': 'Nilai KPI hanya muncul setelah Anda menyelesaikan ujian.',
    '/approvals': 'Tinjau target dan evaluasi kinerja bawahan langsung.',
    '/team': 'Lihat daftar tim yang melapor langsung kepada Anda.',
    '/profile': 'Pastikan informasi profil dan foto Anda tetap terbaru.',
  }[activeRoute];

  return (
    <main className="app-shell">
      <header className="topbar">
        <a className="brand" href="#/home" aria-label="Human Resources Information System">
          <span className="brand-mark">H</span>
          <span>Human Resources <small>Information System</small></span>
        </a>
        <div className="account">
          <LiveClock branchName={branchName} />
          <ThemeToggle />
          <div className="account-copy"><strong>{user.full_name}</strong><span>{user.role} · {branchName}</span></div>
          <button type="button" className="secondary compact" onClick={logout}>Keluar</button>
        </div>
      </header>

      <div className="workspace-shell has-sidebar">
        <aside className="workspace-sidebar">
          <div className="sidebar-caption">{isHR ? 'ADMINISTRASI' : 'RUANG KERJA'}</div>
          <nav aria-label="Navigasi utama">
            {navItems.map((item) => (
              <a key={item.path} href={`#${item.path}`} className={`workspace-nav-link ${activeRoute === item.path ? 'is-current' : ''}`}>
                <span className="nav-icon" aria-hidden="true">{item.icon}</span>
                <span>{item.label}</span>
              </a>
            ))}
          </nav>
          <div className="sidebar-environment"><span className="status-dot" /> Lokal · WIB</div>
        </aside>

        <div className="content workspace-content">
          <section className="welcome">
            <div>
              <div className="eyebrow">{activePage.eyebrow}</div>
              <h1>{activeRoute === '/home' ? `Selamat datang, ${user.full_name}` : activePage.label}</h1>
              <p>{pageSummary}</p>
            </div>
            <div className="welcome-side">
              <div className="welcome-badge"><span className="status-dot" /> {branchName} · WIB</div>
              <div className="welcome-time">
                <span>{formatWib(new Date(), { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}</span>
                <strong>{formatWib(new Date(), { hour: '2-digit', minute: '2-digit' })} WIB</strong>
              </div>
            </div>
          </section>

          {error && <div className="alert error" role="alert">{error}</div>}

          {activeRoute === '/home' && (
            <>
              <section className="summary-grid" aria-label="Ringkasan">
                <article className="summary-card"><span>Cabang</span><strong>{isHR ? branches.length : branchName}</strong><small>{isHR ? 'Dalam cakupan HR' : 'Lokasi kerja Anda'}</small></article>
                <article className="summary-card"><span>{isHR ? 'Karyawan aktif' : 'Nomor karyawan'}</span><strong>{isHR ? (busy ? '—' : employees.length) : user.employee_number}</strong><small>{isHR ? 'Sesuai cakupan yang dipilih' : 'Profil organisasi'}</small></article>
                <article className="summary-card"><span>Peran akses</span><strong className="role-value">{user.role}</strong><small>Akses sesuai kewenangan</small></article>
              </section>
              <div className="dashboard-actions">
                {isHR && <a className="primary" href="#/admin/provisioning">+ Tambah karyawan</a>}
                <a className="secondary" href={isHR ? '#/admin/exam' : '#/exam'}>Ujian KPI</a>
                <a className="secondary" href={isHR ? '#/admin/exam-results' : '#/exam-results'}>Nilai KPI</a>
                <a className="secondary" href={isHR ? '#/admin/attendance' : '#/attendance'}>Absensi</a>
                <a className="secondary" href="#/profile">Profil & foto</a>
              </div>
              <EmployeeProfile
                profilePath="/api/v1/employees/me/profile"
                photoPath="/api/v1/employees/me/photo"
                canUpload
                canEdit={false}
              />
              {isHR && editTarget && (
                <EmployeeEditor
                  key={`edit-${editTarget}`}
                  userID={editTarget}
                  branches={branches}
                  employees={employees}
                  onSaved={loadData}
                  onClose={() => setEditTarget(null)}
                />
              )}
              {!isHR ? null : (
                <div className="page-tools">
                  <span className="eyebrow">KELOLA DATA</span>
                  <p className="muted">Ingin mengoreksi data karyawan? Buka <a href="#/admin/employees">Direktori karyawan</a> lalu pilih <strong>Detail → Edit data</strong>.</p>
                </div>
              )}
              {isHR ? (
                <AttendanceOverview user={user} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} />
              ) : (
                <section className="panel quick-links-panel">
                  <div className="section-heading"><div><div className="eyebrow">AKSES CEPAT</div><h2>Yang ingin Anda lakukan?</h2></div></div>
                  <div className="quick-link-grid">
                    {navItems.filter((item) => item.path !== '/home').slice(0, 4).map((item, index) => (
                      <a className="quick-link-card" href={`#${item.path}`} key={item.path}>
                        <span className="quick-link-number">{String(index + 1).padStart(2, '0')}</span><strong>{item.label}</strong><span>Lihat dan kelola informasi</span>
                      </a>
                    ))}
                  </div>
                </section>
              )}
            </>
          )}

          {activeRoute === '/admin/leave' && isHR && (
            <>
              <LeaveSettings />
              <LeavePanel user={user} />
            </>
          )}
          {activeRoute === '/admin/leave' && !isHR && <LeavePanel user={user} />}

          {(activeRoute === '/admin/performance' || activeRoute === '/performance') && user.role !== 'finance' && (
            <KPIWorkspace user={user} employees={employees} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} view="review" />
          )}

          {(activeRoute === '/admin/exam' || activeRoute === '/exam') && user.role !== 'finance' && (
            <KPIExam user={user} />
          )}
          {(activeRoute === '/admin/exam-results' || activeRoute === '/exam-results') && isHR && (
            <KPIExamResults user={user} />
          )}
          {(activeRoute === '/admin/exam-results' || activeRoute === '/exam-results') && !isHR && user.role !== 'finance' && (
            <ExamResultPanel user={user} />
          )}
          {activeRoute === '/admin/exam-schedule' && isHR && (
            <ExamScheduleEditor branchID={branchFilter} onSaved={loadData} />
          )}

          {(activeRoute === '/admin/approvals' || activeRoute === '/approvals') && (
            <KPIWorkspace user={user} employees={employees} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} view="approvals" />
          )}

          {(activeRoute === '/attendance' || activeRoute === '/admin/my-attendance') && canUseSelfService(user.role) && (
            <>
              <AttendancePanel />
              {user.role === 'manager' && (
                <>
                  <AttendanceOverview user={user} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} />
                  <AttendanceReport user={user} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} />
                </>
              )}
            </>
          )}
          {activeRoute === '/admin/attendance' && (
            <>
              {canUseSelfService(user.role) && <AttendancePanel />}
              <AttendanceOverview user={user} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} />
              <AttendanceReport user={user} branches={branches} branchID={branchFilter} onBranchChange={setBranchFilter} />
            </>
          )}

          {(activeRoute === '/admin/schedule') && isHR && <AttendanceSettings />}
          {activeRoute === '/leave' && canUseSelfService(user.role) && <LeavePanel user={user} />}
          {(activeRoute === '/requests' || activeRoute === '/admin/requests') &&
            (canUseSelfService(user.role) || user.role === 'superadmin') && <EmployeeRequestsPanel user={user} />}
          {(activeRoute === '/profile' || activeRoute === '/admin/profile') && (
            <>
              <EmployeeProfile
                profilePath="/api/v1/employees/me/profile"
                photoPath="/api/v1/employees/me/photo"
                canUpload
                canEdit={false}
              />
              {isHR && editTarget && (
                <EmployeeEditor
                  key={`edit-${editTarget}`}
                  userID={editTarget}
                  branches={branches}
                  employees={employees}
                  onSaved={loadData}
                  onClose={() => setEditTarget(null)}
                />
              )}
            </>
          )}

          {(activeRoute === '/admin/employees' || activeRoute === '/team') && (isHR || user.role === 'manager') && (
            <>
              <section className="panel">
                <div className="section-heading">
                  <div><div className="eyebrow">{isHR ? 'DIREKTORI ORGANISASI' : 'TIM LANGSUNG'}</div><h2>{isHR ? 'Daftar karyawan' : 'Tim saya'}</h2></div>
                  {isHR && <label className="inline-filter">Cabang
                    <select value={branchFilter} onChange={(event) => { setBranchFilter(event.target.value); setSelectedProfile(null); }}>
                      <option value="">Semua cabang</option>
                      {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
                    </select>
                  </label>}
                </div>
                <div className="table-wrap">
                  <table>
                    <thead><tr><th>Nama karyawan</th><th>Email</th><th>Cabang</th><th>Divisi</th><th>Jabatan</th><th>Grade</th><th>Role</th><th>Status</th><th>Detail</th></tr></thead>
                    <tbody>
                      {busy ? <tr><td colSpan="9" className="empty">Memuat data karyawan...</td></tr>
                        : employees.length === 0 ? <tr><td colSpan="9" className="empty">Belum ada karyawan pada daftar ini.</td></tr>
                          : employees.map((employee) => (
                            <tr key={employee.user_id}>
                              <td><strong>{employee.full_name}</strong><small className="subtext">{employee.employee_number}</small></td>
                              <td>{employee.email}</td><td>{employee.branch_name}</td>
                              <td>{employee.department || '—'}</td><td>{employee.position || '—'}</td><td>{employee.job_grade || '—'}</td>
                              <td><span className="role-pill">{employee.role}</span></td>
                              <td><span className={`active-pill ${employee.active ? 'is-active' : 'is-inactive'}`}>{employee.active ? 'Aktif' : 'Nonaktif'}</span></td>
                              <td><div className="row-actions">
                                <button type="button" className="secondary compact" onClick={() => setSelectedProfile(employee.user_id)}>Detail</button>
                                {isHR && <button type="button" className="secondary compact" onClick={() => setEditTarget(employee.user_id)}>Edit</button>}
                              </div></td>
                            </tr>
                          ))}
                    </tbody>
                  </table>
                </div>
              </section>
              {selectedProfile && (
                <EmployeeProfile
                  key={selectedProfile}
                  profilePath={`/api/v1/employees/${selectedProfile}/profile`}
                  photoPath={`/api/v1/employees/${selectedProfile}/photo`}
                  canUpload={isHR}
                  canEdit={isHR}
                  onEdit={(userID) => setEditTarget(userID)}
                  onClose={() => setSelectedProfile(null)}
                />
              )}
              {isHR && editTarget && (
                <EmployeeEditor
                  key={`edit-${editTarget}`}
                  userID={editTarget}
                  branches={branches}
                  employees={employees}
                  onSaved={loadData}
                  onClose={() => setEditTarget(null)}
                />
              )}
            </>
          )}

          {activeRoute === '/admin/provisioning' && isHR && (
            <EmployeeProvisioning
              branches={branches}
              branchID={branchFilter || branches[0]?.id || ''}
              employees={employees}
              onCreated={loadData}
              isSuperadmin={user.role === 'superadmin'}
            />
          )}

          {user.role === 'finance' && activeRoute !== '/home' && activeRoute !== '/profile' && (
            <section className="panel"><h2>Akses finance</h2><p className="muted">Akses laporan finance akan ditambahkan pada tahap laporan. Data karyawan pribadi tidak tersedia untuk role ini.</p></section>
          )}

          <footer className="app-footer">Human Resources Information System · Lingkungan lokal · Data diproses sesuai peran akses</footer>
        </div>
      </div>
    </main>
  );
}

function App() {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let active = true;
    api('/api/v1/auth/me')
      .then((profile) => { if (active) setUser(profile); })
      .catch(() => { if (active) setUser(null); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [refreshKey]);

  if (loading) {
    return <main className="page"><section className="card"><p className="muted">Memeriksa sesi...</p></section></main>;
  }
  if (!user) return <Login onLogin={setUser} />;
  if (user.must_change_password) {
    return <ChangePassword onChanged={() => setRefreshKey((key) => key + 1)} onLogout={() => { setUser(null); setRefreshKey((key) => key + 1); }} />;
  }
  return (
    <Dashboard
      user={user}
      onLogout={() => { setUser(null); setRefreshKey((key) => key + 1); }}
    />
  );
}

createRoot(document.getElementById('root')).render(
  <React.StrictMode><App /></React.StrictMode>,
);
