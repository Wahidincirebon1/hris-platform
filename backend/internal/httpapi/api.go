package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hris-platform/backend/internal/security"
)

type API struct {
	db           *pgxpool.Pool
	jwtSecret    []byte
	cookieSecure bool
	loginLimiter *loginLimiter
	ipLimiter    *loginLimiter
}

type contextKey struct{}

type principal struct {
	ID                 uuid.UUID  `json:"id"`
	Email              string     `json:"email"`
	Role               string     `json:"role"`
	BranchID           uuid.UUID  `json:"branch_id"`
	BranchCode         string     `json:"branch_code"`
	BranchName         string     `json:"branch_name"`
	ManagerID          *uuid.UUID `json:"manager_id,omitempty"`
	FullName           string     `json:"full_name"`
	EmployeeNumber     string     `json:"employee_number"`
	MustChangePassword bool       `json:"must_change_password"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type statusResponse struct {
	Status string `json:"status"`
}

func New(db *pgxpool.Pool) (*API, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET must be a unique random value of at least 32 characters")
	}
	return &API{
		db:           db,
		jwtSecret:    []byte(secret),
		cookieSecure: strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),
		loginLimiter: newLoginLimiter(5, time.Minute),
		ipLimiter:    newLoginLimiter(60, time.Minute),
	}, nil
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /api/v1/health/ready", a.ready)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.authenticated(a.logout))
	mux.HandleFunc("GET /api/v1/auth/me", a.authenticated(a.me))
	mux.HandleFunc("PUT /api/v1/auth/password", a.authenticated(a.changePassword))
	mux.HandleFunc("GET /api/v1/branches", a.authenticated(a.listBranches))
	mux.HandleFunc("GET /api/v1/departments", a.authenticated(a.listDepartments))
	mux.HandleFunc("POST /api/v1/departments", a.authenticated(a.createDepartment))
	mux.HandleFunc("GET /api/v1/employees", a.authenticated(a.listEmployees))
	mux.HandleFunc("POST /api/v1/employees", a.authenticated(a.createEmployee))
	mux.HandleFunc("GET /api/v1/employees/{id}", a.authenticated(a.getEmployee))
	mux.HandleFunc("PATCH /api/v1/employees/{id}", a.authenticated(a.updateEmployee))
	mux.HandleFunc("GET /api/v1/audit", a.authenticated(a.listAudit))
	a.registerProfileRoutes(mux)
	a.registerAttendanceRoutes(mux)
	a.registerLeaveRoutes(mux)
	a.registerEmployeeRequestRoutes(mux)
	a.registerKPIRoutes(mux)
	a.registerExamRoutes(mux)
	a.registerReportRoutes(mux)
	return a.securityHeaders(a.sameOrigin(mux))
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.Email == "" || input.Password == "" {
		writeError(w, http.StatusUnauthorized, "Email atau password tidak sesuai.")
		return
	}
	ip := clientIP(r)
	if !a.ipLimiter.allow(ip) || !a.loginLimiter.allow(ip+"|"+input.Email) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak percobaan login. Silakan coba lagi nanti.")
		return
	}

	user, hash, err := a.loadPrincipalByEmail(r.Context(), input.Email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("login lookup failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
			return
		}
		a.loginLimiter.consumeDummy(input.Password)
		writeError(w, http.StatusUnauthorized, "Email atau password tidak sesuai.")
		return
	}
	if !security.VerifyPassword(hash, input.Password) {
		writeError(w, http.StatusUnauthorized, "Email atau password tidak sesuai.")
		return
	}
	token, err := a.createToken(user.ID)
	if err != nil {
		log.Printf("create login token failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), "UPDATE users SET last_login_at = now() WHERE id = $1", user.ID); err != nil {
		log.Printf("update last login failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "LOGIN", "user", user.ID.String(), map[string]string{}); err != nil {
		log.Printf("audit login failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Login sementara tidak dapat diproses.")
		return
	}
	a.setSessionCookie(w, token, 8*time.Hour)
	writeJSON(w, http.StatusOK, user)
}

func (a *API) logout(w http.ResponseWriter, _ *http.Request) {
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, statusResponse{Status: "logged_out"})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentPrincipal(r))
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user := currentPrincipal(r)
	var hash string
	if err := a.db.QueryRow(r.Context(), "SELECT password_hash FROM users WHERE id = $1 AND active", user.ID).Scan(&hash); err != nil {
		writeError(w, http.StatusUnauthorized, "Sesi tidak berlaku.")
		return
	}
	if !security.VerifyPassword(hash, input.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "Kredensial tidak valid.")
		return
	}
	newHash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Password baru harus berisi 12 sampai 72 byte.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Password tidak dapat diperbarui.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(),
		"UPDATE users SET password_hash = $1, must_change_password = FALSE, updated_at = now() WHERE id = $2",
		newHash, user.ID,
	); err != nil {
		log.Printf("update password failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Password tidak dapat diperbarui.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "CHANGE_PASSWORD", "user", user.ID.String(), map[string]string{}); err != nil {
		log.Printf("audit password change failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Password tidak dapat diperbarui.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Password tidak dapat diperbarui.")
		return
	}
	token, err := a.createToken(user.ID)
	if err != nil {
		log.Printf("create post-password-change token failed: %v", err)
		a.clearSessionCookie(w)
		writeError(w, http.StatusInternalServerError, "Password berubah; silakan login kembali.")
		return
	}
	a.setSessionCookie(w, token, 8*time.Hour)
	writeJSON(w, http.StatusOK, statusResponse{Status: "password_changed"})
}

func (a *API) listBranches(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	rows, err := a.db.Query(r.Context(), `
		SELECT id, code, name, timezone, geofence_radius_meters
		FROM branches
		WHERE active AND ($1::boolean OR id = $2)
		ORDER BY name
	`, isHR(user.Role), user.BranchID)
	if err != nil {
		log.Printf("list branches failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data cabang.")
		return
	}
	defer rows.Close()
	type branch struct {
		ID                   uuid.UUID `json:"id"`
		Code                 string    `json:"code"`
		Name                 string    `json:"name"`
		Timezone             string    `json:"timezone"`
		GeofenceRadiusMeters int       `json:"geofence_radius_meters"`
	}
	result := make([]branch, 0)
	for rows.Next() {
		var item branch
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Timezone, &item.GeofenceRadiusMeters); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data cabang.")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca data cabang.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) listDepartments(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	branchID := user.BranchID
	if isHR(user.Role) && r.URL.Query().Get("branch_id") != "" {
		parsed, err := uuid.Parse(r.URL.Query().Get("branch_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "ID cabang tidak valid.")
			return
		}
		branchID = parsed
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, branch_id, name
		FROM departments
		WHERE active AND branch_id = $1
		ORDER BY name
	`, branchID)
	if err != nil {
		log.Printf("list departments failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data departemen.")
		return
	}
	defer rows.Close()
	type department struct {
		ID       uuid.UUID `json:"id"`
		BranchID uuid.UUID `json:"branch_id"`
		Name     string    `json:"name"`
	}
	result := make([]department, 0)
	for rows.Next() {
		var item department
		if err := rows.Scan(&item.ID, &item.BranchID, &item.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data departemen.")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca data departemen.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) createDepartment(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat mengelola departemen.")
		return
	}
	var input struct {
		BranchID string `json:"branch_id"`
		Name     string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	branchID, err := uuid.Parse(input.BranchID)
	input.Name = strings.TrimSpace(input.Name)
	if err != nil || input.Name == "" || len(input.Name) > 100 {
		writeError(w, http.StatusBadRequest, "Cabang dan nama departemen yang valid wajib diisi.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Departemen tidak dapat dibuat.")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO departments (branch_id, name)
		SELECT id, $2 FROM branches WHERE id = $1 AND active
		RETURNING id
	`, branchID, input.Name).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Departemen dengan nama tersebut sudah ada di cabang.")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "Cabang tidak ditemukan atau tidak aktif.")
			return
		}
		log.Printf("create department failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Departemen tidak dapat dibuat.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "CREATE_DEPARTMENT", "department", id.String(), map[string]string{
		"branch_id": branchID.String(), "name": input.Name,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "Departemen tidak dapat dibuat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Departemen tidak dapat dibuat.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "branch_id": branchID, "name": input.Name})
}

func (a *API) listEmployees(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	var branchID, managerID *uuid.UUID
	if isHR(user.Role) {
		if raw := r.URL.Query().Get("branch_id"); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "ID cabang tidak valid.")
				return
			}
			branchID = &parsed
		}
	} else if user.Role == "manager" {
		branchID = &user.BranchID
		managerID = &user.ID
	} else {
		writeError(w, http.StatusForbidden, "Akses daftar karyawan tidak diizinkan.")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		SELECT u.id, e.employee_number, e.full_name, u.email, u.role, u.branch_id,
		       b.name, d.name, e.position_title, e.job_grade, u.active, u.manager_id
		FROM users u
		JOIN employees e ON e.user_id = u.id
		JOIN branches b ON b.id = u.branch_id
		LEFT JOIN departments d ON d.id = e.department_id
		WHERE ($1::uuid IS NULL OR u.branch_id = $1)
		  AND ($2::uuid IS NULL OR u.manager_id = $2)
		  AND e.employment_status <> 'archived'
		ORDER BY e.full_name
	`, branchID, managerID)
	if err != nil {
		log.Printf("list employees failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data karyawan.")
		return
	}
	defer rows.Close()
	type employeeSummary struct {
		UserID         uuid.UUID  `json:"user_id"`
		EmployeeNumber string     `json:"employee_number"`
		FullName       string     `json:"full_name"`
		Email          string     `json:"email"`
		Role           string     `json:"role"`
		BranchID       uuid.UUID  `json:"branch_id"`
		BranchName     string     `json:"branch_name"`
		Department     *string    `json:"department"`
		Position       *string    `json:"position"`
		JobGrade       *string    `json:"job_grade,omitempty"`
		Active         bool       `json:"active"`
		ManagerID      *uuid.UUID `json:"manager_id,omitempty"`
	}
	result := make([]employeeSummary, 0)
	for rows.Next() {
		var item employeeSummary
		if err := rows.Scan(&item.UserID, &item.EmployeeNumber, &item.FullName, &item.Email,
			&item.Role, &item.BranchID, &item.BranchName, &item.Department, &item.Position,
			&item.JobGrade, &item.Active, &item.ManagerID); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data karyawan.")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca data karyawan.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) getEmployee(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID karyawan tidak valid.")
		return
	}
	user := currentPrincipal(r)
	if !isHR(user.Role) && (user.Role != "manager" || targetID == user.ID) {
		writeError(w, http.StatusForbidden, "Akses data karyawan tidak diizinkan.")
		return
	}
	var employeeID, branchID uuid.UUID
	var name, email, role, number, branchName string
	var managerID *uuid.UUID
	var active bool
	err = a.db.QueryRow(r.Context(), `
		SELECT u.id, e.employee_number, e.full_name, u.email, u.role, u.branch_id,
		       b.name, u.manager_id, u.active
		FROM users u JOIN employees e ON e.user_id = u.id
		JOIN branches b ON b.id = u.branch_id
		WHERE u.id = $1
		  AND ($2::boolean OR (u.branch_id = $3 AND u.manager_id = $4))
	`, targetID, isHR(user.Role), user.BranchID, user.ID).Scan(
		&employeeID, &number, &name, &email, &role, &branchID, &branchName, &managerID, &active,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Karyawan tidak ditemukan.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data karyawan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": employeeID, "employee_number": number, "full_name": name, "email": email,
		"role": role, "branch_id": branchID, "branch_name": branchName,
		"manager_id": managerID, "active": active,
	})
}

