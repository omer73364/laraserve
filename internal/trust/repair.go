package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"

	"laraserve/internal/certs"
	"laraserve/internal/manifest"
	"laraserve/internal/ui"
)

// Repair completes an already-authorized installation on normal startup.
// Never call this for an unapproved CA or when --no-trust was requested.
func Repair(ca *certs.CA, entry *manifest.CAEntry) {
	if entry == nil || entry.SHA256 != ca.SHA256 || goruntime.GOOS != "linux" {
		return
	}
	// A missing/replaced system anchor needs installation again. Existing anchors
	// avoid another sudo invocation; NSS recovery is entirely user-level.
	needsSystemRepair := false
	if entry.Location != "" {
		b, err := os.ReadFile(entry.Location)
		needsSystemRepair = err != nil || certificateDigest(b) != ca.SHA256
	} else if _, err := linuxDistro(); err == nil {
		// A prior browser-only installation may predate the system CA utility.
		needsSystemRepair = true
	}
	if needsSystemRepair {
		ui.Info("Repairing the previously approved system certificate")
		if replacement, err := Install(ca); err == nil {
			previous := entry.NSSDBs
			*entry = *replacement
			entry.NSSDBs = mergeDatabases(entry.NSSDBs, previous)
			return
		} else {
			ui.Warn("System certificate repair failed: %v; normal startup will retry", err)
		}
	}
	dbs := browserDatabases()
	if len(dbs) == 0 {
		return
	}
	tool, err := nssTool()
	if err != nil {
		ui.Warn("Browser trust needs its private tools; start again when online: %v", err)
		return
	}
	changed := 0
	for _, db := range dbs {
		if !browserHasCA(tool, db, ca.SHA256) {
			if err := run(false, tool, "-d", "sql:"+db, "-A", "-t", "C,,", "-n", certs.CAName, "-i", ca.PEMPath); err != nil {
				ui.Warn("Browser certificate import needs another attempt; close the browser and restart the app")
				continue
			}
			changed++
		}
		entry.NSSDBs = mergeDatabases(entry.NSSDBs, []string{db})
	}
	if changed > 0 {
		ui.OK("Browser trust repaired for %d profile(s)", changed)
		ui.Info("Fully quit and reopen the browser to reload certificate trust")
	}
}

func browserHasCA(tool, db, digest string) bool {
	out, err := exec.Command(tool, "-d", "sql:"+db, "-L", "-n", certs.CAName, "-a").Output()
	if err != nil || certificateDigest(out) != digest {
		return false
	}
	// The same certificate without CA trust flags is still an incomplete import.
	list, err := exec.Command(tool, "-d", "sql:"+db, "-L").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(list), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), certs.CAName+" ") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return strings.HasPrefix(fields[len(fields)-1], "C,")
			}
		}
	}
	return false
}
func certificateDigest(b []byte) string {
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "CERTIFICATE" {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:])
}
func mergeDatabases(current, extra []string) []string {
	seen := map[string]bool{}
	for _, db := range current {
		seen[db] = true
	}
	for _, db := range extra {
		if !seen[db] {
			current = append(current, db)
			seen[db] = true
		}
	}
	return current
}
