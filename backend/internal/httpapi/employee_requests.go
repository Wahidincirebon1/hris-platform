package httpapi

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type employeeRequestView struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	EmployeeName   string     `json:"employee_name"`
	EmployeeNumber string     `json:"employee_number"`
	BranchName     string     `json:"branch_name"`
	ManagerID      *uuid.UUID `json:"manager_id,omitempty"`
	RequestType    string     `json:"request_type"`
	StartDate      string     `json:"start_date"`
	EndDate        string     `json:"end_date"`
	StartTime      *string    `json:"start_time,omitempty"`
	EndTime        *string    `json:"end_time,omitempty"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ManagerNote    *string    `json:"manager_note,omitempty"`
	HRNote         *string    `json:"hr_note,omitempty"`
	SuperadminNote *string    `json:"superadmin_note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (a *API) registerEmployeeRequestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/employee-requests/me", a.authenticated(a.myEmployeeRequests))
	mux.HandleFunc("GET /api/v1/employee-requests/approvals", a.authenticated(a.employeeRequestApprovals))
	mux.HandleFunc("POST /api/v1/employee-requests", a.authenticated(a.createEmployeeRequest))
	mux.HandleFunc("POST /api/v1/employee-requests/{id}/withdraw", a.authenticated(a.withdrawEmployeeRequest))
	mux.HandleFunc("POST /api/v1/employee-requests/{id}/manager-decision", a.authenticated(a.managerEmployeeRequestDecision))
	mux.HandleFunc("POST /api/v1/employee-requests/{id}/hr-decision", a.authenticated(a.hrEmployeeRequestDecision))
	mux.HandleFunc("POST /api/v1/employee-requests/{id}/superadmin-decision", a.authenticated(a.superadminEmployeeRequestDecision))
}

