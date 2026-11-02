package hosting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleHTMLHomePage(t *testing.T) {
	for _, name := range []string{"exported-page.html", "my lovely site.htm", "PAGE.HTML"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			body := []byte("<!doctype html><title>Single file</title>")
			if e := os.WriteFile(filepath.Join(root, name), body, 0600); e != nil {
				t.Fatal(e)
			}
			chosen, e := prepareStaticIndex(root)
			if e != nil || chosen != name {
				t.Fatal(chosen, e)
			}
			actual, e := os.ReadFile(filepath.Join(root, "index.html"))
			if e != nil || string(actual) != string(body) {
				t.Fatal(string(actual), e)
			}
			original, e := os.ReadFile(filepath.Join(root, name))
			if e != nil || string(original) != string(body) {
				t.Fatal("original changed", e)
			}
			if chosen, e = prepareStaticIndex(root); e != nil || chosen != "" {
				t.Fatal("existing index changed", chosen, e)
			}
		})
	}
}
func TestStaticEntryIsUnambiguousAndRegular(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"first.html", "second.html"} {
		_ = os.WriteFile(filepath.Join(root, name), []byte("page"), 0600)
	}
	if _, e := prepareStaticIndex(root); e == nil || !strings.Contains(e.Error(), "several HTML") {
		t.Fatal("ambiguous page accepted", e)
	}
	if _, e := os.Stat(filepath.Join(root, "index.html")); !os.IsNotExist(e) {
		t.Fatal("ambiguous index created")
	}
	hidden := t.TempDir()
	_ = os.WriteFile(filepath.Join(hidden, ".hidden.html"), []byte("hidden"), 0600)
	if _, e := prepareStaticIndex(hidden); e == nil {
		t.Fatal("hidden page selected")
	}
	invalid := t.TempDir()
	_ = os.Mkdir(filepath.Join(invalid, "index.html"), 0700)
	if _, e := prepareStaticIndex(invalid); e == nil {
		t.Fatal("directory index accepted")
	}
}