func (a *API) createEmployee(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat membuat akun karyawan.")
		return
	}
	var input struct {
		Email             string  `json:"email"`
		TemporaryPassword string  `json:"temporary_password"`
		EmployeeNumber    string  `json:"employee_number"`
		FullName          string  `json:"full_name"`
		NIK               *string `json:"nik"`
		Phone             *string `json:"phone"`
		Address           *string `json:"address"`
		JobGrade          *string `json:"job_grade"`
		BirthDate         *string `json:"birth_date"`
		BranchID          string  `json:"branch_id"`
		DepartmentID      *string `json:"department_id"`
		PositionTitle     *string `json:"position_title"`
		ManagerID         *string `json:"manager_id"`
		Role              string  `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.EmployeeNumber = strings.TrimSpace(input.EmployeeNumber)
	input.FullName = strings.TrimSpace(input.FullName)
	// Grade hanya boleh 1–10.
	if input.JobGrade != nil {
		value := strings.TrimSpace(*input.JobGrade)
		if value == "" {
			input.JobGrade = nil
		} else {
			grade, err := strconv.Atoi(value)
			if err != nil || grade < 1 || grade > 10 {
				writeError(w, http.StatusBadRequest, "Grade harus berupa angka 1 sampai 10.")
				return
			}
			normalized := strconv.Itoa(grade)
			input.JobGrade = &normalized
		}
	}
	email, err := mail.ParseAddress(input.Email)
	if err != nil || email.Address != input.Email || input.FullName == "" {
		writeError(w, http.StatusBadRequest, "Nama dan email valid wajib diisi.")
		return
	}
	if len(input.EmployeeNumber) > 64 {
		writeError(w, http.StatusBadRequest, "Nomor karyawan maksimal 64 karakter.")
		return
	}
	if !validManagedRole(input.Role) && !(actor.Role == "superadmin" && input.Role == "hr") {
		writeError(w, http.StatusBadRequest, "HR dapat membuat employee, manager, atau finance. Superadmin juga dapat membuat akun HR.")
		return
	}
	passwordHash, err := security.HashPassword(input.TemporaryPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Password sementara harus berisi 12 sampai 72 byte.")
		return
	}
	branchID, err := uuid.Parse(input.BranchID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cabang tidak valid.")
		return
	}
	if input.DepartmentID != nil && *input.DepartmentID != "" {
		if _, err := uuid.Parse(*input.DepartmentID); err != nil {
			writeError(w, http.StatusBadRequest, "Departemen tidak valid.")
			return
		}
	} else {
		input.DepartmentID = nil
	}
	if input.ManagerID != nil && *input.ManagerID != "" {
		if _, err := uuid.Parse(*input.ManagerID); err != nil {
			writeError(w, http.StatusBadRequest, "Atasan tidak valid.")
			return
		}
	} else {
		input.ManagerID = nil
	}
	var birthDate *time.Time
	if input.BirthDate != nil && strings.TrimSpace(*input.BirthDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*input.BirthDate), jakartaTime)
		if err != nil || parsed.After(time.Now()) {
			writeError(w, http.StatusBadRequest, "Tanggal lahir harus berformat YYYY-MM-DD dan tidak boleh di masa depan.")
			return
		}
		birthDate = &parsed
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Akun karyawan tidak dapat dibuat.")
		return
	}
	defer tx.Rollback(r.Context())
	branchActive, err := isActiveBranch(r.Context(), tx, branchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "Cabang tidak ditemukan atau tidak aktif.")
		} else {
			log.Printf("validate employee branch failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Cabang karyawan tidak dapat diverifikasi.")
		}
		return
	}
	if !branchActive {
		writeError(w, http.StatusBadRequest, "Cabang tidak ditemukan atau tidak aktif.")
		return
	}
	if input.DepartmentID != nil {
		var departmentActive bool
		err := tx.QueryRow(r.Context(), "SELECT active FROM departments WHERE id = $1 AND branch_id = $2 FOR SHARE", *input.DepartmentID, branchID).Scan(&departmentActive)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "Departemen tidak ditemukan pada cabang tersebut.")
			} else {
				log.Printf("validate employee department failed: %v", err)
				writeError(w, http.StatusInternalServerError, "Departemen karyawan tidak dapat diverifikasi.")
			}
			return
		}
		if !departmentActive {
			writeError(w, http.StatusBadRequest, "Departemen tidak ditemukan pada cabang tersebut.")
			return
		}
	}
	if input.ManagerID != nil {
		var validManager bool
		if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND branch_id = $2 AND role = 'manager' AND active)", *input.ManagerID, branchID).Scan(&validManager); err != nil {
			log.Printf("validate employee manager failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Atasan karyawan tidak dapat diverifikasi.")
			return
		}
		if !validManager {
			writeError(w, http.StatusBadRequest, "Atasan harus merupakan manager aktif pada cabang yang sama.")
			return
		}
	}

	var userID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (email, password_hash, role, branch_id, manager_id, active, must_change_password)
		VALUES ($1, $2, $3, $4, $5, TRUE, TRUE)
		RETURNING id
	`, input.Email, passwordHash, input.Role, branchID, input.ManagerID).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Email sudah digunakan.")
			return
		}
		log.Printf("create employee user failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Akun karyawan tidak dapat dibuat.")
		return
	}
	if input.NIK == nil || strings.TrimSpace(*input.NIK) == "" {
		autoNIK, err := nextNIK(r.Context(), tx)
		if err != nil {
			log.Printf("generate automatic NIK failed: %v", err)
			writeError(w, http.StatusInternalServerError, "NIK tidak dapat dibuat otomatis.")
			return
		}
		input.NIK = &autoNIK
	}
	// Nomor karyawan dibuat otomatis bila HR tidak mengisinya.
	if input.EmployeeNumber == "" {
		autoNumber, err := nextEmployeeNumber(r.Context(), tx, branchID)
		if err != nil {
			log.Printf("generate automatic employee number failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Nomor karyawan tidak dapat dibuat otomatis.")
			return
		}
		input.EmployeeNumber = autoNumber
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO employees
			(user_id, employee_number, full_name, nik, phone, address, branch_id, department_id, position_title, job_grade, birth_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, userID, input.EmployeeNumber, input.FullName, input.NIK, input.Phone, input.Address,
		branchID, input.DepartmentID, input.PositionTitle, input.JobGrade, birthDate)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Nomor karyawan atau NIK sudah digunakan.")
			return
		}
		log.Printf("create employee profile failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Akun karyawan tidak dapat dibuat.")
		return
	}
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock annual leave grant for new employee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti karyawan tidak dapat disiapkan.")
		return
	}
	currentYear := time.Now().In(jakartaTime).Year()
	rows, err := tx.Query(r.Context(), `
		SELECT leave_year, annual_days FROM leave_entitlements
		WHERE leave_year >= $1 ORDER BY leave_year
	`, currentYear)
	if err != nil {
		log.Printf("load current and future leave entitlements for new employee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti karyawan tidak dapat disiapkan.")
		return
	}
	type annualPolicy struct {
		year       int
		annualDays int
	}
	policies := make([]annualPolicy, 0)
	for rows.Next() {
		var policy annualPolicy
		if err := rows.Scan(&policy.year, &policy.annualDays); err != nil {
			rows.Close()
			log.Printf("scan current and future leave entitlements for new employee failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Hak cuti karyawan tidak dapat disiapkan.")
			return
		}
		policies = append(policies, policy)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		log.Printf("read current and future leave entitlements for new employee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti karyawan tidak dapat disiapkan.")
		return
	}
	for _, policy := range policies {
		if err := syncAnnualGrant(r.Context(), tx, userID, policy.year, policy.annualDays, "Hak cuti tahunan tahun berjalan", actor.ID); err != nil {
			log.Printf("grant annual leave to new employee failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Hak cuti karyawan tidak dapat disiapkan.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "CREATE_EMPLOYEE", "employee", userID.String(), map[string]string{
		"branch_id": branchID.String(), "role": input.Role,
	}); err != nil {
		log.Printf("audit employee creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Akun karyawan tidak dapat dibuat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit employee creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Akun karyawan tidak dapat dibuat.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": userID, "must_change_password": true,
		"employee_number": input.EmployeeNumber, "nik": input.NIK,
		"message": "Akun dibuat. Sampaikan password sementara melalui kanal aman.",
	})
}

