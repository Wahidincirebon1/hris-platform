package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWritePDFReportCreatesDownloadableDocument(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/reports/employees.pdf", nil)
	response := httptest.NewRecorder()
	err := writeReport(response, request, "karyawan.csv", "Daftar Karyawan", []string{
		"nomor_karyawan", "nama", "grade",
	}, [][]string{{"DEMO-001", "Ayu (Sintetis)", "G5"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", got)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="karyawan.pdf"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	body := response.Body.String()
	for _, expected := range []string{"%PDF-1.4", "xref", "%%EOF", "Daftar Karyawan", "DEMO-001", "Ayu \\(Sintetis\\)", "G5"} {
		if !strings.Contains(body, expected) {
			t.Errorf("PDF body does not contain %q", expected)
		}
	}
}

func TestWrapReportPDFTextSplitsLongValues(t *testing.T) {
	value := strings.Repeat("x", 220)
	lines := wrapReportPDFText(value, 80)
	if len(lines) != 3 {
		t.Fatalf("wrapped into %d lines, want 3", len(lines))
	}
	for _, line := range lines {
		if len([]rune(line)) > 80 {
			t.Errorf("line length = %d, want at most 80", len([]rune(line)))
		}
	}
}

func TestWritePDFReportPaginatesLongRows(t *testing.T) {
	response := httptest.NewRecorder()
	err := writePDFReport(response, "audit.pdf", "Audit", []string{"detail"}, [][]string{
		{strings.Repeat("detail ", 1000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Body.String(), "/Count 2") {
		t.Fatal("long report row was not paginated across pages")
	}
	if !strings.Contains(response.Body.String(), "Halaman 2 dari 2") {
		t.Fatal("second page footer is missing")
	}
}

func TestWritePDFReportRejectsMissingColumns(t *testing.T) {
	response := httptest.NewRecorder()
	if err := writePDFReport(response, "empty.pdf", "Laporan", nil, nil); err == nil {
		t.Fatal("expected missing-column error")
	}
}

func TestWriteReportPreservesCSVDownloadAndFormulaProtection(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/reports/employees.csv", nil)
	response := httptest.NewRecorder()
	err := writeReport(response, request, "karyawan.csv", "Daftar karyawan", []string{"nama"}, [][]string{{"=1+1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="karyawan.csv"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := response.Body.String(); !strings.Contains(got, "'=1+1") {
		t.Fatalf("CSV formula prefix was not escaped: %q", got)
	}
}
