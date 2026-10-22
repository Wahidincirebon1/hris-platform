package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const leaveOperationLock int64 = 16473190

type calendarEntry struct {
	ID     uuid.UUID `json:"id"`
	Date   string    `json:"date"`
	Name   string    `json:"name"`
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
}

type leaveBalance struct {
	Year             int     `json:"year"`
	PolicyConfigured bool    `json:"policy_configured"`
	AnnualDays       int     `json:"annual_days"`
	BalanceDays      float64 `json:"balance_days"`
	UsedDays         float64 `json:"used_days"`
}

type leaveRequestView struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	EmployeeName   string     `json:"employee_name"`
	EmployeeNumber string     `json:"employee_number"`
	BranchName     string     `json:"branch_name"`
	ManagerID      *uuid.UUID `json:"manager_id,omitempty"`
	Year           int        `json:"year"`
	StartDate      string     `json:"start_date"`
	EndDate        string     `json:"end_date"`
	DayCount       int        `json:"day_count"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ManagerNote    *string    `json:"manager_note,omitempty"`
	HRNote         *string    `json:"hr_note,omitempty"`
	SuperadminNote *string    `json:"superadmin_note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (a *API) registerLeaveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/leave/me", a.authenticated(a.myLeaveData))
	mux.HandleFunc("GET /api/v1/leave/requests", a.authenticated(a.listLeaveRequests))
	mux.HandleFunc("POST /api/v1/leave/requests", a.authenticated(a.createLeaveRequest))
	mux.HandleFunc("POST /api/v1/leave/requests/{id}/withdraw", a.authenticated(a.withdrawLeaveRequest))
	mux.HandleFunc("POST /api/v1/leave/requests/{id}/manager-decision", a.authenticated(a.managerLeaveDecision))
	mux.HandleFunc("POST /api/v1/leave/requests/{id}/hr-decision", a.authenticated(a.hrLeaveDecision))
	mux.HandleFunc("POST /api/v1/leave/requests/{id}/superadmin-decision", a.authenticated(a.superadminLeaveDecision))
	mux.HandleFunc("POST /api/v1/leave/requests/{id}/cancel", a.authenticated(a.cancelApprovedLeave))
	mux.HandleFunc("GET /api/v1/leave/settings", a.authenticated(a.leaveSettings))
	mux.HandleFunc("PUT /api/v1/leave/entitlements", a.authenticated(a.setLeaveEntitlement))
	mux.HandleFunc("POST /api/v1/leave/calendar", a.authenticated(a.createCalendarEntry))
	mux.HandleFunc("PATCH /api/v1/leave/calendar/{id}", a.authenticated(a.updateCalendarEntry))
}

func (a *API) myLeaveData(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Layanan cuti pribadi tidak tersedia untuk role ini.")
		return
	}
	year, ok := requestedLeaveYear(w, r)
	if !ok {
		return
	}
	balance, err := loadLeaveBalance(r.Context(), a.db, user.ID, year)
	if err != nil {
		log.Printf("load personal leave balance failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Saldo cuti tidak dapat dimuat.")
		return
	}
	requests, err := queryLeaveRequests(r.Context(), a.db, "WHERE l.user_id = $1", user.ID)
	if err != nil {
		log.Printf("load personal leave requests failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"balance": balance, "requests": requests,
	})
}

