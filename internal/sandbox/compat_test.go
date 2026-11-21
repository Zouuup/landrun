package sandbox

import (
	"os"
	"strings"
	"testing"
)

func TestCheckCompatibility(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/f"
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name    string
		cfg     Config
		abi     int
		wantErr string   // substring; empty means no error
		warns   []string // substrings expected across warnings and skipped
		noWarn  bool
	}{
		{name: "defaults on latest kernel", cfg: Config{}, abi: 11, noWarn: true},
		{name: "defaults on ABI v8 run, UDP unenforced", cfg: Config{}, abi: 8,
			warns: []string{"ABI v8", "UDP bind/connect/send (ABI v10)"}},
		{name: "defaults on ABI v1 list every missing default", cfg: Config{}, abi: 1,
			warns: []string{"file truncation", "TCP bind/connect", "UDP", "scoping"}},
		{name: "no Landlock at all fails in strict mode", cfg: Config{}, abi: 0,
			wantErr: "filesystem restrictions needs ABI v1"},
		{name: "everything unrestricted needs nothing", abi: 0, noWarn: true,
			cfg: Config{UnrestrictedFilesystem: true, UnrestrictedNetwork: true, UnrestrictedScoped: true}},
		{name: "connect-udp needs v10", cfg: Config{ConnectUDPPorts: []int{53}}, abi: 9,
			wantErr: "--bind-udp/--connect-udp needs ABI v10"},
		{name: "connect-tcp needs v4", cfg: Config{ConnectTCPPorts: []int{443}}, abi: 3,
			wantErr: "--bind-tcp/--connect-tcp needs ABI v4"},
		{name: "connect-tcp fine on v4, UDP default warned", cfg: Config{ConnectTCPPorts: []int{443}}, abi: 4,
			warns: []string{"UDP bind/connect/send"}},
		{name: "unix needs v9", cfg: Config{UnixSocketPaths: []string{"/run/x.sock"}}, abi: 8,
			wantErr: "--unix needs ABI v9"},
		{name: "audit flags need v7", cfg: Config{DisableLogSubdomains: true}, abi: 6,
			wantErr: "needs ABI v7"},
		{name: "rw directory needs refer (v2)", cfg: Config{ReadWritePaths: []string{dir}}, abi: 1,
			wantErr: "rename/link across directories"},
		{name: "rw file does not need refer", cfg: Config{ReadWritePaths: []string{file}}, abi: 1,
			warns: []string{"file truncation"}},
		{name: "unrestricted-network bind-udp needs v10", abi: 8,
			cfg:     Config{UnrestrictedNetwork: true, RestrictBindUDP: true},
			wantErr: "UDP bind restrictions with --unrestricted-network needs ABI v10"},
		{name: "unrestricted-network ignores connect-udp", abi: 8,
			cfg: Config{UnrestrictedNetwork: true, ConnectUDPPorts: []int{53}}},
		{name: "best effort turns missing requirements into warnings", abi: 8,
			cfg:   Config{BestEffort: true, ConnectUDPPorts: []int{53}},
			warns: []string{"not enforced: --bind-udp/--connect-udp needs ABI v10 (kernel has v8)"}},
		{name: "best effort on v1 with rw dir warns sandbox is disabled", abi: 1,
			cfg:   Config{BestEffort: true, ReadWritePaths: []string{dir}},
			warns: []string{"sandbox is disabled entirely"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := checkCompatibility(tc.cfg, tc.abi)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				if !strings.Contains(err.Error(), "--best-effort") {
					t.Errorf("error should point at --best-effort: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			warnings := append(append([]string{}, rep.warnings...), rep.skipped...)
			if tc.noWarn && len(warnings) > 0 {
				t.Errorf("expected no warnings, got %q", warnings)
			}
			all := strings.Join(warnings, "\n")
			for _, w := range tc.warns {
				if !strings.Contains(all, w) {
					t.Errorf("warnings %q missing %q", warnings, w)
				}
			}
		})
	}
}

// TestCheckCompatibilityForwardCompatible checks that kernels newer than
// anything landrun knows about are treated like the newest known ABI.
func TestCheckCompatibilityForwardCompatible(t *testing.T) {
	cfg := Config{
		ConnectUDPPorts:       []int{53},
		ConnectTCPPorts:       []int{443},
		UnixSocketPaths:       []string{"/run/x.sock"},
		DisableLogOriginating: true,
	}
	rep, err := checkCompatibility(cfg, 99)
	if err != nil || len(rep.warnings)+len(rep.skipped) > 0 {
		t.Fatalf("ABI v99: err=%v report=%+v, want neither", err, rep)
	}
}

// TestCheckCompatibilityBestEffortKeepsWarnings checks that --best-effort
// only demotes skipped explicit options: default-restriction warnings and
// a fully disabled sandbox must still be reported as warnings.
func TestCheckCompatibilityBestEffortKeepsWarnings(t *testing.T) {
	rep, err := checkCompatibility(Config{BestEffort: true, ConnectUDPPorts: []int{53}}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.skipped) != 1 || !strings.Contains(rep.skipped[0], "--bind-udp/--connect-udp") {
		t.Errorf("skipped = %q, want the UDP option", rep.skipped)
	}
	if len(rep.warnings) != 1 || !strings.Contains(rep.warnings[0], "default restrictions are not enforced") {
		t.Errorf("warnings = %q, want the default-restriction warning", rep.warnings)
	}

	rep, err = checkCompatibility(Config{BestEffort: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.warnings) != 1 || !strings.Contains(rep.warnings[0], "disabled entirely") {
		t.Errorf("ABI v0 warnings = %q, want sandbox-disabled warning", rep.warnings)
	}
}

// TestApplyStrictRejectsBeforeRestricting checks that Apply itself enforces
// the strict-mode check, before anything is restricted, using a fake ABI.
func TestApplyStrictRejectsBeforeRestricting(t *testing.T) {
	old := detectABIVersion
	defer func() { detectABIVersion = old }()
	detectABIVersion = func() int { return 3 }

	err := Apply(Config{UnrestrictedFilesystem: true, UnrestrictedScoped: true, ConnectUDPPorts: []int{53}})
	if err == nil || !strings.Contains(err.Error(), "needs ABI v10") {
		t.Fatalf("Apply err = %v, want ABI v10 requirement error", err)
	}
}
