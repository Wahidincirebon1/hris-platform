CREATE TABLE kpi_cycles (
    kpi_year INTEGER PRIMARY KEY CHECK (kpi_year BETWEEN 2000 AND 2100),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 3 AND 120),
    status TEXT NOT NULL DEFAULT 'planning'
        CHECK (status IN ('planning', 'open', 'closed')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE kpi_goal_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kpi_year INTEGER NOT NULL REFERENCES kpi_cycles(kpi_year),
    user_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    manager_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft', 'pending_hr', 'needs_revision', 'approved', 'pending_hr_final', 'finalized'
    )),
    manager_comment TEXT,
    manager_review_submitted_at TIMESTAMPTZ,
    hr_target_approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    hr_target_approved_at TIMESTAMPTZ,
    hr_target_note TEXT,
    hr_finalized_by UUID REFERENCES users(id) ON DELETE SET NULL,
    hr_finalized_at TIMESTAMPTZ,
    appeal_deadline DATE,
    final_score_override NUMERIC(5, 2) CHECK (final_score_override BETWEEN 0 AND 100),
    final_review_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kpi_year, user_id),
    UNIQUE (id, user_id),
    FOREIGN KEY (user_id, branch_id) REFERENCES users(id, branch_id),
    FOREIGN KEY (manager_id, branch_id) REFERENCES users(id, branch_id),
    CHECK ((status = 'finalized') = (hr_finalized_at IS NOT NULL)),
    CHECK ((status = 'pending_hr_final' OR status = 'finalized') = (manager_review_submitted_at IS NOT NULL)),
    CHECK ((status IN ('approved', 'pending_hr_final', 'finalized')) = (hr_target_approved_at IS NOT NULL)),
    CHECK ((status = 'finalized') = (appeal_deadline IS NOT NULL))
);

CREATE INDEX idx_kpi_goal_sets_manager_status
    ON kpi_goal_sets (manager_id, status, kpi_year);
CREATE INDEX idx_kpi_goal_sets_user_year
    ON kpi_goal_sets (user_id, kpi_year);
CREATE INDEX idx_kpi_goal_sets_branch_year
    ON kpi_goal_sets (branch_id, kpi_year);

CREATE TABLE kpi_goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_set_id UUID NOT NULL REFERENCES kpi_goal_sets(id) ON DELETE CASCADE,
    title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 3 AND 160),
    description TEXT NOT NULL CHECK (length(trim(description)) BETWEEN 5 AND 1000),
    target_value TEXT NOT NULL CHECK (length(trim(target_value)) BETWEEN 1 AND 200),
    weight NUMERIC(5, 2) NOT NULL CHECK (weight > 0 AND weight <= 100),
    manager_score NUMERIC(5, 2) CHECK (manager_score BETWEEN 0 AND 100),
    manager_comment TEXT CHECK (manager_comment IS NULL OR length(manager_comment) <= 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_kpi_goals_set ON kpi_goals (goal_set_id);

CREATE TABLE kpi_progress_updates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id UUID NOT NULL REFERENCES kpi_goals(id) ON DELETE CASCADE,
    entered_by UUID REFERENCES users(id) ON DELETE SET NULL,
    current_value TEXT NOT NULL CHECK (length(trim(current_value)) BETWEEN 1 AND 200),
    progress_percent NUMERIC(5, 2) CHECK (progress_percent BETWEEN 0 AND 100),
    note TEXT NOT NULL CHECK (length(trim(note)) BETWEEN 5 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_kpi_progress_goal_created ON kpi_progress_updates (goal_id, created_at DESC);

CREATE TABLE kpi_appeals (
    goal_set_id UUID PRIMARY KEY REFERENCES kpi_goal_sets(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 5 AND 1000),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    decision TEXT CHECK (decision IN ('upheld', 'adjusted')),
    decision_note TEXT,
    adjusted_score NUMERIC(5, 2) CHECK (adjusted_score BETWEEN 0 AND 100),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ,
    CHECK (
        (status = 'pending' AND decision IS NULL AND decided_at IS NULL)
        OR (status = 'resolved' AND decision IS NOT NULL AND decided_at IS NOT NULL)
    ),
    CHECK ((decision = 'adjusted') = (adjusted_score IS NOT NULL))
);

CREATE INDEX idx_kpi_appeals_status ON kpi_appeals (status, submitted_at);
