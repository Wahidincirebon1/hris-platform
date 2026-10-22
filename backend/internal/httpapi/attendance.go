package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// canUseSelfService adalah satu-satunya sumber kebenaran hak layanan mandiri
// (absen masuk/pulang, cuti, izin, lembur) untuk semua peran termasuk HR
// dan superadmin, sehingga hak akses di frontend dan backend selalu konsisten.
func canUseSelfService(role string) bool {
	return role == "employee" || role == "manager" || role == "hr" || role == "superadmin"
}

const maxAttendanceGPSAccuracyMeters = 30

var jakartaTime = time.FixedZone("WIB", 7*60*60)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type workSchedule struct {
	ID            uuid.UUID
	EffectiveFrom time.Time
	Timezone      string
	WorkStart     string
	WorkEnd       string
	BreakStart    string
	BreakEnd      string
	WorkingDays   []int16
}

type officeGeofence struct {
	ID            uuid.UUID
	EffectiveFrom time.Time
	Latitude      float64
	Longitude     float64
	RadiusMeters  int
}

type locationInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy_meters"`
}

type attendanceRecord struct {
	ID                    uuid.UUID  `json:"id"`
	UserID                uuid.UUID  `json:"user_id"`
	EmployeeNumber        string     `json:"employee_number"`
	FullName              string     `json:"full_name"`
	Role                  string     `json:"role"`
	BranchID              uuid.UUID  `json:"branch_id"`
	BranchName            string     `json:"branch_name"`
	Date                  string     `json:"date"`
	CheckInAt             time.Time  `json:"check_in_at"`
	CheckInDistanceMeters float64    `json:"check_in_distance_meters"`
	CheckInAccuracyMeters float64    `json:"check_in_accuracy_meters"`
	Late                  bool       `json:"late"`
	LateMinutes           int        `json:"late_minutes"`
	CheckOutAt            *time.Time `json:"check_out_at"`
	CheckOutDistance      *float64   `json:"check_out_distance_meters"`
	CheckOutAccuracy      *float64   `json:"check_out_accuracy_meters"`
}

type scheduleResponse struct {
	ID            uuid.UUID `json:"id"`
	EffectiveFrom string    `json:"effective_from"`
	Timezone      string    `json:"timezone"`
	WorkStart     string    `json:"work_start"`
	WorkEnd       string    `json:"work_end"`
	BreakStart    string    `json:"break_start"`
	BreakEnd      string    `json:"break_end"`
	WorkingDays   []int16   `json:"working_days"`
}

func (s workSchedule) response() scheduleResponse {
	return scheduleResponse{
		ID: s.ID, EffectiveFrom: s.EffectiveFrom.Format("2006-01-02"),
		Timezone: s.Timezone, WorkStart: s.WorkStart, WorkEnd: s.WorkEnd,
		BreakStart: s.BreakStart, BreakEnd: s.BreakEnd,
		WorkingDays: append([]int16(nil), s.WorkingDays...),
	}
}

func (a *API) registerAttendanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/attendance/today", a.authenticated(a.attendanceToday))
	mux.HandleFunc("GET /api/v1/attendance/overview", a.authenticated(a.attendanceOverview))
	mux.HandleFunc("GET /api/v1/attendance/me", a.authenticated(a.myAttendance))
	mux.HandleFunc("GET /api/v1/attendance", a.authenticated(a.attendanceReport))
	mux.HandleFunc("POST /api/v1/attendance/check-in", a.authenticated(a.checkIn))
	mux.HandleFunc("POST /api/v1/attendance/check-out", a.authenticated(a.checkOut))
	mux.HandleFunc("PATCH /api/v1/attendance/{id}", a.authenticated(a.correctAttendance))
	mux.HandleFunc("GET /api/v1/attendance/settings", a.authenticated(a.attendanceSettings))
	mux.HandleFunc("PUT /api/v1/attendance/schedule", a.authenticated(a.createAttendanceSchedule))
	mux.HandleFunc("PUT /api/v1/branches/{id}/geofence", a.authenticated(a.createGeofence))
}

