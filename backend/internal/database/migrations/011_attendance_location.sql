-- Pencatatan lokasi absensi (peta) beserta status kepatuhan area kantor.
-- Koordinat diperlukan agar HR dapat menelusuri absensi yang dilakukan di luar kantor.
ALTER TABLE attendance_records
    ADD COLUMN check_in_latitude DOUBLE PRECISION,
    ADD COLUMN check_in_longitude DOUBLE PRECISION,
    ADD COLUMN check_in_outside_geofence BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN check_out_latitude DOUBLE PRECISION,
    ADD COLUMN check_out_longitude DOUBLE PRECISION,
    ADD COLUMN check_out_outside_geofence BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE attendance_records
    ADD CONSTRAINT attendance_records_check_in_coords_check
        CHECK ((check_in_latitude IS NULL AND check_in_longitude IS NULL)
            OR (check_in_latitude BETWEEN -90 AND 90 AND check_in_longitude BETWEEN -180 AND 180)),
    ADD CONSTRAINT attendance_records_check_out_coords_check
        CHECK ((check_out_latitude IS NULL AND check_out_longitude IS NULL)
            OR (check_out_latitude BETWEEN -90 AND 90 AND check_out_longitude BETWEEN -180 AND 180));

CREATE INDEX idx_attendance_records_outside ON attendance_records (branch_id, attendance_date DESC)
    WHERE check_in_outside_geofence OR check_out_outside_geofence;
