package runtime

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ZIP distributions use regular files only. Reject Windows-specific path
// escapes even when extraction is being tested on Unix.
func extractZip(archive, dir string, strip bool) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	var total uint64
	prefix := ""
	seen := map[string]bool{}
	for _, f := range z.File {
		raw := f.Name
		if strings.ContainsAny(raw, ":\\") {
			return fmt.Errorf("unsafe ZIP path %q", raw)
		}
		name, err := archiveName(raw, strip)
		if err != nil {
			return err
		}
		if strip {
			top := strings.SplitN(raw, "/", 2)[0]
			if prefix == "" {
				prefix = top
			} else if prefix != top {
				return fmt.Errorf("ZIP has inconsistent root directories")
			}
		}
		if name == "" {
			continue
		}
		for _, part := range strings.Split(filepath.ToSlash(name), "/") {
			base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
			if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
				return fmt.Errorf("unsafe Windows ZIP path %q", raw)
			}
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate ZIP path %q", raw)
		}
		seen[key] = true
		target := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported ZIP entry %q", raw)
		}
		if f.UncompressedSize64 > 4<<30 || total > 4<<30-f.UncompressedSize64 {
			return fmt.Errorf("ZIP exceeds extraction limit")
		}
		total += f.UncompressedSize64
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err = extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}
func extractZipFile(f *zip.File, target string) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, 4<<30+1))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
