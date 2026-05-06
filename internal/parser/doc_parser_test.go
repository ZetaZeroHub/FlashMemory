package parser

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocParserMarkdown(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "guide.md")
	content := "# Intro\nintro text\n## Setup\nstep1\nstep2\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write markdown failed: %v", err)
	}

	p := &DocParser{Lang: "markdown"}
	funcs, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(funcs) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(funcs))
	}
	if funcs[0].FunctionType != "llm_parser" {
		t.Fatalf("unexpected function type: %s", funcs[0].FunctionType)
	}
	if funcs[0].Name == "" || funcs[0].Description == "" {
		t.Fatalf("expected non-empty name and description: %+v", funcs[0])
	}
	if funcs[0].Source == "" {
		t.Fatalf("expected source provenance for markdown section")
	}
}

func TestDocParserText(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "notes.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write text failed: %v", err)
	}

	p := &DocParser{Lang: "text"}
	funcs, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatalf("expected at least one section")
	}
}

func TestDetectLangDocuments(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"guide.md", "markdown"},
		{"guide.markdown", "markdown"},
		{"notes.txt", "text"},
		{"notes.rst", "text"},
		{"manual.pdf", "pdf"},
		{"deck.pptx", "pptx"},
		{"report.docx", "docx"},
		{"screen.png", "image"},
		{"photo.jpg", "image"},
		{"scan.tiff", "image"},
	}

	for _, tc := range cases {
		got := DetectLang(tc.path)
		if got != tc.want {
			t.Fatalf("DetectLang(%s)=%s want %s", tc.path, got, tc.want)
		}
	}
}

func TestDocParserPPTX(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "deck.pptx")

	err := writeZip(path, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:t>Slide Title</a:t><a:t>Slide Body</a:t></p:sld>`,
	})
	if err != nil {
		t.Fatalf("write pptx failed: %v", err)
	}

	p := &DocParser{Lang: "pptx"}
	funcs, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatalf("expected parsed sections from pptx")
	}
	if !strings.Contains(strings.ToLower(funcs[0].Name), "slide_title") && funcs[0].Description == "" {
		t.Fatalf("expected first section to carry title semantics")
	}
	if funcs[0].Description == "" {
		t.Fatalf("expected non-empty description")
	}
	if funcs[0].Slide == 0 {
		t.Fatalf("expected slide provenance, got slide=%d", funcs[0].Slide)
	}
	if funcs[0].Source == "" {
		t.Fatalf("expected source provenance for pptx section")
	}
}

func TestDocParserDOCX(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "report.docx")

	err := writeZip(path, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>Docx Title</w:t></w:r></w:p><w:p><w:r><w:t>Docx Body</w:t></w:r></w:p></w:document>`,
	})
	if err != nil {
		t.Fatalf("write docx failed: %v", err)
	}

	p := &DocParser{Lang: "docx"}
	funcs, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatalf("expected parsed sections from docx")
	}
	if funcs[0].Source == "" {
		t.Fatalf("expected source provenance for docx section")
	}
}

func TestDocParserPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skipf("pdftotext not available: %v", err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "manual.pdf")
	content := `%PDF-1.1
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT
/F1 18 Tf
50 80 Td
(Hello PDF) Tj
ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f 
0000000010 00000 n 
0000000060 00000 n 
0000000117 00000 n 
0000000243 00000 n 
0000000338 00000 n 
trailer
<< /Root 1 0 R /Size 6 >>
startxref
408
%%EOF
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write pdf failed: %v", err)
	}

	p := &DocParser{Lang: "pdf"}
	funcs, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatalf("expected parsed sections from pdf")
	}
	if funcs[0].Page <= 0 {
		t.Fatalf("expected page provenance, got page=%d", funcs[0].Page)
	}
	if funcs[0].Source == "" {
		t.Fatalf("expected source provenance for pdf section")
	}
	if funcs[0].Description == "" {
		t.Fatalf("expected non-empty description")
	}
}

// Internal splitter tests moved to internal/parser/docs/{pdf,pptx}_test.go
// after the structure-aware splitters landed in 20260502-doc-parser-overhaul.

func writeZip(path string, entries map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range entries {
		item, err := w.Create(name)
		if err != nil {
			_ = w.Close()
			return err
		}
		if _, err := item.Write([]byte(content)); err != nil {
			_ = w.Close()
			return err
		}
	}
	return w.Close()
}
