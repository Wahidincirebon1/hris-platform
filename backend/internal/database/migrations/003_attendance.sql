CREATE TABLE attendance_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    effective_from DATE NOT NULL UNIQUE,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta'
        CHECK (timezone = 'Asia/Jakarta'),
    work_start TIME NOT NULL DEFAULT '08:30',
    work_end TIME NOT NULL DEFAULT '18:00',
    break_start TIME NOT NULL DEFAULT '12:00',
    break_end TIME NOT NULL DEFAULT '13:00',
    working_days SMALLINT[] NOT NULL DEFAULT ARRAY[1, 2, 3, 4, 5]::SMALLINT[],
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (work_start < break_start AND break_start < break_end AND break_end < work_end)
);

INSERT INTO attendance_schedules (effective_from)
VALUES ('2000-01-01');

CREATE TABLE attendance_geofences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id UUID NOT NULL REFERENCES branches(id),
    effective_from DATE NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_meters INTEGER NOT NULL DEFAULT 50
        CHECK (radius_meters BETWEEN 25 AND 2000),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (branch_id, effective_from)
);

CREATE INDEX idx_attendance_geofences_effective
    ON attendance_geofences (branch_id, effective_from DESC);

CREATE TABLE attendance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    attendance_date DATE NOT NULL,
    schedule_id UUID NOT NULL REFERENCES attendance_schedules(id),
    check_in_geofence_id UUID NOT NULL REFERENCES attendance_geofences(id),
    check_in_at TIMESTAMPTZ NOT NULL,
    check_in_distance_meters DOUBLE PRECISION NOT NULL CHECK (check_in_distance_meters >= 0),
    check_in_accuracy_meters DOUBLE PRECISION NOT NULL CHECK (check_in_accuracy_meters > 0),
    late BOOLEAN NOT NULL DEFAULT FALSE,
    late_minutes INTEGER NOT NULL DEFAULT 0 CHECK (late_minutes >= 0),
    check_out_geofence_id UUID REFERENCES attendance_geofences(id),
    check_out_at TIMESTAMPTZ,
    check_out_distance_meters DOUBLE PRECISION CHECK (check_out_distance_meters >= 0),
    check_out_accuracy_meters DOUBLE PRECISION CHECK (check_out_accuracy_meters > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, branch_id) REFERENCES users(id, branch_id),
    UNIQUE (user_id, attendance_date),
    CHECK (
        (check_out_at IS NULL AND check_out_geofence_id IS NULL
            AND check_out_distance_meters IS NULL AND check_out_accuracy_meters IS NULL)
        OR
        (check_out_at IS NOT NULL AND check_out_at >= check_in_at
            AND (
                (check_out_geofence_id IS NULL AND check_out_distance_meters IS NULL
                    AND check_out_accuracy_meters IS NULL)
                OR
                (check_out_geofence_id IS NOT NULL AND check_out_distance_meters IS NOT NULL
                    AND check_out_accuracy_meters IS NOT NULL)
            ))
    )
);

CREATE INDEX idx_attendance_records_branch_date
    ON attendance_records (branch_id, attendance_date DESC);
CREATE INDEX idx_attendance_records_user_date
    ON attendance_records (user_id, attendance_date DESC);
