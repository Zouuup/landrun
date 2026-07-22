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

// knownMachineTags are architecture qualifiers that appear in `ldconfig -p`
// output (e.g. "libc.so.6 (libc6,x86-64) => ...").
var knownMachineTags = []string{
	"x86-64",
	"x32",
	"AArch64",
	"ARM",
	"IA-64",
	"PPC64",
	"PPC",
	"RISCV64",
	"RISCV32",
	"S390X",
	"S390",
	"SPARC64",
	"SPARC",
	"MIPS64",
	"MIPS",
}

type ldEntry struct {
	path string
	info string // parenthetical flags from ldconfig, e.g. "libc6,x86-64"
}

// ldconfigMachineTag returns the ldconfig arch qualifier that should be
// preferred when resolving libraries for the given ELF class/machine.
// An empty string means "no machine qualifier" (typical for i386).
func ldconfigMachineTag(class elf.Class, machine elf.Machine) string {
	switch machine {
	case elf.EM_X86_64:
		if class == elf.ELFCLASS32 {
			return "x32"
		}
		return "x86-64"
	case elf.EM_AARCH64:
		return "AArch64"
	case elf.EM_ARM:
		return "ARM"
	case elf.EM_386:
		return ""
	case elf.EM_PPC64:
		return "PPC64"
	case elf.EM_PPC:
		return "PPC"
	case elf.EM_RISCV:
		if class == elf.ELFCLASS64 {
			return "RISCV64"
		}
		return "RISCV32"
	case elf.EM_S390:
		if class == elf.ELFCLASS64 {
			return "S390X"
		}
		return "S390"
	case elf.EM_IA_64:
		return "IA-64"
	case elf.EM_SPARCV9:
		return "SPARC64"
	case elf.EM_SPARC:
		return "SPARC"
	case elf.EM_MIPS:
		if class == elf.ELFCLASS64 {
			return "MIPS64"
		}
		return "MIPS"
	default:
		return ""
	}
}

// standardLibDirs returns library directories to search for the given ELF arch,
// including Debian/Ubuntu multiarch paths.
func standardLibDirs(class elf.Class, machine elf.Machine) []string {
	var dirs []string
	switch {
	case machine == elf.EM_X86_64 && class == elf.ELFCLASS64:
		dirs = []string{
			"/lib64", "/usr/lib64",
			"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu",
		}
	case machine == elf.EM_X86_64 && class == elf.ELFCLASS32: // x32
		dirs = []string{"/libx32", "/usr/libx32"}
	case machine == elf.EM_386:
		dirs = []string{
			"/lib32", "/usr/lib32",
			"/lib/i386-linux-gnu", "/usr/lib/i386-linux-gnu",
		}
	case machine == elf.EM_AARCH64:
		dirs = []string{
			"/lib64", "/usr/lib64",
			"/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
		}
	case machine == elf.EM_ARM:
		dirs = []string{
			"/lib/arm-linux-gnueabihf", "/usr/lib/arm-linux-gnueabihf",
			"/lib/arm-linux-gnueabi", "/usr/lib/arm-linux-gnueabi",
		}
	case machine == elf.EM_RISCV && class == elf.ELFCLASS64:
		dirs = []string{
			"/lib64", "/usr/lib64",
			"/lib/riscv64-linux-gnu", "/usr/lib/riscv64-linux-gnu",
		}
	case machine == elf.EM_PPC64:
		dirs = []string{
			"/lib64", "/usr/lib64",
			"/lib/powerpc64le-linux-gnu", "/usr/lib/powerpc64le-linux-gnu",
			"/lib/powerpc64-linux-gnu", "/usr/lib/powerpc64-linux-gnu",
		}
	case machine == elf.EM_S390 && class == elf.ELFCLASS64:
		dirs = []string{
			"/lib64", "/usr/lib64",
			"/lib/s390x-linux-gnu", "/usr/lib/s390x-linux-gnu",
		}
	}
	return append(dirs, "/lib", "/usr/lib", "/usr/local/lib")
}

func hasKnownMachineTag(info string) bool {
	for _, tag := range knownMachineTags {
		if tokenInInfo(info, tag) {
			return true
		}
	}
	return false
}

func tokenInInfo(info, token string) bool {
	if token == "" || info == "" {
		return false
	}
	for _, part := range strings.Split(info, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}

func pickLdEntry(entries []ldEntry, preferredTag string) string {
	if len(entries) == 0 {
		return ""
	}
	if preferredTag != "" {
		for _, e := range entries {
			if tokenInInfo(e.info, preferredTag) {
				return e.path
			}
		}
	} else {
		// Prefer entries without a machine qualifier (e.g. plain i386 "(libc6)").
		for _, e := range entries {
			if !hasKnownMachineTag(e.info) {
				return e.path
			}
		}
	}
	// Fall back only when there is a single candidate, to avoid picking a
	// library for the wrong architecture when several exist.
	if len(entries) == 1 {
		return entries[0].path
	}
	return ""
}

// getLdmap runs `ldconfig -p` and returns a map of soname -> path, preferring
// entries that match preferredTag (from ldconfigMachineTag).
func getLdmap(preferredTag string) map[string]string {
	m := map[string]string{}
	out, err := ldconfigRunner()
	if err != nil {
		return m
	}

	bySoname := map[string][]ldEntry{}
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
		info := ""
		if i := strings.Index(left, "("); i >= 0 {
			if j := strings.Index(left[i:], ")"); j >= 0 {
				info = strings.TrimSpace(left[i+1 : i+j])
			}
		}
		if _, err := os.Stat(path); err == nil {
			bySoname[soname] = append(bySoname[soname], ldEntry{path: path, info: info})
		}
	}

	for soname, entries := range bySoname {
		if p := pickLdEntry(entries, preferredTag); p != "" {
			m[soname] = p
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
func resolveSingleSoname(soname string, rpaths []string, stdDirs []string, preferredTag string, ldmap *map[string]string) string {
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
		*ldmap = getLdmap(preferredTag)
	}
	if p, ok := (*ldmap)[soname]; ok {
		return p
	}

	return ""
}

// resolveSonames attempts to resolve sonames to absolute paths using rpaths,
// standard library directories and falling back to parsing `ldconfig -p` output.
func resolveSonames(needed []string, rpaths []string, class elf.Class, machine elf.Machine) []string {
	resolved := map[string]string{}
	stdDirs := standardLibDirs(class, machine)
	preferredTag := ldconfigMachineTag(class, machine)
	var ldmap map[string]string

	for _, soname := range needed {
		if _, ok := resolved[soname]; ok {
			continue
		}
		resolved[soname] = resolveSingleSoname(soname, rpaths, stdDirs, preferredTag, &ldmap)
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
		libPaths := resolveSonames(needed, rpaths, f.Class, f.Machine)
		f.Close()

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
