package elevate

import (
	"encoding/base64"
	"encoding/binary"
	"os/exec"
	"strings"
	"unicode/utf16"
)

func powershellLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func encodedPowerShell(s string) string {
	r := utf16.Encode([]rune(s))
	b := make([]byte, len(r)*2)
	for i, v := range r {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(b)
}
func windowsCopy(source, destination string) *exec.Cmd {
	inner := "$ErrorActionPreference='Stop'; Copy-Item -LiteralPath " + powershellLiteral(source) + " -Destination " + powershellLiteral(destination) + " -Force"
	script := "$ErrorActionPreference='Stop'; try { $p=Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile','-NonInteractive','-EncodedCommand'," + powershellLiteral(encodedPowerShell(inner)) + ") -Verb RunAs -PassThru -Wait; exit $p.ExitCode } catch { Write-Error 'Administrator authorization was cancelled or denied.'; exit 1 }"
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPowerShell(script))
}
