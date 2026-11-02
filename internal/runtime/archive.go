package runtime

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func archiveName(name string, strip bool) (string, error) {
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
	}
	clean := path.Clean(name)
	if path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(name, ":\\") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	if strip {
		parts := strings.SplitN(clean, "/", 2)
		if len(parts) == 1 {
			return "", nil
		}
		clean = parts[1]
	}
	return filepath.FromSlash(clean), nil
}

// Official distributions have a single enclosing directory. Links are delayed
// until after regular files are written, and must resolve inside the root.
func extractDistribution(archive, dir string) error { return extractRuntimeArchive(archive, dir, true) }

func extractRuntimeArchive(archive, dir string, strip bool, debData ...bool) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	b := bufio.NewReader(f)
	magic, err := b.Peek(6)
	if err != nil {
		return err
	}
	var reader io.Reader
	if magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(b)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	} else if magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd {
		decoder, decodeErr := zstd.NewReader(b, zstd.WithDecoderMaxMemory(64<<20))
		if decodeErr != nil {
			return decodeErr
		}
		defer decoder.Close()
		reader = decoder
	} else {
		reader, err = xz.NewReader(b)
		if err != nil {
			return err
		}
	}
	tr := tar.NewReader(reader)
	type link struct {
		name, target string
		hard         bool
	}
	var links []link
	var total int64
	prefix := ""
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := archiveName(h.Name, strip)
		if err != nil {
			return err
		}
		top := strings.SplitN(path.Clean(h.Name), "/", 2)[0]
		if !strip {
			// Flat runtime archives may contain multiple root directories.
		} else if prefix == "" {
			prefix = top
		} else if top != prefix {
			return fmt.Errorf("archive has inconsistent root directories")
		}
		if name == "" {
			continue
		}
		// Debian documentation/manpage links may refer to other packages.
		// Only library links are needed for private runtime extraction.
		if len(debData) > 0 && debData[0] && (h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink) && !strings.Contains(filepath.Base(name), ".so") {
			continue
		}
		dst := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(dst, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			total += h.Size
			if h.Size < 0 || total > 4<<30 {
				return fmt.Errorf("runtime archive exceeds extraction limit")
			}
			if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0755)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if path.IsAbs(h.Linkname) || strings.Contains(h.Linkname, "\\") {
				return fmt.Errorf("unsafe archive link %q", h.Linkname)
			}
			target := path.Clean(path.Join(path.Dir(filepath.ToSlash(name)), h.Linkname))
			if target == ".." || strings.HasPrefix(target, "../") {
				return fmt.Errorf("archive link escapes root")
			}
			links = append(links, link{name, filepath.FromSlash(h.Linkname), false})
		case tar.TypeLink:
			target, err := archiveName(h.Linkname, strip)
			if err != nil || target == "" {
				return fmt.Errorf("unsafe archive hard link")
			}
			links = append(links, link{name, target, true})
		default:
			return fmt.Errorf("unsupported runtime archive entry %q", h.Name)
		}
	}
	for len(links) > 0 {
		pending := make([]link, 0, len(links))
		for _, l := range links {
			dst := filepath.Join(dir, l.name)
			// No link may become an ancestor of a previously written file or another
			// link. Targets must already exist and resolve within the extraction root.
			if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return err
			}
			parent, err := filepath.EvalSymlinks(filepath.Dir(dst))
			if err != nil {
				return err
			}
			if !inside(dir, parent) {
				return fmt.Errorf("archive link parent escapes root")
			}
			target := filepath.Join(filepath.Dir(dst), l.target)
			if l.hard {
				target = filepath.Join(dir, l.target)
			}
			resolved, err := filepath.EvalSymlinks(target)
			if os.IsNotExist(err) {
				pending = append(pending, l)
				continue
			}
			if err != nil {
				return fmt.Errorf("invalid archive link %s: %w", l.name, err)
			}
			if !inside(dir, resolved) {
				return fmt.Errorf("archive link target escapes root")
			}
			st, err := os.Stat(resolved)
			if err != nil || !st.Mode().IsRegular() {
				return fmt.Errorf("archive links must target regular files")
			}
			if l.hard {
				err = os.Link(resolved, dst)
			} else {
				err = os.Symlink(l.target, dst)
			}
			if err != nil {
				return err
			}
		}
		if len(pending) == len(links) {
			return fmt.Errorf("runtime archive has dangling or cyclic links")
		}
		links = pending
	}
	return nil
}
func inside(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