func (a *API) attendanceToday(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Layanan absensi pribadi tidak tersedia untuk role ini.")
		return
	}
	now := time.Now().In(jakartaTime)
	date := localDate(now)
	schedule, err := loadSchedule(r.Context(), a.db, date)
	if err != nil {
		log.Printf("load attendance schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal absensi tidak dapat dimuat.")
		return
	}
	geofence, err := loadGeofence(r.Context(), a.db, user.BranchID, date)
	geofenceConfigured := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load attendance geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat diverifikasi.")
		return
	}
	record, err := loadAttendanceRecord(r.Context(), a.db, user.ID, date)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("load today's attendance failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Catatan absensi hari ini tidak dapat dimuat.")
		return
	}
	workday, err := isWorkingDayOnCalendar(r.Context(), a.db, date, schedule.WorkingDays)
	if err != nil {
		log.Printf("load attendance calendar failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender kerja tidak dapat dimuat.")
		return
	}
	var yearToDate struct {
		DaysPresent  int `json:"days_present"`
		LateArrivals int `json:"late_arrivals"`
		LateMinutes  int `json:"late_minutes"`
	}
	if err := a.db.QueryRow(r.Context(), `
		SELECT count(*)::integer,
		       count(*) FILTER (WHERE late)::integer,
		       COALESCE(sum(late_minutes), 0)::integer
		FROM attendance_records
		WHERE user_id = $1
		  AND attendance_date >= date_trunc('year', $2::date)::date
		  AND attendance_date <= $2
	`, user.ID, date).Scan(&yearToDate.DaysPresent, &yearToDate.LateArrivals, &yearToDate.LateMinutes); err != nil {
		log.Printf("load attendance year-to-date summary failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Ringkasan keterlambatan tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"date": date.Format("2006-01-02"), "is_workday": workday,
		"schedule": schedule.response(), "geofence_configured": geofenceConfigured,
		"geofence_radius_meters":      geofenceRadius(geofence),
		"maximum_gps_accuracy_meters": maxAttendanceGPSAccuracyMeters,
		"record":                      record, "year_to_date": yearToDate,
	})
}