func (a *API) updateEmployee(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	targetID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID karyawan tidak valid.")
		return
	}
	if !isHR(actor.Role) && actor.ID != targetID {
		writeError(w, http.StatusForbidden, "Anda hanya dapat memperbarui data karyawan sendiri.")
		return
	}
	var input struct {
		FullName      string  `json:"full_name"`
		Phone         *string `json:"phone"`
		Address       *string `json:"address"`
		JobGrade      *string `json:"job_grade"`
		BirthDate     *string `json:"birth_date"`
		NIK           *string `json:"nik"`
		DepartmentID  *string `json:"department_id"`
		PositionTitle *string `json:"position_title"`
		BranchID      string  `json:"branch_id"`
		ManagerID     *string `json:"manager_id"`
		Role          string  `json:"role"`
		Active        bool    `json:"active"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.FullName = strings.TrimSpace(input.FullName)
	if !isHR(actor.Role) {
		// Karyawan hanya boleh memperbarui data kontak pribadi sendiri.
		input.Role = ""
		input.Active = true
		input.BranchID = ""
		input.ManagerID = nil
		input.DepartmentID = nil
		input.NIK = nil
	}
	if input.FullName == "" || !validManagedRole(input.Role) {
		writeError(w, http.StatusBadRequest, "Nama dan role karyawan tidak valid.")
		return
	}
	if input.JobGrade != nil {
		value := strings.TrimSpace(*input.JobGrade)
		if value == "" {
			input.JobGrade = nil
		} else {
			grade, err := strconv.Atoi(value)
			if err != nil || grade < 1 || grade > 10 {
				writeError(w, http.StatusBadRequest, "Grade harus berupa angka 1 sampai 10.")
				return
			}
			normalized := strconv.Itoa(grade)
			input.JobGrade = &normalized
		}
	}
	branchID, err := uuid.Parse(input.BranchID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cabang tidak valid.")
		return
	}
	birthDate, err := optionalBirthDate(input.BirthDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Tanggal lahir harus berformat YYYY-MM-DD dan tidak boleh di masa depan.")
		return
	}
	departmentID, err := optionalUUID(input.DepartmentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Departemen tidak valid.")
		return
	}
	managerID, err := optionalUUID(input.ManagerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Atasan tidak valid.")
		return
	}
	if managerID != nil && *managerID == targetID {
		writeError(w, http.StatusBadRequest, "Karyawan tidak dapat menjadi atasannya sendiri.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Data karyawan tidak dapat diperbarui.")
		return
	}
	defer tx.Rollback(r.Context())
	var oldBranch uuid.UUID
	var oldRole string
	if err := tx.QueryRow(r.Context(), "SELECT branch_id, role FROM users WHERE id = $1 FOR UPDATE", targetID).Scan(&oldBranch, &oldRole); err != nil {
		writeError(w, http.StatusNotFound, "Karyawan tidak ditemukan.")
		return
	}
	if oldRole == "superadmin" {
		writeError(w, http.StatusForbidden, "Akun superadmin tidak dapat diubah melalui profil karyawan.")
		return
	}
	if oldRole == "hr" && actor.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "Hanya superadmin yang dapat mengubah akun HR.")
		return
	}
	if oldBranch != branchID || input.Role != oldRole || !input.Active {
		var hasReports bool
		if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE manager_id = $1)", targetID).Scan(&hasReports); err != nil {
			writeError(w, http.StatusInternalServerError, "Status bawahan tidak dapat diperiksa.")
			return
		}
		if hasReports {
			writeError(w, http.StatusConflict, "Pindahkan bawahan ke atasan aktif yang sesuai sebelum mengubah cabang, role, atau status manager.")
			return
		}
	}
	branchActive, err := isActiveBranch(r.Context(), tx, branchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "Cabang tidak ditemukan atau tidak aktif.")
		} else {
			log.Printf("validate updated employee branch failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Cabang karyawan tidak dapat diverifikasi.")
		}
		return
	}
	if !branchActive {
		writeError(w, http.StatusBadRequest, "Cabang tidak ditemukan atau tidak aktif.")
		return
	}
	if departmentID != nil {
		var exists bool
		if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM departments WHERE id = $1 AND branch_id = $2 AND active)", *departmentID, branchID).Scan(&exists); err != nil {
			log.Printf("validate updated employee department failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Departemen karyawan tidak dapat diverifikasi.")
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, "Departemen harus aktif pada cabang yang dipilih.")
			return
		}
	}
	if managerID != nil {
		var validManager bool
		if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND branch_id = $2 AND role = 'manager' AND active)", *managerID, branchID).Scan(&validManager); err != nil {
			log.Printf("validate updated employee manager failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Atasan karyawan tidak dapat diverifikasi.")
			return
		}
		if !validManager {
			writeError(w, http.StatusBadRequest, "Atasan harus merupakan manager aktif pada cabang yang sama.")
			return
		}
		var managerInTeam bool
		if err := tx.QueryRow(r.Context(), `
			WITH RECURSIVE team(id) AS (
				SELECT $1::uuid
				UNION
				SELECT u.id FROM users u JOIN team t ON u.manager_id = t.id
			)
			SELECT EXISTS(SELECT 1 FROM team WHERE id = $2)
		`, targetID, *managerID).Scan(&managerInTeam); err != nil {
			log.Printf("validate reporting hierarchy failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Struktur atasan tidak dapat diverifikasi.")
			return
		}
		if managerInTeam {
			writeError(w, http.StatusBadRequest, "Atasan yang dipilih tidak boleh berada di bawah karyawan tersebut.")
			return
		}
	}

	if _, err := tx.Exec(r.Context(), `
		UPDATE users SET branch_id = $1, manager_id = $2, role = $3, active = $4, updated_at = now()
		WHERE id = $5
	`, branchID, managerID, input.Role, input.Active, targetID); err != nil {
		log.Printf("update employee account failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Data karyawan tidak dapat diperbarui.")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE employees SET birth_date = $9, phone = COALESCE($4, phone), address = COALESCE($5, address),
			branch_id = $1, department_id = $2, full_name = $3,
			position_title = $6, job_grade = $7, nik = COALESCE($8, nik), updated_at = now()
		WHERE user_id = $10
	`, branchID, departmentID, input.FullName, input.Phone, input.Address, input.PositionTitle, input.JobGrade, input.NIK, birthDate, targetID); err != nil {
		log.Printf("update employee profile failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Data karyawan tidak dapat diperbarui.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "UPDATE_EMPLOYEE", "employee", targetID.String(), map[string]string{
		"branch_id": branchID.String(), "role": input.Role, "active": fmt.Sprint(input.Active),
	}); err != nil {
		log.Printf("audit employee update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Data karyawan tidak dapat diperbarui.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Data karyawan tidak dapat diperbarui.")
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "updated"})
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR dan superadmin yang dapat melihat audit.")
		return
	}
	limit, offset := 50, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeError(w, http.StatusBadRequest, "Limit harus antara 1 dan 100.")
			return
		}
		limit = value
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "Offset tidak valid.")
			return
		}
		offset = value
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, actor_user_id, action, entity_type, entity_id, details, created_at
		FROM audit_logs
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		log.Printf("list audit logs failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Gagal mengambil audit log.")
		return
	}
	defer rows.Close()
	type auditEntry struct {
		ID        int64           `json:"id"`
		ActorID   *uuid.UUID      `json:"actor_user_id"`
		Action    string          `json:"action"`
		Entity    string          `json:"entity_type"`
		EntityID  string          `json:"entity_id"`
		Details   json.RawMessage `json:"details"`
		CreatedAt time.Time       `json:"created_at"`
	}
	entries := make([]auditEntry, 0, limit)
	for rows.Next() {
		var item auditEntry
		var details []byte
		if err := rows.Scan(&item.ID, &item.ActorID, &item.Action, &item.Entity, &item.EntityID, &details, &item.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca audit log.")
			return
		}
		item.Details = append(json.RawMessage(nil), details...)
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membaca audit log.")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (a *API) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("hris_session")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Silakan login untuk melanjutkan.")
			return
		}
		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected JWT signing method")
			}
			return a.jwtSecret, nil
		}, jwt.WithIssuer("hris-platform"), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || token == nil || !token.Valid {
			a.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "Sesi tidak valid. Silakan login kembali.")
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			a.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "Sesi tidak valid. Silakan login kembali.")
			return
		}
		user, _, err := a.loadPrincipalByID(r.Context(), userID)
		if err != nil {
			a.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "Akun tidak aktif atau sesi tidak berlaku.")
			return
		}
		if user.MustChangePassword && r.URL.Path != "/api/v1/auth/password" && r.URL.Path != "/api/v1/auth/me" && r.URL.Path != "/api/v1/auth/logout" {
			writeError(w, http.StatusForbidden, "Ganti password sementara sebelum menggunakan aplikasi.")
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, user)
		next(w, r.WithContext(ctx))
	}
}

