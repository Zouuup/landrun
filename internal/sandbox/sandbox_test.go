package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/landlock-lsm/go-landlock/landlock"
	"github.com/landlock-lsm/go-landlock/landlock/syscall"
)

func TestGetReadOnlyRights(t *testing.T) {
	file := getReadOnlyRights(false)
	if file&landlock.AccessFSSet(syscall.AccessFSReadFile) == 0 {
		t.Fatal("file RO should include read_file")
	}
	if file&landlock.AccessFSSet(syscall.AccessFSReadDir) != 0 {
		t.Fatal("file RO should not include read_dir")
	}
	if file&landlock.AccessFSSet(syscall.AccessFSExecute) != 0 {
		t.Fatal("file RO should not include execute")
	}

	dir := getReadOnlyRights(true)
	if dir&landlock.AccessFSSet(syscall.AccessFSReadDir) == 0 {
		t.Fatal("dir RO should include read_dir")
	}
}

func TestGetReadOnlyExecutableRights(t *testing.T) {
	file := getReadOnlyExecutableRights(false)
	for _, bit := range []landlock.AccessFSSet{
		landlock.AccessFSSet(syscall.AccessFSExecute),
		landlock.AccessFSSet(syscall.AccessFSReadFile),
	} {
		if file&bit == 0 {
			t.Fatalf("ROX file missing bit %#x", bit)
		}
	}
	if file&landlock.AccessFSSet(syscall.AccessFSReadDir) != 0 {
		t.Fatal("ROX file should not include read_dir")
	}

	dir := getReadOnlyExecutableRights(true)
	if dir&landlock.AccessFSSet(syscall.AccessFSReadDir) == 0 {
		t.Fatal("ROX dir should include read_dir")
	}
}

func TestGetReadWriteRights(t *testing.T) {
	file := getReadWriteRights(false)
	for _, bit := range []landlock.AccessFSSet{
		landlock.AccessFSSet(syscall.AccessFSReadFile),
		landlock.AccessFSSet(syscall.AccessFSWriteFile),
		landlock.AccessFSSet(syscall.AccessFSTruncate),
		landlock.AccessFSSet(syscall.AccessFSIoctlDev),
	} {
		if file&bit == 0 {
			t.Fatalf("RW file missing bit %#x", bit)
		}
	}
	if file&landlock.AccessFSSet(syscall.AccessFSRefer) != 0 {
		t.Fatal("RW file should not include refer")
	}
	if file&landlock.AccessFSSet(syscall.AccessFSExecute) != 0 {
		t.Fatal("RW file should not include execute")
	}

	dir := getReadWriteRights(true)
	for _, bit := range []landlock.AccessFSSet{
		landlock.AccessFSSet(syscall.AccessFSReadDir),
		landlock.AccessFSSet(syscall.AccessFSRemoveDir),
		landlock.AccessFSSet(syscall.AccessFSRemoveFile),
		landlock.AccessFSSet(syscall.AccessFSMakeReg),
		landlock.AccessFSSet(syscall.AccessFSRefer),
	} {
		if dir&bit == 0 {
			t.Fatalf("RW dir missing bit %#x", bit)
		}
	}
}

func TestGetReadWriteExecutableRights(t *testing.T) {
	file := getReadWriteExecutableRights(false)
	if file&landlock.AccessFSSet(syscall.AccessFSExecute) == 0 {
		t.Fatal("RWX file should include execute")
	}
	dir := getReadWriteExecutableRights(true)
	if dir&landlock.AccessFSSet(syscall.AccessFSRefer) == 0 {
		t.Fatal("RWX dir should include refer")
	}
	if dir&landlock.AccessFSSet(syscall.AccessFSExecute) == 0 {
		t.Fatal("RWX dir should include execute")
	}
}

func TestGetUnixSocketRights(t *testing.T) {
	file := getUnixSocketRights(false)
	if file&landlock.AccessFSSet(syscall.AccessFSResolveUnix) == 0 {
		t.Fatal("unix file should include resolve_unix")
	}
	if file&landlock.AccessFSSet(syscall.AccessFSReadFile) == 0 {
		t.Fatal("unix file should include read_file")
	}
	if file&landlock.AccessFSSet(syscall.AccessFSReadDir) != 0 {
		t.Fatal("unix file should not include read_dir")
	}

	dir := getUnixSocketRights(true)
	if dir&landlock.AccessFSSet(syscall.AccessFSReadDir) == 0 {
		t.Fatal("unix dir should include read_dir")
	}
}

func TestIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if !isDirectory(dir) {
		t.Fatalf("expected %s to be a directory", dir)
	}

	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if isDirectory(f) {
		t.Fatalf("expected %s not to be a directory", f)
	}

	if isDirectory(filepath.Join(dir, "missing")) {
		t.Fatal("missing path should not be a directory")
	}
}

func TestPathRule(t *testing.T) {
	rights := getReadOnlyRights(false)
	rule := pathRule(rights, "/tmp", false)
	if rule == nil {
		t.Fatal("expected non-nil rule")
	}
	ignored := pathRule(rights, "/nonexistent-landrun-path", true)
	if ignored == nil {
		t.Fatal("expected non-nil ignore-missing rule")
	}
}

func TestApplyAllUnrestricted(t *testing.T) {
	err := Apply(Config{
		UnrestrictedFilesystem: true,
		UnrestrictedNetwork:    true,
		UnrestrictedScoped:     true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestApplySubprocessBestEffort(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperBestEffort")
}

func TestApplySubprocessIgnoreMissing(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperIgnoreMissing")
}

func TestApplySubprocessMissingPathFails(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperMissingPathFails")
}

func TestApplySubprocessNetAndFlags(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperNetAndFlags")
}

func TestApplySubprocessUnrestrictedFSIgnoresUnix(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperUnrestrictedFSIgnoresUnix")
}

func TestApplySubprocessUnrestrictedNetwork(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperUnrestrictedNetwork")
}

func TestApplySubprocessEmptyRules(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperEmptyRules")
}

func runApplyInSubprocess(t *testing.T, testName string) {
	t.Helper()
	if os.Getenv("LANDRUN_SANDBOX_HELPER") == "1" {
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), "LANDRUN_SANDBOX_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", testName, err, out)
	}
}

// Helpers invoked only inside the subprocess (see runApplyInSubprocess).

func TestApplyHelperBestEffort(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	dir := t.TempDir()
	err := Apply(Config{
		BestEffort:              true,
		ReadOnlyPaths:           []string{dir},
		ReadOnlyExecutablePaths: []string{"/usr"},
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// Landlock is process-wide; exit before testing.T tries to clean TempDir.
	os.Exit(0)
}

func TestApplyHelperIgnoreMissing(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	dir := t.TempDir()
	err := Apply(Config{
		BestEffort:              true,
		IgnoreMissingPaths:      true,
		ReadOnlyPaths:           []string{dir, "/nonexistent-landrun-ignore-me"},
		ReadOnlyExecutablePaths: []string{"/usr"},
	})
	if err != nil {
		t.Fatalf("Apply with ignore-missing failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperMissingPathFails(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	err := Apply(Config{
		BestEffort:              true,
		IgnoreMissingPaths:      false,
		ReadOnlyPaths:           []string{"/nonexistent-landrun-must-fail"},
		ReadOnlyExecutablePaths: []string{"/usr"},
	})
	if err == nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestApplyHelperNetAndFlags(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "app.sock")
	if err := os.WriteFile(sock, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	rwFile := filepath.Join(dir, "rw.txt")
	if err := os.WriteFile(rwFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Apply(Config{
		BestEffort:               true,
		ReadOnlyPaths:            []string{dir},
		ReadWritePaths:           []string{rwFile},
		ReadOnlyExecutablePaths:  []string{"/usr"},
		ReadWriteExecutablePaths: []string{rwFile},
		UnixSocketPaths:          []string{sock, dir},
		BindTCPPorts:             []int{18080},
		ConnectTCPPorts:          []int{443},
		DisableLogOriginating:    true,
		EnableLogSubprocesses:    true,
		DisableLogSubdomains:     true,
	})
	if err != nil {
		t.Fatalf("Apply net/flags failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperUnrestrictedFSIgnoresUnix(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	err := Apply(Config{
		BestEffort:             true,
		UnrestrictedFilesystem: true,
		UnixSocketPaths:        []string{"/run/ignored.sock"},
		BindTCPPorts:           []int{18081},
	})
	if err != nil {
		t.Fatalf("Apply unrestricted FS failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperUnrestrictedNetwork(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	dir := t.TempDir()
	err := Apply(Config{
		BestEffort:              true,
		UnrestrictedNetwork:     true,
		UnrestrictedScoped:      true,
		ReadOnlyPaths:           []string{dir},
		ReadOnlyExecutablePaths: []string{"/usr"},
		BindTCPPorts:            []int{9999}, // ignored because net unrestricted
	})
	if err != nil {
		t.Fatalf("Apply unrestricted network failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperEmptyRules(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	// No path/port rules: still create a restrictive ruleset for handled domains.
	err := Apply(Config{BestEffort: true})
	if err != nil {
		t.Fatalf("Apply empty rules failed: %v", err)
	}
	os.Exit(0)
}
