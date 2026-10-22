ALTER TABLE employees
    ADD COLUMN birth_date DATE,
    ADD COLUMN gender TEXT CHECK (gender IS NULL OR gender IN ('L', 'P'));

-- Seed 17 divisions (professional naming, ordered by function)
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
WHERE b.code = 'BDG'
ON CONFLICT (branch_id, name) DO NOTHING;
