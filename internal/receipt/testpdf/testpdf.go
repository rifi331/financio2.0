// Package testpdf builds minimal valid single-page PDFs for tests (the bot's
// PDF flow tests need real files for ledongthuc/pdf to extract text from).
package testpdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Build returns a one-page PDF whose text content is exactly the given text.
func Build(text string) []byte {
	esc := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", esc)

	var buf bytes.Buffer
	offsets := make([]int, 6) // object ids 1..5
	buf.WriteString("%PDF-1.4\n")

	write := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	write(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	write(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}
