package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (a *API) registerReportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/reports/employees.csv", a.authenticated(a.exportEmployees))
	mux.HandleFunc("GET /api/v1/reports/employees.pdf", a.authenticated(a.exportEmployees))
	mux.HandleFunc("GET /api/v1/reports/attendance.csv", a.authenticated(a.exportAttendance))
	mux.HandleFunc("GET /api/v1/reports/attendance.pdf", a.authenticated(a.exportAttendance))
	mux.HandleFunc("GET /api/v1/reports/leave.csv", a.authenticated(a.exportLeaveLedger))
	mux.HandleFunc("GET /api/v1/reports/leave.pdf", a.authenticated(a.exportLeaveLedger))
	mux.HandleFunc("GET /api/v1/reports/kpi.csv", a.authenticated(a.exportKPI))
	mux.HandleFunc("GET /api/v1/reports/kpi.pdf", a.authenticated(a.exportKPI))
	mux.HandleFunc("GET /api/v1/reports/audit.csv", a.authenticated(a.exportCorrections))
	mux.HandleFunc("GET /api/v1/reports/audit.pdf", a.authenticated(a.exportCorrections))
}

func (a *API) exportEmployees(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Ekspor daftar karyawan hanya tersedia untuk HR.")
		return
	}
	var branchID *uuid.UUID
	if raw := r.URL.Query().Get("branch_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ID cabang tidak valid.")
			return
		}
		branchID = &parsed
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT e.employee_number, e.full_name, u.role, b.name,
			COALESCE(d.name, ''), COALESCE(e.position_title, ''), COALESCE(e.job_grade, ''),
			CASE WHEN u.active AND e.employment_status = 'active' THEN 'Aktif' ELSE 'Nonaktif' END
		FROM users u
		JOIN employees e ON e.user_id = u.id
		JOIN branches b ON b.id = u.branch_id
		LEFT JOIN departments d ON d.id = e.department_id
		WHERE ($1::uuid IS NULL OR u.branch_id = $1)
			AND e.employment_status <> 'archived'
		ORDER BY b.name, e.full_name
	`, branchID)
	if err != nil {
		log.Printf("load employee report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan karyawan tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	data, err := collectCSVRows(rows, 8)
	if err != nil {
		log.Printf("read employee report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan karyawan tidak dapat dibaca.")
		return
	}
	if err := writeReport(w, r, "karyawan.csv", "Daftar karyawan", []string{
		"nomor_karyawan", "nama", "role", "cabang", "departemen", "jabatan", "grade", "status",
	}, data); err != nil {
		log.Printf("write employee report failed: %v", err)
	}
}

func (a *API) exportAttendance(w http.ResponseWriter, r *http.Request) {
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
		branchID, managerID = &user.BranchID, &user.ID
		if raw := r.URL.Query().Get("branch_id"); raw != "" && raw != user.BranchID.String() {
			writeError(w, http.StatusForbidden, "Manager hanya dapat mengekspor cabangnya sendiri.")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "Ekspor absensi hanya tersedia untuk HR dan manager.")
		return
	}
	from, to, ok := attendanceDateRange(w, r)
	if !ok {
		return
	}
	records, err := queryAttendance(r.Context(), a.db, from, to, nil, branchID, managerID)
	if err != nil {
		log.Printf("load attendance export failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan absensi tidak dapat dimuat.")
		return
	}
	data := make([][]string, 0, len(records))
	for _, item := range records {
		checkOut := ""
		if item.CheckOutAt != nil {
			checkOut = item.CheckOutAt.In(jakartaTime).Format("2006-01-02 15:04:05")
		}
		data = append(data, []string{
			from.Format("2006-01-02"), to.Format("2006-01-02"),
			item.EmployeeNumber, item.FullName, item.BranchName, item.Date,
			item.CheckInAt.In(jakartaTime).Format("2006-01-02 15:04:05"), checkOut,
			strconv.FormatBool(item.Late), strconv.Itoa(item.LateMinutes), "Asia/Jakarta",
		})
	}
	if err := writeReport(w, r, "absensi.csv", "Laporan absensi dan keterlambatan", []string{
		"periode_mulai", "periode_selesai", "nomor_karyawan", "nama", "cabang",
		"tanggal", "masuk_wib", "pulang_wib", "terlambat", "menit_terlambat", "zona_waktu",
	}, data); err != nil {
		log.Printf("write attendance export failed: %v", err)
	}
}

func (a *API) exportLeaveLedger(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Ekspor saldo dan transaksi cuti hanya tersedia untuk HR.")
		return
	}
	year, ok := requestedLeaveYear(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT e.employee_number, e.full_name, b.name, l.leave_year,
			l.entry_type, l.amount::text, COALESCE(l.entry_date::text, ''),
			l.reason, l.created_at
		FROM leave_ledger_entries l
		JOIN employees e ON e.user_id = l.user_id
		JOIN users u ON u.id = l.user_id
		JOIN branches b ON b.id = u.branch_id
		WHERE l.leave_year = $1
		ORDER BY b.name, e.full_name, l.created_at, l.id
	`, year)
	if err != nil {
		log.Printf("load leave ledger report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan transaksi cuti tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	rawRows, err := scanLeaveLedgerRows(rows)
	if err != nil {
		log.Printf("read leave ledger report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan transaksi cuti tidak dapat dibaca.")
		return
	}
	if err := writeReport(w, r, fmt.Sprintf("transaksi-cuti-%d.csv", year), fmt.Sprintf("Transaksi cuti tahun %d", year), []string{
		"nomor_karyawan", "nama", "cabang", "tahun", "jenis_transaksi",
		"jumlah_hari", "tanggal", "keterangan", "dibuat_wib", "zona_waktu",
	}, rawRows); err != nil {
		log.Printf("write leave ledger report failed: %v", err)
	}
}

func (a *API) exportKPI(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	year, ok := requestedKPIYear(w, r)
	if !ok {
		return
	}
	where, args, permitted := kpiReportScope(user, year, r.URL.Query().Get("branch_id"))
	if !permitted {
		writeError(w, http.StatusForbidden, "Ekspor KPI hanya tersedia untuk HR dan manager sesuai cakupannya.")
		return
	}
	sets, err := loadKPISetViews(r.Context(), a.db, where, args...)
	if err != nil {
		log.Printf("load KPI export failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan KPI tidak dapat dimuat.")
		return
	}
	data := make([][]string, 0)
	for _, item := range sets {
		for _, goal := range item.Goals {
			score, finalScore := "", ""
			if goal.ManagerScore != nil {
				score = strconv.FormatFloat(*goal.ManagerScore, 'f', 2, 64)
			}
			if item.FinalScore != nil {
				finalScore = strconv.FormatFloat(*item.FinalScore, 'f', 2, 64)
			}
			progress := ""
			if len(goal.ProgressUpdates) > 0 {
				latest := goal.ProgressUpdates[0]
				progress = latest.CurrentValue
				if latest.ProgressPercent != nil {
					progress += " (" + strconv.FormatFloat(*latest.ProgressPercent, 'f', 2, 64) + "%)"
				}
			}
			data = append(data, []string{
				strconv.Itoa(item.Year), item.EmployeeNumber, item.EmployeeName, item.BranchName,
				item.ManagerName, item.Status, goal.Title, goal.Description, goal.TargetValue,
				strconv.FormatFloat(goal.Weight, 'f', 2, 64), progress, score, finalScore, "Asia/Jakarta",
			})
		}
	}
	if err := writeReport(w, r, fmt.Sprintf("kpi-%d.csv", year), fmt.Sprintf("KPI dan tinjauan kinerja tahun %d", year), []string{
		"tahun", "nomor_karyawan", "nama", "cabang", "manager", "status",
		"indikator", "deskripsi", "target", "bobot_persen", "progres_terakhir",
		"nilai_manager_0_100", "nilai_akhir_0_100", "zona_waktu",
	}, data); err != nil {
		log.Printf("write KPI export failed: %v", err)
	}
}

func (a *API) exportCorrections(w http.ResponseWriter, r *http.Request) {
	user := currentPrincipal(r)
	if !isHR(user.Role) {
		writeError(w, http.StatusForbidden, "Ekspor audit koreksi hanya tersedia untuk HR.")
		return
	}
	from, to, ok := attendanceDateRange(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT a.action, a.entity_id, COALESCE(actor.full_name, 'Akun tidak aktif'),
			a.details::text, a.created_at
		FROM audit_logs a
		LEFT JOIN employees actor ON actor.user_id = a.actor_user_id
		WHERE a.action = 'CORRECT_ATTENDANCE'
			AND a.created_at >= $1::date
			AND a.created_at < ($2::date + interval '1 day')
		ORDER BY a.created_at DESC
		LIMIT 5000
	`, from, to)
	if err != nil {
		log.Printf("load correction audit report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan koreksi absensi tidak dapat dimuat.")
		return
	}
	defer rows.Close()
	data, err := scanAuditRows(rows)
	if err != nil {
		log.Printf("read correction audit report failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Laporan koreksi absensi tidak dapat dibaca.")
		return
	}
	if err := writeReport(w, r, "koreksi-absensi.csv", "Audit koreksi absensi", []string{
		"aksi", "id_absensi", "dilakukan_oleh", "detail_perubahan", "waktu_wib", "zona_waktu",
	}, data); err != nil {
		log.Printf("write correction audit report failed: %v", err)
	}
}

func writeReport(w http.ResponseWriter, r *http.Request, filename, title string, headers []string, rows [][]string) error {
	if path.Ext(r.URL.Path) == ".pdf" {
		pdfName := strings.TrimSuffix(filename, ".csv") + ".pdf"
		return writePDFReport(w, pdfName, title, headers, rows)
	}
	return writeCSV(w, filename, headers, rows)
}

func collectCSVRows(rows pgx.Rows, columns int) ([][]string, error) {
	result := make([][]string, 0)
	for rows.Next() {
		values := make([]any, columns)
		pointers := make([]any, columns)
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make([]string, columns)
		for i, value := range values {
			if value != nil {
				row[i] = fmt.Sprint(value)
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func scanLeaveLedgerRows(rows pgx.Rows) ([][]string, error) {
	result := make([][]string, 0)
	for rows.Next() {
		var number, name, branch, entryType, amount, entryDate, reason string
		var year int
		var createdAt time.Time
		if err := rows.Scan(&number, &name, &branch, &year, &entryType, &amount, &entryDate, &reason, &createdAt); err != nil {
			return nil, err
		}
		result = append(result, []string{
			number, name, branch, strconv.Itoa(year), entryType, amount, entryDate, reason,
			createdAt.In(jakartaTime).Format("2006-01-02 15:04:05"), "Asia/Jakarta",
		})
	}
	return result, rows.Err()
}

func scanAuditRows(rows pgx.Rows) ([][]string, error) {
	result := make([][]string, 0)
	for rows.Next() {
		var action, entityID, actor, details string
		var createdAt time.Time
		if err := rows.Scan(&action, &entityID, &actor, &details, &createdAt); err != nil {
			return nil, err
		}
		result = append(result, []string{
			action, entityID, actor, details, createdAt.In(jakartaTime).Format("2006-01-02 15:04:05"), "Asia/Jakarta",
		})
	}
	return result, rows.Err()
}
