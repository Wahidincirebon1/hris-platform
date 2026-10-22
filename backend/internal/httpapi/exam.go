package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const kpiExamPassingPercent = 70.0

type kpiExamQuestion struct {
	ID           int      `json:"id"`
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	DivisionName string   `json:"division_name"`
}

type kpiExamAttempt struct {
	ID           int       `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	DivisionName string    `json:"division_name"`
	CorrectCount int       `json:"correct_count"`
	TotalCount   int       `json:"total_count"`
	ScorePercent float64   `json:"score_percent"`
	Passed       bool      `json:"passed"`
	SubmittedAt  time.Time `json:"submitted_at"`
}

type kpiExamSchedule struct {
	ID            uuid.UUID `json:"id"`
	DepartmentID  uuid.UUID `json:"department_id"`
	DivisionName  string    `json:"division_name"`
	Title         string    `json:"title"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	QuestionCount int       `json:"question_count"`
}

func (a *API) registerExamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/exam/me", a.authenticated(a.myExamStatus))
	mux.HandleFunc("GET /api/v1/exam/questions", a.authenticated(a.examQuestions))
	mux.HandleFunc("POST /api/v1/exam/submit", a.authenticated(a.submitExam))
	mux.HandleFunc("GET /api/v1/exam/schedules", a.authenticated(a.listExamSchedules))
	mux.HandleFunc("PUT /api/v1/exam/schedules/{departmentID}", a.authenticated(a.saveExamSchedule))
	mux.HandleFunc("GET /api/v1/exam/all-questions", a.authenticated(a.allExamQuestions))
	mux.HandleFunc("GET /api/v1/exam/attempts", a.authenticated(a.examAttempts))
}

// employeeDivisionAndSchedule mengambil divisi karyawan beserta jadwal ujian divisi tersebut.
// Jadwal nil (true) berarti HR belum menentukan jadwal untuk divisi karyawan tersebut.
func (a *API) employeeDivisionAndSchedule(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (string, *kpiExamSchedule, bool) {
	var division string
	if err := a.db.QueryRow(r.Context(), `
		SELECT COALESCE(d.name, '')
		FROM users u
		JOIN employees e ON e.user_id = u.id
		LEFT JOIN departments d ON d.id = e.department_id
		WHERE u.id = $1
	`, userID).Scan(&division); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Profil karyawan tidak ditemukan.")
		} else {
			log.Printf("load employee division failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Divisi karyawan tidak dapat dimuat.")
		}
		return "", nil, false
	}
	var schedule kpiExamSchedule
	err := a.db.QueryRow(r.Context(), `
		SELECT s.id, s.department_id, d.name, s.title, s.starts_at, s.ends_at, s.updated_at,
		       (SELECT count(*)::integer FROM kpi_exam_questions q
		        WHERE q.active AND q.division_name = d.name)
		FROM kpi_exam_schedules s
		JOIN departments d ON d.id = s.department_id
		WHERE d.name = $1
		ORDER BY s.updated_at DESC
		LIMIT 1
	`, division).Scan(&schedule.ID, &schedule.DepartmentID, &schedule.DivisionName, &schedule.Title,
		&schedule.StartsAt, &schedule.EndsAt, &schedule.UpdatedAt, &schedule.QuestionCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return division, nil, true
	}
	if err != nil {
		log.Printf("load exam schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal ujian tidak dapat dimuat.")
		return "", nil, false
	}
	return division, &schedule, true
}

// myExamStatus: status ujian pribadi (jadwal, divisi, dan hasil bila sudah selesai).
func (a *API) myExamStatus(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	division, schedule, ok := a.employeeDivisionAndSchedule(w, r, user.ID)
	if !ok {
		return
	}
	var attempt kpiExamAttempt
	err := a.db.QueryRow(r.Context(), `
		SELECT id, user_id, COALESCE(division_name, ''), correct_count, total_count,
		       score_percent::float8, passed, submitted_at
		FROM kpi_exam_attempts WHERE user_id = $1
	`, user.ID).Scan(&attempt.ID, &attempt.UserID, &attempt.DivisionName, &attempt.CorrectCount,
		&attempt.TotalCount, &attempt.ScorePercent, &attempt.Passed, &attempt.SubmittedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load exam status failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Status ujian tidak dapat dimuat.")
		return
	}
	now := time.Now()
	state := "not_scheduled"
	if schedule != nil {
		switch {
		case now.Before(schedule.StartsAt):
			state = "not_open"
		case now.After(schedule.EndsAt):
			state = "closed"
		default:
			state = "open"
		}
	}
	response := map[string]any{
		"attempted":       err == nil,
		"passing_percent": kpiExamPassingPercent,
		"division_name":   division,
		"schedule":        schedule,
		"state":           state,
		"server_time":     now,
	}
	if err == nil {
		response["attempt"] = attempt
	}
	writeJSON(w, http.StatusOK, response)
}