func (a *API) listLeaveRequests(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	var (
		where string
		args  []any
	)
	switch {
	case user.Role == "hr":
		where = "WHERE l.status IN ('pending_hr', 'approved')"
	case user.Role == "superadmin":
		where = "WHERE l.status = 'pending_superadmin'"
	case user.Role == "manager":
		where, args = "WHERE l.manager_id = $1 AND l.status = 'pending_manager'", []any{user.ID}
	default:
		writeError(w, http.StatusForbidden, "Daftar persetujuan cuti hanya tersedia untuk manager dan HR.")
		return
	}
	requests, err := queryLeaveRequests(r.Context(), a.db, where, args...)
	if err != nil {
		log.Printf("load leave approval queue failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Daftar persetujuan cuti tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (a *API) createLeaveRequest(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat mengajukan cuti.")
		return
	}
	var input struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Reason    string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	start, err := parseEffectiveDate(input.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Tanggal mulai cuti tidak valid.")
		return
	}
	end, err := parseEffectiveDate(input.EndDate)
	if err != nil || end.Before(start) {
		writeError(w, http.StatusBadRequest, "Rentang tanggal cuti tidak valid.")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 1000 {
		writeError(w, http.StatusBadRequest, "Alasan cuti harus berisi 5 sampai 1.000 karakter.")
		return
	}
	year := start.Year()
	if end.Year() != year || year < 2000 || year > 2100 {
		writeError(w, http.StatusBadRequest, "Pengajuan harus berada dalam satu tahun kalender.")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock leave request creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		return
	}
	var active bool
	var managerID *uuid.UUID
	if err := tx.QueryRow(r.Context(), `
		SELECT u.active, u.manager_id
		FROM users u
		JOIN employees e ON e.user_id = u.id
		WHERE u.id = $1 AND e.employment_status = 'active'
		FOR UPDATE OF u
	`, user.ID).Scan(&active, &managerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusForbidden, "Profil karyawan aktif tidak ditemukan.")
		} else {
			log.Printf("validate leave applicant failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		}
		return
	}
	if !active {
		writeError(w, http.StatusForbidden, "Akun karyawan tidak aktif.")
		return
	}
	status := ""
	switch user.Role {
	case "employee":
		status = "pending_manager"
	case "manager":
		status, managerID = "pending_hr", nil
	case "hr":
		status, managerID = "pending_superadmin", nil
	}
	if user.Role == "employee" && managerID == nil {
		writeError(w, http.StatusConflict, "Atasan langsung belum ditetapkan; hubungi HR.")
		return
	}
	var entitlementConfigured bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM leave_entitlements WHERE leave_year = $1)", year).Scan(&entitlementConfigured); err != nil {
		log.Printf("check leave entitlement policy failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kebijakan cuti tidak dapat diverifikasi.")
		return
	}
	if !entitlementConfigured {
		writeError(w, http.StatusConflict, "Hak cuti tahun ini belum ditetapkan HR.")
		return
	}
	if start.Before(jakartaDate(time.Now())) {
		writeError(w, http.StatusBadRequest, "Pengajuan cuti tidak dapat dibuat untuk tanggal yang sudah lewat.")
		return
	}
	workDates, err := workDatesInRange(r.Context(), tx, start, end)
	if err != nil {
		log.Printf("calculate requested leave days failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hari kerja cuti tidak dapat dihitung.")
		return
	}
	if len(workDates) == 0 {
		writeError(w, http.StatusBadRequest, "Rentang tersebut tidak berisi hari kerja.")
		return
	}
	var overlap bool
	if err := tx.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM leave_request_days d
			JOIN leave_requests l ON l.id = d.request_id
			WHERE l.user_id = $1 AND d.leave_date BETWEEN $2 AND $3
				AND l.status IN ('pending_manager', 'pending_hr', 'pending_superadmin', 'approved')
		)
	`, user.ID, start, end).Scan(&overlap); err != nil {
		log.Printf("check overlapping leave requests failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal cuti tidak dapat diverifikasi.")
		return
	}
	if overlap {
		writeError(w, http.StatusConflict, "Tanggal tersebut bertumpang tindih dengan pengajuan cuti aktif.")
		return
	}
	_, _, balance, _, err := leaveBalanceValues(r.Context(), tx, user.ID, year)
	if err != nil {
		log.Printf("check available leave balance failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Saldo cuti tidak dapat diverifikasi.")
		return
	}
	if balance+1e-9 < float64(len(workDates)) {
		writeError(w, http.StatusConflict, "Saldo cuti tidak mencukupi.")
		return
	}

	var requestID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO leave_requests
			(user_id, branch_id, manager_id, leave_year, start_date, end_date, day_count, reason, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, user.ID, user.BranchID, managerID, year, start, end, len(workDates), input.Reason, status).Scan(&requestID)
	if err != nil {
		log.Printf("create leave request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		return
	}
	for _, date := range workDates {
		if _, err := tx.Exec(r.Context(), "INSERT INTO leave_request_days (request_id, leave_date) VALUES ($1, $2)", requestID, date); err != nil {
			log.Printf("save leave request dates failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, user.ID, "CREATE_LEAVE_REQUEST", "leave_request", requestID.String(), map[string]string{
		"start_date": input.StartDate, "end_date": input.EndDate, "day_count": strconv.Itoa(len(workDates)),
	}); err != nil {
		log.Printf("audit leave request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit leave request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat dibuat.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": requestID, "status": status, "day_count": len(workDates)})
}

func (a *API) withdrawLeaveRequest(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	requestID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat ditarik.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock leave withdrawal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat ditarik.")
		return
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE leave_requests
		SET status = 'cancelled', cancelled_by = $3, cancelled_at = now(),
			cancellation_reason = 'Ditarik oleh karyawan', updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status IN ('pending_manager', 'pending_hr', 'pending_superadmin')
	`, requestID, user.ID, user.ID)
	if err != nil {
		log.Printf("withdraw leave request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat ditarik.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Pengajuan tidak ditemukan atau sudah tidak dapat ditarik.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "WITHDRAW_LEAVE_REQUEST", "leave_request", requestID.String(), map[string]string{}); err != nil {
		log.Printf("audit leave withdrawal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat ditarik.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Pengajuan cuti tidak dapat ditarik.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (a *API) managerLeaveDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "manager" {
		writeError(w, http.StatusForbidden, "Hanya manager yang ditetapkan yang dapat memberikan keputusan tahap pertama.")
		return
	}
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
		writeError(w, http.StatusBadRequest, "Catatan keputusan maksimal 500 karakter; alasan penolakan wajib diisi (5–500 karakter).")
		return
	}
	requestID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock manager leave decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	nextStatus := "pending_hr"
	if input.Decision == "reject" {
		nextStatus = "rejected"
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE leave_requests
		SET status = $3, manager_decision_by = $4, manager_decision_at = now(),
			manager_note = NULLIF($5, ''), updated_at = now()
		WHERE id = $1 AND manager_id = $2 AND status = 'pending_manager'
	`, requestID, user.ID, nextStatus, user.ID, input.Note)
	if err != nil {
		log.Printf("save manager leave decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Pengajuan tidak ditemukan atau bukan wewenang Anda.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "MANAGER_LEAVE_"+strings.ToUpper(input.Decision), "leave_request", requestID.String(), map[string]string{"note": input.Note}); err != nil {
		log.Printf("audit manager leave decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": nextStatus})
}

func (a *API) hrLeaveDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "hr" {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat memberikan keputusan akhir cuti.")
		return
	}
	a.finalizeLeaveRequest(w, r, user, "pending_hr", "hr")
}

func (a *API) superadminLeaveDecision(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "Hanya superadmin yang dapat meninjau pengajuan cuti pribadi HR.")
		return
	}
	a.finalizeLeaveRequest(w, r, user, "pending_superadmin", "superadmin")
}

func (a *API) finalizeLeaveRequest(w http.ResponseWriter, r *http.Request, user principal, requiredStatus, stage string) {
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
		writeError(w, http.StatusBadRequest, "Catatan keputusan maksimal 500 karakter; alasan penolakan wajib diisi (5–500 karakter).")
		return
	}
	requestID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock %s leave decision failed: %v", stage, err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	var applicantID uuid.UUID
	var leaveYear int
	var dayCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT user_id, leave_year, day_count
		FROM leave_requests WHERE id = $1 AND status = $2
		FOR UPDATE
	`, requestID, requiredStatus).Scan(&applicantID, &leaveYear, &dayCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Pengajuan tidak ditemukan atau belum berada pada tahap persetujuan Anda.")
		} else {
			log.Printf("load leave request for %s decision failed: %v", stage, err)
			writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		}
		return
	}
	if applicantID == user.ID {
		writeError(w, http.StatusForbidden, "Approver tidak dapat memberikan keputusan atas pengajuan cutinya sendiri.")
		return
	}
	nextStatus := "rejected"
	if input.Decision == "approve" {
		var available float64
		_, _, available, _, err = leaveBalanceValues(r.Context(), tx, applicantID, leaveYear)
		if err != nil {
			log.Printf("recheck leave balance before approval failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Saldo cuti tidak dapat diverifikasi.")
			return
		}
		if available+1e-9 < float64(dayCount) {
			writeError(w, http.StatusConflict, "Saldo cuti tidak lagi mencukupi untuk persetujuan.")
			return
		}
		nextStatus = "approved"
	}
	decisionColumn := stage
	tag, err := tx.Exec(r.Context(), `
		UPDATE leave_requests
		SET status = $2, `+decisionColumn+`_decision_by = $3, `+decisionColumn+`_decision_at = now(),
			`+decisionColumn+`_note = NULLIF($4, ''), updated_at = now()
		WHERE id = $1 AND status = $5
	`, requestID, nextStatus, user.ID, input.Note, requiredStatus)
	if err != nil || tag.RowsAffected() != 1 {
		log.Printf("save %s leave decision failed: %v", stage, err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	if input.Decision == "approve" {
		rows, err := tx.Query(r.Context(), "SELECT leave_date FROM leave_request_days WHERE request_id = $1 ORDER BY leave_date", requestID)
		if err != nil {
			log.Printf("load approved leave dates failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Debit saldo cuti tidak dapat disimpan.")
			return
		}
		var dates []time.Time
		for rows.Next() {
			var date time.Time
			if err := rows.Scan(&date); err != nil {
				rows.Close()
				log.Printf("scan approved leave date failed: %v", err)
				writeError(w, http.StatusInternalServerError, "Debit saldo cuti tidak dapat disimpan.")
				return
			}
			dates = append(dates, date)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			log.Printf("read approved leave dates failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Debit saldo cuti tidak dapat disimpan.")
			return
		}
		for _, date := range dates {
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO leave_ledger_entries
					(user_id, leave_year, entry_type, amount, request_id, entry_date, reason, created_by)
				VALUES ($1, $2, 'leave_request', -1, $3, $4, $5, $6)
			`, applicantID, leaveYear, requestID, date, "Pemakaian cuti disetujui", user.ID); err != nil {
				log.Printf("debit approved leave balance failed: %v", err)
				writeError(w, http.StatusInternalServerError, "Debit saldo cuti tidak dapat disimpan.")
				return
			}
		}
	}
	if err := insertAudit(r.Context(), tx, user.ID, strings.ToUpper(stage)+"_LEAVE_"+strings.ToUpper(input.Decision), "leave_request", requestID.String(), map[string]string{"note": input.Note}); err != nil {
		log.Printf("audit %s leave decision failed: %v", stage, err)
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan cuti tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": nextStatus})
}

func (a *API) cancelApprovedLeave(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Cuti yang sudah disetujui hanya dapat dibatalkan HR.")
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if !validDecisionNote(input.Reason) {
		writeError(w, http.StatusBadRequest, "Alasan pembatalan wajib diisi (5–500 karakter).")
		return
	}
	requestID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock approved leave cancellation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		return
	}
	var applicantID uuid.UUID
	var leaveYear int
	if err := tx.QueryRow(r.Context(), `
		UPDATE leave_requests
		SET status = 'cancelled', cancelled_by = $2, cancelled_at = now(),
			cancellation_reason = $3, updated_at = now()
		WHERE id = $1 AND status = 'approved'
		RETURNING user_id, leave_year
	`, requestID, user.ID, input.Reason).Scan(&applicantID, &leaveYear); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Cuti tidak ditemukan atau tidak berstatus disetujui.")
		} else {
			log.Printf("cancel approved leave failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		}
		return
	}
	if applicantID == user.ID {
		writeError(w, http.StatusForbidden, "HR tidak dapat membatalkan cutinya sendiri.")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO leave_ledger_entries
			(user_id, leave_year, entry_type, amount, request_id, entry_date, reason, created_by)
		SELECT user_id, leave_year, 'leave_request_reversal', 1, request_id, entry_date, $2, $3
		FROM leave_ledger_entries
		WHERE request_id = $1 AND entry_type = 'leave_request'
	`, requestID, input.Reason, user.ID); err != nil {
		log.Printf("reverse cancelled leave debit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "CANCEL_APPROVED_LEAVE", "leave_request", requestID.String(), map[string]string{
		"reason": input.Reason, "employee_id": applicantID.String(), "leave_year": strconv.Itoa(leaveYear),
	}); err != nil {
		log.Printf("audit approved leave cancellation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Pembatalan cuti tidak dapat diproses.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (a *API) leaveSettings(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Pengaturan cuti hanya tersedia untuk HR.")
		return
	}
	year, ok := requestedLeaveYear(w, r)
	if !ok {
		return
	}
	var annualDays int
	var reason string
	err := a.db.QueryRow(r.Context(), `
		SELECT annual_days, reason FROM leave_entitlements WHERE leave_year = $1
	`, year).Scan(&annualDays, &reason)
	configured := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load leave entitlement settings failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengaturan hak cuti tidak dapat dimuat.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, calendar_date, name, kind, reason
		FROM company_calendar_days
		WHERE calendar_date >= $1 AND calendar_date < $2
		ORDER BY calendar_date
	`, time.Date(year, 1, 1, 0, 0, 0, 0, jakartaTime), time.Date(year+1, 1, 1, 0, 0, 0, 0, jakartaTime))
	if err != nil {
		log.Printf("load company leave calendar failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	calendar := make([]calendarEntry, 0)
	for rows.Next() {
		var item calendarEntry
		var date time.Time
		if err := rows.Scan(&item.ID, &date, &item.Name, &item.Kind, &item.Reason); err != nil {
			log.Printf("scan company calendar entry failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat dimuat.")
			return
		}
		item.Date = date.Format("2006-01-02")
		calendar = append(calendar, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read company leave calendar failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"year": year, "policy_configured": configured, "annual_days": annualDays,
		"reason": reason, "calendar": calendar,
	})
}

func (a *API) setLeaveEntitlement(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat menetapkan hak cuti.")
		return
	}
	var input struct {
		Year       int    `json:"year"`
		AnnualDays int    `json:"annual_days"`
		Reason     string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Year < 2000 || input.Year > 2100 || input.AnnualDays < 0 || input.AnnualDays > 60 || !validDecisionNote(input.Reason) {
		writeError(w, http.StatusBadRequest, "Tahun, hak cuti (0–60 hari), atau alasan tidak valid.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock leave entitlement update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	var oldDays int
	err = tx.QueryRow(r.Context(), "SELECT annual_days FROM leave_entitlements WHERE leave_year = $1 FOR UPDATE", input.Year).Scan(&oldDays)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load previous leave entitlement failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	if exists {
		_, err = tx.Exec(r.Context(), `
			UPDATE leave_entitlements
			SET annual_days = $2, reason = $3, updated_by = $4, updated_at = now()
			WHERE leave_year = $1
		`, input.Year, input.AnnualDays, input.Reason, user.ID)
	} else {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO leave_entitlements (leave_year, annual_days, reason, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $4)
		`, input.Year, input.AnnualDays, input.Reason, user.ID)
	}
	if err != nil {
		log.Printf("save leave entitlement policy failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	rows, err := tx.Query(r.Context(), `
		SELECT u.id
		FROM users u
		JOIN employees e ON e.user_id = u.id
		WHERE u.active AND e.employment_status = 'active'
		ORDER BY u.id
		FOR UPDATE OF u
	`)
	if err != nil {
		log.Printf("load active employees for entitlement grant failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diberikan.")
		return
	}
	employeeIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var employeeID uuid.UUID
		if err := rows.Scan(&employeeID); err != nil {
			rows.Close()
			log.Printf("scan active employee for entitlement grant failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diberikan.")
			return
		}
		employeeIDs = append(employeeIDs, employeeID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		log.Printf("read active employees for entitlement grant failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diberikan.")
		return
	}
	for _, employeeID := range employeeIDs {
		if err := syncAnnualGrant(r.Context(), tx, employeeID, input.Year, input.AnnualDays, input.Reason, user.ID); err != nil {
			log.Printf("synchronize employee leave grant failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diberikan.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, user.ID, "SET_LEAVE_ENTITLEMENT", "leave_entitlement", strconv.Itoa(input.Year), map[string]string{
		"annual_days": strconv.Itoa(input.AnnualDays), "reason": input.Reason,
	}); err != nil {
		log.Printf("audit leave entitlement update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": input.Year, "annual_days": input.AnnualDays, "employees_updated": len(employeeIDs)})
}

func (a *API) createCalendarEntry(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Kalender perusahaan hanya dapat diubah HR.")
		return
	}
	input, ok := decodeCalendarInput(w, r)
	if !ok {
		return
	}
	date, err := parseEffectiveDate(input.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Tanggal kalender tidak valid.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock company calendar creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat disimpan.")
		return
	}
	if err := ensureCalendarDateHasNoLeaveRequests(r.Context(), tx, date); err != nil {
		writeError(w, http.StatusConflict, "Tanggal tersebut sudah memiliki pengajuan cuti aktif; selesaikan pengajuan terlebih dahulu.")
		return
	}
	cycleID := uuid.New()
	if input.Kind != "collective_leave" {
		cycleID = uuid.Nil
	}
	var configured bool
	var annualDays int
	err = tx.QueryRow(r.Context(), "SELECT annual_days FROM leave_entitlements WHERE leave_year = $1", date.Year()).Scan(&annualDays)
	configured = err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load entitlement for company calendar failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diverifikasi.")
		return
	}
	var dayID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO company_calendar_days
			(calendar_date, name, kind, active_leave_cycle, reason, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id
	`, date, input.Name, input.Kind, nullableUUID(cycleID), input.Reason, user.ID).Scan(&dayID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Tanggal kalender sudah terdaftar.")
			return
		}
		log.Printf("create company calendar entry failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat disimpan.")
		return
	}
	if input.Kind == "collective_leave" && isScheduledWorkday(date) {
		if !configured {
			writeError(w, http.StatusConflict, "Tetapkan hak cuti tahun ini sebelum menerapkan cuti bersama.")
			return
		}
		if err := applyCollectiveLeave(r.Context(), tx, dayID, date, cycleID, annualDays, input.Reason, user.ID); err != nil {
			log.Printf("apply collective leave debit failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pemotongan saldo cuti bersama tidak dapat disimpan.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, user.ID, "CREATE_COMPANY_CALENDAR_DAY", "company_calendar_day", dayID.String(), map[string]string{
		"date": input.Date, "name": input.Name, "kind": input.Kind, "reason": input.Reason,
	}); err != nil {
		log.Printf("audit company calendar creation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, calendarEntry{ID: dayID, Date: input.Date, Name: input.Name, Kind: input.Kind, Reason: input.Reason})
}

func (a *API) updateCalendarEntry(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Kalender perusahaan hanya dapat diubah HR.")
		return
	}
	input, ok := decodeCalendarInput(w, r)
	if !ok {
		return
	}
	date, err := parseEffectiveDate(input.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Tanggal kalender tidak valid.")
		return
	}
	dayID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockLeaveOperations(r.Context(), tx); err != nil {
		log.Printf("lock company calendar update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		return
	}
	var oldDate time.Time
	var oldKind string
	var oldCycle pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
		SELECT calendar_date, kind, active_leave_cycle
		FROM company_calendar_days WHERE id = $1 FOR UPDATE
	`, dayID).Scan(&oldDate, &oldKind, &oldCycle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Tanggal kalender tidak ditemukan.")
		} else {
			log.Printf("load calendar day for update failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		}
		return
	}
	oldDate = jakartaDate(oldDate)
	if !oldDate.Equal(date) {
		writeError(w, http.StatusBadRequest, "Tanggal entri kalender tidak dapat dipindahkan; hapus lalu buat entri baru.")
		return
	}
	if err := ensureCalendarDateHasNoLeaveRequests(r.Context(), tx, date); err != nil {
		writeError(w, http.StatusConflict, "Tanggal tersebut sudah memiliki pengajuan cuti aktif; selesaikan pengajuan terlebih dahulu.")
		return
	}
	var cycleID uuid.UUID
	if input.Kind == "collective_leave" {
		if oldKind == "collective_leave" && oldCycle.Valid {
			cycleID, err = uuid.FromBytes(oldCycle.Bytes[:])
			if err != nil {
				log.Printf("parse existing collective leave cycle failed: %v", err)
				writeError(w, http.StatusInternalServerError, "Siklus cuti bersama tidak dapat dimuat.")
				return
			}
		} else {
			cycleID = uuid.New()
		}
	}
	var annualDays int
	entitlementErr := tx.QueryRow(r.Context(), "SELECT annual_days FROM leave_entitlements WHERE leave_year = $1", date.Year()).Scan(&annualDays)
	if entitlementErr != nil && !errors.Is(entitlementErr, pgx.ErrNoRows) {
		log.Printf("load entitlement for calendar update failed: %v", entitlementErr)
		writeError(w, http.StatusInternalServerError, "Hak cuti tidak dapat diverifikasi.")
		return
	}
	collectiveWorkday := isScheduledWorkday(date) || oldKind == "company_workday"
	if input.Kind == "collective_leave" && oldKind != "collective_leave" && collectiveWorkday && errors.Is(entitlementErr, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Tetapkan hak cuti tahun ini sebelum menerapkan cuti bersama.")
		return
	}
	if oldKind == "collective_leave" && oldCycle.Valid && input.Kind != "collective_leave" {
		if err := reverseCollectiveLeave(r.Context(), tx, dayID, oldCycle, input.Reason, user.ID); err != nil {
			log.Printf("reverse collective leave debit failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pengembalian saldo cuti tidak dapat disimpan.")
			return
		}
	}
	if input.Kind == "collective_leave" && oldKind != "collective_leave" && collectiveWorkday {
		if err := applyCollectiveLeave(r.Context(), tx, dayID, date, cycleID, annualDays, input.Reason, user.ID); err != nil {
			log.Printf("apply updated collective leave debit failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pemotongan saldo cuti bersama tidak dapat disimpan.")
			return
		}
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE company_calendar_days
		SET name = $2, kind = $3, active_leave_cycle = $4, reason = $5,
			updated_by = $6, updated_at = now()
		WHERE id = $1
	`, dayID, input.Name, input.Kind, nullableUUID(cycleID), input.Reason, user.ID)
	if err != nil {
		log.Printf("update company calendar entry failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "UPDATE_COMPANY_CALENDAR_DAY", "company_calendar_day", dayID.String(), map[string]string{
		"date": input.Date, "old_kind": oldKind, "kind": input.Kind, "reason": input.Reason,
	}); err != nil {
		log.Printf("audit company calendar update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Kalender perusahaan tidak dapat diperbarui.")
		return
	}
	writeJSON(w, http.StatusOK, calendarEntry{ID: dayID, Date: input.Date, Name: input.Name, Kind: input.Kind, Reason: input.Reason})
}

type calendarInput struct {
	Date   string `json:"date"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

func decodeCalendarInput(w http.ResponseWriter, r *http.Request) (calendarInput, bool) {
	var input calendarInput
	if !decodeJSON(w, r, &input) {
		return calendarInput{}, false
	}
	input.Date = strings.TrimSpace(input.Date)
	input.Name = strings.TrimSpace(input.Name)
	input.Kind = strings.TrimSpace(input.Kind)
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Name)) > 120 ||
		(input.Kind != "public_holiday" && input.Kind != "collective_leave" && input.Kind != "company_workday") ||
		!validDecisionNote(input.Reason) {
		writeError(w, http.StatusBadRequest, "Nama, jenis kalender, atau alasan tidak valid.")
		return calendarInput{}, false
	}
	return input, true
}

func queryLeaveRequests(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, where string, args ...any) ([]leaveRequestView, error) {
	rows, err := q.Query(ctx, `
		SELECT l.id, l.user_id, e.full_name, e.employee_number, b.name,
			l.manager_id, l.leave_year, l.start_date, l.end_date, l.day_count,
			l.reason, l.status, l.manager_note, l.hr_note, l.superadmin_note, l.created_at
		FROM leave_requests l
		JOIN employees e ON e.user_id = l.user_id
		JOIN branches b ON b.id = l.branch_id
		`+where+`
		ORDER BY l.created_at DESC
		LIMIT 200
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]leaveRequestView, 0)
	for rows.Next() {
		var item leaveRequestView
		var start, end time.Time
		var dayCount int16
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.EmployeeName, &item.EmployeeNumber, &item.BranchName,
			&item.ManagerID, &item.Year, &start, &end, &dayCount,
			&item.Reason, &item.Status, &item.ManagerNote, &item.HRNote, &item.SuperadminNote, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.StartDate = start.Format("2006-01-02")
		item.EndDate = end.Format("2006-01-02")
		item.DayCount = int(dayCount)
		result = append(result, item)
	}
	return result, rows.Err()
}

