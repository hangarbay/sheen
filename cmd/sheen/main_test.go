package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFromArgFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "page.html")
	if err := os.WriteFile(f, []byte("<p>hi</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := sourceFromArg(f)
	if err != nil {
		t.Fatal(err)
	}
	defer src.reader.Close()
	if src.isRemote {
		t.Error("file source should not be remote")
	}
	if src.URL == nil || !strings.HasSuffix(src.URL.Path, "/") {
		t.Errorf("expected file base URL ending in /, got %v", src.URL)
	}
}

func TestSourceFromArgDirWithIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>i</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.html"), []byte("<p>o</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := sourceFromArg(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.reader.Close()
	b, _ := io.ReadAll(src.reader)
	if string(b) != "<p>i</p>" {
		t.Errorf("expected index.html contents, got %q", b)
	}
}

func TestSourceFromArgDirWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only.html"), []byte("<p>o</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := sourceFromArg(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.reader.Close()
	b, _ := io.ReadAll(src.reader)
	if string(b) != "<p>o</p>" {
		t.Errorf("expected fallback to first .html file, got %q", b)
	}
}

func TestSourceFromArgDirEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := sourceFromArg(dir); err == nil {
		t.Error("expected error for dir without HTML files")
	}
}

func TestSourceFromArgMissingFile(t *testing.T) {
	if _, err := sourceFromArg("/nonexistent/nope.html"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFindIndexHTMLCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "INDEX.HTM"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := findIndexHTML(dir)
	if filepath.Base(got) != "INDEX.HTM" {
		t.Errorf("got %q", got)
	}
}

func TestResolveWidthStyleLinksDefaults(t *testing.T) {
	// test process has no TTY on stdout
	widthFlag = 0
	styleFlag = "auto"
	linksFlag = "auto"
	if w := resolveWidth(); w != 80 {
		t.Errorf("default width = %d, want 80", w)
	}
	if s := resolveStyle(); s != "notty" {
		t.Errorf("non-tty style = %q, want notty", s)
	}
	if l := resolveLinks(); l != "inline" {
		t.Errorf("non-tty links = %q, want inline", l)
	}
}

func TestResolveStyleExplicit(t *testing.T) {
	for _, s := range []string{"dark", "light", "notty", "ascii"} {
		styleFlag = s
		if got := resolveStyle(); got != s {
			t.Errorf("styleFlag=%s got %s", s, got)
		}
	}
}

func TestStylesheetFetcherBlocksRemoteLocalFiles(t *testing.T) {
	src := &source{isRemote: true}
	fetch := stylesheetFetcher(src)
	if _, err := fetch("file:///etc/passwd"); err == nil {
		t.Error("remote page must not load file:// stylesheets")
	}
	if _, err := fetch("ftp://x/y.css"); err == nil {
		t.Error("unsupported scheme should error")
	}
}

func TestLocalFileBeatsDomainLookingName(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "weird.hostname")
	if err := os.WriteFile(f, []byte("<p>local</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := sourceFromArg(f)
	if err != nil {
		t.Fatal(err)
	}
	defer src.reader.Close()
	if src.isRemote {
		t.Error("existing local file must never be treated as a URL")
	}
}

func TestDomainDetection(t *testing.T) {
	cases := map[string]bool{
		"example.com":         true,
		"example.com/x.html":  true,
		"sub.domain.io:8080/": true,
		"not a domain":        false,
		"justwords":           false,
	}
	for in, want := range cases {
		if got := domainRe.MatchString(in); got != want {
			t.Errorf("domainRe(%q) = %v, want %v", in, got, want)
		}
	}
}
