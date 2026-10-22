CREATE UNIQUE INDEX idx_users_lower_email ON users (lower(email));
CREATE UNIQUE INDEX idx_departments_id_branch ON departments (id, branch_id);
CREATE UNIQUE INDEX idx_users_id_branch ON users (id, branch_id);

ALTER TABLE users
    ADD CONSTRAINT users_manager_same_branch_fk
    FOREIGN KEY (manager_id, branch_id) REFERENCES users (id, branch_id);

ALTER TABLE employees ADD COLUMN branch_id UUID;
UPDATE employees e
SET branch_id = u.branch_id
FROM users u
WHERE u.id = e.user_id;
ALTER TABLE employees ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE employees
    ADD CONSTRAINT employees_branch_fk
    FOREIGN KEY (branch_id) REFERENCES branches(id);
ALTER TABLE employees
    ADD CONSTRAINT employees_department_same_branch_fk
    FOREIGN KEY (department_id, branch_id) REFERENCES departments (id, branch_id);

CREATE INDEX idx_employees_branch_name ON employees (branch_id, full_name);
CREATE INDEX idx_audit_logs_actor_created ON audit_logs (actor_user_id, created_at DESC);