func requestedLeaveYear(w http.ResponseWriter, r *http.Request) (int, bool) {
	value := r.URL.Query().Get("year")
	if value == "" {
		return time.Now().In(jakartaTime).Year(), true
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 2000 || year > 2100 {
		writeError(w, http.StatusBadRequest, "Tahun cuti tidak valid.")
		return 0, false
	}
	return year, true
}

func loadLeaveBalance(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID uuid.UUID, year int) (leaveBalance, error) {
	var result leaveBalance
	result.Year = year
	var err error
	result.PolicyConfigured, result.AnnualDays, result.BalanceDays, result.UsedDays, err = leaveBalanceValues(ctx, q, userID, year)
	return result, err
}

func leaveBalanceValues(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID uuid.UUID, year int) (bool, int, float64, float64, error) {
	var configured bool
	var annualDays int
	err := q.QueryRow(ctx, "SELECT annual_days FROM leave_entitlements WHERE leave_year = $1", year).Scan(&annualDays)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, 0, 0, 0, err
	}
	configured = err == nil
	var balance, used float64
	err = q.QueryRow(ctx, `
		SELECT
			COALESCE(sum(amount), 0)::double precision,
			COALESCE(sum(CASE WHEN entry_type IN (
				'collective_leave', 'collective_leave_reversal', 'leave_request', 'leave_request_reversal'
			) THEN -amount ELSE 0 END), 0)::double precision
		FROM leave_ledger_entries
		WHERE user_id = $1 AND leave_year = $2
	`, userID, year).Scan(&balance, &used)
	if err != nil {
		return false, 0, 0, 0, err
	}
	return configured, annualDays, balance, used, nil
}

