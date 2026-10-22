CREATE TABLE leave_entitlements (
    leave_year INTEGER PRIMARY KEY CHECK (leave_year BETWEEN 2000 AND 2100),
    annual_days SMALLINT NOT NULL CHECK (annual_days BETWEEN 0 AND 60),
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 500),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE company_calendar_days (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 2 AND 120),
    kind TEXT NOT NULL CHECK (kind IN ('public_holiday', 'collective_leave', 'company_workday')),
    active_leave_cycle UUID,
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 500),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (kind = 'collective_leave' AND active_leave_cycle IS NOT NULL)
        OR (kind <> 'collective_leave' AND active_leave_cycle IS NULL)
    )
);

CREATE INDEX idx_company_calendar_date ON company_calendar_days (calendar_date);

CREATE TABLE leave_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    manager_id UUID,
    leave_year INTEGER NOT NULL REFERENCES leave_entitlements(leave_year),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    day_count SMALLINT NOT NULL CHECK (day_count BETWEEN 1 AND 366),
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 1000),
    status TEXT NOT NULL DEFAULT 'pending_manager'
        CHECK (status IN ('pending_manager', 'pending_hr', 'approved', 'rejected', 'cancelled')),
    manager_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    manager_decision_at TIMESTAMPTZ,
    manager_note TEXT,
    hr_decision_by UUID REFERENCES users(id) ON DELETE SET NULL,
    hr_decision_at TIMESTAMPTZ,
    hr_note TEXT,
    cancelled_by UUID REFERENCES users(id) ON DELETE SET NULL,
    cancelled_at TIMESTAMPTZ,
    cancellation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, branch_id) REFERENCES users(id, branch_id),
    FOREIGN KEY (manager_id, branch_id) REFERENCES users(id, branch_id),
    CHECK (start_date <= end_date),
    CHECK (EXTRACT(YEAR FROM start_date)::INTEGER = leave_year),
    CHECK (EXTRACT(YEAR FROM end_date)::INTEGER = leave_year)
);

CREATE INDEX idx_leave_requests_user_dates ON leave_requests (user_id, start_date, end_date);
CREATE INDEX idx_leave_requests_manager_status ON leave_requests (manager_id, status, created_at DESC);
CREATE INDEX idx_leave_requests_status_created ON leave_requests (status, created_at DESC);

CREATE TABLE leave_request_days (
    request_id UUID NOT NULL REFERENCES leave_requests(id) ON DELETE CASCADE,
    leave_date DATE NOT NULL,
    PRIMARY KEY (request_id, leave_date)
);

CREATE TABLE leave_ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    leave_year INTEGER NOT NULL REFERENCES leave_entitlements(leave_year),
    entry_type TEXT NOT NULL CHECK (entry_type IN (
        'annual_grant',
        'annual_adjustment',
        'collective_leave',
        'collective_leave_reversal',
        'leave_request',
        'leave_request_reversal'
    )),
    amount NUMERIC(8, 2) NOT NULL CHECK (amount <> 0),
    calendar_day_id UUID REFERENCES company_calendar_days(id) ON DELETE RESTRICT,
    source_cycle_id UUID,
    request_id UUID REFERENCES leave_requests(id) ON DELETE RESTRICT,
    entry_date DATE,
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 500),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (entry_type IN ('annual_grant', 'annual_adjustment')
            AND calendar_day_id IS NULL AND source_cycle_id IS NULL
            AND request_id IS NULL AND entry_date IS NULL)
        OR
        (entry_type IN ('collective_leave', 'collective_leave_reversal')
            AND calendar_day_id IS NOT NULL AND source_cycle_id IS NOT NULL
            AND request_id IS NULL AND entry_date IS NOT NULL)
        OR
        (entry_type IN ('leave_request', 'leave_request_reversal')
            AND calendar_day_id IS NULL AND source_cycle_id IS NULL
            AND request_id IS NOT NULL AND entry_date IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_leave_ledger_annual_grant
    ON leave_ledger_entries (user_id, leave_year)
    WHERE entry_type = 'annual_grant';
CREATE UNIQUE INDEX idx_leave_ledger_calendar_cycle
    ON leave_ledger_entries (user_id, calendar_day_id, source_cycle_id, entry_type)
    WHERE calendar_day_id IS NOT NULL;
CREATE UNIQUE INDEX idx_leave_ledger_request_day
    ON leave_ledger_entries (user_id, request_id, entry_date, entry_type)
    WHERE request_id IS NOT NULL;
CREATE INDEX idx_leave_ledger_balance
    ON leave_ledger_entries (user_id, leave_year, created_at);
