package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestApplySubprocessUnrestrictedNetworkWithBindRestrictions(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperUnrestrictedNetworkWithBindRestrictions")
}

func TestApplySubprocessUnrestrictedNetworkWithRestrictBindTCP(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperUnrestrictedNetworkWithRestrictBindTCP")
}

func TestApplySubprocessEmptyRules(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperEmptyRules")
}

func TestApplySubprocessUDPOnly(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperUDPOnly")
}

func TestApplySubprocessInvalidPortFails(t *testing.T) {
	runApplyInSubprocess(t, "TestApplyHelperInvalidPortFails")
}

func TestBuildLandlockConfig_DecoupledBindTCP(t *testing.T) {
	cfg := Config{
		UnrestrictedNetwork: true,
		BindTCPPorts:        []int{8080, 9090},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: {bind_tcp}") {
		t.Errorf("expected Net domain to handle {bind_tcp}, got %s", str)
	}
	if strings.Contains(str, "connect_tcp") {
		t.Errorf("expected connect_tcp not to be handled, got %s", str)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	expectedRules := []string{
		"ALLOW {bind_tcp} on port 8080",
		"ALLOW {bind_tcp} on port 9090",
	}
	for i, exp := range expectedRules {
		if fmt.Sprint(rules[i]) != exp {
			t.Errorf("rule[%d] got %s, want %s", i, fmt.Sprint(rules[i]), exp)
		}
	}
}

func TestBuildLandlockConfig_RestrictBindTCPExplicit(t *testing.T) {
	cfg := Config{
		UnrestrictedNetwork: true,
		RestrictBindTCP:     true,
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: {bind_tcp}") {
		t.Errorf("expected Net domain to handle {bind_tcp}, got %s", str)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rules))
	}
}

func TestBuildLandlockConfig_UnrestrictedNetworkFully(t *testing.T) {
	// With other domains handled (e.g. FS), Net should be unhandled (Net: ∅).
	cfg := Config{
		UnrestrictedNetwork: true,
		ReadOnlyPaths:       []string{"/usr"},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: ∅") {
		t.Errorf("expected Net: ∅ when unrestricted network has no bind restrictions, got %s", str)
	}
	// Verify no network rules are generated.
	for _, r := range rules {
		if strings.Contains(fmt.Sprint(r), "TCP") {
			t.Errorf("unexpected net rule: %v", r)
		}
	}
}

