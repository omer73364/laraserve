// Package pack creates and extracts .laraserve bundles (zip files holding
// the app, its vendor/ directory and built assets) so an app can be moved to
// an offline machine.
package pack

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, ".idea": true, ".vscode": true}

// Create writes the bundle. Secrets (.env) are excluded unless includeEnv.
func Create(root, out string, includeEnv bool) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	absOut, _ := filepath.Abs(out)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(relSlash, "storage/logs") ||
				strings.HasPrefix(relSlash, "storage/framework/cache") ||
				strings.HasPrefix(relSlash, "storage/framework/sessions") ||
				strings.HasPrefix(relSlash, "storage/framework/views") {
				return fs.SkipDir
			}
			return nil
		}
		if abs, _ := filepath.Abs(p); abs == absOut || !d.Type().IsRegular() {
			return nil
		}
		if !includeEnv && (relSlash == ".env" || strings.HasPrefix(relSlash, ".env.") && relSlash != ".env.example") {
			return nil
		}
		if strings.HasSuffix(relSlash, ".sqlite") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name, h.Method = relSlash, zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// Extract unpacks a bundle into dst, rejecting paths that escape it.
func Extract(bundle, dst string) error {
	zr, err := zip.OpenReader(bundle)
	if err != nil {
		return err
	}
	defer zr.Close()
	base := filepath.Clean(dst) + string(os.PathSeparator)
	for _, zf := range zr.File {
		target := filepath.Join(dst, filepath.FromSlash(zf.Name))
		if !strings.HasPrefix(target, base) {
			return fmt.Errorf("unsafe path in bundle: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipFile(zf, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(zf *zip.File, target string) error {
	in, err := zf.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, zf.Mode()|0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// Copy duplicates a project tree (for --copy), skipping VCS and node_modules.
func Copy(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()|0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