func (a *API) loadPrincipalByEmail(ctx context.Context, email string) (principal, string, error) {
	return a.loadPrincipal(ctx, `
		SELECT u.id, u.email, u.role, u.branch_id, b.code, b.name, u.manager_id,
		       e.full_name, e.employee_number, u.must_change_password, u.password_hash
		FROM users u JOIN branches b ON b.id = u.branch_id
		JOIN employees e ON e.user_id = u.id
		WHERE lower(u.email) = $1 AND u.active AND e.employment_status = 'active'
	`, email)
}

func (a *API) loadPrincipalByID(ctx context.Context, id uuid.UUID) (principal, string, error) {
	return a.loadPrincipal(ctx, `
		SELECT u.id, u.email, u.role, u.branch_id, b.code, b.name, u.manager_id,
		       e.full_name, e.employee_number, u.must_change_password, u.password_hash
		FROM users u JOIN branches b ON b.id = u.branch_id
		JOIN employees e ON e.user_id = u.id
		WHERE u.id = $1 AND u.active AND e.employment_status = 'active'
	`, id)
}

func (a *API) loadPrincipal(ctx context.Context, query string, arg any) (principal, string, error) {
	var user principal
	var passwordHash string
	err := a.db.QueryRow(ctx, query, arg).Scan(
		&user.ID, &user.Email, &user.Role, &user.BranchID, &user.BranchCode, &user.BranchName,
		&user.ManagerID, &user.FullName, &user.EmployeeNumber, &user.MustChangePassword, &passwordHash,
	)
	return user, passwordHash, err
}