func TestBuildLandlockConfig_RestrictedNetworkDefault(t *testing.T) {
	cfg := Config{
		UnrestrictedNetwork: false,
		BindTCPPorts:        []int{8080},
		ConnectTCPPorts:     []int{443},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: all") {
		t.Errorf("expected Net domain to handle all, got %s", str)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	ruleMap := make(map[string]bool)
	for _, r := range rules {
		ruleMap[fmt.Sprint(r)] = true
	}
	if !ruleMap["ALLOW {bind_tcp} on port 8080"] {
		t.Errorf("missing bind rule in %v", rules)
	}
	if !ruleMap["ALLOW {connect_tcp} on port 443"] {
		t.Errorf("missing connect rule in %v", rules)
	}
}

func TestBuildLandlockConfig_UnrestrictedNetworkIgnoresConnectTCP(t *testing.T) {
	cfg := Config{
		UnrestrictedNetwork: true,
		BindTCPPorts:        []int{8080},
		ConnectTCPPorts:     []int{443},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: {bind_tcp}") {
		t.Errorf("expected Net domain to handle {bind_tcp}, got %s", str)
	}
	if len(rules) != 1 {
		t.Fatalf("expected only 1 rule (bind), got %d: %v", len(rules), rules)
	}
	if fmt.Sprint(rules[0]) != "ALLOW {bind_tcp} on port 8080" {
		t.Errorf("unexpected rule: %v", rules[0])
	}
}

func TestBuildLandlockConfig_InvalidPortNumbers(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"negative bind port", Config{BindTCPPorts: []int{-1}}},
		{"overflow bind port", Config{BindTCPPorts: []int{65536}}},
		{"negative connect port", Config{ConnectTCPPorts: []int{-1}}},
		{"overflow connect port", Config{ConnectTCPPorts: []int{70000}}},
		{"negative bind with unrestricted net", Config{UnrestrictedNetwork: true, BindTCPPorts: []int{-1}}},
		{"overflow bind with unrestricted net", Config{UnrestrictedNetwork: true, BindTCPPorts: []int{65536}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := BuildLandlockConfig(tc.cfg)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
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
		BindUDPPorts:             []int{53, 0},
		ConnectUDPPorts:          []int{53},
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
		BindUDPPorts:           []int{18081},
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
		BindUDPPorts:            []int{9999}, // ignored because net unrestricted
		ConnectUDPPorts:         []int{53},   // ignored because net unrestricted
	})
	if err != nil {
		t.Fatalf("Apply unrestricted network failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperUnrestrictedNetworkWithBindRestrictions(t *testing.T) {
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
		BindTCPPorts:            []int{18080},
	})
	if err != nil {
		t.Fatalf("Apply unrestricted network with bind restrictions failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperUnrestrictedNetworkWithRestrictBindTCP(t *testing.T) {
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
		RestrictBindTCP:         true,
	})
	if err != nil {
		t.Fatalf("Apply unrestricted network with restrict-bind-tcp failed: %v", err)
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

func TestApplyHelperUDPOnly(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	err := Apply(Config{
		BestEffort:      true,
		BindUDPPorts:    []int{0, 5353},
		ConnectUDPPorts: []int{53},
	})
	if err != nil {
		t.Fatalf("Apply UDP only failed: %v", err)
	}
	os.Exit(0)
}

func TestApplyHelperInvalidPortFails(t *testing.T) {
	if os.Getenv("LANDRUN_SANDBOX_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	err := Apply(Config{
		BestEffort:   true,
		BindUDPPorts: []int{-1},
	})
	if err == nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestBuildLandlockConfigABI10(t *testing.T) {
	cfg := Config{
		BindTCPPorts:    []int{8080},
		ConnectTCPPorts: []int{443},
		BindUDPPorts:    []int{53, 0},
		ConnectUDPPorts: []int{5353},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llCfg == nil {
		t.Fatal("expected non-nil llCfg")
	}
	str := llCfg.String()
	if !strings.HasPrefix(str, "{Landlock V10;") {
		t.Errorf("expected Landlock V10 config, got %s", str)
	}

	expectedRuleStrings := []string{
		"ALLOW {bind_tcp} on port 8080",
		"ALLOW {connect_tcp} on port 443",
		"ALLOW {bind_udp} on port 53",
		"ALLOW {bind_udp} on port 0",
		"ALLOW {connect_send_udp} on port 5353",
	}

	foundMap := make(map[string]bool)
	for _, r := range rules {
		foundMap[fmt.Sprint(r)] = true
	}
	for _, expected := range expectedRuleStrings {
		if !foundMap[expected] {
			t.Errorf("missing expected rule: %s (got rules: %v)", expected, rules)
		}
	}
}

func TestBuildLandlockConfigBestEffort(t *testing.T) {
	cfg := Config{
		BestEffort:   true,
		BindUDPPorts: []int{53},
	}
	llCfg, _, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(llCfg.String(), "(best effort)") {
		t.Errorf("expected (best effort) in config string, got %s", llCfg.String())
	}
}

func TestBuildLandlockConfigUnrestrictedNetwork(t *testing.T) {
	cfg := Config{
		UnrestrictedNetwork: true,
		BindUDPPorts:        []int{53},
		ConnectUDPPorts:     []int{53},
	}
	llCfg, rules, err := BuildLandlockConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	str := llCfg.String()
	if !strings.Contains(str, "Net: ∅") {
		t.Errorf("expected Net domain to be unhandled (Net: ∅), got %s", str)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules when net unrestricted, got %d", len(rules))
	}
}

func TestBuildLandlockConfigInvalidPorts(t *testing.T) {
	testCases := []struct {
		name string
		cfg  Config
	}{
		{"tcp bind negative", Config{BindTCPPorts: []int{-1}}},
		{"tcp bind overflow", Config{BindTCPPorts: []int{65536}}},
		{"tcp connect negative", Config{ConnectTCPPorts: []int{-5}}},
		{"tcp connect overflow", Config{ConnectTCPPorts: []int{70000}}},
		{"udp bind negative", Config{BindUDPPorts: []int{-1}}},
		{"udp bind overflow", Config{BindUDPPorts: []int{65536}}},
		{"udp connect negative", Config{ConnectUDPPorts: []int{-10}}},
		{"udp connect overflow", Config{ConnectUDPPorts: []int{65536}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := BuildLandlockConfig(tc.cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
