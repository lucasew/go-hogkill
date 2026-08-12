package proc

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var appBundle = regexp.MustCompile(`/([^/]+)\.app/`)

var interpreters = map[string]struct{}{
	"node": {}, "bun": {}, "deno": {},
	"python": {}, "python2": {}, "python3": {},
	"ruby": {}, "perl": {}, "php": {}, "java": {}, "dotnet": {},
	"sh": {}, "bash": {}, "zsh": {}, "fish": {},
	"powershell": {}, "pwsh": {}, "cmd": {},
}

// BaseName last path segment, minus Windows .exe.
func BaseName(path string) string {
	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
		base = strings.TrimSuffix(base, ".EXE")
	}
	return base
}

// DisplayName short label for a single process.
func DisplayName(command, exe string) string {
	base := BaseName(exe)
	if base == "" || base == "." {
		base = command
	}
	if bundle := AppBundle(exe); bundle != "" && bundle != base {
		return base + " — " + bundle
	}
	if _, ok := interpreters[base]; ok {
		if script := scriptArgument(command, exe); script != "" {
			return base + " " + script
		}
	}
	return base
}

// GroupName label shared by every process in the same app.
func GroupName(command, exe string) string {
	if bundle := AppBundle(exe); bundle != "" {
		return bundle
	}
	base := BaseName(exe)
	if base == "" || base == "." {
		base = command
	}
	if _, ok := interpreters[base]; ok {
		if script := scriptArgument(command, exe); script != "" {
			return base + " " + script
		}
	}
	return base
}

// AppBundle outermost .app name on macOS-style paths.
func AppBundle(exe string) string {
	m := appBundle.FindStringSubmatch(exe)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func scriptArgument(command, exe string) string {
	rest := command
	if strings.HasPrefix(command, exe) {
		rest = command[len(exe):]
	}
	exeBase := BaseName(exe)
	for part := range strings.FieldsSeq(strings.TrimSpace(rest)) {
		if part == "" || strings.HasPrefix(part, "-") {
			continue
		}
		// Skip the interpreter token itself when cmdline is "node script.js".
		if BaseName(part) == exeBase {
			continue
		}
		// Windows /c style single-letter drive flag, not absolute path.
		if len(part) == 2 && part[0] == '/' && part[1] >= 'a' && part[1] <= 'z' {
			continue
		}
		if strings.Contains(part, "=") && !strings.ContainsAny(part, `/\`) {
			continue
		}
		return BaseName(part)
	}
	return ""
}

// ResolveExe recovers an executable path from a joined command line when needed.
func ResolveExe(command, hinted string) string {
	if hinted != "" {
		if st, err := os.Stat(hinted); err == nil && !st.IsDir() {
			return hinted
		}
		return hinted
	}
	if command == "" {
		return ""
	}
	chunks := strings.Split(command, " ")
	first := chunks[0]
	if strings.HasPrefix(command, "/") {
		candidate := ""
		longest := ""
		for _, chunk := range chunks {
			if strings.HasPrefix(chunk, "-") && candidate != "" {
				break
			}
			if candidate == "" {
				candidate = chunk
			} else {
				candidate = candidate + " " + chunk
			}
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				longest = candidate
			}
		}
		if longest != "" {
			return longest
		}
		if m := appBundle.FindStringIndex(command); m != nil {
			return command[:m[1]-1]
		}
	}
	return first
}