func (a *API) createToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    "hris-platform",
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(8 * time.Hour)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
}

func (a *API) setSessionCookie(w http.ResponseWriter, token string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: "hris_session", Value: token, Path: "/api", HttpOnly: true,
		Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode,
		MaxAge: int(maxAge.Seconds()), Expires: time.Now().Add(maxAge),
	})
}

func (a *API) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: "hris_session", Value: "", Path: "/api", HttpOnly: true,
		Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
		Expires: time.Unix(0, 0),
	})
}

func (a *API) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeError(w, http.StatusForbidden, "Permintaan lintas situs ditolak.")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(parsed.Host, r.Host) || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				writeError(w, http.StatusForbidden, "Permintaan lintas situs ditolak.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func currentPrincipal(r *http.Request) principal {
	return r.Context().Value(contextKey{}).(principal)
}

func isHR(role string) bool {
	return role == "hr" || role == "superadmin"
}

func validManagedRole(role string) bool {
	return role == "employee" || role == "manager" || role == "finance"
}

func optionalBirthDate(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*value), jakartaTime)
	if err != nil || parsed.After(time.Now()) {
		return nil, err
	}
	return &parsed, nil
}

func optionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func isActiveBranch(ctx context.Context, tx pgx.Tx, branchID uuid.UUID) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, "SELECT active FROM branches WHERE id = $1 FOR SHARE", branchID).Scan(&active)
	return active, err
}

