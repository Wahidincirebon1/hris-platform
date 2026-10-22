package httpapi

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	maxProfilePhotoUploadBytes = 5 << 20
	maxProfilePhotoPixels      = 25_000_000
	profilePhotoSize           = 512
	maxProfilePhotoBytes       = 512 << 10
)

type employeeProfile struct {
	UserID           uuid.UUID  `json:"user_id"`
	EmployeeNumber   string     `json:"employee_number"`
	FullName         string     `json:"full_name"`
	Email            string     `json:"email"`
	NIK              *string    `json:"nik,omitempty"`
	Phone            *string    `json:"phone,omitempty"`
	Address          *string    `json:"address,omitempty"`
	BranchID         uuid.UUID  `json:"branch_id"`
	BranchName       string     `json:"branch_name"`
	Department       *string    `json:"department,omitempty"`
	PositionTitle    *string    `json:"position_title,omitempty"`
	JobGrade         *string    `json:"job_grade,omitempty"`
	Role             string     `json:"role"`
	EmploymentStatus string     `json:"employment_status"`
	HiredOn          *string    `json:"hired_on,omitempty"`
	ManagerName      *string    `json:"manager_name,omitempty"`
	ManagerEmail     *string    `json:"manager_email,omitempty"`
	ProfilePhotoURL  *string    `json:"profile_photo_url,omitempty"`
	PhotoUpdatedAt   *time.Time `json:"photo_updated_at,omitempty"`
	ProfileUpdatedAt time.Time  `json:"profile_updated_at"`
}

func (a *API) registerProfileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/employees/me/profile", a.authenticated(a.myEmployeeProfile))
	mux.HandleFunc("GET /api/v1/employees/{id}/profile", a.authenticated(a.getEmployeeProfile))
	mux.HandleFunc("GET /api/v1/employees/me/photo", a.authenticated(a.myEmployeePhoto))
	mux.HandleFunc("POST /api/v1/employees/me/photo", a.authenticated(a.saveMyEmployeePhoto))
	mux.HandleFunc("GET /api/v1/employees/{id}/photo", a.authenticated(a.getEmployeePhoto))
	mux.HandleFunc("POST /api/v1/employees/{id}/photo", a.authenticated(a.saveEmployeePhoto))
}

func (a *API) myEmployeeProfile(w http.ResponseWriter, r *http.Request) {
	writeEmployeeProfile(w, r, a, currentPrincipal(r).ID)
}

func (a *API) getEmployeeProfile(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID karyawan tidak valid.")
		return
	}
	writeEmployeeProfile(w, r, a, employeeID)
}

func writeEmployeeProfile(w http.ResponseWriter, r *http.Request, a *API, employeeID uuid.UUID) {
	actor := currentPrincipal(r)
	var profile employeeProfile
	var photoExists bool
	err := a.db.QueryRow(r.Context(), `
		SELECT u.id, e.employee_number, e.full_name, u.email, e.nik, e.phone, e.address,
		       u.branch_id, b.name, d.name, e.position_title, e.job_grade, u.role,
		       e.employment_status, e.hired_on::text, manager.full_name, manager.email,
		       e.profile_photo_jpeg IS NOT NULL, e.profile_photo_updated_at, e.updated_at
		FROM users u
		JOIN employees e ON e.user_id = u.id
		JOIN branches b ON b.id = u.branch_id
		LEFT JOIN departments d ON d.id = e.department_id
		LEFT JOIN users manager ON manager.id = u.manager_id
		WHERE u.id = $1
		  AND (
		      $2::boolean
		      OR u.id = $3
		      OR (u.manager_id = $3 AND u.branch_id = $4)
		  )
	`, employeeID, isHR(actor.Role), actor.ID, actor.BranchID).Scan(
		&profile.UserID, &profile.EmployeeNumber, &profile.FullName, &profile.Email,
		&profile.NIK, &profile.Phone, &profile.Address, &profile.BranchID, &profile.BranchName,
		&profile.Department, &profile.PositionTitle, &profile.JobGrade, &profile.Role,
		&profile.EmploymentStatus, &profile.HiredOn, &profile.ManagerName, &profile.ManagerEmail,
		&photoExists, &profile.PhotoUpdatedAt, &profile.ProfileUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Profil karyawan tidak ditemukan atau Anda tidak memiliki akses.")
			return
		}
		log.Printf("load employee profile failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Profil karyawan tidak dapat dimuat.")
		return
	}
	if !isHR(actor.Role) && profile.UserID != actor.ID {
		profile.NIK = nil
		profile.Phone = nil
		profile.Address = nil
		profile.ManagerEmail = nil
	}
	if photoExists {
		photoPath := "/api/v1/employees/" + profile.UserID.String() + "/photo"
		profile.ProfilePhotoURL = &photoPath
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *API) myEmployeePhoto(w http.ResponseWriter, r *http.Request) {
	writeEmployeePhoto(w, r, a, currentPrincipal(r).ID)
}

func (a *API) getEmployeePhoto(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID karyawan tidak valid.")
		return
	}
	writeEmployeePhoto(w, r, a, employeeID)
}

