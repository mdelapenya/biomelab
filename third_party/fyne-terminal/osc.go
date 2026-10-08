package terminal

import (
	"log"
	"net/url"
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
	dir, ok := parseDirectoryURI(uri)
	if !ok {
		if t.debug {
			log.Println("Ignoring malformed OSC 7 directory:", uri)
		}
		return
	}
	t.config.PWD = dir
	t.onConfigure()
}

// parseDirectoryURI accepts file://host/path (host may be empty) and returns
// the decoded local path. Windows shells report file://host/C:/dir, so a
// leading slash before a drive letter is dropped.
func parseDirectoryURI(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return "", false
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' && isDriveLetter(p[1]) {
		p = strings.ReplaceAll(p[1:], "/", `\`)
	}
	return p, true
}

func isDriveLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func (t *Terminal) setTitle(title string) {
	t.config.Title = title
	t.onConfigure()
}
