package agent

import "testing"

func TestParsePlanJSON(t *testing.T) {
	plan, err := parsePlan(`{"summary":"demo","urls":["https://example.com/a.png","http://bad","ftp://no","https://example.com/a.png","https://example.com/b.pdf"]}`)
	if err != nil {
		t.Fatal(err)
	}
	got := sanitizeURLs(plan.URLs, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 urls, got %v", got)
	}
}

func TestParsePlanFallback(t *testing.T) {
	plan, err := parsePlan(`Here are links: https://example.com/file.zip and https://cdn.example.com/x.png`)
	if err != nil {
		t.Fatal(err)
	}
	got := sanitizeURLs(plan.URLs, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 urls, got %v", got)
	}
}
