import React, { useCallback, useEffect, useState } from 'react';
import { api, formatWib } from './shared.js';

export function ExamScheduleEditor({ branchID, onSaved }) {
  const [rows, setRows] = useState([]);
  const [drafts, setDrafts] = useState({});
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');

  const loadRows = useCallback(async () => {
    setError('');
    try {
      const data = await api('/api/v1/exam/schedules');
      setRows(data);
      setDrafts((current) => {
        const next = { ...current };
        data.forEach((row) => {
          if (!next[row.department_id] && row.scheduled) {
            next[row.department_id] = {
              title: row.title || '',
              starts_at: toLocalInput(row.starts_at),
              ends_at: toLocalInput(row.ends_at),
            };
          }
          if (!next[row.department_id]) {
            next[row.department_id] = { title: '', starts_at: '', ends_at: '' };
          }
        });
        return next;
      });
    } catch (requestError) {
      setError(requestError.message);
    }
  }, []);

  useEffect(() => { loadRows(); }, [loadRows]);

  function update(departmentID, field, value) {
    setDrafts((current) => ({ ...current, [departmentID]: { ...current[departmentID], [field]: value } }));
  }

  async function save(event, row) {
    event.preventDefault();
    const draft = drafts[row.department_id] || {};
    setBusy(row.department_id);
    setError('');
    setNotice('');
    try {
      await api(`/api/v1/exam/schedules/${row.department_id}`, {
        method: 'PUT',
        body: JSON.stringify({
          title: draft.title || `Ujian KPI ${row.division_name}`,
          starts_at: new Date(draft.starts_at).toISOString(),
          ends_at: new Date(draft.ends_at).toISOString(),
        }),
      });
      setNotice(`Jadwal ujian divisi ${row.division_name} tersimpan.`);
      await loadRows();
      onSaved?.();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy('');
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">PENGATURAN UJIAN KPI</div><h2>Jadwal ujian per divisi</h2></div>
        <span className="role-pill">{rows.length} divisi</span>
      </div>
      <p className="muted">HR menentukan sendiri kapan ujian dibuka dan ditutup pada setiap divisi. Karyawan tidak dapat membuka ujian sebelum waktu mulai atau setelah waktu selesai (dijaga di server), dan hanya boleh mengerjakan satu kali.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}
      {rows.length === 0 && <p className="muted">Memuat divisi...</p>}
      <div className="exam-schedule-grid">
        {rows.map((row) => {
          const draft = drafts[row.department_id] || { title: '', starts_at: '', ends_at: '' };
          const status = !row.scheduled ? 'Belum dijadwalkan'
            : new Date(row.starts_at) > new Date() ? 'Akan dibuka'
              : new Date(row.ends_at) < new Date() ? 'Sudah ditutup' : 'Sedang dibuka';
          return (
            <form className="exam-schedule-card" key={row.department_id} onSubmit={(event) => save(event, row)}>
              <div className="section-heading">
                <div><h3>{row.division_name}</h3><small className="subtext">{row.question_count} soal tersedia</small></div>
                <span className={`active-pill ${status === 'Sedang dibuka' ? 'is-active' : 'is-inactive'}`}>{status}</span>
              </div>
              <label>Judul ujian<input value={draft.title} onChange={(event) => update(row.department_id, 'title', event.target.value)} maxLength={150} placeholder={`Ujian KPI ${row.division_name}`} /></label>
              <label>Mulai (WIB)<input type="datetime-local" value={draft.starts_at} onChange={(event) => update(row.department_id, 'starts_at', event.target.value)} required /></label>
              <label>Selesai (WIB)<input type="datetime-local" value={draft.ends_at} onChange={(event) => update(row.department_id, 'ends_at', event.target.value)} required /></label>
              <button className="primary compact" disabled={Boolean(busy)}>{busy === row.department_id ? 'Menyimpan...' : row.scheduled ? 'Perbarui jadwal' : 'Aktifkan ujian'}</button>
            </form>
          );
        })}
      </div>
    </section>
  );
}

