package elfdeps

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Test helpers against a known binary in the system: find `true` via LookPath
func TestParseAndResolveTrue(t *testing.T) {
	bin, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("failed to find 'true' binary: %v", err)
	}

	f, err := elf.Open(bin)
	if err != nil {
		t.Fatalf("failed to open %s: %v", bin, err)
	}
	defer f.Close()

	interp := parseInterp(f)
	if interp == "" {
		t.Fatalf("expected interpreter for %s, got empty", bin)
	}

	needed, rpaths := parseDynamic(f)
	if needed == nil {
		needed = []string{}
	}

	origin := filepath.Dir(bin)
	rpaths = normalizeRpaths(rpaths, origin)
	paths := resolveSonames(needed, rpaths, f.Class, f.Machine)
	if paths == nil {
		paths = []string{}
	}

	// Ensure interpreter path exists on filesystem
	if _, err := os.Stat(interp); err != nil {
		t.Fatalf("interp path %s does not exist: %v", interp, err)
	}

	// If there are resolved library paths, they must exist and match the
	// binary's architecture (not e.g. an x32 libc for an x86-64 binary).
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("resolved library path %s does not exist: %v", p, err)
		}
		lib, err := elf.Open(p)
		if err != nil {
			continue
		}
		if lib.Class != f.Class || lib.Machine != f.Machine {
			lib.Close()
			t.Fatalf("resolved %s has class/machine %v/%v, want %v/%v",
				p, lib.Class, lib.Machine, f.Class, f.Machine)
		}
		lib.Close()
	}
}

func TestRecursiveDependencies(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not found, skipping test")
	}
	// Create a temporary directory for compiled artifacts
	tempDir := t.TempDir()

	// Compile liba.so
	libaSrc := "testdata/liba.c"
	libaSo := filepath.Join(tempDir, "liba.so")
	cmd := exec.Command("gcc", "-fPIC", "-shared", "-o", libaSo, libaSrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile liba.so: %v\n%s", err, string(out))
	}

	// Compile libb.so
	libbSrc := "testdata/libb.c"
	libbSo := filepath.Join(tempDir, "libb.so")
	cmd = exec.Command("gcc", "-fPIC", "-shared", "-o", libbSo, libbSrc, "-L"+tempDir, "-la", "-Wl,-rpath,$ORIGIN")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile libb.so: %v\n%s", err, string(out))
	}

	// Compile test_binary
	mainSrc := "testdata/main.c"
	testBin := filepath.Join(tempDir, "test_binary")
	cmd = exec.Command("gcc", "-o", testBin, mainSrc, "-L"+tempDir, "-lb", "-Wl,-rpath,$ORIGIN")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile test_binary: %v\n%s", err, string(out))
	}

	// Run the actual test logic
	deps, err := GetLibraryDependencies(testBin)
	if err != nil {
		t.Fatalf("GetLibraryDependencies failed: %v", err)
	}

	foundA := false
	foundB := false
	for _, dep := range deps {
		if dep == libaSo {
			foundA = true
		}
		if dep == libbSo {
			foundB = true
		}
	}

	if !foundA {
		t.Errorf("expected to find %s in dependency list, but didn't. Found: %v", libaSo, deps)
	}
	if !foundB {
		t.Errorf("expected to find %s in dependency list, but didn't. Found: %v", libbSo, deps)
	}
}

func TestGetLibraryDependencies(t *testing.T) {
	bin, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("failed to find 'true' binary: %v", err)
	}
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatalf("failed to open %s: %v", bin, err)
	}
	class, machine := f.Class, f.Machine
	f.Close()

	paths, err := GetLibraryDependencies(bin)
	if err != nil {
		t.Fatalf("GetLibraryDependencies failed: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("expected non-empty dependency list for %s", bin)
	}
	// ensure returned paths are absolute, exist, and match the binary arch
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Fatalf("expected absolute path, got %s", p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("path %s does not exist: %v", p, err)
		}
		if strings.Contains(p, "libx32") && class == elf.ELFCLASS64 {
			t.Fatalf("x86-64 binary resolved to x32 path %s", p)
		}
		lib, err := elf.Open(p)
		if err != nil {
			continue // e.g. ld.so.cache
		}
		mismatched := lib.Class != class || lib.Machine != machine
		lib.Close()
		if mismatched {
			t.Fatalf("dependency %s does not match binary arch", p)
		}
	}
}