// examQuestions: 10 soal milik divisi karyawan TANPA kunci jawaban, hanya dalam jadwal HR.
func (a *API) examQuestions(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	division, schedule, ok := a.employeeDivisionAndSchedule(w, r, user.ID)
	if !ok {
		return
	}
	if schedule == nil {
		writeError(w, http.StatusConflict, "Jadwal ujian KPI untuk divisi Anda belum ditentukan HR.")
		return
	}
	now := time.Now()
	if now.Before(schedule.StartsAt) || now.After(schedule.EndsAt) {
		writeError(w, http.StatusConflict, "Ujian KPI hanya dapat dikerjakan dalam jadwal yang ditetapkan HR.")
		return
	}
	var existing bool
	if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM kpi_exam_attempts WHERE user_id = $1)", user.ID).Scan(&existing); err != nil {
		writeError(w, http.StatusInternalServerError, "Status ujian tidak dapat diperiksa.")
		return
	}
	if existing {
		writeError(w, http.StatusConflict, "Anda sudah mengikuti ujian KPI; soal tidak dapat dibuka kembali.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, question, options
		FROM kpi_exam_questions
		WHERE active AND division_name = $1
		ORDER BY id
		LIMIT 10
	`, division)
	if err != nil {
		log.Printf("load exam questions failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	questions := make([]kpiExamQuestion, 0)
	for rows.Next() {
		var item kpiExamQuestion
		var optionsJSON []byte
		if err := rows.Scan(&item.ID, &item.Question, &optionsJSON); err != nil {
			log.Printf("scan exam question failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
			return
		}
		if err := json.Unmarshal(optionsJSON, &item.Options); err != nil {
			log.Printf("unmarshal exam options failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
			return
		}
		item.DivisionName = division
		questions = append(questions, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read exam questions failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
		return
	}
	if len(questions) == 0 {
		writeError(w, http.StatusConflict, "Soal ujian untuk divisi Anda belum tersedia. Hubungi HR.")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

// submitExam: koreksi jawaban, simpan hasil sekali saja (UNIQUE user_id).
func (a *API) submitExam(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	var input struct {
		Answers map[string]int `json:"answers"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Answers) == 0 {
		writeError(w, http.StatusBadRequest, "Jawaban ujian wajib diisi.")
		return
	}
	division, schedule, ok := a.employeeDivisionAndSchedule(w, r, user.ID)
	if !ok {
		return
	}
	if schedule == nil {
		writeError(w, http.StatusConflict, "Jadwal ujian KPI untuk divisi Anda belum ditentukan HR.")
		return
	}
	now := time.Now()
	if now.Before(schedule.StartsAt) || now.After(schedule.EndsAt) {
		writeError(w, http.StatusConflict, "Ujian hanya dapat dikirim dalam jadwal yang ditetapkan HR.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, question, options, correct_index FROM kpi_exam_questions
		WHERE active AND division_name = $1 ORDER BY id LIMIT 10
	`, division)
	if err != nil {
		log.Printf("load exam answer key failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Koreksi ujian tidak dapat diproses.")
		return
	}
	defer rows.Close()
	type questionKey struct {
		ID           int
		Question     string
		Options      []string
		CorrectIndex int
	}
	keys := make([]questionKey, 0)
	for rows.Next() {
		var item questionKey
		var optionsJSON []byte
		if err := rows.Scan(&item.ID, &item.Question, &optionsJSON, &item.CorrectIndex); err != nil {
			log.Printf("scan exam answer key failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Koreksi ujian tidak dapat diproses.")
			return
		}
		_ = json.Unmarshal(optionsJSON, &item.Options)
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read exam answer key failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Koreksi ujian tidak dapat diproses.")
		return
	}
	if len(keys) == 0 {
		writeError(w, http.StatusConflict, "Soal ujian untuk divisi Anda belum tersedia. Hubungi HR.")
		return
	}
	correct := 0
	for _, key := range keys {
		if answer, ok := input.Answers[intToString(key.ID)]; ok && answer == key.CorrectIndex {
			correct++
		}
	}
	total := len(keys)
	score := 0.0
	if total > 0 {
		score = float64(correct) / float64(total) * 100
	}
	passed := score >= kpiExamPassingPercent
	answersJSON, _ := json.Marshal(input.Answers)
	snapshotJSON, _ := json.Marshal(keys)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Hasil ujian tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var attemptID int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO kpi_exam_attempts
			(user_id, answers, correct_count, total_count, score_percent, passed,
			 schedule_id, division_name, question_snapshot)
		VALUES ($1, $2::jsonb, $3, $4, $5, $6, $7, $8, $9::jsonb)
		RETURNING id
	`, user.ID, string(answersJSON), correct, total, score, passed,
		schedule.ID, division, string(snapshotJSON)).Scan(&attemptID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Anda sudah mengikuti ujian; hasil tidak dapat diubah.")
			return
		}
		log.Printf("save exam attempt failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hasil ujian tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "KPI_EXAM_SUBMIT", "kpi_exam_attempt",
		intToString(int(attemptID)), map[string]string{
			"score_percent": formatScore(score), "passed": boolToString(passed), "division": division,
		}); err != nil {
		log.Printf("audit exam submit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hasil ujian tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit exam attempt failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Hasil ujian tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"correct_count": correct, "total_count": total,
		"score_percent": score, "passed": passed, "passing_percent": kpiExamPassingPercent,
		"division_name": division,
	})
}

// allExamQuestions: superadmin dapat melihat soal BESERTA kunci jawaban.
func (a *API) allExamQuestions(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if actor.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "Hanya superadmin yang dapat melihat kunci jawaban ujian.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, COALESCE(division_name, ''), question, options, correct_index
		FROM kpi_exam_questions
		WHERE active
		ORDER BY division_name, id
	`)
	if err != nil {
		log.Printf("load all exam questions failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	type examQuestionWithKey struct {
		ID           int      `json:"id"`
		DivisionName string   `json:"division_name"`
		Question     string   `json:"question"`
		Options      []string `json:"options"`
		CorrectIndex int      `json:"correct_index"`
	}
	questions := make([]examQuestionWithKey, 0)
	for rows.Next() {
		var item examQuestionWithKey
		var optionsJSON []byte
		if err := rows.Scan(&item.ID, &item.DivisionName, &item.Question, &optionsJSON, &item.CorrectIndex); err != nil {
			log.Printf("scan all exam questions failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
			return
		}
		if err := json.Unmarshal(optionsJSON, &item.Options); err != nil {
			log.Printf("unmarshal all exam options failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
			return
		}
		questions = append(questions, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read all exam questions failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Soal ujian tidak dapat dibaca.")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

// examAttempts: HR dan superadmin dapat melihat hasil ujian semua karyawan.
func (a *API) examAttempts(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hasil ujian hanya tersedia untuk HR dan superadmin.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT att.id, att.user_id, COALESCE(att.division_name, ''), e.employee_number, e.full_name,
		       att.correct_count, att.total_count, att.score_percent::float8, att.passed, att.submitted_at
		FROM kpi_exam_attempts att
		JOIN employees e ON e.user_id = att.user_id
		JOIN users u ON u.id = att.user_id
		WHERE ($1::uuid IS NULL OR u.branch_id = $1)
		ORDER BY att.submitted_at DESC
	`, examBranchScope(actor))
	if err != nil {
		log.Printf("load exam attempts failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Daftar hasil ujian tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	type attemptWithEmployee struct {
		kpiExamAttempt
		EmployeeNumber string `json:"employee_number"`
		FullName       string `json:"full_name"`
	}
	attempts := make([]attemptWithEmployee, 0)
	for rows.Next() {
		var item attemptWithEmployee
		if err := rows.Scan(&item.ID, &item.UserID, &item.DivisionName, &item.EmployeeNumber, &item.FullName,
			&item.CorrectCount, &item.TotalCount, &item.ScorePercent, &item.Passed, &item.SubmittedAt); err != nil {
			log.Printf("scan exam attempts failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Daftar hasil ujian tidak dapat dibaca.")
			return
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read exam attempts failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Daftar hasil ujian tidak dapat dibaca.")
		return
	}
	writeJSON(w, http.StatusOK, attempts)
}

// listExamSchedules: HR melihat jadwal ujian tiap divisi pada cabangnya.
func (a *API) listExamSchedules(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Jadwal ujian hanya tersedia untuk HR dan superadmin.")
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT d.id, d.name, s.title, s.starts_at, s.ends_at, s.updated_at,
		       (SELECT count(*)::integer FROM kpi_exam_questions q
		        WHERE q.active AND q.division_name = d.name)
		FROM departments d
		LEFT JOIN kpi_exam_schedules s ON s.department_id = d.id
		WHERE d.active AND ($1::uuid IS NULL OR d.branch_id = $1)
		ORDER BY d.name
	`, examBranchScope(actor))
	if err != nil {
		log.Printf("list exam schedules failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Daftar jadwal ujian tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	type scheduleRow struct {
		DepartmentID  uuid.UUID  `json:"department_id"`
		DivisionName  string     `json:"division_name"`
		Title         *string    `json:"title"`
		StartsAt      *time.Time `json:"starts_at"`
		EndsAt        *time.Time `json:"ends_at"`
		UpdatedAt     *time.Time `json:"updated_at"`
		QuestionCount int        `json:"question_count"`
		Scheduled     bool       `json:"scheduled"`
	}
	result := make([]scheduleRow, 0)
	for rows.Next() {
		var item scheduleRow
		if err := rows.Scan(&item.DepartmentID, &item.DivisionName, &item.Title,
			&item.StartsAt, &item.EndsAt, &item.UpdatedAt, &item.QuestionCount); err != nil {
			log.Printf("scan exam schedule failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Daftar jadwal ujian tidak dapat dibaca.")
			return
		}
		item.Scheduled = item.StartsAt != nil && item.EndsAt != nil
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read exam schedules failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Daftar jadwal ujian tidak dapat dibaca.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// saveExamSchedule: HR menentukan jadwal ujian manual untuk satu divisi.
func (a *API) saveExamSchedule(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR dan superadmin yang dapat mengatur jadwal ujian.")
		return
	}
	departmentID, err := uuid.Parse(r.PathValue("departmentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID divisi tidak valid.")
		return
	}
	var input struct {
		Title    string `json:"title"`
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	startsAt, errStart := time.Parse(time.RFC3339, input.StartsAt)
	endsAt, errEnd := time.Parse(time.RFC3339, input.EndsAt)
	if errStart != nil || errEnd != nil || len(input.Title) < 3 || len(input.Title) > 150 || !endsAt.After(startsAt) {
		writeError(w, http.StatusBadRequest, "Judul wajib 3–150 karakter dan waktu selesai harus setelah waktu mulai.")
		return
	}
	var branchID uuid.UUID
	if err := a.db.QueryRow(r.Context(), "SELECT branch_id FROM departments WHERE id = $1 AND active", departmentID).Scan(&branchID); err != nil {
		writeError(w, http.StatusNotFound, "Divisi tidak ditemukan atau tidak aktif.")
		return
	}
	if actor.Role == "hr" && branchID != actor.BranchID {
		writeError(w, http.StatusForbidden, "HR hanya dapat mengatur jadwal divisi pada cabangnya sendiri.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jadwal ujian tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO kpi_exam_schedules (department_id, title, starts_at, ends_at, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (department_id) DO UPDATE
		SET title = EXCLUDED.title, starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at,
		    updated_at = now()
		RETURNING id
	`, departmentID, input.Title, startsAt, endsAt, actor.ID).Scan(&id)
	if err != nil {
		log.Printf("save exam schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal ujian tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "KPI_EXAM_SCHEDULE", "department", departmentID.String(), map[string]string{
		"starts_at": startsAt.Format(time.RFC3339), "ends_at": endsAt.Format(time.RFC3339), "title": input.Title,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "Jadwal ujian tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Jadwal ujian tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "department_id": departmentID, "title": input.Title,
		"starts_at": startsAt, "ends_at": endsAt,
	})
}

// examBranchScope membatasi HR ke cabangnya; superadmin melihat semua cabang.
func examBranchScope(actor principal) *uuid.UUID {
	if actor.Role == "superadmin" {
		return nil
	}
	branch := actor.BranchID
	return &branch
}
