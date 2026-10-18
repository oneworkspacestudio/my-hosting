package hosting

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxArchiveBytes int64 = 256 << 20

type archiveBudget struct {
	bytes int64
	files int
	names map[string]bool
	limit int64
}

func (b *archiveBudget) target(root, name string, size int64) (string, error) {
	if strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || size < 0 || size > 128<<20 {
		return "", errors.New("Archive contains an unsafe name or oversized file")
	}
	name = strings.TrimSuffix(name, "/")
	clean := path.Clean(name)
	if clean == "." || clean != name || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", errors.New("Archive path leaves the release directory")
	}
	for _, part := range strings.Split(clean, "/") {
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.HasPrefix(part, " ") || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return "", errors.New("Archive contains a reserved Windows filename")
		}
	}
	if b.limit == 0 {
		b.limit = maxArchiveBytes
	}
	b.bytes += size
	b.files++
	key := strings.ToLower(clean)
	if b.bytes > b.limit || b.files > 25000 || b.names[key] {
		return "", errors.New("Archive exceeds limits or contains duplicate names")
	}
	b.names[key] = true
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}
func extractZIP(source, root string, strip bool) error {
	return extractZIPLimit(source, root, strip, maxArchiveBytes)
}
func extractZIPLimit(source, root string, strip bool, limit int64) error {
	z, e := zip.OpenReader(source)
	if e != nil {
		return errors.New("Choose a valid ZIP archive")
	}
	defer z.Close()
	budget := archiveBudget{names: map[string]bool{}, limit: limit}
	prefix := ""
	for _, f := range z.File {
		n := f.Name
		if strip {
			parts := strings.SplitN(n, "/", 2)
			if len(parts) == 1 {
				if f.FileInfo().IsDir() {
					continue
				}
				return errors.New("Expected a single runtime root directory")
			}
			if prefix == "" {
				prefix = parts[0]
			}
			if parts[0] != prefix {
				return errors.New("Archive has multiple root directories")
			}
			n = parts[1]
			if n == "" {
				continue
			}
		}
		if f.Mode()&os.ModeSymlink != 0 || f.Mode()&os.ModeType != 0 && !f.FileInfo().IsDir() {
			return errors.New("Archive links and special files are forbidden")
		}
		dest, e := budget.target(root, n, int64(f.UncompressedSize64))
		if e != nil {
			return e
		}
		if f.FileInfo().IsDir() {
			if e = os.MkdirAll(dest, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return e
		}
		nbytes, e := io.Copy(out, io.LimitReader(in, int64(f.UncompressedSize64)+1))
		closeErr := out.Close()
		in.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if nbytes != int64(f.UncompressedSize64) {
			return errors.New("Archive size/checksum mismatch")
		}
	}
	return nil
}
func extractTar(source, root string) error {
	f, e := os.Open(source)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	t := tar.NewReader(gz)
	budget := archiveBudget{names: map[string]bool{}}
	prefix := ""
	metadata := 0
	for {
		h, e := t.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		// GitHub tarballs start with a PAX global comment identifying the
		// revision. It is metadata, not a file outside the repository root.
		// Never accept global path/link/size overrides or unbounded metadata.
		if h.Typeflag == tar.TypeXGlobalHeader {
			metadata++
			if metadata > 16 || h.Size > 64<<10 {
				return errors.New("Source archive metadata exceeds limits")
			}
			for key := range h.PAXRecords {
				if key == "path" || key == "linkpath" || key == "size" {
					return errors.New("Source archive has unsafe global path metadata")
				}
			}
			continue
		}
		parts := strings.SplitN(h.Name, "/", 2)
		if len(parts) < 2 {
			if h.Typeflag == tar.TypeDir {
				continue
			}
			return errors.New("Source archive must have one repository root")
		}
		if prefix == "" {
			prefix = parts[0]
		}
		if parts[0] != prefix {
			return errors.New("Multiple repository roots")
		}
		if parts[1] == "" {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return errors.New("Source links and special files are forbidden")
		}
		dest, e := budget.target(root, parts[1], h.Size)
		if e != nil {
			return e
		}
		if h.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(dest, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		o, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = io.CopyN(o, t, h.Size)
		ce := o.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	return nil
}
func treeSize(root string) (int64, error) {
	var size int64
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("Release contains a symbolic link/junction; links cannot be published")
		}
		if !d.IsDir() {
			f, e := d.Info()
			if e != nil {
				return e
			}
			size += f.Size()
		}
		return nil
	})
	return size, e
}
func insideRelease(root, rel string) (string, error) {
	if !relPath(rel) {
		return "", errors.New("Invalid release path")
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	for q := p; ; q = filepath.Dir(q) {
		f, e := os.Lstat(q)
		if e != nil {
			return "", e
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Release paths cannot traverse links")
		}
		if q == root {
			break
		}
		if filepath.Dir(q) == q {
			return "", errors.New("Release path escaped its root")
		}
	}
	return p, nil
}
