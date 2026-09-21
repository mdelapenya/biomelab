package gui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"github.com/mdelapenya/biomelab/internal/command"
)

// saveBytesNative prompts the user to pick a save location using the
// OS-native dialog (NSSavePanel via osascript on macOS, zenity / kdialog
// on Linux, PowerShell SaveFileDialog on Windows) and writes data there.
// Falls back to Fyne's dialog.NewFileSave when no native tool is
// available — that keeps the feature working on minimal Linux setups
// and on unsupported platforms.
//
// The default file extension comes from defaultName's suffix and is also
// applied to the Fyne fallback's filter so the dialog UX is consistent.
// User cancellation in either path is silent (no error popup).
func (a *App) saveBytesNative(parent fyne.Window, defaultName string, data []byte) {
	path, err := nativeSaveDialog(defaultName)
	if err == nil {
		// Native succeeded (or user cancelled — path == "" in that case).
		if path == "" {
			return
		}
		if werr := os.WriteFile(path, data, 0o644); werr != nil {
			dialog.ShowError(werr, parent)
		}
		return
	}
	// No native tool available — fall back to Fyne's bundled dialog.
	a.saveBytesFyne(parent, defaultName, data)
}

// nativeSaveDialog dispatches to the per-OS helper. Returns ("", nil) on
// user cancellation; a non-nil error means no native tool was usable and
// the caller should fall back. Per-OS helpers swallow user cancellation
// internally so callers don't have to inspect the underlying tool's
// exit code.
func nativeSaveDialog(defaultName string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return nativeSaveDarwin(defaultName)
	case "linux":
		return nativeSaveLinux(defaultName)
	case "windows":
		return nativeSaveWindows(defaultName)
	default:
		return "", fmt.Errorf("no native save dialog for %s", runtime.GOOS)
	}
}

// nativeSaveDarwin invokes NSSavePanel via osascript. The AppleScript
// "choose file name" coercion to POSIX path returns an empty stdout on
// cancellation (osascript exits non-zero in that case), so we treat any
// non-zero exit as cancellation.
func nativeSaveDarwin(defaultName string) (string, error) {
	bin, err := exec.LookPath("osascript")
	if err != nil {
		return "", err
	}
	// AppleScript treats backslash and double-quote as escape characters
	// inside string literals. Escape both so unusual branch names don't
	// break the script.
	safe := strings.ReplaceAll(defaultName, `\`, `\\`)
	safe = strings.ReplaceAll(safe, `"`, `\"`)
	script := fmt.Sprintf(`POSIX path of (choose file name with prompt "Save file" default name "%s")`, safe)
	out, runErr := exec.Command(bin, "-e", script).Output()
	if runErr != nil {
		// User cancelled; no error to surface.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// nativeSaveLinux tries zenity first, then kdialog. Either's exit-1
// (cancel) is treated as cancellation. Returns a non-nil error only
// when neither tool is on PATH so the caller can fall back.
func nativeSaveLinux(defaultName string) (string, error) {
	if bin, err := exec.LookPath("zenity"); err == nil {
		out, runErr := exec.Command(bin, "--file-selection", "--save", "--confirm-overwrite", "--filename", defaultName).Output()
		if runErr != nil {
			return "", nil
		}
		return strings.TrimSpace(string(out)), nil
	}
	if bin, err := exec.LookPath("kdialog"); err == nil {
		out, runErr := exec.Command(bin, "--getsavefilename", defaultName).Output()
		if runErr != nil {
			return "", nil
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("no native save dialog (install zenity or kdialog)")
}

// nativeSaveWindows uses System.Windows.Forms.SaveFileDialog via
// PowerShell. ShowDialog returns OK / Cancel; we emit the chosen path
// only on OK so an empty stdout cleanly signals cancellation.
func nativeSaveWindows(defaultName string) (string, error) {
	bin, err := exec.LookPath("powershell")
	if err != nil {
		return "", err
	}
	// Single-quote string literals in PowerShell: '' escapes a literal '.
	safe := strings.ReplaceAll(defaultName, `'`, `''`)
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;
$dlg = New-Object System.Windows.Forms.SaveFileDialog;
$dlg.FileName = '%s';
if ($dlg.ShowDialog() -eq 'OK') { Write-Output $dlg.FileName }`, safe)
	out, runErr := command.Background(bin, "-NoProfile", "-Command", script).Output()
	if runErr != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// saveBytesFyne is the cross-platform fallback used when no native save
// dialog is reachable. Same widget that powered the export flow before
// the native helper landed — kept here so callers don't have to.
func (a *App) saveBytesFyne(parent fyne.Window, defaultName string, data []byte) {
	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, ferr error) {
		if ferr != nil {
			dialog.ShowError(ferr, parent)
			return
		}
		if writer == nil {
			return // user cancelled
		}
		if _, werr := writer.Write(data); werr != nil {
			_ = writer.Close()
			dialog.ShowError(werr, parent)
			return
		}
		if cerr := writer.Close(); cerr != nil {
			dialog.ShowError(cerr, parent)
		}
	}, parent)
	save.SetFileName(defaultName)
	if ext := extOf(defaultName); ext != "" {
		save.SetFilter(storage.NewExtensionFileFilter([]string{ext}))
	}
	save.Show()
}

// extOf returns the .ext suffix of name (with the leading dot), or ""
// when name has no extension. Used to seed the Fyne file filter so the
// fallback dialog shows the right file kind.
func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 || i == len(name)-1 {
		return ""
	}
	return name[i:]
}