func (a *API) myEmployeeRequests(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Layanan mandiri tidak tersedia untuk role ini.")
		return
	}
	requests, err := queryEmployeeRequests(r.Context(), a.db, "WHERE q.user_id = $1", user.ID)
	if err != nil {
		log.Printf("load personal requests failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Riwayat pengajuan tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (a *API) employeeRequestApprovals(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	var (
		where string
		args  []any
	)
	switch user.Role {
	case "manager":
		where, args = "WHERE q.status = 'pending_manager' AND q.manager_id = $1", []any{user.ID}
	case "hr":
		// HR adalah pemroses data: melihat semua antrean, termasuk yang masih menunggu manager.
		where, args = "WHERE q.status IN ('pending_manager', 'pending_hr') AND q.branch_id = $1", []any{user.BranchID}
	case "superadmin":
		where = "WHERE q.status IN ('pending_manager', 'pending_hr', 'pending_superadmin')"
	default:
		writeError(w, http.StatusForbidden, "Antrean persetujuan hanya tersedia untuk manager, HR, dan superadmin.")
		return
	}
	requests, err := queryEmployeeRequests(r.Context(), a.db, where, args...)
	if err != nil {
		log.Printf("load employee request approvals failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Antrean persetujuan tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (a *API) createEmployeeRequest(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat membuat pengajuan pribadi.")
		return
	}
	var input struct {
		RequestType string `json:"request_type"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Reason      string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.RequestType = strings.TrimSpace(input.RequestType)
	input.StartTime = strings.TrimSpace(input.StartTime)
	input.EndTime = strings.TrimSpace(input.EndTime)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.RequestType != "overtime" && input.RequestType != "permission" {
		writeError(w, http.StatusBadRequest, "Jenis pengajuan harus lembur atau izin.")
		return
	}
	start, err := parseEffectiveDate(input.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Tanggal mulai tidak valid.")
		return
	}
	end, err := parseEffectiveDate(input.EndDate)
	if err != nil || end.Before(start) {
		writeError(w, http.StatusBadRequest, "Rentang tanggal tidak valid.")
		return
	}
	if start.Before(jakartaDate(time.Now())) {
		writeError(w, http.StatusBadRequest, "Pengajuan tidak dapat dibuat untuk tanggal yang sudah lewat.")
		return
	}
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 1000 {
		writeError(w, http.StatusBadRequest, "Alasan harus berisi 5 sampai 1.000 karakter.")
		return
	}
	if input.RequestType == "overtime" {
		if start != end || !validRequestTime(input.StartTime) || !validRequestTime(input.EndTime) ||
			input.EndTime <= input.StartTime {
			writeError(w, http.StatusBadRequest, "Pengajuan lembur harus pada satu tanggal dengan jam mulai dan selesai yang valid.")
			return
		}
	} else if (input.StartTime == "") != (input.EndTime == "") ||
		(input.StartTime != "" && (!validRequestTime(input.StartTime) || !validRequestTime(input.EndTime) ||
			start != end || input.EndTime <= input.StartTime)) {
		writeError(w, http.StatusBadRequest, "Jam izin harus diisi berpasangan dan berlaku untuk satu hari.")
		return
	}

	var status string
	var managerID *uuid.UUID
	switch user.Role {
	case "employee":
		if user.ManagerID == nil {
			writeError(w, http.StatusConflict, "Atasan langsung belum ditetapkan; hubungi HR.")
			return
		}
		status, managerID = "pending_manager", user.ManagerID
	case "manager":
		status = "pending_hr"
	case "hr":
		// Pengajuan HR dikirim ke superadmin, tetapi HR tetap dapat melihat seluruh antrean.
		status = "pending_superadmin"
	default:
		writeError(w, http.StatusForbidden, "Role ini tidak dapat membuat pengajuan pribadi.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO employee_requests
			(user_id, branch_id, manager_id, request_type, start_date, end_date,
			 start_time, end_time, reason, status)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::time, NULLIF($8, '')::time, $9, $10)
		RETURNING id
	`, user.ID, user.BranchID, managerID, input.RequestType, start, end,
		input.StartTime, input.EndTime, input.Reason, status).Scan(&id)
	if err != nil {
		log.Printf("create employee request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "CREATE_"+strings.ToUpper(input.RequestType)+"_REQUEST", "employee_request", id.String(), map[string]string{
		"start_date": input.StartDate, "end_date": input.EndDate, "status": status,
	}); err != nil {
		log.Printf("audit employee request creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan tersimpan, tetapi audit gagal dicatat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit employee request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
}

func (a *API) withdrawEmployeeRequest(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat menarik pengajuan.")
		return
	}
	id, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat ditarik.")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		UPDATE employee_requests
		SET status = 'cancelled', cancelled_by = $2, cancelled_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
		  AND status IN ('pending_manager', 'pending_hr', 'pending_superadmin')
	`, id, user.ID)
	if err != nil {
		log.Printf("withdraw employee request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat ditarik.")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "Pengajuan tidak ditemukan atau tidak dapat ditarik.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "WITHDRAW_EMPLOYEE_REQUEST", "employee_request", id.String(), map[string]string{}); err != nil {
		log.Printf("audit employee request withdrawal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan ditarik, tetapi audit gagal dicatat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit employee request withdrawal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan tidak dapat ditarik.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (a *API) managerEmployeeRequestDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "manager" {
		writeError(w, http.StatusForbidden, "Hanya manager yang berwenang pada tahap persetujuan ini.")
		return
	}
	a.decideEmployeeRequest(w, r, user, "manager", "pending_manager", "pending_hr")
}

func (a *API) hrEmployeeRequestDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "hr" {
		writeError(w, http.StatusForbidden, "Hanya HR yang berwenang pada tahap persetujuan ini.")
		return
	}
	a.decideEmployeeRequest(w, r, user, "hr", "pending_hr", "approved")
}

func (a *API) superadminEmployeeRequestDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "Hanya superadmin yang dapat meninjau pengajuan pribadi HR.")
		return
	}
	a.decideEmployeeRequest(w, r, user, "superadmin", "pending_superadmin", "approved")
}

func (a *API) decideEmployeeRequest(w http.ResponseWriter, r *http.Request, user principal, stage, requiredStatus, approvedStatus string) {
	var input struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if input.Decision != "approve" && input.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "Keputusan harus approve atau reject.")
		return
	}
	if len([]rune(input.Note)) > 500 || (input.Decision == "reject" && !validDecisionNote(input.Note)) {
		writeError(w, http.StatusBadRequest, "Alasan penolakan wajib diisi (5–500 karakter).")
		return
	}
	id, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	nextStatus := approvedStatus
	if input.Decision == "reject" {
		nextStatus = "rejected"
	}
	columnPrefix := map[string]string{
		"manager": "manager", "hr": "hr", "superadmin": "superadmin",
	}[stage]
	query := `
		UPDATE employee_requests
		SET status = $1,
		    ` + columnPrefix + `_decision_by = $2,
		    ` + columnPrefix + `_decision_at = now(),
		    ` + columnPrefix + `_note = NULLIF($3, ''),
		    updated_at = now()
		WHERE id = $4 AND status = $5`
	args := []any{nextStatus, user.ID, input.Note, id, requiredStatus}
	if stage == "manager" {
		query += " AND manager_id = $6"
		args = append(args, user.ID)
	}
	if stage == "hr" || stage == "superadmin" {
		query += " AND user_id <> $6"
		args = append(args, user.ID)
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan pengajuan tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), query, args...)
	if err != nil {
		log.Printf("%s employee request decision failed: %v", stage, err)
		writeError(w, http.StatusInternalServerError, "Keputusan pengajuan tidak dapat disimpan.")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "Pengajuan tidak ditemukan atau bukan wewenang Anda.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, strings.ToUpper(stage)+"_"+strings.ToUpper(input.Decision)+"_EMPLOYEE_REQUEST", "employee_request", id.String(), map[string]string{"note": input.Note}); err != nil {
		log.Printf("audit employee request decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan disimpan, tetapi audit gagal dicatat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit employee request decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan pengajuan tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": nextStatus})
}

func queryEmployeeRequests(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, where string, args ...any) ([]employeeRequestView, error) {
	rows, err := q.Query(ctx, `
		SELECT q.id, q.user_id, e.full_name, e.employee_number, b.name, q.manager_id,
			q.request_type, q.start_date, q.end_date,
			to_char(q.start_time, 'HH24:MI'), to_char(q.end_time, 'HH24:MI'),
			q.reason, q.status, q.manager_note, q.hr_note, q.superadmin_note, q.created_at
		FROM employee_requests q
		JOIN employees e ON e.user_id = q.user_id
		JOIN branches b ON b.id = q.branch_id
		`+where+`
		ORDER BY q.created_at DESC
		LIMIT 200
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]employeeRequestView, 0)
	for rows.Next() {
		var item employeeRequestView
		var start, end time.Time
		var startTime, endTime pgtype.Text
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.EmployeeName, &item.EmployeeNumber, &item.BranchName,
			&item.ManagerID, &item.RequestType, &start, &end, &startTime, &endTime,
			&item.Reason, &item.Status, &item.ManagerNote, &item.HRNote, &item.SuperadminNote,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.StartDate = start.Format("2006-01-02")
		item.EndDate = end.Format("2006-01-02")
		if startTime.Valid {
			item.StartTime = &startTime.String
		}
		if endTime.Valid {
			item.EndTime = &endTime.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func canUseEmployeeSelfService(role string) bool {
	return canUseSelfService(role)
}

func validRequestTime(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	_, err := time.Parse("15:04", value)
	return err == nil
}
