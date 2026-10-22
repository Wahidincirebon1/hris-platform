ALTER TABLE leave_requests
    DROP CONSTRAINT leave_requests_status_check;

ALTER TABLE leave_requests
    ADD CONSTRAINT leave_requests_status_check
    CHECK (status IN (
        'pending_manager', 'pending_hr', 'pending_superadmin',
        'approved', 'rejected', 'cancelled'
    )),
    ADD COLUMN superadmin_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN superadmin_decision_at TIMESTAMPTZ,
    ADD COLUMN superadmin_note TEXT;

CREATE TABLE employee_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    manager_id UUID,
    request_type TEXT NOT NULL CHECK (request_type IN ('overtime', 'permission')),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    start_time TIME,
    end_time TIME,
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 1000),
    status TEXT NOT NULL CHECK (status IN (
        'pending_manager', 'pending_hr', 'pending_superadmin',
        'approved', 'rejected', 'cancelled'
    )),
    manager_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    manager_decision_at TIMESTAMPTZ,
    manager_note TEXT,
    hr_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    hr_decision_at TIMESTAMPTZ,
    hr_note TEXT,
    superadmin_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    superadmin_decision_at TIMESTAMPTZ,
    superadmin_note TEXT,
    cancelled_by UUID REFERENCES users(id) ON DELETE SET NULL,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, branch_id) REFERENCES users(id, branch_id),
    FOREIGN KEY (manager_id, branch_id) REFERENCES users(id, branch_id),
    CHECK (start_date <= end_date),
    CHECK (
        (request_type = 'overtime' AND start_date = end_date
            AND start_time IS NOT NULL AND end_time IS NOT NULL
            AND end_time > start_time)
        OR
        (request_type = 'permission'
            AND ((start_time IS NULL AND end_time IS NULL)
                OR (start_time IS NOT NULL AND end_time IS NOT NULL
                    AND end_date = start_date AND end_time > start_time)))
    )
);

CREATE INDEX idx_employee_requests_user_created
    ON employee_requests (user_id, created_at DESC);
CREATE INDEX idx_employee_requests_manager_queue
    ON employee_requests (manager_id, status, created_at DESC);
CREATE INDEX idx_employee_requests_status_created
    ON employee_requests (status, created_at DESC);
