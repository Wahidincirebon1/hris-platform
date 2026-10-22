-- Cabang Jakarta (Kantor Pusat) beserta 17 divisi yang sama dengan cabang lain.
-- Koordinat area default memakai kawasan Monas, Jakarta Pusat; HR dapat
-- menggantinya kapan saja melalui menu "Jadwal dan lokasi kantor".
INSERT INTO branches (code, name, timezone, geofence_radius_meters, active)
VALUES ('JKT', 'Jakarta (Kantor Pusat)', 'Asia/Jakarta', 300, TRUE)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name, timezone = EXCLUDED.timezone, active = TRUE;

INSERT INTO departments (branch_id, name)
SELECT b.id, v.name
FROM branches b
CROSS JOIN (VALUES
    ('Information Technology'),
    ('Infrastructure'),
    ('Networking'),
    ('DevOps'),
    ('Programmer'),
    ('IT Security'),
    ('Technician'),
    ('Accounting'),
    ('Tax'),
    ('Purchasing'),
    ('Project Administration'),
    ('B2B Sales'),
    ('SMB Sales'),
    ('Sales Management'),
    ('IT Management'),
    ('Messenger'),
    ('Office Boy')
) AS v(name)
WHERE b.code = 'JKT'
ON CONFLICT (branch_id, name) DO NOTHING;
