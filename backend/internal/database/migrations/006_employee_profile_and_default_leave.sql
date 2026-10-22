ALTER TABLE employees
    ADD COLUMN job_grade TEXT CHECK (job_grade IS NULL OR length(trim(job_grade)) <= 80),
    ADD COLUMN profile_photo_jpeg BYTEA CHECK (
        profile_photo_jpeg IS NULL OR octet_length(profile_photo_jpeg) <= 524288
    ),
    ADD COLUMN profile_photo_updated_at TIMESTAMPTZ;

INSERT INTO leave_entitlements (leave_year, annual_days, reason)
VALUES (
    EXTRACT(YEAR FROM timezone('Asia/Jakarta', now()))::INTEGER,
    12,
    'Default awal 12 hari; dapat disesuaikan HR.'
)
ON CONFLICT (leave_year) DO NOTHING;

INSERT INTO leave_ledger_entries (
    user_id, leave_year, entry_type, amount, reason
)
SELECT u.id, policy.leave_year, 'annual_grant', policy.annual_days,
       'Pemberian hak cuti tahunan default 12 hari.'
FROM users u
JOIN employees e ON e.user_id = u.id
CROSS JOIN leave_entitlements policy
WHERE u.active
  AND e.employment_status = 'active'
  AND policy.leave_year = EXTRACT(YEAR FROM timezone('Asia/Jakarta', now()))::INTEGER
ON CONFLICT (user_id, leave_year) WHERE entry_type = 'annual_grant' DO NOTHING;
