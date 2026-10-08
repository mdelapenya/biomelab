package terminal

import (
	"log"
	"net/url"
	"os"
	"runtime"
	"strings"
)

func (t *Terminal) handleOSC(code string) {
	if len(code) <= 2 || code[1] != ';' {
		return
	}

	switch code[0] {
	case '0':
		// set icon name, if Fyne supports in the future
		t.setTitle(code[2:])
	case '1':
		// set icon name, if Fyne supports in the future
	case '2':
		t.setTitle(code[2:])
	case '7':
		t.setDirectory(code[2:])
	default:
		if t.debug {
			log.Println("Unrecognised OSC:", code)
		}
	}
}

// setDirectory records the shell-reported working directory (OSC 7) for
// listeners. It deliberately does not chdir: the sequence is untrusted
// process output, and changing the host process's working directory would
// affect the whole application (and on Windows would lock the directory).
func (t *Terminal) setDirectory(uri string) {
	dir, ok := parseDirectoryURI(uri, localHostname(), runtime.GOOS == "windows")
	if !ok {
		if t.debug {
			log.Println("Ignoring OSC 7 directory:", uri)
		}
		return
	}
	t.config.PWD = dir
	t.onConfigure()
}

// parseDirectoryURI accepts file://host/path and returns the local path. It
// is parsed by hand rather than with net/url because many shell prompts emit
// $PWD without percent-encoding, so '?' and '#' are path characters here and
// a stray '%' must not discard the report. Reports from another host (for
// example after ssh inside the terminal) are not local paths and are
// rejected; an empty host or "localhost" means this machine. On Windows a
// /C:/dir path becomes C:\dir.
func parseDirectoryURI(uri, hostname string, windows bool) (string, bool) {
	rest, ok := strings.CutPrefix(uri, "file://")
	if !ok {
		return "", false
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", false
	}
	host, p := rest[:slash], rest[slash:]
	if host != "" && !strings.EqualFold(host, "localhost") && !strings.EqualFold(host, hostname) {
		return "", false
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	if windows {
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' && isDriveLetter(p[1]) {
			p = p[1:]
		}
		p = strings.ReplaceAll(p, "/", `\`)
	}
	return p, true
}

func localHostname() string {
	name, _ := os.Hostname()
	return name
}

func isDriveLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func (t *Terminal) setTitle(title string) {
	t.config.Title = title
	t.onConfigure()
}
