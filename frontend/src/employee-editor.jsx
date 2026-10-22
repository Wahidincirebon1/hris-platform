import React, { useCallback, useEffect, useState } from 'react';
import { api, formatWib } from './shared.js';

// Form edit data karyawan. Hanya dipakai HR/superadmin (backend tetap memverifikasi).
export function EmployeeEditor({ userID, branches, employees, onClose, onSaved }) {
  const [form, setForm] = useState(null);
  const [departments, setDepartments] = useState([]);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    api(`/api/v1/employees/${userID}/profile`)
      .then((profile) => {
        if (!active) return;
        setForm({
          full_name: profile.full_name || '',
          birth_date: profile.birth_date || '',
          nik: profile.nik || '',
          department_id: '',
          job_grade: profile.job_grade || '',
          position_title: profile.position_title || '',
          email: profile.email,
          phone: profile.phone || '',
          address: profile.address || '',
          branch_id: profile.branch_id,
          role: profile.role,
          manager_id: '',
          active: profile.employment_status === 'active',
          department_name: profile.department || '',
        });
      })
      .catch((requestError) => { if (active) setError(requestError.message); });
    return () => { active = false; };
  }, [userID]);

  useEffect(() => {
    if (!form?.branch_id) return undefined;
    let active = true;
    api(`/api/v1/departments?branch_id=${encodeURIComponent(form.branch_id)}`)
      .then((rows) => {
        if (!active) return;
        setDepartments(rows);
        setForm((current) => {
          if (!current || current.department_id) return current;
          const match = rows.find((row) => row.name === current.department_name);
          return match ? { ...current, department_id: match.id } : current;
        });
      })
      .catch((requestError) => { if (active) setError(requestError.message); });
    return () => { active = false; };
  }, [form?.branch_id]);

  const managers = employees.filter(
    (employee) => employee.role === 'manager' && employee.active && employee.branch_id === form?.branch_id && employee.user_id !== userID,
  );

  const update = (field, value) => setForm((current) => ({ ...current, [field]: value }));

  const submit = useCallback(async (event) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/employees/${userID}`, {
        method: 'PATCH',
        body: JSON.stringify({
          full_name: form.full_name,
          birth_date: form.birth_date || null,
          nik: form.nik || null,
          phone: form.phone || null,
          address: form.address || null,
          job_grade: form.job_grade || null,
          department_id: form.department_id || null,
          position_title: form.position_title || null,
          branch_id: form.branch_id,
          manager_id: form.manager_id || null,
          role: form.role,
          active: form.active,
        }),
      });
      setNotice('Data karyawan berhasil diperbarui.');
      onSaved?.();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }, [form, onSaved, userID]);

  if (!form) {
    return <section className="panel"><p className="muted">{error || 'Memuat data karyawan...'}</p></section>;
  }

  return (
    <section className="panel employee-editor-panel">
      <div className="section-heading">
        <div><div className="eyebrow">EDIT DATA KARYAWAN</div><h2>{form.full_name || 'Perbarui data karyawan'}</h2></div>
        {onClose && <button type="button" className="secondary compact" onClick={onClose}>Tutup</button>}
      </div>
      <p className="muted">Gunakan formulir ini bila ada kesalahan input saat pembuatan akun. Setiap perubahan dicatat di audit log.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      <form className="employee-form" onSubmit={submit}>
        <label>Nama lengkap<input value={form.full_name} onChange={(event) => update('full_name', event.target.value)} required maxLength={150} /></label>
        <label>Tanggal lahir<input type="date" value={form.birth_date} onChange={(event) => update('birth_date', event.target.value)} /></label>
        <label>NIK<input value={form.nik} onChange={(event) => update('nik', event.target.value)} maxLength={64} /></label>
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
        <label>Posisi / jabatan<input value={form.position_title} onChange={(event) => update('position_title', event.target.value)} maxLength={150} /></label>
        <label>Email<input value={form.email} disabled title="Perubahan email masuk melalui kebijakan akun" /></label>
        <label>Nomor telepon<input type="tel" value={form.phone} onChange={(event) => update('phone', event.target.value)} maxLength={40} /></label>
        <label>Alamat<input value={form.address} onChange={(event) => update('address', event.target.value)} maxLength={500} /></label>
        <label>Cabang
          <select value={form.branch_id} onChange={(event) => update('branch_id', event.target.value)} required>
            {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
          </select>
        </label>
        <label>Role akses
          <select value={form.role} onChange={(event) => update('role', event.target.value)}>
            <option value="employee">Karyawan</option>
            <option value="manager">Atasan / manajer</option>
            <option value="finance">Finance</option>
            <option value="hr">HR</option>
          </select>
        </label>
        <label>Atasan langsung
          <select value={form.manager_id} onChange={(event) => update('manager_id', event.target.value)}>
            <option value="">Belum ditentukan</option>
            {managers.map((manager) => <option key={manager.user_id} value={manager.user_id}>{manager.full_name}</option>)}
          </select>
        </label>
        <label>Status akun
          <select value={form.active ? 'active' : 'inactive'} onChange={(event) => update('active', event.target.value === 'active')}>
            <option value="active">Aktif</option>
            <option value="inactive">Nonaktif</option>
          </select>
        </label>
        <div className="form-actions">
          <button className="primary" disabled={busy}>{busy ? 'Menyimpan...' : 'Simpan perubahan'}</button>
        </div>
      </form>
    </section>
  );
}

export function formatUpdated(value) {
  return value ? formatWib(value, { dateStyle: 'medium', timeStyle: 'short' }) : '—';
}
