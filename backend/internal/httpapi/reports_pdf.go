package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	pdfPageWidth  = 612
	pdfPageHeight = 792
	pdfMargin     = 44
	pdfLineHeight = 11
)

type reportPDFPage struct {
	commands []string
	y        int
}

func writePDFReport(w http.ResponseWriter, filename, title string, headers []string, rows [][]string) error {
	if len(headers) == 0 {
		return fmt.Errorf("PDF report requires at least one column")
	}
	pages := []reportPDFPage{newReportPDFPage(title, len(rows))}
	for rowIndex, row := range rows {
		lines := make([]string, 0, len(headers))
		for columnIndex, header := range headers {
			value := ""
			if columnIndex < len(row) {
				value = row[columnIndex]
			}
			lines = append(lines, wrapReportPDFText(fmt.Sprintf("%s: %s", humanizeReportHeader(header), value), 104)...)
		}
		page := &pages[len(pages)-1]
		if page.y-pdfLineHeight < pdfMargin+24 {
			pages = append(pages, newReportPDFPage(title, len(rows)))
			page = &pages[len(pages)-1]
		}
		pdfText(page, pdfMargin, page.y, 8, fmt.Sprintf("%d. %s", rowIndex+1, humanizeReportHeader(headers[0])))
		page.y -= pdfLineHeight
		for _, line := range lines {
			if page.y-pdfLineHeight < pdfMargin+24 {
				pages = append(pages, newReportPDFPage(title, len(rows)))
				page = &pages[len(pages)-1]
			}
			pdfText(page, pdfMargin+12, page.y, 8, line)
			page.y -= pdfLineHeight
		}
		if page.y-8 >= pdfMargin+24 {
			page.commands = append(page.commands, fmt.Sprintf("0.82 w %d %d m %d %d l S", pdfMargin, page.y+5, pdfPageWidth-pdfMargin, page.y+5))
			page.y -= 8
		}
	}

	var document bytes.Buffer
	document.WriteString("%PDF-1.4\n")
	objects := make([]string, 0, 3+len(pages)*2)
	objects = append(objects, "") // Catalog
	objects = append(objects, "") // Pages
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	pageIDs := make([]int, 0, len(pages))
	for index, page := range pages {
		pageID := 4 + index*2
		contentID := pageID + 1
		pageIDs = append(pageIDs, pageID)
		pdfText(&page, pdfMargin, pdfMargin-6, 8, fmt.Sprintf("HRIS Kantor  ·  Dibuat %s  ·  Halaman %d dari %d",
			time.Now().In(jakartaTime).Format("02 Jan 2006 15:04 WIB"), index+1, len(pages)))
		stream := strings.Join(page.commands, "\n") + "\n"
		objects = append(objects, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			pdfPageWidth, pdfPageHeight, contentID,
		))
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	kids := make([]string, 0, len(pageIDs))
	for _, id := range pageIDs {
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
	}
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageIDs))

	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = document.Len()
		document.WriteString(strconv.Itoa(index + 1))
		document.WriteString(" 0 obj\n")
		document.WriteString(object)
		document.WriteString("\nendobj\n")
	}
	xrefOffset := document.Len()
	document.WriteString(fmt.Sprintf("xref\n0 %d\n", len(offsets)))
	document.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		document.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	document.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset))

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(document.Len()))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(document.Bytes()); err != nil {
		return fmt.Errorf("write PDF report: %w", err)
	}
	return nil
}

func newReportPDFPage(title string, rowCount int) reportPDFPage {
	page := reportPDFPage{y: pdfPageHeight - pdfMargin}
	pdfText(&page, pdfMargin, page.y, 16, title)
	page.y -= 22
	pdfText(&page, pdfMargin, page.y, 9, fmt.Sprintf("Ringkasan laporan · %d baris · Zona waktu Asia/Jakarta (WIB)", rowCount))
	page.y -= 17
	page.commands = append(page.commands, fmt.Sprintf("0.5 w %d %d m %d %d l S",
		pdfMargin, page.y+5, pdfPageWidth-pdfMargin, page.y+5))
	page.y -= 8
	return page
}

func pdfText(page *reportPDFPage, x, y, size int, text string) {
	page.commands = append(page.commands, fmt.Sprintf("BT /F1 %d Tf %d %d Td (%s) Tj ET", size, x, y, escapePDFText(text)))
}

func escapePDFText(value string) string {
	value = strings.ToValidUTF8(value, "?")
	var escaped strings.Builder
	for _, char := range value {
		switch {
		case char == '\\' || char == '(' || char == ')':
			escaped.WriteByte('\\')
			escaped.WriteRune(char)
		case char == '\r' || char == '\n' || char == '\t':
			escaped.WriteByte(' ')
		case char >= 32 && char <= 126:
			escaped.WriteByte(byte(char))
		default:
			escaped.WriteByte('?')
		}
	}
	return escaped.String()
}

func wrapReportPDFText(value string, maxRunes int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{""}
	}
	words := strings.Fields(value)
	lines := make([]string, 0, 1)
	current := ""
	for _, word := range words {
		if utf8.RuneCountInString(word) > maxRunes {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			runes := []rune(word)
			for len(runes) > maxRunes {
				lines = append(lines, string(runes[:maxRunes]))
				runes = runes[maxRunes:]
			}
			current = string(runes)
			continue
		}
		if utf8.RuneCountInString(current)+utf8.RuneCountInString(word)+1 <= maxRunes {
			if current != "" {
				current += " "
			}
			current += word
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func humanizeReportHeader(header string) string {
	header = strings.ReplaceAll(header, "_", " ")
	if header == "" {
		return "Kolom"
	}
	return strings.ToUpper(header[:1]) + header[1:]
}