func (a *API) saveMyEmployeePhoto(w http.ResponseWriter, r *http.Request) {
	saveEmployeePhoto(w, r, a, currentPrincipal(r).ID)
}

func (a *API) saveEmployeePhoto(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID karyawan tidak valid.")
		return
	}
	saveEmployeePhoto(w, r, a, employeeID)
}

func writeEmployeePhoto(w http.ResponseWriter, r *http.Request, a *API, employeeID uuid.UUID) {
	actor := currentPrincipal(r)
	var photo []byte
	err := a.db.QueryRow(r.Context(), `
		SELECT e.profile_photo_jpeg
		FROM employees e
		JOIN users u ON u.id = e.user_id
		WHERE e.user_id = $1
		  AND e.profile_photo_jpeg IS NOT NULL
		  AND ($2::boolean OR u.id = $3 OR (u.manager_id = $3 AND u.branch_id = $4))
	`, employeeID, isHR(actor.Role), actor.ID, actor.BranchID).Scan(&photo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Foto profil tidak ditemukan atau Anda tidak memiliki akses.")
			return
		}
		log.Printf("load employee profile photo failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat dimuat.")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(photo)))
	w.Header().Set("Content-Disposition", `inline; filename="profile.jpg"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(photo); err != nil {
		log.Printf("write employee profile photo failed: %v", err)
	}
}

func saveEmployeePhoto(w http.ResponseWriter, r *http.Request, a *API, employeeID uuid.UUID) {
	actor := currentPrincipal(r)
	if !isHR(actor.Role) && actor.ID != employeeID {
		writeError(w, http.StatusForbidden, "Anda hanya dapat mengubah foto profil sendiri.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Metode tidak diizinkan.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfilePhotoUploadBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "Ukuran foto sumber maksimal 5 MB.")
		} else {
			writeError(w, http.StatusBadRequest, "Upload harus menggunakan formulir multipart yang valid.")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Pilih foto JPG atau PNG terlebih dahulu.")
		return
	}
	defer file.Close()
	if header.Size > maxProfilePhotoUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Ukuran foto sumber maksimal 5 MB.")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxProfilePhotoUploadBytes+1))
	if err != nil {
		log.Printf("read profile photo upload failed: %v", err)
		writeError(w, http.StatusBadRequest, "Foto profil tidak dapat dibaca.")
		return
	}
	if len(raw) == 0 || len(raw) > maxProfilePhotoUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Ukuran foto sumber harus antara 1 byte dan 5 MB.")
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") {
		writeError(w, http.StatusBadRequest, "Format foto tidak didukung. Gunakan JPG atau PNG.")
		return
	}
	if config.Width < 1 || config.Height < 1 ||
		config.Width > 10000 || config.Height > 10000 ||
		int64(config.Width)*int64(config.Height) > maxProfilePhotoPixels {
		writeError(w, http.StatusBadRequest, "Dimensi foto terlalu besar; batasnya 10.000 piksel per sisi dan 25 megapiksel.")
		return
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != format {
		writeError(w, http.StatusBadRequest, "Isi file bukan gambar JPG atau PNG yang valid.")
		return
	}
	normalized := cropAndResizePhoto(decoded, profilePhotoSize)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, normalized, &jpeg.Options{Quality: 84}); err != nil {
		log.Printf("encode normalized profile photo failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat diproses.")
		return
	}
	if encoded.Len() > maxProfilePhotoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Foto hasil pemrosesan melebihi batas 512 KB.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat disimpan.")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		UPDATE employees
		SET profile_photo_jpeg = $1, profile_photo_updated_at = now(), updated_at = now()
		WHERE user_id = $2
		  AND EXISTS (SELECT 1 FROM users WHERE id = $2 AND active)
	`, encoded.Bytes(), employeeID)
	if err != nil {
		log.Printf("save employee profile photo failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat disimpan.")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "Akun karyawan tidak ditemukan atau tidak aktif.")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, "UPDATE_PROFILE_PHOTO", "employee", employeeID.String(), map[string]string{
		"format": "jpeg", "width": strconv.Itoa(profilePhotoSize), "height": strconv.Itoa(profilePhotoSize),
	}); err != nil {
		log.Printf("audit profile photo update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat disimpan.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit profile photo audit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Foto profil tidak dapat disimpan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "photo_updated", "width": profilePhotoSize, "height": profilePhotoSize,
		"content_type": "image/jpeg", "size_bytes": encoded.Len(),
	})
}

func cropAndResizePhoto(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	cropSize := min(bounds.Dx(), bounds.Dy())
	left := bounds.Min.X + (bounds.Dx()-cropSize)/2
	top := bounds.Min.Y + (bounds.Dy()-cropSize)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		sourceY := top + min(cropSize-1, y*cropSize/size)
		for x := 0; x < size; x++ {
			sourceX := left + min(cropSize-1, x*cropSize/size)
			r, g, b, alpha := src.At(sourceX, sourceY).RGBA()
			inverseAlpha := uint32(65535) - alpha
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(min(uint32(65535), r+inverseAlpha) >> 8),
				G: uint8(min(uint32(65535), g+inverseAlpha) >> 8),
				B: uint8(min(uint32(65535), b+inverseAlpha) >> 8),
				A: 255,
			})
		}
	}
	return dst
}
