package httpapi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type kpiGoal struct {
	ID              uuid.UUID     `json:"id"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`
	TargetValue     string        `json:"target_value"`
	Weight          float64       `json:"weight"`
	ManagerScore    *float64      `json:"manager_score,omitempty"`
	ManagerComment  *string       `json:"manager_comment,omitempty"`
	ProgressUpdates []kpiProgress `json:"progress_updates"`
}

type kpiProgress struct {
	ID              uuid.UUID  `json:"id"`
	EnteredBy       *uuid.UUID `json:"entered_by,omitempty"`
	EnteredByName   string     `json:"entered_by_name"`
	CurrentValue    string     `json:"current_value"`
	ProgressPercent *float64   `json:"progress_percent,omitempty"`
	Note            string     `json:"note"`
	CreatedAt       time.Time  `json:"created_at"`
}

type kpiAppeal struct {
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	Decision      *string    `json:"decision,omitempty"`
	DecisionNote  *string    `json:"decision_note,omitempty"`
	AdjustedScore *float64   `json:"adjusted_score,omitempty"`
	SubmittedAt   time.Time  `json:"submitted_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
}

type kpiSetView struct {
	ID                       uuid.UUID  `json:"id"`
	Year                     int        `json:"year"`
	UserID                   uuid.UUID  `json:"user_id"`
	EmployeeNumber           string     `json:"employee_number"`
	EmployeeName             string     `json:"employee_name"`
	BranchID                 uuid.UUID  `json:"branch_id"`
	BranchName               string     `json:"branch_name"`
	ManagerID                uuid.UUID  `json:"manager_id"`
	ManagerName              string     `json:"manager_name"`
	Status                   string     `json:"status"`
	ManagerComment           *string    `json:"manager_comment,omitempty"`
	ManagerReviewSubmittedAt *time.Time `json:"manager_review_submitted_at,omitempty"`
	TargetApprovedAt         *time.Time `json:"target_approved_at,omitempty"`
	TargetReviewNote         *string    `json:"target_review_note,omitempty"`
	FinalizedAt              *time.Time `json:"finalized_at,omitempty"`
	AppealDeadline           *string    `json:"appeal_deadline,omitempty"`
	FinalScore               *float64   `json:"final_score,omitempty"`
	FinalReviewNote          *string    `json:"final_review_note,omitempty"`
	Goals                    []kpiGoal  `json:"goals"`
	Appeal                   *kpiAppeal `json:"appeal,omitempty"`
}

type kpiCycleView struct {
	Year      int       `json:"year"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *API) registerKPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/kpi/me", a.authenticated(a.myKPI))
	mux.HandleFunc("GET /api/v1/kpi/report", a.authenticated(a.kpiReport))
	mux.HandleFunc("GET /api/v1/kpi/cycles", a.authenticated(a.listKPICycles))
	mux.HandleFunc("PUT /api/v1/kpi/cycles", a.authenticated(a.saveKPICycle))
	mux.HandleFunc("POST /api/v1/kpi/employees/{id}/targets", a.authenticated(a.saveKPITargets))
	mux.HandleFunc("PUT /api/v1/kpi/employees/{id}/targets", a.authenticated(a.saveKPITargets))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/submit", a.authenticated(a.submitKPITargets))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/approve-targets", a.authenticated(a.approveKPITargets))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/return-targets", a.authenticated(a.returnKPITargets))
	mux.HandleFunc("POST /api/v1/kpi/goals/{id}/progress", a.authenticated(a.addKPIProgress))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/evaluation", a.authenticated(a.submitKPIEvaluation))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/finalize", a.authenticated(a.finalizeKPIReview))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/appeal", a.authenticated(a.submitKPIAppeal))
	mux.HandleFunc("POST /api/v1/kpi/sets/{id}/appeal/decision", a.authenticated(a.decideKPIAppeal))
}