func syncAnnualGrant(ctx context.Context, tx pgx.Tx, userID uuid.UUID, year, annualDays int, reason string, actorID uuid.UUID) error {
	var annualGrantExists bool
	var currentEntitlement float64
	err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM leave_ledger_entries
				WHERE user_id = $1 AND leave_year = $2 AND entry_type = 'annual_grant'
			),
			COALESCE(sum(amount), 0)::double precision
		FROM leave_ledger_entries
		WHERE user_id = $1 AND leave_year = $2
			AND entry_type IN ('annual_grant', 'annual_adjustment')
	`, userID, year).Scan(&annualGrantExists, &currentEntitlement)
	if err != nil {
		return err
	}
	if annualDays > 0 && !annualGrantExists {
		_, err = tx.Exec(ctx, `
			INSERT INTO leave_ledger_entries
				(user_id, leave_year, entry_type, amount, reason, created_by)
			VALUES ($1, $2, 'annual_grant', $3, $4, $5)
		`, userID, year, annualDays, reason, actorID)
		return err
	}
	difference := float64(annualDays) - currentEntitlement
	if difference == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO leave_ledger_entries
			(user_id, leave_year, entry_type, amount, reason, created_by)
		VALUES ($1, $2, 'annual_adjustment', $3, $4, $5)
	`, userID, year, difference, reason, actorID)
	return err
}

