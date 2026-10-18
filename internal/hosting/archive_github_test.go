package hosting

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestGitHubPAXArchiveAndUnsafeEntries(t *testing.T) {
	for _, name := range []string{"repository-a483058/index.html", "repository-a483058/../escape.txt"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "source.tar.gz")
			file, e := os.Create(archive)
			if e != nil {
				t.Fatal(e)
			}
			gz := gzip.NewWriter(file)
			w := tar.NewWriter(gz)
			if e = w.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "a48305816dd300dce1199160920c69eec5e53f0e"}}); e != nil {
				t.Fatal(e)
			}
			if e = w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 2}); e != nil {
				t.Fatal(e)
			}
			_, _ = w.Write([]byte("ok"))
			w.Close()
			gz.Close()
			file.Close()
			dest := filepath.Join(root, "output")
			_ = os.Mkdir(dest, 0700)
			e = extractTar(archive, dest)
			if name == "repository-a483058/index.html" {
				if e != nil {
					t.Fatal("Actual GitHub global metadata rejected", e)
				}
				body, e := os.ReadFile(filepath.Join(dest, "index.html"))
				if e != nil || string(body) != "ok" {
					t.Fatal(string(body), e)
				}
			} else if e == nil {
				t.Fatal("Traversal accepted after global metadata")
			}
		})
	}
}