func TestGetLdmapWithFakeOutput(t *testing.T) {
	// fake ldconfig output with a single mapping
	original := ldconfigRunner
	defer func() { ldconfigRunner = original }()

	// create a fake file on disk to satisfy os.Stat checks in getLdmap
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "libfake.so")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("failed to create tmp file: %v", err)
	}
	f.Close()

	// Because getLdmap checks the path exists, return tmp in the fake output
	ldconfigRunner = func() ([]byte, error) {
		return []byte("libfake.so (libc6,x86-64) => " + tmp + "\n"), nil
	}

	m := getLdmap("x86-64")
	if got, ok := m["libfake.so"]; !ok {
		t.Fatalf("expected libfake.so in map")
	} else if got != tmp {
		t.Fatalf("expected path %s, got %s", tmp, got)
	}
}

func TestGetLdmapPrefersMatchingArch(t *testing.T) {
	original := ldconfigRunner
	defer func() { ldconfigRunner = original }()

	tmpDir := t.TempDir()
	x32Path := filepath.Join(tmpDir, "libc-x32.so.6")
	x64Path := filepath.Join(tmpDir, "libc-x64.so.6")
	for _, p := range []string{x32Path, x64Path} {
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("failed to create %s: %v", p, err)
		}
		f.Close()
	}

	// x32 listed first — the old bug picked this for every arch.
	ldconfigRunner = func() ([]byte, error) {
		return []byte(
			"libc.so.6 (libc6,x32) => " + x32Path + "\n" +
				"libc.so.6 (libc6,x86-64) => " + x64Path + "\n",
		), nil
	}

	m := getLdmap("x86-64")
	if got := m["libc.so.6"]; got != x64Path {
		t.Fatalf("expected x86-64 libc %s, got %s", x64Path, got)
	}

	m = getLdmap("x32")
	if got := m["libc.so.6"]; got != x32Path {
		t.Fatalf("expected x32 libc %s, got %s", x32Path, got)
	}
}

func TestResolveSonamesUsesLdmapFallback(t *testing.T) {
	original := ldconfigRunner
	defer func() { ldconfigRunner = original }()

	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "libfake2.so")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("failed to create tmp file: %v", err)
	}
	f.Close()

	ldconfigRunner = func() ([]byte, error) {
		return []byte("libfake2.so (libc6,x86-64) => " + tmp + "\n"), nil
	}

	// needed contains a soname that won't be found in rpaths or std dirs
	rpaths := normalizeRpaths([]string{}, tmpDir)
	out := resolveSonames([]string{"libfake2.so"}, rpaths, elf.ELFCLASS64, elf.EM_X86_64)
	if len(out) != 1 {
		t.Fatalf("expected 1 resolved path, got %d", len(out))
	}
	if out[0] != tmp {
		t.Fatalf("expected %s, got %s", tmp, out[0])
	}
}