func applyCollectiveLeave(ctx context.Context, tx pgx.Tx, dayID uuid.UUID, date time.Time, cycleID uuid.UUID, annualDays int, reason string, actorID uuid.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT u.id FROM users u
		JOIN employees e ON e.user_id = u.id
		WHERE u.active AND e.employment_status = 'active'
		ORDER BY u.id FOR UPDATE OF u
	`)
	if err != nil {
		return err
	}
	var userIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		userIDs = append(userIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		if err := syncAnnualGrant(ctx, tx, userID, date.Year(), annualDays, reason, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO leave_ledger_entries
				(user_id, leave_year, entry_type, amount, calendar_day_id, source_cycle_id, entry_date, reason, created_by)
			VALUES ($1, $2, 'collective_leave', -1, $3, $4, $5, $6, $7)
		`, userID, date.Year(), dayID, cycleID, date, reason, actorID); err != nil {
			return err
		}
	}
	return nil
}

func reverseCollectiveLeave(ctx context.Context, tx pgx.Tx, dayID uuid.UUID, cycleID pgtype.UUID, reason string, actorID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO leave_ledger_entries
			(user_id, leave_year, entry_type, amount, calendar_day_id, source_cycle_id, entry_date, reason, created_by)
		SELECT user_id, leave_year, 'collective_leave_reversal', -amount, calendar_day_id, source_cycle_id, entry_date, $3, $4
		FROM leave_ledger_entries
		WHERE calendar_day_id = $1 AND source_cycle_id = $2 AND entry_type = 'collective_leave'
	`, dayID, cycleID, reason, actorID)
	return err
}

func isWorkingDayOnCalendar(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, date time.Time, workingDays []int16) (bool, error) {
	var kind string
	err := q.QueryRow(ctx, "SELECT kind FROM company_calendar_days WHERE calendar_date = $1", jakartaDate(date)).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return isWorkingDay(date, workingDays), nil
	}
	if err != nil {
		return false, err
	}
	return calendarWorkday(date, kind, true, workingDays), nil
}

func workDatesInRange(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, start, end time.Time) ([]time.Time, error) {
	rows, err := q.Query(ctx, `
		SELECT calendar_date, kind FROM company_calendar_days
		WHERE calendar_date BETWEEN $1 AND $2 ORDER BY calendar_date
	`, start, end)
	if err != nil {
		return nil, err
	}
	calendar := make(map[string]string)
	for rows.Next() {
		var date time.Time
		var kind string
		if err := rows.Scan(&date, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		calendar[date.Format("2006-01-02")] = kind
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]time.Time, 0)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		kind, found := calendar[date.Format("2006-01-02")]
		if calendarWorkday(date, kind, found, []int16{1, 2, 3, 4, 5}) {
			result = append(result, date)
		}
	}
	return result, nil
}

func calendarWorkday(date time.Time, kind string, found bool, workingDays []int16) bool {
	if found {
		return kind == "company_workday"
	}
	return isWorkingDay(date, workingDays)
}

func isScheduledWorkday(date time.Time) bool {
	return isWorkingDay(date, []int16{1, 2, 3, 4, 5})
}

func ensureCalendarDateHasNoLeaveRequests(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, date time.Time) error {
	var hasRequests bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM leave_request_days d
			JOIN leave_requests l ON l.id = d.request_id
			WHERE d.leave_date = $1 AND l.status IN ('pending_manager', 'pending_hr', 'pending_superadmin', 'approved')
		)
	`, date).Scan(&hasRequests); err != nil {
		return err
	}
	if hasRequests {
		return errors.New("active leave requests exist for date")
	}
	return nil
}

func lockLeaveOperations(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", leaveOperationLock)
	return err
}

func validDecisionNote(value string) bool {
	length := len([]rune(strings.TrimSpace(value)))
	return length >= 5 && length <= 500
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID tidak valid.")
		return uuid.Nil, false
	}
	return id, true
}

func nullableUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