func (a *API) myKPI(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	year, ok := requestedKPIYear(w, r)
	if !ok {
		return
	}
	cycle, err := loadKPICycle(r.Context(), a.db, year)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load personal KPI cycle failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat dimuat.")
		return
	}
	var sets []kpiSetView
	if cycle != nil {
		sets, err = loadKPISetViews(r.Context(), a.db, "WHERE s.user_id = $1 AND s.kpi_year = $2", user.ID, year)
		if err != nil {
			log.Printf("load personal KPI targets failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat dimuat.")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cycle": cycle, "goal_set": firstKPISet(sets)})
}

func (a *API) listKPICycles(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "manager" && !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Daftar periode KPI tidak tersedia untuk role ini.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT kpi_year, name, status, updated_at
		FROM kpi_cycles ORDER BY kpi_year DESC
	`)
	if err != nil {
		log.Printf("list KPI cycles failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	result := make([]kpiCycleView, 0)
	for rows.Next() {
		var item kpiCycleView
		if err := rows.Scan(&item.Year, &item.Name, &item.Status, &item.UpdatedAt); err != nil {
			log.Printf("scan KPI cycle failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat dimuat.")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read KPI cycles failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) saveKPICycle(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat mengelola periode KPI.")
		return
	}
	var input struct {
		Year   int    `json:"year"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Year < 2000 || input.Year > 2100 || len([]rune(input.Name)) < 3 || len([]rune(input.Name)) > 120 ||
		(input.Status != "planning" && input.Status != "open" && input.Status != "closed") {
		writeError(w, http.StatusBadRequest, "Tahun, nama, atau status periode KPI tidak valid.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var oldStatus string
	err = tx.QueryRow(r.Context(), "SELECT status FROM kpi_cycles WHERE kpi_year = $1 FOR UPDATE", input.Year).Scan(&oldStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load KPI cycle for update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat disimpan.")
		return
	}
	if err == nil && !validKPICycleTransition(oldStatus, input.Status) {
		writeError(w, http.StatusConflict, "Periode KPI hanya dapat bergerak dari perencanaan ke dibuka lalu ditutup.")
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE kpi_cycles SET name = $2, status = $3, updated_by = $4, updated_at = now()
			WHERE kpi_year = $1
		`, input.Year, input.Name, input.Status, user.ID)
	} else {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO kpi_cycles (kpi_year, name, status, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $4)
		`, input.Year, input.Name, input.Status, user.ID)
	}
	if err != nil {
		log.Printf("save KPI cycle failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "SAVE_KPI_CYCLE", "kpi_cycle", strconv.Itoa(input.Year), map[string]string{
		"name": input.Name, "old_status": oldStatus, "status": input.Status,
	}); err != nil {
		log.Printf("audit KPI cycle failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, kpiCycleView{Year: input.Year, Name: input.Name, Status: input.Status, UpdatedAt: time.Now()})
}

func (a *API) saveKPITargets(w http.ResponseWriter, r *http.Request) {
	manager := currentPrincipal(r)
	if manager.Role != "manager" {
		writeError(w, http.StatusForbidden, "Hanya manager yang dapat menyusun target KPI.")
		return
	}
	employeeID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Year  int `json:"year"`
		Goals []struct {
			Title       string  `json:"title"`
			Description string  `json:"description"`
			TargetValue string  `json:"target_value"`
			Weight      float64 `json:"weight"`
		} `json:"goals"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Year < 2000 || input.Year > 2100 || len(input.Goals) == 0 || len(input.Goals) > 30 {
		writeError(w, http.StatusBadRequest, "Tahun dan 1–30 target KPI wajib diberikan.")
		return
	}
	totalWeight := 0.0
	for index := range input.Goals {
		goal := &input.Goals[index]
		goal.Title = strings.TrimSpace(goal.Title)
		goal.Description = strings.TrimSpace(goal.Description)
		goal.TargetValue = strings.TrimSpace(goal.TargetValue)
		if len([]rune(goal.Title)) < 3 || len([]rune(goal.Title)) > 160 ||
			len([]rune(goal.Description)) < 5 || len([]rune(goal.Description)) > 1000 ||
			len([]rune(goal.TargetValue)) == 0 || len([]rune(goal.TargetValue)) > 200 ||
			math.IsNaN(goal.Weight) || math.IsInf(goal.Weight, 0) || goal.Weight <= 0 || goal.Weight > 100 {
			writeError(w, http.StatusBadRequest, "Setiap target harus memiliki nama, deskripsi, hasil terukur, dan bobot antara 0–100.")
			return
		}
		totalWeight += goal.Weight
	}
	if math.Abs(totalWeight-100) > 1e-9 {
		writeError(w, http.StatusBadRequest, "Jumlah bobot seluruh target harus tepat 100%.")
		return
	}
	for _, goal := range input.Goals {
		if math.Abs(goal.Weight*100-math.Round(goal.Weight*100)) > 1e-9 {
			writeError(w, http.StatusBadRequest, "Bobot target hanya dapat menggunakan maksimal dua angka desimal.")
			return
		}
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var cycleStatus string
	if err := tx.QueryRow(r.Context(), "SELECT status FROM kpi_cycles WHERE kpi_year = $1 FOR SHARE", input.Year).Scan(&cycleStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Periode KPI belum dibuat HR.")
		} else {
			log.Printf("load KPI cycle for target creation failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Periode KPI tidak dapat diverifikasi.")
		}
		return
	}
	if cycleStatus != "open" {
		writeError(w, http.StatusConflict, "Periode KPI belum dibuka untuk penyusunan target.")
		return
	}
	var activeEmployee bool
	if err := tx.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM users u JOIN employees e ON e.user_id = u.id
			WHERE u.id = $1 AND u.branch_id = $2 AND u.manager_id = $3
				AND u.active AND e.employment_status = 'active'
		)
	`, employeeID, manager.BranchID, manager.ID).Scan(&activeEmployee); err != nil {
		log.Printf("validate KPI target employee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Karyawan tidak dapat diverifikasi.")
		return
	}
	if !activeEmployee {
		writeError(w, http.StatusForbidden, "Target hanya dapat disusun untuk karyawan aktif yang menjadi bawahan langsung Anda.")
		return
	}
	var setID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		SELECT id FROM kpi_goal_sets WHERE kpi_year = $1 AND user_id = $2 AND manager_id = $3 FOR UPDATE
	`, input.Year, employeeID, manager.ID).Scan(&setID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load existing KPI target set failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO kpi_goal_sets (kpi_year, user_id, branch_id, manager_id)
			VALUES ($1, $2, $3, $4) RETURNING id
		`, input.Year, employeeID, manager.BranchID, manager.ID).Scan(&setID)
	} else {
		var status string
		if err := tx.QueryRow(r.Context(), "SELECT status FROM kpi_goal_sets WHERE id = $1", setID).Scan(&status); err != nil {
			log.Printf("load KPI target set status failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
			return
		}
		if status != "draft" && status != "needs_revision" {
			writeError(w, http.StatusConflict, "Target sudah diajukan atau dikunci dan tidak dapat diubah.")
			return
		}
		_, err = tx.Exec(r.Context(), "DELETE FROM kpi_goals WHERE goal_set_id = $1", setID)
	}
	if err != nil {
		log.Printf("create or replace KPI target set failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
		return
	}
	for _, goal := range input.Goals {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO kpi_goals (goal_set_id, title, description, target_value, weight)
			VALUES ($1, $2, $3, $4, $5)
		`, setID, goal.Title, goal.Description, goal.TargetValue, goal.Weight); err != nil {
			log.Printf("create KPI goal failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, manager.ID, "SAVE_KPI_TARGETS", "kpi_goal_set", setID.String(), map[string]string{
		"employee_id": employeeID.String(), "year": strconv.Itoa(input.Year), "goal_count": strconv.Itoa(len(input.Goals)),
	}); err != nil {
		log.Printf("audit KPI target save failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": setID, "status": "draft", "weight_total": 100})
}

func (a *API) submitKPITargets(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role != "manager" {
		writeError(w, http.StatusForbidden, "Hanya manager yang dapat mengajukan target untuk persetujuan HR.")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat diajukan.")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		UPDATE kpi_goal_sets s
		SET status = 'pending_hr', updated_at = now()
		WHERE s.id = $1 AND s.manager_id = $2 AND s.status IN ('draft', 'needs_revision')
			AND EXISTS (SELECT 1 FROM kpi_goals g WHERE g.goal_set_id = s.id)
			AND abs((SELECT COALESCE(sum(g.weight), 0) FROM kpi_goals g WHERE g.goal_set_id = s.id) - 100) <= 0.01
	`, setID, user.ID)
	if err != nil {
		log.Printf("submit KPI targets failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat diajukan.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Target tidak ditemukan, kosong, bobotnya bukan 100%, atau bukan wewenang Anda.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "SUBMIT_KPI_TARGETS", "kpi_goal_set", setID.String(), map[string]string{}); err != nil {
		log.Printf("audit KPI target submission failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat diajukan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat diajukan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pending_hr"})
}

func (a *API) approveKPITargets(w http.ResponseWriter, r *http.Request) {
	hr := currentPrincipal(r)
	if !isHR(hr.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat mengunci target KPI.")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disetujui.")
		return
	}
	defer tx.Rollback(r.Context())
	var employeeID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		UPDATE kpi_goal_sets SET status = 'approved', hr_target_approved_by = $2,
			hr_target_approved_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending_hr' AND manager_id <> $3 AND user_id <> $3
		RETURNING user_id
	`, setID, hr.ID, hr.ID).Scan(&employeeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Target KPI tidak ditemukan atau sudah diproses.")
		} else {
			log.Printf("approve KPI targets failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disetujui.")
		}
		return
	}
	if employeeID == hr.ID {
		writeError(w, http.StatusForbidden, "HR tidak dapat menyetujui target KPI miliknya sendiri.")
		return
	}
	if err := insertAudit(r.Context(), tx, hr.ID, "APPROVE_KPI_TARGETS", "kpi_goal_set", setID.String(), map[string]string{"employee_id": employeeID.String()}); err != nil {
		log.Printf("audit KPI target approval failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disetujui.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat disetujui.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (a *API) returnKPITargets(w http.ResponseWriter, r *http.Request) {
	hr := currentPrincipal(r)
	if !isHR(hr.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat mengembalikan target KPI untuk revisi.")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if len([]rune(input.Note)) < 5 || len([]rune(input.Note)) > 1000 {
		writeError(w, http.StatusBadRequest, "Catatan revisi HR wajib diisi (5–1.000 karakter).")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat dikembalikan.")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		UPDATE kpi_goal_sets SET status = 'needs_revision', hr_target_note = $2, updated_at = now()
		WHERE id = $1 AND status = 'pending_hr' AND manager_id <> $3 AND user_id <> $3
	`, setID, input.Note, hr.ID)
	if err != nil {
		log.Printf("return KPI targets for revision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat dikembalikan.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Target tidak ditemukan, sudah diproses, atau konflik kepentingan.")
		return
	}
	if err := insertAudit(r.Context(), tx, hr.ID, "RETURN_KPI_TARGETS", "kpi_goal_set", setID.String(), map[string]string{"note": input.Note}); err != nil {
		log.Printf("audit KPI target revision request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat dikembalikan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Target KPI tidak dapat dikembalikan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "needs_revision"})
}

func (a *API) addKPIProgress(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if user.Role == "finance" || user.Role == "superadmin" {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat mencatat progres KPI.")
		return
	}
	goalID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		CurrentValue    string   `json:"current_value"`
		ProgressPercent *float64 `json:"progress_percent"`
		Note            string   `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.CurrentValue = strings.TrimSpace(input.CurrentValue)
	input.Note = strings.TrimSpace(input.Note)
	if len([]rune(input.CurrentValue)) == 0 || len([]rune(input.CurrentValue)) > 200 ||
		len([]rune(input.Note)) < 5 || len([]rune(input.Note)) > 1000 ||
		(input.ProgressPercent != nil && (math.IsNaN(*input.ProgressPercent) || math.IsInf(*input.ProgressPercent, 0) || *input.ProgressPercent < 0 || *input.ProgressPercent > 100)) {
		writeError(w, http.StatusBadRequest, "Nilai progres, persentase 0–100, atau catatan tidak valid.")
		return
	}
	var goalSetID, employeeID, managerID uuid.UUID
	var cycleStatus, setStatus string
	err := a.db.QueryRow(r.Context(), `
		SELECT g.goal_set_id, s.user_id, s.manager_id, c.status, s.status
		FROM kpi_goals g
		JOIN kpi_goal_sets s ON s.id = g.goal_set_id
		JOIN kpi_cycles c ON c.kpi_year = s.kpi_year
		WHERE g.id = $1
	`, goalID).Scan(&goalSetID, &employeeID, &managerID, &cycleStatus, &setStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Target KPI tidak ditemukan.")
		} else {
			log.Printf("load KPI goal for progress failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Progres KPI tidak dapat dicatat.")
		}
		return
	}
	if user.ID != employeeID && (user.Role != "manager" || user.ID != managerID) {
		writeError(w, http.StatusForbidden, "Progres hanya dapat dicatat oleh karyawan pemilik target atau manager langsungnya.")
		return
	}
	if cycleStatus != "open" || setStatus != "approved" {
		writeError(w, http.StatusConflict, "Progres hanya dapat dicatat pada target yang sudah dikunci dalam periode terbuka.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Progres KPI tidak dapat dicatat.")
		return
	}
	defer tx.Rollback(r.Context())
	var updateID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO kpi_progress_updates (goal_id, entered_by, current_value, progress_percent, note)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, goalID, user.ID, input.CurrentValue, input.ProgressPercent, input.Note).Scan(&updateID)
	if err != nil {
		log.Printf("save KPI progress failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Progres KPI tidak dapat dicatat.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "ADD_KPI_PROGRESS", "kpi_progress_update", updateID.String(), map[string]string{
		"goal_id": goalID.String(), "goal_set_id": goalSetID.String(),
	}); err != nil {
		log.Printf("audit KPI progress failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Progres KPI tidak dapat dicatat.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Progres KPI tidak dapat dicatat.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": updateID})
}

func (a *API) submitKPIEvaluation(w http.ResponseWriter, r *http.Request) {
	manager := currentPrincipal(r)
	if manager.Role != "manager" {
		writeError(w, http.StatusForbidden, "Hanya manager langsung yang dapat menilai KPI.")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Comment string `json:"comment"`
		Scores  []struct {
			GoalID  string  `json:"goal_id"`
			Score   float64 `json:"score"`
			Comment string  `json:"comment"`
		} `json:"scores"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Comment = strings.TrimSpace(input.Comment)
	if len([]rune(input.Comment)) < 5 || len([]rune(input.Comment)) > 2000 || len(input.Scores) == 0 || len(input.Scores) > 30 {
		writeError(w, http.StatusBadRequest, "Ringkasan evaluasi dan nilai seluruh target wajib diisi.")
		return
	}
	scores := make(map[uuid.UUID]struct {
		score   float64
		comment string
	}, len(input.Scores))
	for _, item := range input.Scores {
		goalID, err := uuid.Parse(item.GoalID)
		item.Comment = strings.TrimSpace(item.Comment)
		if err != nil || math.IsNaN(item.Score) || math.IsInf(item.Score, 0) || item.Score < 0 || item.Score > 100 ||
			len([]rune(item.Comment)) > 1000 {
			writeError(w, http.StatusBadRequest, "Nilai target harus 0–100 dan komentar maksimal 1.000 karakter.")
			return
		}
		if _, duplicate := scores[goalID]; duplicate {
			writeError(w, http.StatusBadRequest, "Target KPI yang sama tidak boleh dinilai lebih dari sekali.")
			return
		}
		scores[goalID] = struct {
			score   float64
			comment string
		}{item.Score, item.Comment}
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	defer tx.Rollback(r.Context())
	var cycleYear int
	err = tx.QueryRow(r.Context(), `
		SELECT s.kpi_year FROM kpi_goal_sets s
		JOIN kpi_cycles c ON c.kpi_year = s.kpi_year
		WHERE s.id = $1 AND s.manager_id = $2 AND s.status = 'approved' AND c.status = 'closed'
		FOR UPDATE OF s
	`, setID, manager.ID).Scan(&cycleYear)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Target tidak ditemukan, belum dikunci HR, periode belum ditutup, atau bukan bawahan langsung.")
		} else {
			log.Printf("load KPI set for manager evaluation failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		}
		return
	}
	rows, err := tx.Query(r.Context(), "SELECT id FROM kpi_goals WHERE goal_set_id = $1 ORDER BY id", setID)
	if err != nil {
		log.Printf("load KPI goals for evaluation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	var goalIDs []uuid.UUID
	for rows.Next() {
		var goalID uuid.UUID
		if err := rows.Scan(&goalID); err != nil {
			rows.Close()
			log.Printf("scan KPI goal for evaluation failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
			return
		}
		goalIDs = append(goalIDs, goalID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		log.Printf("read KPI goals for evaluation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	if len(goalIDs) != len(scores) {
		writeError(w, http.StatusBadRequest, "Penilaian harus mencakup setiap target tepat satu kali.")
		return
	}
	for _, goalID := range goalIDs {
		item, exists := scores[goalID]
		if !exists {
			writeError(w, http.StatusBadRequest, "Penilaian harus mencakup setiap target tepat satu kali.")
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE kpi_goals SET manager_score = $2, manager_comment = NULLIF($3, '')
			WHERE id = $1 AND goal_set_id = $4
		`, goalID, item.score, item.comment, setID); err != nil {
			log.Printf("save manager KPI score failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
			return
		}
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE kpi_goal_sets
		SET status = 'pending_hr_final', manager_comment = $2,
			manager_review_submitted_at = now(), updated_at = now()
		WHERE id = $1
	`, setID, input.Comment)
	if err != nil {
		log.Printf("submit manager KPI evaluation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	if err := insertAudit(r.Context(), tx, manager.ID, "SUBMIT_KPI_EVALUATION", "kpi_goal_set", setID.String(), map[string]string{
		"year": strconv.Itoa(cycleYear), "goal_count": strconv.Itoa(len(goalIDs)),
	}); err != nil {
		log.Printf("audit manager KPI evaluation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Penilaian KPI tidak dapat dikirim.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pending_hr_final"})
}

func (a *API) finalizeKPIReview(w http.ResponseWriter, r *http.Request) {
	hr := currentPrincipal(r)
	if !isHR(hr.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat melakukan tinjauan akhir KPI.")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if len([]rune(input.Note)) < 5 || len([]rune(input.Note)) > 2000 {
		writeError(w, http.StatusBadRequest, "Catatan tinjauan HR wajib diisi (5–2.000 karakter).")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Tinjauan akhir KPI tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var employeeID uuid.UUID
	var managerID uuid.UUID
	var year int
	err = tx.QueryRow(r.Context(), `
		SELECT user_id, manager_id, kpi_year
		FROM kpi_goal_sets WHERE id = $1 AND status = 'pending_hr_final'
		FOR UPDATE
	`, setID).Scan(&employeeID, &managerID, &year)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Penilaian tidak ditemukan atau belum dikirim manager.")
		} else {
			log.Printf("load KPI set for HR finalization failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Tinjauan akhir KPI tidak dapat disimpan.")
		}
		return
	}
	if employeeID == hr.ID || managerID == hr.ID {
		writeError(w, http.StatusForbidden, "HR tidak dapat memfinalisasi KPI miliknya sendiri atau hasil penilaiannya sendiri.")
		return
	}
	deadline, err := kpiAppealDeadline(r.Context(), tx, time.Now().In(jakartaTime), 5)
	if err != nil {
		log.Printf("calculate KPI appeal deadline failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Batas waktu keberatan KPI tidak dapat dihitung.")
		return
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE kpi_goal_sets
		SET status = 'finalized', hr_finalized_by = $2, hr_finalized_at = now(),
			appeal_deadline = $3, final_review_note = $4, updated_at = now()
		WHERE id = $1 AND status = 'pending_hr_final'
	`, setID, hr.ID, deadline, input.Note)
	if err != nil || tag.RowsAffected() != 1 {
		log.Printf("finalize KPI review failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Tinjauan akhir KPI tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, hr.ID, "FINALIZE_KPI_REVIEW", "kpi_goal_set", setID.String(), map[string]string{
		"appeal_deadline": deadline.Format("2006-01-02"), "employee_id": employeeID.String(),
	}); err != nil {
		log.Printf("audit final KPI review failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Tinjauan akhir KPI tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Tinjauan akhir KPI tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "finalized", "appeal_deadline": deadline.Format("2006-01-02")})
}

func (a *API) submitKPIAppeal(w http.ResponseWriter, r *http.Request) {
	employee := currentPrincipal(r)
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 1000 {
		writeError(w, http.StatusBadRequest, "Alasan keberatan wajib diisi (5–1.000 karakter).")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Keberatan KPI tidak dapat dikirim.")
		return
	}
	defer tx.Rollback(r.Context())
	var deadline time.Time
	err = tx.QueryRow(r.Context(), `
		SELECT appeal_deadline FROM kpi_goal_sets
		WHERE id = $1 AND user_id = $2 AND status = 'finalized'
		FOR UPDATE
	`, setID, employee.ID).Scan(&deadline)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Hasil KPI final tidak ditemukan.")
		} else {
			log.Printf("load final KPI result for appeal failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Keberatan KPI tidak dapat dikirim.")
		}
		return
	}
	today := jakartaDate(time.Now())
	if today.After(jakartaDate(deadline)) {
		writeError(w, http.StatusConflict, "Masa keberatan KPI selama lima hari kerja telah berakhir.")
		return
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO kpi_appeals (goal_set_id, reason) VALUES ($1, $2)", setID, input.Reason)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Keberatan untuk hasil KPI ini sudah pernah dikirim.")
			return
		}
		log.Printf("save KPI appeal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keberatan KPI tidak dapat dikirim.")
		return
	}
	if err := insertAudit(r.Context(), tx, employee.ID, "SUBMIT_KPI_APPEAL", "kpi_goal_set", setID.String(), map[string]string{
		"appeal_deadline": deadline.Format("2006-01-02"),
	}); err != nil {
		log.Printf("audit KPI appeal failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keberatan KPI tidak dapat dikirim.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Keberatan KPI tidak dapat dikirim.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "pending"})
}

func (a *API) decideKPIAppeal(w http.ResponseWriter, r *http.Request) {
	hr := currentPrincipal(r)
	if !isHR(hr.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR yang dapat memutuskan keberatan KPI.")
		return
	}
	var input struct {
		Decision      string   `json:"decision"`
		Note          string   `json:"note"`
		AdjustedScore *float64 `json:"adjusted_score"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if (input.Decision != "upheld" && input.Decision != "adjusted") ||
		len([]rune(input.Note)) < 5 || len([]rune(input.Note)) > 2000 ||
		(input.Decision == "adjusted" && (input.AdjustedScore == nil || math.IsNaN(*input.AdjustedScore) || math.IsInf(*input.AdjustedScore, 0) || *input.AdjustedScore < 0 || *input.AdjustedScore > 100)) ||
		(input.Decision == "upheld" && input.AdjustedScore != nil) {
		writeError(w, http.StatusBadRequest, "Keputusan, catatan wajib, atau nilai penyesuaian 0–100 tidak valid.")
		return
	}
	setID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan keberatan KPI tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var employeeID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		SELECT s.user_id FROM kpi_appeals a
		JOIN kpi_goal_sets s ON s.id = a.goal_set_id
		WHERE a.goal_set_id = $1 AND a.status = 'pending'
		FOR UPDATE OF a, s
	`, setID).Scan(&employeeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "Keberatan tidak ditemukan atau sudah diputuskan.")
		} else {
			log.Printf("load KPI appeal for decision failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Keputusan keberatan KPI tidak dapat disimpan.")
		}
		return
	}
	if employeeID == hr.ID {
		writeError(w, http.StatusForbidden, "HR tidak dapat memutuskan keberatan atas hasil KPI miliknya sendiri.")
		return
	}
	var adjusted any
	if input.Decision == "adjusted" {
		adjusted = *input.AdjustedScore
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE kpi_appeals SET status = 'resolved', decision = $2, decision_note = $3,
			adjusted_score = $4, decided_by = $5, decided_at = now()
		WHERE goal_set_id = $1 AND status = 'pending'
	`, setID, input.Decision, input.Note, adjusted, hr.ID)
	if err != nil || tag.RowsAffected() != 1 {
		log.Printf("save KPI appeal decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan keberatan KPI tidak dapat disimpan.")
		return
	}
	if input.Decision == "adjusted" {
		if _, err := tx.Exec(r.Context(), "UPDATE kpi_goal_sets SET final_score_override = $2, updated_at = now() WHERE id = $1", setID, *input.AdjustedScore); err != nil {
			log.Printf("apply adjusted final KPI score failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Nilai akhir hasil keberatan tidak dapat disimpan.")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, hr.ID, "DECIDE_KPI_APPEAL", "kpi_goal_set", setID.String(), map[string]string{
		"decision": input.Decision, "employee_id": employeeID.String(),
	}); err != nil {
		log.Printf("audit KPI appeal decision failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keputusan keberatan KPI tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Keputusan keberatan KPI tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved", "decision": input.Decision})
}

func (a *API) kpiReport(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	year, ok := requestedKPIYear(w, r)
	if !ok {
		return
	}
	where, args, permitted := kpiReportScope(user, year, r.URL.Query().Get("branch_id"))
	if !permitted {
		writeError(w, http.StatusForbidden, "Laporan KPI hanya tersedia untuk HR dan manager.")
		return
	}
	sets, err := loadKPISetViews(r.Context(), a.db, where, args...)
	if err != nil {
		log.Printf("load KPI report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan KPI tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, sets)
}

func loadKPICycle(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, year int) (*kpiCycleView, error) {
	var item kpiCycleView
	err := q.QueryRow(ctx, "SELECT kpi_year, name, status, updated_at FROM kpi_cycles WHERE kpi_year = $1", year).
		Scan(&item.Year, &item.Name, &item.Status, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func loadKPISetViews(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, where string, args ...any) ([]kpiSetView, error) {
	rows, err := q.Query(ctx, `
		SELECT s.id, s.kpi_year, s.user_id, e.employee_number, e.full_name, s.branch_id, b.name,
			s.manager_id, m.full_name, s.status, s.manager_comment, s.manager_review_submitted_at,
			s.hr_target_approved_at, s.hr_target_note, s.hr_finalized_at, s.appeal_deadline,
			COALESCE(s.final_score_override, (
				SELECT CASE WHEN count(*) = 0 OR count(g.manager_score) <> count(*) THEN NULL
					ELSE round(sum(g.weight * g.manager_score) / 100, 2) END
				FROM kpi_goals g WHERE g.goal_set_id = s.id
			))::double precision,
			s.final_review_note
		FROM kpi_goal_sets s
		JOIN employees e ON e.user_id = s.user_id
		JOIN branches b ON b.id = s.branch_id
		JOIN employees m ON m.user_id = s.manager_id
		`+where+`
		ORDER BY s.kpi_year DESC, e.full_name
		LIMIT 500
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sets := make([]kpiSetView, 0)
	setIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var item kpiSetView
		var deadline *time.Time
		if err := rows.Scan(
			&item.ID, &item.Year, &item.UserID, &item.EmployeeNumber, &item.EmployeeName, &item.BranchID,
			&item.BranchName, &item.ManagerID, &item.ManagerName, &item.Status, &item.ManagerComment,
			&item.ManagerReviewSubmittedAt, &item.TargetApprovedAt, &item.TargetReviewNote, &item.FinalizedAt, &deadline,
			&item.FinalScore, &item.FinalReviewNote,
		); err != nil {
			return nil, err
		}
		if deadline != nil {
			formatted := deadline.Format("2006-01-02")
			item.AppealDeadline = &formatted
		}
		item.Goals = make([]kpiGoal, 0)
		sets = append(sets, item)
		setIDs = append(setIDs, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(setIDs) == 0 {
		return sets, nil
	}
	index := make(map[uuid.UUID]int, len(setIDs))
	for i, id := range setIDs {
		index[id] = i
	}
	goalRows, err := q.Query(ctx, `
		SELECT id, goal_set_id, title, description, target_value, weight, manager_score, manager_comment
		FROM kpi_goals WHERE goal_set_id = ANY($1) ORDER BY goal_set_id, created_at, id
	`, setIDs)
	if err != nil {
		return nil, err
	}
	goalIndex := make(map[uuid.UUID]int)
	goalLocations := make(map[uuid.UUID]struct{ set, goal int })
	goalIDs := make([]uuid.UUID, 0)
	for goalRows.Next() {
		var item kpiGoal
		var setID uuid.UUID
		if err := goalRows.Scan(&item.ID, &setID, &item.Title, &item.Description, &item.TargetValue, &item.Weight, &item.ManagerScore, &item.ManagerComment); err != nil {
			goalRows.Close()
			return nil, err
		}
		goalIndex[item.ID] = len(sets[index[setID]].Goals)
		goalIDs = append(goalIDs, item.ID)
		item.ProgressUpdates = make([]kpiProgress, 0)
		sets[index[setID]].Goals = append(sets[index[setID]].Goals, item)
		goalLocations[item.ID] = struct{ set, goal int }{index[setID], goalIndex[item.ID]}
	}
	err = goalRows.Err()
	goalRows.Close()
	if err != nil {
		return nil, err
	}
	if len(goalIDs) == 0 {
		return sets, nil
	}
	progressRows, err := q.Query(ctx, `
		SELECT p.id, p.goal_id, p.entered_by, COALESCE(e.full_name, 'Akun tidak aktif'),
			p.current_value, p.progress_percent, p.note, p.created_at
		FROM kpi_progress_updates p
		LEFT JOIN employees e ON e.user_id = p.entered_by
		WHERE p.goal_id = ANY($1)
		ORDER BY p.created_at DESC
	`, goalIDs)
	if err != nil {
		return nil, err
	}
	for progressRows.Next() {
		var item kpiProgress
		var goalID uuid.UUID
		if err := progressRows.Scan(&item.ID, &goalID, &item.EnteredBy, &item.EnteredByName, &item.CurrentValue, &item.ProgressPercent, &item.Note, &item.CreatedAt); err != nil {
			progressRows.Close()
			return nil, err
		}
		if location, found := goalLocations[goalID]; found {
			sets[location.set].Goals[location.goal].ProgressUpdates = append(sets[location.set].Goals[location.goal].ProgressUpdates, item)
		}
	}
	err = progressRows.Err()
	progressRows.Close()
	if err != nil {
		return nil, err
	}
	appealRows, err := q.Query(ctx, `
		SELECT goal_set_id, status, reason, decision, decision_note, adjusted_score, submitted_at, decided_at
		FROM kpi_appeals WHERE goal_set_id = ANY($1)
	`, setIDs)
	if err != nil {
		return nil, err
	}
	for appealRows.Next() {
		var setID uuid.UUID
		var item kpiAppeal
		if err := appealRows.Scan(&setID, &item.Status, &item.Reason, &item.Decision, &item.DecisionNote, &item.AdjustedScore, &item.SubmittedAt, &item.DecidedAt); err != nil {
			appealRows.Close()
			return nil, err
		}
		sets[index[setID]].Appeal = &item
	}
	if err := appealRows.Err(); err != nil {
		appealRows.Close()
		return nil, err
	}
	appealRows.Close()
	return sets, nil
}

func firstKPISet(sets []kpiSetView) *kpiSetView {
	if len(sets) == 0 {
		return nil
	}
	return &sets[0]
}

func requestedKPIYear(w http.ResponseWriter, r *http.Request) (int, bool) {
	value := r.URL.Query().Get("year")
	if value == "" {
		return time.Now().In(jakartaTime).Year(), true
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 2000 || year > 2100 {
		writeError(w, http.StatusBadRequest, "Tahun KPI tidak valid.")
		return 0, false
	}
	return year, true
}

func validKPICycleTransition(oldStatus, newStatus string) bool {
	return oldStatus == newStatus ||
		(oldStatus == "planning" && newStatus == "open") ||
		(oldStatus == "open" && newStatus == "closed")
}

func weightedKPIScore(weights, scores []float64) (float64, bool) {
	if len(weights) == 0 || len(weights) != len(scores) {
		return 0, false
	}
	weightTotal := 0.0
	scoreTotal := 0.0
	for index, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 || weight > 100 ||
			math.IsNaN(scores[index]) || math.IsInf(scores[index], 0) || scores[index] < 0 || scores[index] > 100 {
			return 0, false
		}
		weightTotal += weight
		scoreTotal += weight * scores[index]
	}
	if math.Abs(weightTotal-100) > 1e-9 {
		return 0, false
	}
	return math.Round(scoreTotal) / 100, true
}

func kpiAppealDeadline(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, finalizedAt time.Time, businessDays int) (time.Time, error) {
	date := jakartaDate(finalizedAt)
	counted := 0
	for counted < businessDays {
		date = date.AddDate(0, 0, 1)
		workday, err := isWorkingDayOnCalendar(ctx, q, date, []int16{1, 2, 3, 4, 5})
		if err != nil {
			return time.Time{}, err
		}
		if workday {
			counted++
		}
	}
	return date, nil
}

func kpiReportScope(user principal, year int, rawBranch string) (string, []any, bool) {
	if isHR(user.Role) {
		if rawBranch == "" {
			return "WHERE s.kpi_year = $1", []any{year}, true
		}
		branchID, err := uuid.Parse(rawBranch)
		if err != nil {
			return "", nil, false
		}
		return "WHERE s.kpi_year = $1 AND s.branch_id = $2", []any{year, branchID}, true
	}
	if user.Role == "manager" {
		return "WHERE s.kpi_year = $1 AND s.manager_id = $2", []any{year, user.ID}, rawBranch == "" || rawBranch == user.BranchID.String()
	}
	return "", nil, false
}

func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		safeRow := make([]string, len(row))
		for i, value := range row {
			safeRow[i] = safeCSVCell(value)
		}
		if err := writer.Write(safeRow); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func safeCSVCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + value
	case '-':
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return "'" + value
		}
	}
	return value
}