func TestResolveSonamesOriginExpansion(t *testing.T) {
	// Create a temp dir and a lib subdir to simulate $ORIGIN/lib
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.Mkdir(libDir, 0755); err != nil {
		t.Fatalf("failed create lib dir: %v", err)
	}

	libName := "liborigin.so"
	libPath := filepath.Join(libDir, libName)
	f, err := os.Create(libPath)
	if err != nil {
		t.Fatalf("failed to create lib file: %v", err)
	}
	f.Close()

	// rpath using $ORIGIN should resolve to tmpDir/lib
	rpaths1 := normalizeRpaths([]string{"$ORIGIN/lib"}, tmpDir)
	out := resolveSonames([]string{libName}, rpaths1, elf.ELFCLASS64, elf.EM_X86_64)
	if len(out) != 1 {
		t.Fatalf("expected 1 resolved path for $ORIGIN, got %d", len(out))
	}
	if out[0] != libPath {
		t.Fatalf("expected %s, got %s", libPath, out[0])
	}

	// relative rpath should also resolve against origin
	rpaths2 := normalizeRpaths([]string{"lib"}, tmpDir)
	out2 := resolveSonames([]string{libName}, rpaths2, elf.ELFCLASS64, elf.EM_X86_64)
	if len(out2) != 1 {
		t.Fatalf("expected 1 resolved path for relative rpath, got %d", len(out2))
	}
	if out2[0] != libPath {
		t.Fatalf("expected %s, got %s", libPath, out2[0])
	}
}

func TestLdconfigMachineTag(t *testing.T) {
	tests := []struct {
		class   elf.Class
		machine elf.Machine
		want    string
	}{
		{elf.ELFCLASS64, elf.EM_X86_64, "x86-64"},
		{elf.ELFCLASS32, elf.EM_X86_64, "x32"},
		{elf.ELFCLASS64, elf.EM_AARCH64, "AArch64"},
		{elf.ELFCLASS32, elf.EM_ARM, "ARM"},
		{elf.ELFCLASS32, elf.EM_386, ""},
		{elf.ELFCLASS64, elf.EM_PPC64, "PPC64"},
		{elf.ELFCLASS32, elf.EM_PPC, "PPC"},
		{elf.ELFCLASS64, elf.EM_RISCV, "RISCV64"},
		{elf.ELFCLASS32, elf.EM_RISCV, "RISCV32"},
		{elf.ELFCLASS64, elf.EM_S390, "S390X"},
		{elf.ELFCLASS32, elf.EM_S390, "S390"},
		{elf.ELFCLASS64, elf.EM_IA_64, "IA-64"},
		{elf.ELFCLASS64, elf.EM_SPARCV9, "SPARC64"},
		{elf.ELFCLASS32, elf.EM_SPARC, "SPARC"},
		{elf.ELFCLASS64, elf.EM_MIPS, "MIPS64"},
		{elf.ELFCLASS32, elf.EM_MIPS, "MIPS"},
		{elf.ELFCLASS64, elf.EM_NONE, ""},
	}
	for _, tc := range tests {
		got := ldconfigMachineTag(tc.class, tc.machine)
		if got != tc.want {
			t.Fatalf("ldconfigMachineTag(%v,%v)=%q want %q", tc.class, tc.machine, got, tc.want)
		}
	}
}

