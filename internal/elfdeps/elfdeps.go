package elfdeps

import (
	"debug/elf"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

// ldconfigRunner runs `ldconfig -p` and returns its output. Tests may override
// this variable to inject fake output. It is unexported on purpose to allow
// test injection within the package.
var ldconfigRunner = func() ([]byte, error) {
	return osexec.Command("ldconfig", "-p").Output()
}

// getLdmap runs `ldconfig -p` and returns a map of soname -> path.
func getLdmap() map[string]string {
	m := map[string]string{}
	out, err := ldconfigRunner()
	if err != nil {
		return m
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "=>") {
			continue
		}
		parts := strings.Split(line, "=>")
		if len(parts) < 2 {
			continue
		}
		path := strings.TrimSpace(parts[len(parts)-1])
		left := strings.TrimSpace(parts[0])
		toks := strings.Fields(left)
		if len(toks) == 0 {
			continue
		}
		soname := toks[0]
		if path == "" || soname == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			if _, exists := m[soname]; !exists {
				m[soname] = path
			}
		}
	}
	return m
}

// parseInterp extracts the PT_INTERP interpreter path from an ELF file.
func parseInterp(f *elf.File) string {
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			r := prog.Open()
			if r == nil {
				// Can't read interpreter
				return ""
			}
			if data, err := io.ReadAll(r); err == nil {
				return strings.TrimRight(string(data), "\x00")
			}
		}
	}
	return ""
}

// parseDynamic extracts DT_NEEDED and RPATH/RUNPATH entries from the .dynamic section.
func parseDynamic(f *elf.File) (needed []string, rpaths []string) {
	needed = []string{}
	rpaths = []string{}

	if libs, err := f.DynString(elf.DT_NEEDED); err == nil {
		needed = append(needed, libs...)
	}

	// DT_RPATH and DT_RUNPATH may both be present; split on ':' and append
	if rp, err := f.DynString(elf.DT_RPATH); err == nil {
		for _, v := range rp {
			if v == "" {
				continue
			}
			rpaths = append(rpaths, strings.Split(v, ":")...)
		}
	}
	if rp, err := f.DynString(elf.DT_RUNPATH); err == nil {
		for _, v := range rp {
			if v == "" {
				continue
			}
			rpaths = append(rpaths, strings.Split(v, ":")...)
		}
	}
	return
}

// normalizeRpaths expands common tokens like $ORIGIN and makes relative
// rpath entries absolute using the provided origin directory.
func normalizeRpaths(rpaths []string, origin string) []string {
	out := []string{}
	for _, rp := range rpaths {
		if rp == "" {
			continue
		}
		// expand $ORIGIN (common token in RPATH/RUNPATH)
		rp = strings.ReplaceAll(rp, "$ORIGIN", origin)
		rp = strings.ReplaceAll(rp, "${ORIGIN}", origin)
		// make relative rpath entries absolute using origin
		if !filepath.IsAbs(rp) {
			rp = filepath.Join(origin, rp)
		}
		out = append(out, rp)
	}
	return out
}

// resolveSingleSoname attempts to resolve a single soname using rpaths,
// standard dirs and ldconfig fallback. It takes a pointer to ldmap so the
// caller can lazily populate and reuse it.
func resolveSingleSoname(soname string, rpaths []string, stdDirs []string, ldmap *map[string]string) string {
	// check rpaths first
	for _, rp := range rpaths {
		candidate := filepath.Join(rp, soname)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// then check standard dirs
	for _, d := range stdDirs {
		candidate := filepath.Join(d, soname)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// fallback: consult parsed ldconfig map (populate lazily)
	if *ldmap == nil {
		*ldmap = getLdmap()
	}
	if p, ok := (*ldmap)[soname]; ok {
		return p
	}

	return ""
}

// resolveSonames attempts to resolve sonames to absolute paths using rpaths,
// standard library directories and falling back to parsing `ldconfig -p` output.
func resolveSonames(needed []string, rpaths []string) []string {
	resolved := map[string]string{}
	stdDirs := []string{"/lib", "/lib64", "/usr/lib", "/usr/lib64", "/usr/local/lib"}
	var ldmap map[string]string

	for _, soname := range needed {
		if _, ok := resolved[soname]; ok {
			continue
		}
		resolved[soname] = resolveSingleSoname(soname, rpaths, stdDirs, &ldmap)
	}

	out := []string{}
	for _, r := range resolved {
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}

// GetLibraryDependencies returns a list of library paths that the given binary depends on
func GetLibraryDependencies(binary string) ([]string, error) {
	queue := []string{binary}
	processed := map[string]struct{}{}
	finalMap := map[string]struct{}{}

	// Add /etc/ld.so.cache if present
	if _, err := os.Stat("/etc/ld.so.cache"); err == nil {
		finalMap["/etc/ld.so.cache"] = struct{}{}
	}

	for len(queue) > 0 {
		// Dequeue
		curr := queue[0]
		queue = queue[1:]

		if _, ok := processed[curr]; ok {
			continue
		}
		processed[curr] = struct{}{}

		f, err := elf.Open(curr)
		if err != nil {
			// This can happen with non-ELF files in the dependency chain
			// (e.g. ld.so.cache). Ignore them.
			continue
		}
		defer f.Close()

		// The first binary in the queue is the main one; grab its interpreter
		if curr == binary {
			if interpPath := parseInterp(f); interpPath != "" {
				finalMap[interpPath] = struct{}{}
				queue = append(queue, interpPath)
			}
		}

		needed, rpaths := parseDynamic(f)
		origin := filepath.Dir(curr)
		rpaths = normalizeRpaths(rpaths, origin)
		libPaths := resolveSonames(needed, rpaths)

		for _, p := range libPaths {
			if _, ok := finalMap[p]; !ok {
				finalMap[p] = struct{}{}
				queue = append(queue, p)
			}
		}
	}

	out := make([]string, 0, len(finalMap))
	for p := range finalMap {
		out = append(out, p)
	}

	return out, nil
}