func insertAudit(ctx context.Context, tx pgx.Tx, actor uuid.UUID, action, entityType, entityID string, details map[string]string) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, details)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, actor, action, entityType, entityID, string(detailsJSON))
	return err
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "Isi permintaan tidak valid.")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "Isi permintaan harus berupa satu objek JSON.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func clientIP(r *http.Request) string {
	if forwardedIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwardedIP != nil {
		return forwardedIP.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func boolToString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'f', 2, 64)
}

// nextNIK menghasilkan NIK unik 16 digit otomatis.
func nextNIK(ctx context.Context, q rowQuerier) (string, error) {
	var next int
	if err := q.QueryRow(ctx, "SELECT COALESCE(max(substring(nik from 11)::integer), 0) + 1 FROM employees WHERE nik ~ '^90[0-9]{14}$'").Scan(&next); err != nil {
		return "", err
	}
	return "90" + time.Now().In(jakartaTime).Format("20060102") + fmt.Sprintf("%06d", next), nil
}

// nextEmployeeNumber menghasilkan nomor karyawan otomatis: kode cabang + tahun + urutan 4 digit.
func nextEmployeeNumber(ctx context.Context, q rowQuerier, branchID uuid.UUID) (string, error) {
	var code string
	if err := q.QueryRow(ctx, "SELECT code FROM branches WHERE id = $1", branchID).Scan(&code); err != nil {
		return "", err
	}
	year := time.Now().In(jakartaTime).Format("2006")
	prefix := strings.ToUpper(code) + "-" + year + "-"
	var last string
	err := q.QueryRow(ctx, `
		SELECT employee_number FROM employees
		WHERE employee_number LIKE $1 || '%'
		ORDER BY employee_number DESC LIMIT 1
	`, prefix).Scan(&last)
	next := 1
	if err == nil {
		if parsed, convErr := strconv.Atoi(strings.TrimPrefix(last, prefix)); convErr == nil {
			next = parsed + 1
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return fmt.Sprintf("%s%04d", prefix, next), nil
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]attemptWindow
	limit    int
	window   time.Duration
	dummy    string
}

type attemptWindow struct {
	start time.Time
	count int
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	dummyHash, err := security.HashPassword("dummy-timing-password-never-used")
	if err != nil {
		panic("initialize login timing protection: " + err.Error())
	}
	return &loginLimiter{
		attempts: make(map[string]attemptWindow),
		limit:    limit, window: window, dummy: dummyHash,
	}
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.attempts) > 4096 {
		for key, value := range l.attempts {
			if now.Sub(value.start) >= l.window {
				delete(l.attempts, key)
			}
		}
	}
	current := l.attempts[ip]
	if now.Sub(current.start) >= l.window {
		current = attemptWindow{start: now}
	}
	if current.count >= l.limit {
		l.attempts[ip] = current
		return false
	}
	current.count++
	l.attempts[ip] = current
	return true
}

func (l *loginLimiter) consumeDummy(password string) {
	_ = security.VerifyPassword(l.dummy, password)
}