func TestStandardLibDirs(t *testing.T) {
	cases := []struct {
		class   elf.Class
		machine elf.Machine
		needle  string
	}{
		{elf.ELFCLASS64, elf.EM_X86_64, "/lib/x86_64-linux-gnu"},
		{elf.ELFCLASS32, elf.EM_X86_64, "/libx32"},
		{elf.ELFCLASS32, elf.EM_386, "/lib/i386-linux-gnu"},
		{elf.ELFCLASS64, elf.EM_AARCH64, "/lib/aarch64-linux-gnu"},
		{elf.ELFCLASS32, elf.EM_ARM, "/lib/arm-linux-gnueabihf"},
		{elf.ELFCLASS64, elf.EM_RISCV, "/lib/riscv64-linux-gnu"},
		{elf.ELFCLASS64, elf.EM_PPC64, "/lib/powerpc64le-linux-gnu"},
		{elf.ELFCLASS64, elf.EM_S390, "/lib/s390x-linux-gnu"},
	}
	for _, tc := range cases {
		dirs := standardLibDirs(tc.class, tc.machine)
		found := false
		for _, d := range dirs {
			if d == tc.needle {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("standardLibDirs(%v,%v) missing %s in %v", tc.class, tc.machine, tc.needle, dirs)
		}
		// Always includes generic fallbacks.
		hasLib := false
		for _, d := range dirs {
			if d == "/lib" {
				hasLib = true
			}
		}
		if !hasLib {
			t.Fatalf("expected /lib fallback in %v", dirs)
		}
	}
}

func TestTokenInInfoAndHasKnownMachineTag(t *testing.T) {
	if !tokenInInfo("libc6,x86-64", "x86-64") {
		t.Fatal("expected token match")
	}
	if tokenInInfo("libc6,x86-64", "x32") {
		t.Fatal("unexpected token match")
	}
	if tokenInInfo("", "x86-64") {
		t.Fatal("empty info should not match")
	}
	if tokenInInfo("libc6", "") {
		t.Fatal("empty token should not match")
	}
	if !hasKnownMachineTag("libc6,x86-64") {
		t.Fatal("expected known machine tag")
	}
	if hasKnownMachineTag("libc6") {
		t.Fatal("plain libc6 should not have known machine tag")
	}
}

func TestPickLdEntry(t *testing.T) {
	if pickLdEntry(nil, "x86-64") != "" {
		t.Fatal("empty entries should return empty")
	}

	entries := []ldEntry{
		{path: "/lib/x32.so", info: "libc6,x32"},
		{path: "/lib/x64.so", info: "libc6,x86-64"},
	}
	if got := pickLdEntry(entries, "x86-64"); got != "/lib/x64.so" {
		t.Fatalf("preferred tag: got %s", got)
	}

	// No preferred tag: prefer entry without known machine qualifier.
	plain := []ldEntry{
		{path: "/lib/wrong.so", info: "libc6,x86-64"},
		{path: "/lib/plain.so", info: "libc6"},
	}
	if got := pickLdEntry(plain, ""); got != "/lib/plain.so" {
		t.Fatalf("no preferred tag: got %s", got)
	}

	// Preferred miss with multiple candidates → empty (avoid wrong arch).
	if got := pickLdEntry(entries, "AArch64"); got != "" {
		t.Fatalf("preferred miss with multiple should be empty, got %s", got)
	}

	// Preferred miss with single candidate → that one.
	single := []ldEntry{{path: "/lib/only.so", info: "libc6,x32"}}
	if got := pickLdEntry(single, "x86-64"); got != "/lib/only.so" {
		t.Fatalf("single fallback: got %s", got)
	}
}

func TestNormalizeRpathsOriginBraceAndEmpty(t *testing.T) {
	origin := "/opt/app"
	got := normalizeRpaths([]string{"", "${ORIGIN}/lib", "$ORIGIN/../lib"}, origin)
	want := []string{"/opt/app/lib", "/opt/app/../lib"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %s want %s", i, got[i], want[i])
		}
	}
}

func TestGetLdmapErrorAndMalformed(t *testing.T) {
	original := ldconfigRunner
	defer func() { ldconfigRunner = original }()

	ldconfigRunner = func() ([]byte, error) {
		return nil, os.ErrPermission
	}
	if m := getLdmap("x86-64"); len(m) != 0 {
		t.Fatalf("expected empty map on error, got %v", m)
	}

	tmpDir := t.TempDir()
	okPath := filepath.Join(tmpDir, "libok.so")
	f, err := os.Create(okPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ldconfigRunner = func() ([]byte, error) {
		return []byte(
			"not a mapping line\n" +
				"libbad.so (libc6,x86-64)\n" + // missing =>
				"libok.so (libc6,x86-64) => " + okPath + "\n" +
				"libmissing.so (libc6,x86-64) => /no/such/libmissing.so\n",
		), nil
	}
	m := getLdmap("x86-64")
	if got := m["libok.so"]; got != okPath {
		t.Fatalf("expected libok.so -> %s, got %s map=%v", okPath, got, m)
	}
	if _, ok := m["libmissing.so"]; ok {
		t.Fatal("missing path should not be in map")
	}
	if _, ok := m["libbad.so"]; ok {
		t.Fatal("malformed line should not be in map")
	}
}