func (a *API) attendanceOverview(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	var branchID, managerID *uuid.UUID
	if isHR(actor.Role) {
		if raw := r.URL.Query().Get("branch_id"); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "ID cabang tidak valid.")
				return
			}
			branchID = &parsed
		}
	} else if actor.Role == "manager" {
		branchID, managerID = &actor.BranchID, &actor.ID
	} else {
		writeError(w, http.StatusForbidden, "Ringkasan absensi hanya tersedia untuk HR dan manager.")
		return
	}
	date := jakartaDate(time.Now())
	rows, err := a.db.Query(r.Context(), `
		SELECT u.id, e.employee_number, e.full_name, u.role, u.branch_id, b.name,
		       today.check_in_at, today.check_out_at,
		       COALESCE(today.late, FALSE), COALESCE(today.late_minutes, 0),
		       COALESCE(ytd.days_present, 0)::integer,
		       COALESCE(ytd.late_arrivals, 0)::integer,
		       COALESCE(ytd.late_minutes, 0)::integer
		FROM users u
		JOIN employees e ON e.user_id = u.id
		JOIN branches b ON b.id = u.branch_id
		LEFT JOIN attendance_records today
		       ON today.user_id = u.id AND today.attendance_date = $1
		LEFT JOIN (
		    SELECT user_id, count(*) AS days_present,
		           count(*) FILTER (WHERE late) AS late_arrivals,
		           sum(late_minutes) AS late_minutes
		    FROM attendance_records
		    WHERE attendance_date >= date_trunc('year', $1::date)::date
		      AND attendance_date <= $1
		    GROUP BY user_id
		) ytd ON ytd.user_id = u.id
		WHERE u.active AND e.employment_status = 'active'
		  AND ($2::uuid IS NULL OR u.branch_id = $2)
		  AND ($3::uuid IS NULL OR u.manager_id = $3)
		ORDER BY (today.check_in_at IS NOT NULL) DESC, b.name, e.full_name
	`, date, branchID, managerID)
	if err != nil {
		log.Printf("load attendance overview failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Ringkasan absensi karyawan tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	type attendanceOverviewEntry struct {
		UserID           uuid.UUID  `json:"user_id"`
		EmployeeNumber   string     `json:"employee_number"`
		FullName         string     `json:"full_name"`
		Role             string     `json:"role"`
		BranchID         uuid.UUID  `json:"branch_id"`
		BranchName       string     `json:"branch_name"`
		CheckInAt        *time.Time `json:"check_in_at,omitempty"`
		CheckOutAt       *time.Time `json:"check_out_at,omitempty"`
		LateToday        bool       `json:"late_today"`
		LateMinutesToday int        `json:"late_minutes_today"`
		DaysPresentYTD   int        `json:"days_present_ytd"`
		LateArrivalsYTD  int        `json:"late_arrivals_ytd"`
		LateMinutesYTD   int        `json:"late_minutes_ytd"`
	}
	result := make([]attendanceOverviewEntry, 0)
	for rows.Next() {
		var item attendanceOverviewEntry
		var checkInAt, checkOutAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.UserID, &item.EmployeeNumber, &item.FullName, &item.Role, &item.BranchID,
			&item.BranchName, &checkInAt, &checkOutAt, &item.LateToday, &item.LateMinutesToday,
			&item.DaysPresentYTD, &item.LateArrivalsYTD, &item.LateMinutesYTD,
		); err != nil {
			log.Printf("scan attendance overview entry failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Ringkasan absensi karyawan tidak dapat dibaca.")
			return
		}
		if checkInAt.Valid {
			value := checkInAt.Time
			item.CheckInAt = &value
		}
		if checkOutAt.Valid {
			value := checkOutAt.Time
			item.CheckOutAt = &value
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("read attendance overview failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Ringkasan absensi karyawan tidak dapat dibaca.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date.Format("2006-01-02"), "employees": result})
}

func geofenceRadius(geofence officeGeofence) int {
	return geofence.RadiusMeters
}

func (a *API) myAttendance(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Riwayat absensi pribadi tidak tersedia untuk role ini.")
		return
	}
	from, to, ok := attendanceDateRange(w, r)
	if !ok {
		return
	}
	records, err := queryAttendance(r.Context(), a.db, from, to, &user.ID, nil, nil)
	if err != nil {
		log.Printf("load personal attendance failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Riwayat absensi tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (a *API) attendanceReport(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	var branchID *uuid.UUID
	var managerID *uuid.UUID
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
		writeError(w, http.StatusForbidden, "Laporan absensi hanya tersedia untuk HR dan atasan.")
		return
	}
	from, to, ok := attendanceDateRange(w, r)
	if !ok {
		return
	}
	records, err := queryAttendance(r.Context(), a.db, from, to, nil, branchID, managerID)
	if err != nil {
		log.Printf("load attendance report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan absensi tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (a *API) attendanceSettings(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Pengaturan absensi hanya tersedia untuk HR dan superadmin.")
		return
	}
	today := localDate(time.Now().In(jakartaTime))
	schedule, err := loadSchedule(r.Context(), a.db, today)
	if err != nil {
		log.Printf("load current attendance schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengaturan jadwal tidak dapat dimuat.")
		return
	}
	rows, err := a.db.Query(r.Context(), "SELECT id, code, name FROM branches ORDER BY name")
	if err != nil {
		log.Printf("load attendance branches failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Pengaturan lokasi tidak dapat dimuat.")
		return
	}
	type branchSettings struct {
		ID            uuid.UUID `json:"id"`
		Code          string    `json:"code"`
		Name          string    `json:"name"`
		Configured    bool      `json:"configured"`
		EffectiveFrom *string   `json:"effective_from,omitempty"`
		Latitude      *float64  `json:"latitude,omitempty"`
		Longitude     *float64  `json:"longitude,omitempty"`
		RadiusMeters  *int      `json:"radius_meters,omitempty"`
	}
	branches := make([]branchSettings, 0)
	for rows.Next() {
		var item branchSettings
		if err := rows.Scan(&item.ID, &item.Code, &item.Name); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "Pengaturan lokasi tidak dapat dibaca.")
			return
		}
		geofence, err := loadGeofence(r.Context(), a.db, item.ID, today)
		if err == nil {
			effective := geofence.EffectiveFrom.Format("2006-01-02")
			item.Configured = true
			item.EffectiveFrom = &effective
			item.Latitude = &geofence.Latitude
			item.Longitude = &geofence.Longitude
			item.RadiusMeters = &geofence.RadiusMeters
		} else if !errors.Is(err, pgx.ErrNoRows) {
			rows.Close()
			log.Printf("load branch geofence failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Pengaturan lokasi tidak dapat dimuat.")
			return
		}
		branches = append(branches, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "Pengaturan lokasi tidak dapat dibaca.")
		return
	}
	rows.Close()
	writeJSON(w, http.StatusOK, map[string]any{
		"schedule": schedule.response(), "branches": branches,
		"maximum_gps_accuracy_meters": maxAttendanceGPSAccuracyMeters,
	})
}