function toLocalInput(value) {
  if (!value) return '';
  const date = new Date(value);
  const pad = (part) => String(part).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function ExamCountdown({ target, onExpire }) {
  const [remaining, setRemaining] = useState(() => Math.max(0, new Date(target).getTime() - Date.now()));
  useEffect(() => {
    const timer = window.setInterval(() => {
      const next = Math.max(0, new Date(target).getTime() - Date.now());
      setRemaining(next);
      if (next === 0) onExpire?.();
    }, 1000);
    return () => window.clearInterval(timer);
  }, [target, onExpire]);
  const minutes = Math.floor(remaining / 60000);
  const seconds = Math.floor((remaining % 60000) / 1000);
  return <strong className={minutes < 5 ? 'exam-time is-low' : 'exam-time'}>{String(minutes).padStart(2, '0')}:{String(seconds).padStart(2, '0')}</strong>;
}

export function KPIExam({ user }) {
  const [status, setStatus] = useState(null);
  const [questions, setQuestions] = useState([]);
  const [answers, setAnswers] = useState({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const isHR = ['hr', 'superadmin'].includes(user.role);

  const loadStatus = useCallback(async () => {
    setError('');
    try {
      setStatus(await api('/api/v1/exam/me'));
    } catch (requestError) {
      setError(requestError.message);
    }
  }, []);

  useEffect(() => { loadStatus(); }, [loadStatus]);

  async function openExam() {
    setError('');
    setNotice('');
    setBusy(true);
    try {
      setQuestions(await api('/api/v1/exam/questions'));
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  async function submitExam(event) {
    event.preventDefault();
    const unanswered = questions.filter((question) => answers[question.id] === undefined);
    if (unanswered.length > 0) {
      setError(`Masih ada ${unanswered.length} soal belum dijawab.`);
      return;
    }
    setBusy(true);
    setError('');
    try {
      const result = await api('/api/v1/exam/submit', { method: 'POST', body: JSON.stringify({ answers }) });
      setQuestions([]);
      setAnswers({});
      setNotice(result.passed
        ? `Selamat! Anda lulus ujian KPI dengan nilai ${result.score_percent}.`
        : `Nilai Anda ${result.score_percent}, belum mencapai batas lulus ${result.passing_percent}.`);
      await loadStatus();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  if (!status) {
    return <section className="panel"><p className="muted">{error || 'Memuat status ujian KPI...'}</p></section>;
  }

  const schedule = status.schedule;
  const stateMessage = {
    not_scheduled: 'Jadwal ujian divisi Anda belum ditentukan HR. Tombol ujian akan aktif setelah HR mengatur waktunya.',
    not_open: 'Ujian belum dibuka. Ikuti jadwal yang ditetapkan HR di bawah.',
    closed: 'Waktu ujian divisi Anda sudah berakhir.',
    open: 'Ujian sedang dibuka. Kerjakan seluruh soal sebelum waktu selesai.',
  }[status.state];

  return (
    <section className="panel exam-panel">
      <div className="section-heading">
        <div><div className="eyebrow">UJIAN KPI</div><h2>Tes pemahaman KPI — {status.division_name || 'divisi belum ditentukan'}</h2></div>
        <span className="welcome-badge"><span className="status-dot" /> Nilai lulus {status.passing_percent}</span>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {notice && <div className="alert success" role="status">{notice}</div>}

      <div className="exam-meta-grid">
        <div><span>Divisi</span><strong>{status.division_name || '—'}</strong></div>
        <div><span>Jumlah soal</span><strong>{schedule?.question_count ?? 0}</strong></div>
        <div><span>Dibuka</span><strong>{schedule ? formatWib(schedule.starts_at, { dateStyle: 'medium', timeStyle: 'short' }) : 'Belum dijadwalkan'}</strong></div>
        <div><span>Ditutup</span><strong>{schedule ? formatWib(schedule.ends_at, { dateStyle: 'medium', timeStyle: 'short' }) : 'Belum dijadwalkan'}</strong></div>
      </div>

      {status.attempted && (
        <div className="exam-result-banner">
          <div>
            <div className="eyebrow">HASIL UJIAN</div>
            <h3>{status.attempt.score_percent} / 100</h3>
            <p>{status.attempt.correct_count} benar dari {status.attempt.total_count} soal · {status.attempt.division_name}</p>
          </div>
          <span className={`exam-result-pill ${status.attempt.passed ? 'is-pass' : 'is-fail'}`}>{status.attempt.passed ? 'LULUS' : 'GAGAL'}</span>
        </div>
      )}

      {!status.attempted && (
        <>
          <div className={`alert ${status.state === 'open' ? 'success' : 'info'}`} role="status">{stateMessage}</div>
          {schedule && status.state === 'open' && (
            <div className="exam-start-row">
              <div><span className="eyebrow">SISA WAKTU</span>
                <ExamCountdown target={schedule.ends_at} onExpire={() => { setQuestions([]); loadStatus(); }} />
              </div>
              <button type="button" className="primary" disabled={busy || questions.length > 0} onClick={openExam}>
                {questions.length > 0 ? 'Ujian sedang berjalan' : busy ? 'Membuka soal...' : 'Mulai ujian'}
              </button>
            </div>
          )}

          {questions.length > 0 && (
            <form className="exam-form" onSubmit={submitExam}>
              {questions.map((question, index) => (
                <div className="exam-question" key={question.id}>
                  <div className="exam-question-head">
                    <span className="quick-link-number">{String(index + 1).padStart(2, '0')}</span>
                    <h3>{question.question}</h3>
                  </div>
                  {question.options.map((option, optionIndex) => (
                    <label key={optionIndex} className={`exam-option ${answers[question.id] === optionIndex ? 'is-selected' : ''}`}>
                      <input
                        type="radio"
                        name={`question-${question.id}`}
                        checked={answers[question.id] === optionIndex}
                        onChange={() => setAnswers((current) => ({ ...current, [question.id]: optionIndex }))}
                      />
                      <span>{option}</span>
                    </label>
                  ))}
                </div>
              ))}
              <div className="exam-submit-row">
                <span className="muted">{Object.keys(answers).length} dari {questions.length} soal dijawab</span>
                <button className="primary" disabled={busy}>{busy ? 'Menyimpan nilai...' : 'Kirim jawaban'}</button>
              </div>
            </form>
          )}
        </>
      )}

      {isHR && (
        <div className="exam-hr-notes">
          <div className="eyebrow">CATATAN HR</div>
          <p className="muted">Hasil seluruh karyawan tersedia di halaman <a href="#/admin/exam-results">Nilai KPI</a>. Jadwal ujian diatur di halaman <a href="#/admin/exam-schedule">Jadwal ujian</a>.</p>
        </div>
      )}
    </section>
  );
}

export function KPIExamResults({ user }) {
  const isSuperadmin = user.role === 'superadmin';
  const [attempts, setAttempts] = useState([]);
  const [allQuestions, setAllQuestions] = useState([]);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    Promise.all([
      api('/api/v1/exam/attempts'),
      isSuperadmin ? api('/api/v1/exam/all-questions') : Promise.resolve([]),
    ]).then(([attemptRows, questionRows]) => {
      if (!active) return;
      setAttempts(attemptRows);
      setAllQuestions(questionRows);
    }).catch((requestError) => { if (active) setError(requestError.message); });
    return () => { active = false; };
  }, [isSuperadmin]);

  const byDivision = allQuestions.reduce((groups, question) => {
    const key = question.division_name || 'Tanpa divisi';
    (groups[key] = groups[key] || []).push(question);
    return groups;
  }, {});
  const passed = attempts.filter((attempt) => attempt.passed).length;

  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">NILAI KPI</div><h2>Hasil ujian karyawan</h2></div>
        <span className="role-pill">{attempts.length} peserta</span>
      </div>
      <p className="muted">Hanya peserta yang sudah menyelesaikan ujian yang muncul di halaman ini.</p>
      {error && <div className="alert error" role="alert">{error}</div>}
      <div className="summary-grid attendance-overview-stats">
        <article className="summary-card"><span>Peserta ujian</span><strong>{attempts.length}</strong><small>Seluruh divisi</small></article>
        <article className="summary-card"><span>Lulus</span><strong>{passed}</strong><small>Nilai minimal 70</small></article>
        <article className="summary-card"><span>Belum lulus</span><strong>{attempts.length - passed}</strong><small>Perlu tindak lanjut HR</small></article>
      </div>
      <div className="table-wrap">
        <table>
          <thead><tr><th>Karyawan</th><th>Divisi</th><th>Nilai</th><th>Benar</th><th>Status</th><th>Waktu</th></tr></thead>
          <tbody>
            {attempts.length === 0
              ? <tr><td colSpan="6" className="empty">Belum ada karyawan yang menyelesaikan ujian.</td></tr>
              : attempts.map((attempt) => (
                <tr key={attempt.id}>
                  <td><strong>{attempt.full_name}</strong><small className="subtext">{attempt.employee_number}</small></td>
                  <td>{attempt.division_name || '—'}</td>
                  <td><strong>{attempt.score_percent}</strong></td>
                  <td>{attempt.correct_count}/{attempt.total_count}</td>
                  <td><span className={`active-pill ${attempt.passed ? 'is-active' : 'is-inactive'}`}>{attempt.passed ? 'Lulus' : 'Gagal'}</span></td>
                  <td>{formatWib(attempt.submitted_at, { dateStyle: 'medium', timeStyle: 'short' })}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>

      {isSuperadmin && (
        <div className="exam-answer-key">
          <div className="section-heading"><div><div className="eyebrow">KHUSUS SUPERADMIN</div><h3>Bank soal & kunci jawaban</h3></div></div>
          <p className="muted">Menampilkan {allQuestions.length} soal aktif untuk {Object.keys(byDivision).length} divisi.</p>
          {Object.entries(byDivision).map(([division, items]) => (
            <details className="exam-answer-group" key={division} open={false}>
              <summary>{division} · {items.length} soal</summary>
              <ol>
                {items.map((question) => (
                  <li key={question.id}>
                    <strong>{question.question}</strong>
                    <ul>
                      {question.options.map((option, optionIndex) => (
                        <li key={optionIndex} className={optionIndex === question.correct_index ? 'is-correct' : ''}>
                          {optionIndex === question.correct_index ? '✓ ' : ''}{option}
                        </li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ol>
            </details>
          ))}
        </div>
      )}
    </section>
  );
}


// Karyawan/manager: nilai hanya tampil setelah menyelesaikan ujian.
export function ExamResultPanel({ user }) {
  const [status, setStatus] = useState(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    api('/api/v1/exam/me')
      .then((data) => { if (active) setStatus(data); })
      .catch((requestError) => { if (active) setError(requestError.message); });
    return () => { active = false; };
  }, [user.id]);

  if (!status) {
    return <section className="panel"><p className="muted">{error || 'Memuat nilai KPI...'}</p></section>;
  }

  const schedule = status.schedule;
  return (
    <section className="panel">
      <div className="section-heading">
        <div><div className="eyebrow">NILAI KPI</div><h2>Hasil ujian KPI saya</h2></div>
        <span className="role-pill">{status.division_name || 'Divisi belum ditentukan'}</span>
      </div>
      {error && <div className="alert error" role="alert">{error}</div>}
      {!status.attempted ? (
        <>
          <div className="alert info" role="status">Nilai akan ditampilkan setelah Anda menyelesaikan ujian KPI. Ujian belum dikerjakan.</div>
          <div className="exam-meta-grid">
            <div><span>Jadwal dibuka</span><strong>{schedule ? formatWib(schedule.starts_at, { dateStyle: 'medium', timeStyle: 'short' }) : 'Belum dijadwalkan HR'}</strong></div>
            <div><span>Jadwal ditutup</span><strong>{schedule ? formatWib(schedule.ends_at, { dateStyle: 'medium', timeStyle: 'short' }) : 'Belum dijadwalkan HR'}</strong></div>
            <div><span>Nilai lulus</span><strong>{status.passing_percent}</strong></div>
            <div><span>Jumlah soal</span><strong>{schedule?.question_count ?? 0}</strong></div>
          </div>
          <div className="exam-submit-row">
            <span className="muted">Kerjakan ujian pada halaman Ujian KPI saat jadwal dibuka.</span>
            <a className="primary" href="#/exam">Buka ujian KPI</a>
          </div>
        </>
      ) : (
        <div className="exam-result-banner">
          <div>
            <div className="eyebrow">HASIL UJIAN</div>
            <h3>{status.attempt.score_percent} / 100</h3>
            <p>{status.attempt.correct_count} benar dari {status.attempt.total_count} soal · {status.attempt.division_name}</p>
            <p className="muted">Dikerjakan {formatWib(status.attempt.submitted_at, { dateStyle: 'long', timeStyle: 'short' })} WIB</p>
          </div>
          <span className={`exam-result-pill ${status.attempt.passed ? 'is-pass' : 'is-fail'}`}>{status.attempt.passed ? 'LULUS' : 'GAGAL'}</span>
        </div>
      )}
    </section>
  );
}
