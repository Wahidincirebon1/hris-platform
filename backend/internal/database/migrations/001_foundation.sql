CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    geofence_latitude DOUBLE PRECISION,
    geofence_longitude DOUBLE PRECISION,
    geofence_radius_meters INTEGER NOT NULL DEFAULT 50
        CHECK (geofence_radius_meters BETWEEN 25 AND 2000),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (geofence_latitude IS NULL AND geofence_longitude IS NULL)
        OR (
            geofence_latitude BETWEEN -90 AND 90
            AND geofence_longitude BETWEEN -180 AND 180
        )
    )
);

CREATE TABLE departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id UUID NOT NULL REFERENCES branches(id),
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (branch_id, name)
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('employee', 'manager', 'hr', 'finance', 'superadmin')),
    branch_id UUID NOT NULL REFERENCES branches(id),
    manager_id UUID REFERENCES users(id),
    active BOOLEAN NOT NULL DEFAULT FALSE,
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (manager_id IS NULL OR manager_id <> id)
);

CREATE TABLE employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id),
    employee_number TEXT NOT NULL UNIQUE,
    full_name TEXT NOT NULL,
    nik TEXT UNIQUE,
    phone TEXT,
    address TEXT,
    department_id UUID REFERENCES departments(id),
    position_title TEXT,
    employment_status TEXT NOT NULL DEFAULT 'active'
        CHECK (employment_status IN ('active', 'inactive', 'archived')),
    hired_on DATE,
    ended_on DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_on IS NULL OR hired_on IS NULL OR ended_on >= hired_on)
);

CREATE TABLE audit_logs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_branch_role ON users (branch_id, role) WHERE active;
CREATE INDEX idx_users_manager ON users (manager_id) WHERE manager_id IS NOT NULL;
CREATE INDEX idx_employees_department ON employees (department_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at DESC);
CREATE INDEX idx_audit_logs_entity ON audit_logs (entity_type, entity_id, created_at DESC);

INSERT INTO branches (code, name) VALUES
    ('BDG', 'Bandung'),
    ('SBY', 'Surabaya')
ON CONFLICT (code) DO NOTHING;