func (a *API) createAttendanceSchedule(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR dan superadmin yang dapat mengubah jadwal.")
		return
	}
	var input struct {
		EffectiveFrom string `json:"effective_from"`
		WorkStart     string `json:"work_start"`
		WorkEnd       string `json:"work_end"`
		BreakStart    string `json:"break_start"`
		BreakEnd      string `json:"break_end"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	effective, err := parseEffectiveDate(input.EffectiveFrom)
	if err != nil || effective.Before(jakartaDate(time.Now())) {
		writeError(w, http.StatusBadRequest, "Tanggal mulai berlaku harus hari ini atau tanggal mendatang.")
		return
	}
	start, startOK := parseClock(input.WorkStart)
	end, endOK := parseClock(input.WorkEnd)
	breakStart, breakStartOK := parseClock(input.BreakStart)
	breakEnd, breakEndOK := parseClock(input.BreakEnd)
	if !startOK || !endOK || !breakStartOK || !breakEndOK ||
		!(start.Before(breakStart) && breakStart.Before(breakEnd) && breakEnd.Before(end)) {
		writeError(w, http.StatusBadRequest, "Urutan jam harus: masuk, mulai istirahat, selesai istirahat, lalu pulang.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Jadwal tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var schedule workSchedule
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_schedules
			(effective_from, work_start, work_end, break_start, break_end, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, effective_from, timezone, work_start::text, work_end::text,
		          break_start::text, break_end::text, working_days
	`, effective, input.WorkStart, input.WorkEnd, input.BreakStart, input.BreakEnd, actor.ID).Scan(
		&schedule.ID, &schedule.EffectiveFrom, &schedule.Timezone, &schedule.WorkStart,
		&schedule.WorkEnd, &schedule.BreakStart, &schedule.BreakEnd, &schedule.WorkingDays,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Jadwal untuk tanggal tersebut sudah ada; pilih tanggal efektif lain.")
			return
		}
		log.Printf("create attendance schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "CREATE_ATTENDANCE_SCHEDULE", "attendance_schedule", schedule.ID.String(), map[string]string{
		"effective_from": schedule.EffectiveFrom.Format("2006-01-02"),
		"work_start":     schedule.WorkStart, "work_end": schedule.WorkEnd,
		"break_start": schedule.BreakStart, "break_end": schedule.BreakEnd,
	}); err != nil {
		log.Printf("audit attendance schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit attendance schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, schedule.response())
}

func (a *API) createGeofence(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Hanya HR dan superadmin yang dapat mengubah lokasi kantor.")
		return
	}
	branchID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID cabang tidak valid.")
		return
	}
	var input struct {
		EffectiveFrom string  `json:"effective_from"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		RadiusMeters  int     `json:"radius_meters"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	effective, err := parseEffectiveDate(input.EffectiveFrom)
	if err != nil || effective.Before(jakartaDate(time.Now())) {
		writeError(w, http.StatusBadRequest, "Tanggal mulai berlaku harus hari ini atau tanggal mendatang.")
		return
	}
	if math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) ||
		math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) ||
		input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 ||
		input.RadiusMeters < 25 || input.RadiusMeters > 2000 {
		writeError(w, http.StatusBadRequest, "Koordinat tidak valid atau radius harus antara 25 dan 2.000 meter.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	var branchExists bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1 AND active)", branchID).Scan(&branchExists); err != nil {
		log.Printf("verify geofence branch failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Cabang tidak dapat diverifikasi.")
		return
	}
	if !branchExists {
		writeError(w, http.StatusNotFound, "Cabang tidak ditemukan atau tidak aktif.")
		return
	}
	var geofence officeGeofence
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_geofences (branch_id, effective_from, latitude, longitude, radius_meters, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, effective_from, latitude, longitude, radius_meters
	`, branchID, effective, input.Latitude, input.Longitude, input.RadiusMeters, actor.ID).Scan(
		&geofence.ID, &geofence.EffectiveFrom, &geofence.Latitude, &geofence.Longitude, &geofence.RadiusMeters,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Lokasi untuk cabang dan tanggal tersebut sudah ada; pilih tanggal efektif lain.")
			return
		}
		log.Printf("create attendance geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "CREATE_ATTENDANCE_GEOFENCE", "attendance_geofence", geofence.ID.String(), map[string]string{
		"branch_id": branchID.String(), "effective_from": geofence.EffectiveFrom.Format("2006-01-02"),
		"latitude":      strconv.FormatFloat(geofence.Latitude, 'f', 6, 64),
		"longitude":     strconv.FormatFloat(geofence.Longitude, 'f', 6, 64),
		"radius_meters": strconv.Itoa(geofence.RadiusMeters),
	}); err != nil {
		log.Printf("audit attendance geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit attendance geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": geofence.ID, "branch_id": branchID,
		"effective_from": geofence.EffectiveFrom.Format("2006-01-02"),
		"radius_meters":  geofence.RadiusMeters,
	})
}

func (a *API) checkIn(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat mencatat absensi pribadi.")
		return
	}
	var input locationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	now := time.Now()
	localNow := now.In(jakartaTime)
	date := localDate(localNow)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Absen masuk tidak dapat diproses.")
		return
	}
	defer tx.Rollback(r.Context())
	schedule, err := loadSchedule(r.Context(), tx, date)
	if err != nil {
		log.Printf("load check-in schedule failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Jadwal absensi tidak dapat dimuat.")
		return
	}
	workday, err := isWorkingDayOnCalendar(r.Context(), tx, date, schedule.WorkingDays)
	if err != nil {
		log.Printf("load check-in calendar failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Kalender kerja tidak dapat dimuat.")
		return
	}
	if !workday {
		writeError(w, http.StatusConflict, "Hari ini bukan hari kerja; absensi tidak dibuka.")
		return
	}
	geofence, err := loadGeofence(r.Context(), tx, user.BranchID, date)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Lokasi kantor belum dikonfigurasi untuk cabang Anda. Hubungi HR.")
		return
	}
	if err != nil {
		log.Printf("load check-in geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat diverifikasi.")
		return
	}
	distance, status, message := validateLocation(input, geofence)
	if status != http.StatusOK {
		writeError(w, status, message)
		return
	}
	lateMinutes, late, err := calculateLateMinutes(now, localNow, schedule.WorkStart)
	if err != nil {
		log.Printf("calculate check-in lateness failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Waktu keterlambatan tidak dapat dihitung.")
		return
	}
	var recordID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_records
			(user_id, branch_id, attendance_date, schedule_id, check_in_geofence_id,
			 check_in_at, check_in_distance_meters, check_in_accuracy_meters, late, late_minutes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, user.ID, user.BranchID, date, schedule.ID, geofence.ID, now,
		distance, input.Accuracy, late, lateMinutes).Scan(&recordID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "Absen masuk hari ini sudah tercatat.")
			return
		}
		log.Printf("insert attendance check-in failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen masuk tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "ATTENDANCE_CHECK_IN", "attendance", recordID.String(), map[string]string{
		"date": date.Format("2006-01-02"), "distance_meters": strconv.FormatFloat(distance, 'f', 1, 64),
		"accuracy_meters": strconv.FormatFloat(input.Accuracy, 'f', 1, 64),
		"late_minutes":    strconv.Itoa(lateMinutes),
	}); err != nil {
		log.Printf("audit check-in failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen masuk tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit check-in failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen masuk tidak dapat disimpan.")
		return
	}
	record, err := loadAttendanceRecord(r.Context(), a.db, user.ID, date)
	if err != nil {
		log.Printf("reload check-in failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen masuk tersimpan, tetapi tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (a *API) checkOut(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !canUseEmployeeSelfService(user.Role) {
		writeError(w, http.StatusForbidden, "Role ini tidak dapat mencatat absensi pribadi.")
		return
	}
	var input locationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	now := time.Now()
	localNow := now.In(jakartaTime)
	date := localDate(localNow)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Absen pulang tidak dapat diproses.")
		return
	}
	defer tx.Rollback(r.Context())
	geofence, err := loadGeofence(r.Context(), tx, user.BranchID, date)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Lokasi kantor belum dikonfigurasi untuk cabang Anda. Hubungi HR.")
		return
	}
	if err != nil {
		log.Printf("load check-out geofence failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Lokasi kantor tidak dapat diverifikasi.")
		return
	}
	distance, status, message := validateLocation(input, geofence)
	if status != http.StatusOK {
		writeError(w, status, message)
		return
	}
	var recordID uuid.UUID
	var checkInAt time.Time
	var checkOutAt pgtype.Timestamptz
	err = tx.QueryRow(r.Context(), `
		SELECT id, check_in_at, check_out_at
		FROM attendance_records
		WHERE user_id = $1 AND attendance_date = $2
		FOR UPDATE
	`, user.ID, date).Scan(&recordID, &checkInAt, &checkOutAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Absen masuk hari ini belum tercatat.")
		return
	}
	if err != nil {
		log.Printf("load check-out record failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Catatan absensi tidak dapat diverifikasi.")
		return
	}
	if checkOutAt.Valid {
		writeError(w, http.StatusConflict, "Absen pulang hari ini sudah tercatat.")
		return
	}
	if now.Before(checkInAt) {
		writeError(w, http.StatusConflict, "Waktu absen pulang tidak boleh lebih awal dari absen masuk.")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE attendance_records
		SET check_out_geofence_id = $1, check_out_at = $2,
		    check_out_distance_meters = $3, check_out_accuracy_meters = $4,
		    updated_at = now()
		WHERE id = $5
	`, geofence.ID, now, distance, input.Accuracy, recordID); err != nil {
		log.Printf("update attendance check-out failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen pulang tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, user.ID, "ATTENDANCE_CHECK_OUT", "attendance", recordID.String(), map[string]string{
		"date": date.Format("2006-01-02"), "distance_meters": strconv.FormatFloat(distance, 'f', 1, 64),
		"accuracy_meters": strconv.FormatFloat(input.Accuracy, 'f', 1, 64),
	}); err != nil {
		log.Printf("audit check-out failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen pulang tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit check-out failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen pulang tidak dapat disimpan.")
		return
	}
	record, err := loadAttendanceRecord(r.Context(), a.db, user.ID, date)
	if err != nil {
		log.Printf("reload check-out failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Absen pulang tersimpan, tetapi tidak dapat dimuat.")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (a *API) correctAttendance(w http.ResponseWriter, r *http.Request) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) {
		writeError(w, http.StatusForbidden, "Koreksi absensi hanya dapat dilakukan HR dan superadmin.")
		return
	}
	recordID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID catatan absensi tidak valid.")
		return
	}
	var input struct {
		CheckInTime  *string `json:"check_in_time"`
		CheckOutTime *string `json:"check_out_time"`
		Reason       string  `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.CheckInTime == nil && input.CheckOutTime == nil {
		writeError(w, http.StatusBadRequest, "Isi waktu masuk atau pulang yang perlu dikoreksi.")
		return
	}
	if len(input.Reason) < 5 || len(input.Reason) > 500 {
		writeError(w, http.StatusBadRequest, "Alasan koreksi wajib berisi 5 sampai 500 karakter.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Koreksi absensi tidak dapat diproses.")
		return
	}
	defer tx.Rollback(r.Context())
	var date time.Time
	var checkInAt time.Time
	var checkOutAt pgtype.Timestamptz
	var workStart string
	err = tx.QueryRow(r.Context(), `
		SELECT r.attendance_date, r.check_in_at, r.check_out_at, s.work_start::text
		FROM attendance_records r
		JOIN attendance_schedules s ON s.id = r.schedule_id
		WHERE r.id = $1
		FOR UPDATE OF r
	`, recordID).Scan(&date, &checkInAt, &checkOutAt, &workStart)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Catatan absensi tidak ditemukan.")
		return
	}
	if err != nil {
		log.Printf("load attendance correction target failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Catatan absensi tidak dapat dimuat.")
		return
	}
	newCheckIn := checkInAt
	newCheckOut := checkOutAt
	if input.CheckInTime != nil {
		parsed, ok := parseClock(*input.CheckInTime)
		if !ok {
			writeError(w, http.StatusBadRequest, "Waktu masuk harus berformat HH:MM.")
			return
		}
		newCheckIn = atLocalTime(date, parsed)
	}
	if input.CheckOutTime != nil {
		parsed, ok := parseClock(*input.CheckOutTime)
		if !ok {
			writeError(w, http.StatusBadRequest, "Waktu pulang harus berformat HH:MM.")
			return
		}
		value := atLocalTime(date, parsed)
		newCheckOut = pgtype.Timestamptz{Time: value, Valid: true}
	}
	if newCheckOut.Valid && newCheckOut.Time.Before(newCheckIn) {
		writeError(w, http.StatusBadRequest, "Waktu pulang tidak boleh lebih awal dari waktu masuk.")
		return
	}
	lateMinutes, late, err := calculateLateMinutes(newCheckIn, date.In(jakartaTime), workStart)
	if err != nil {
		log.Printf("calculate corrected lateness failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Keterlambatan setelah koreksi tidak dapat dihitung.")
		return
	}
	before := fmt.Sprintf("masuk=%s; pulang=%s", checkInAt.Format(time.RFC3339), nullableTimestamp(checkOutAt))
	after := fmt.Sprintf("masuk=%s; pulang=%s", newCheckIn.Format(time.RFC3339), nullableTimestamp(newCheckOut))
	if _, err := tx.Exec(r.Context(), `
		UPDATE attendance_records
		SET check_in_at = $1,
		    check_out_at = $2,
		    late = $3,
		    late_minutes = $4,
		    updated_at = now()
		WHERE id = $5
	`, newCheckIn, nullableTime(newCheckOut), late, lateMinutes, recordID); err != nil {
		log.Printf("apply attendance correction failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Koreksi absensi tidak dapat disimpan.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "CORRECT_ATTENDANCE", "attendance", recordID.String(), map[string]string{
		"before": before, "after": after, "reason": input.Reason,
	}); err != nil {
		log.Printf("audit attendance correction failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Koreksi absensi tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit attendance correction failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Koreksi absensi tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "corrected", "late_minutes": lateMinutes})
}

func attendanceDateRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	today := jakartaDate(time.Now())
	from := today.AddDate(0, 0, -6)
	to := today
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := parseEffectiveDate(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Tanggal awal harus berformat YYYY-MM-DD.")
			return time.Time{}, time.Time{}, false
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := parseEffectiveDate(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Tanggal akhir harus berformat YYYY-MM-DD.")
			return time.Time{}, time.Time{}, false
		}
		to = parsed
	}
	days := int(to.Sub(from).Hours()/24) + 1
	if to.Before(from) || from.After(today) || to.After(today) || days > 31 {
		writeError(w, http.StatusBadRequest, "Periode harus sampai hari ini dan maksimal 31 hari.")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func queryAttendance(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, from, to time.Time, userID, branchID, managerID *uuid.UUID) ([]attendanceRecord, error) {
	rows, err := q.Query(ctx, `
		SELECT r.id, r.user_id, e.employee_number, e.full_name, u.role,
		       r.branch_id, b.name, r.attendance_date, r.check_in_at,
		       r.check_in_distance_meters, r.check_in_accuracy_meters, r.late, r.late_minutes,
		       r.check_out_at, r.check_out_distance_meters, r.check_out_accuracy_meters
		FROM attendance_records r
		JOIN users u ON u.id = r.user_id
		JOIN employees e ON e.user_id = r.user_id
		JOIN branches b ON b.id = r.branch_id
		WHERE r.attendance_date BETWEEN $1 AND $2
		  AND ($3::uuid IS NULL OR r.user_id = $3)
		  AND ($4::uuid IS NULL OR r.branch_id = $4)
		  AND ($5::uuid IS NULL OR u.manager_id = $5)
		ORDER BY r.attendance_date DESC, e.full_name
	`, from, to, userID, branchID, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]attendanceRecord, 0)
	for rows.Next() {
		var record attendanceRecord
		var date time.Time
		var checkOutAt pgtype.Timestamptz
		var checkOutDistance, checkOutAccuracy pgtype.Float8
		if err := rows.Scan(
			&record.ID, &record.UserID, &record.EmployeeNumber, &record.FullName,
			&record.Role, &record.BranchID, &record.BranchName, &date,
			&record.CheckInAt, &record.CheckInDistanceMeters, &record.CheckInAccuracyMeters,
			&record.Late, &record.LateMinutes, &checkOutAt, &checkOutDistance, &checkOutAccuracy,
		); err != nil {
			return nil, err
		}
		record.Date = date.Format("2006-01-02")
		if checkOutAt.Valid {
			value := checkOutAt.Time
			record.CheckOutAt = &value
		}
		if checkOutDistance.Valid {
			value := checkOutDistance.Float64
			record.CheckOutDistance = &value
		}
		if checkOutAccuracy.Valid {
			value := checkOutAccuracy.Float64
			record.CheckOutAccuracy = &value
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func loadAttendanceRecord(ctx context.Context, q rowQuerier, userID uuid.UUID, date time.Time) (*attendanceRecord, error) {
	var record attendanceRecord
	var attendanceDate time.Time
	var checkOutAt pgtype.Timestamptz
	var checkOutDistance, checkOutAccuracy pgtype.Float8
	err := q.QueryRow(ctx, `
		SELECT r.id, r.user_id, e.employee_number, e.full_name, u.role,
		       r.branch_id, b.name, r.attendance_date, r.check_in_at,
		       r.check_in_distance_meters, r.check_in_accuracy_meters, r.late, r.late_minutes,
		       r.check_out_at, r.check_out_distance_meters, r.check_out_accuracy_meters
		FROM attendance_records r
		JOIN users u ON u.id = r.user_id
		JOIN employees e ON e.user_id = r.user_id
		JOIN branches b ON b.id = r.branch_id
		WHERE r.user_id = $1 AND r.attendance_date = $2
	`, userID, date).Scan(
		&record.ID, &record.UserID, &record.EmployeeNumber, &record.FullName,
		&record.Role, &record.BranchID, &record.BranchName, &attendanceDate,
		&record.CheckInAt, &record.CheckInDistanceMeters, &record.CheckInAccuracyMeters,
		&record.Late, &record.LateMinutes, &checkOutAt, &checkOutDistance, &checkOutAccuracy,
	)
	if err != nil {
		return nil, err
	}
	record.Date = attendanceDate.Format("2006-01-02")
	if checkOutAt.Valid {
		value := checkOutAt.Time
		record.CheckOutAt = &value
	}
	if checkOutDistance.Valid {
		value := checkOutDistance.Float64
		record.CheckOutDistance = &value
	}
	if checkOutAccuracy.Valid {
		value := checkOutAccuracy.Float64
		record.CheckOutAccuracy = &value
	}
	return &record, nil
}

func loadSchedule(ctx context.Context, q rowQuerier, date time.Time) (workSchedule, error) {
	var schedule workSchedule
	err := q.QueryRow(ctx, `
		SELECT id, effective_from, timezone, work_start::text, work_end::text,
		       break_start::text, break_end::text, working_days
		FROM attendance_schedules
		WHERE effective_from <= $1
		ORDER BY effective_from DESC
		LIMIT 1
	`, date).Scan(
		&schedule.ID, &schedule.EffectiveFrom, &schedule.Timezone,
		&schedule.WorkStart, &schedule.WorkEnd, &schedule.BreakStart,
		&schedule.BreakEnd, &schedule.WorkingDays,
	)
	return schedule, err
}

func loadGeofence(ctx context.Context, q rowQuerier, branchID uuid.UUID, date time.Time) (officeGeofence, error) {
	var geofence officeGeofence
	err := q.QueryRow(ctx, `
		SELECT id, effective_from, latitude, longitude, radius_meters
		FROM attendance_geofences
		WHERE branch_id = $1 AND effective_from <= $2
		ORDER BY effective_from DESC
		LIMIT 1
	`, branchID, date).Scan(
		&geofence.ID, &geofence.EffectiveFrom, &geofence.Latitude,
		&geofence.Longitude, &geofence.RadiusMeters,
	)
	return geofence, err
}

func validateLocation(input locationInput, geofence officeGeofence) (float64, int, string) {
	if math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) ||
		math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) ||
		input.Latitude < -90 || input.Latitude > 90 ||
		input.Longitude < -180 || input.Longitude > 180 ||
		math.IsNaN(input.Accuracy) || math.IsInf(input.Accuracy, 0) || input.Accuracy <= 0 {
		return 0, http.StatusBadRequest, "Lokasi GPS tidak valid. Aktifkan lokasi dan coba lagi."
	}
	if input.Accuracy > maxAttendanceGPSAccuracyMeters {
		return 0, http.StatusUnprocessableEntity, "Akurasi GPS terlalu rendah. Coba lagi di area terbuka atau ajukan koreksi ke HR."
	}
	distance := haversineMeters(input.Latitude, input.Longitude, geofence.Latitude, geofence.Longitude)
	if distance > float64(geofence.RadiusMeters) {
		return 0, http.StatusForbidden, "Lokasi berada di luar area kantor. Jika Anda sedang bertugas di luar kantor, hubungi atasan dan HR."
	}
	return math.Round(distance*10) / 10, http.StatusOK, ""
}

func haversineMeters(latitude1, longitude1, latitude2, longitude2 float64) float64 {
	const earthRadiusMeters = 6371008.8
	toRadians := func(degrees float64) float64 { return degrees * math.Pi / 180 }
	latitudeDelta := toRadians(latitude2 - latitude1)
	longitudeDelta := toRadians(longitude2 - longitude1)
	latitude1Radians := toRadians(latitude1)
	latitude2Radians := toRadians(latitude2)
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(latitude1Radians)*math.Cos(latitude2Radians)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	a = math.Max(0, math.Min(1, a))
	return 2 * earthRadiusMeters * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func calculateLateMinutes(instant time.Time, localNow time.Time, workStart string) (int, bool, error) {
	startClock, ok := parseClock(workStart)
	if !ok {
		return 0, false, fmt.Errorf("invalid scheduled start time %q", workStart)
	}
	start := atLocalTime(localNow, startClock)
	elapsed := instant.In(jakartaTime).Sub(start)
	if elapsed <= 0 {
		return 0, false, nil
	}
	return int(elapsed / time.Minute), true, nil
}

func parseClock(value string) (time.Time, bool) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil && parsed.Format(layout) == value {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseEffectiveDate(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, jakartaTime)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, fmt.Errorf("invalid effective date")
	}
	return parsed, nil
}

func jakartaDate(value time.Time) time.Time {
	local := value.In(jakartaTime)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, jakartaTime)
}

func localDate(value time.Time) time.Time {
	return jakartaDate(value)
}

func atLocalTime(date time.Time, clock time.Time) time.Time {
	localDate := date.In(jakartaTime)
	return time.Date(localDate.Year(), localDate.Month(), localDate.Day(),
		clock.Hour(), clock.Minute(), clock.Second(), 0, jakartaTime)
}

func isWorkingDay(date time.Time, workingDays []int16) bool {
	weekday := int16(date.In(jakartaTime).Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for _, day := range workingDays {
		if day == weekday {
			return true
		}
	}
	return false
}

func nullableTimestamp(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format(time.RFC3339)
}

func nullableTime(value pgtype.Timestamptz) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
