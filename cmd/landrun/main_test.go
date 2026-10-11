package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	llsyscall "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/urfave/cli/v3"
	"github.com/zouuup/landrun/internal/sandbox"
)

func TestProcessEnvironmentVars(t *testing.T) {
	t.Setenv("LANDRUN_TEST_ENV_A", "value-a")
	os.Unsetenv("LANDRUN_TEST_ENV_MISSING")

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty",
			in:   nil,
			want: []string{},
		},
		{
			name: "key equals value",
			in:   []string{"FOO=bar"},
			want: []string{"FOO=bar"},
		},
		{
			name: "key from environment",
			in:   []string{"LANDRUN_TEST_ENV_A"},
			want: []string{"LANDRUN_TEST_ENV_A=value-a"},
		},
		{
			name: "missing key omitted",
			in:   []string{"LANDRUN_TEST_ENV_MISSING"},
			want: []string{},
		},
		{
			name: "mixed",
			in:   []string{"LANDRUN_TEST_ENV_A", "CUSTOM=1", "LANDRUN_TEST_ENV_MISSING"},
			want: []string{"LANDRUN_TEST_ENV_A=value-a", "CUSTOM=1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := processEnvironmentVars(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestFlagParsingAndBuildConfig(t *testing.T) {
	app := newApp()
	var capturedCfg sandbox.Config
	app.Action = func(ctx context.Context, c *cli.Command) error {
		capturedCfg = buildConfig(c, "/usr/bin/true", []string{"/usr/lib/libc.so"})
		return nil
	}

	args := []string{
		"landrun",
		"--bind-tcp", "8080",
		"--connect-tcp", "80",
		"--bind-udp", "53",
		"--bind-udp", "0",
		"--connect-udp", "53",
		"--best-effort",
		"--unrestricted-scoped",
		"--add-exec",
		"--ro", "/etc",
		"--rox", "/bin",
		"--rw", "/var/log",
		"--rwx", "/tmp",
		"--unix", "/run/app.sock",
		"true",
	}

	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("unexpected error running app: %v", err)
	}

	if !reflect.DeepEqual(capturedCfg.BindTCPPorts, []int{8080}) {
		t.Errorf("BindTCPPorts = %v, want [8080]", capturedCfg.BindTCPPorts)
	}
	if !reflect.DeepEqual(capturedCfg.ConnectTCPPorts, []int{80}) {
		t.Errorf("ConnectTCPPorts = %v, want [80]", capturedCfg.ConnectTCPPorts)
	}
	if !reflect.DeepEqual(capturedCfg.BindUDPPorts, []int{53, 0}) {
		t.Errorf("BindUDPPorts = %v, want [53, 0]", capturedCfg.BindUDPPorts)
	}
	if !reflect.DeepEqual(capturedCfg.ConnectUDPPorts, []int{53}) {
		t.Errorf("ConnectUDPPorts = %v, want [53]", capturedCfg.ConnectUDPPorts)
	}
	if !capturedCfg.BestEffort {
		t.Errorf("BestEffort = false, want true")
	}
	if !capturedCfg.UnrestrictedScoped {
		t.Errorf("UnrestrictedScoped = false, want true")
	}
	if !reflect.DeepEqual(capturedCfg.UnixSocketPaths, []string{"/run/app.sock"}) {
		t.Errorf("UnixSocketPaths = %v, want [/run/app.sock]", capturedCfg.UnixSocketPaths)
	}
	// Check ro contains /etc and /bin
	if !reflect.DeepEqual(capturedCfg.ReadOnlyPaths, []string{"/etc", "/bin"}) {
		t.Errorf("ReadOnlyPaths = %v, want [/etc, /bin]", capturedCfg.ReadOnlyPaths)
	}
	// Check rox contains /bin, binary (/usr/bin/true) and lib (/usr/lib/libc.so)
	expectedROX := []string{"/bin", "/usr/bin/true", "/usr/lib/libc.so"}
	if !reflect.DeepEqual(capturedCfg.ReadOnlyExecutablePaths, expectedROX) {
		t.Errorf("ReadOnlyExecutablePaths = %v, want %v", capturedCfg.ReadOnlyExecutablePaths, expectedROX)
	}
}

func TestFlagParsingCommaSeparatedUDPPorts(t *testing.T) {
	app := newApp()
	var capturedCfg sandbox.Config
	app.Action = func(ctx context.Context, c *cli.Command) error {
		capturedCfg = buildConfig(c, "", nil)
		return nil
	}

	args := []string{
		"landrun",
		"--bind-udp", "53,5353",
		"--connect-udp", "123,514",
		"true",
	}

	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("unexpected error running app: %v", err)
	}

	if !reflect.DeepEqual(capturedCfg.BindUDPPorts, []int{53, 5353}) {
		t.Errorf("BindUDPPorts = %v, want [53, 5353]", capturedCfg.BindUDPPorts)
	}
	if !reflect.DeepEqual(capturedCfg.ConnectUDPPorts, []int{123, 514}) {
		t.Errorf("ConnectUDPPorts = %v, want [123, 514]", capturedCfg.ConnectUDPPorts)
	}
}

func TestParsePortSpecs(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []int
		wantErr bool
	}{
		{name: "empty", in: nil, want: nil},
		{name: "single", in: []string{"8080"}, want: []int{8080}},
		{name: "range", in: []string{"32000-32003"}, want: []int{32000, 32001, 32002, 32003}},
		{name: "single-element range", in: []string{"53-53"}, want: []int{53}},
		{name: "mixed", in: []string{"80", "8000-8001"}, want: []int{80, 8000, 8001}},
		{name: "whitespace", in: []string{" 80 - 81 "}, want: []int{80, 81}},
		{name: "bounds", in: []string{"0", "65535"}, want: []int{0, 65535}},
		{name: "reversed range", in: []string{"9000-8000"}, wantErr: true},
		{name: "out of range", in: []string{"65536"}, wantErr: true},
		{name: "range end out of range", in: []string{"65000-70000"}, wantErr: true},
		{name: "negative", in: []string{"-1"}, wantErr: true},
		{name: "not a number", in: []string{"http"}, wantErr: true},
		{name: "missing end", in: []string{"8000-"}, wantErr: true},
		{name: "too many dashes", in: []string{"1-2-3"}, wantErr: true},
		{name: "empty spec", in: []string{""}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePortSpecs(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFlagParsingPortRanges(t *testing.T) {
	app := newApp()
	var capturedCfg sandbox.Config
	app.Action = func(ctx context.Context, c *cli.Command) error {
		capturedCfg = buildConfig(c, "", nil)
		return nil
	}

	args := []string{
		"landrun",
		"--bind-tcp", "8080,9000-9002",
		"--connect-tcp", "443",
		"--connect-tcp", "32000-32001",
		"--bind-udp", "5353-5354",
		"--connect-udp", "53,123-124",
		"true",
	}

	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("unexpected error running app: %v", err)
	}

	if want := []int{8080, 9000, 9001, 9002}; !reflect.DeepEqual(capturedCfg.BindTCPPorts, want) {
		t.Errorf("BindTCPPorts = %v, want %v", capturedCfg.BindTCPPorts, want)
	}
	if want := []int{443, 32000, 32001}; !reflect.DeepEqual(capturedCfg.ConnectTCPPorts, want) {
		t.Errorf("ConnectTCPPorts = %v, want %v", capturedCfg.ConnectTCPPorts, want)
	}
	if want := []int{5353, 5354}; !reflect.DeepEqual(capturedCfg.BindUDPPorts, want) {
		t.Errorf("BindUDPPorts = %v, want %v", capturedCfg.BindUDPPorts, want)
	}
	if want := []int{53, 123, 124}; !reflect.DeepEqual(capturedCfg.ConnectUDPPorts, want) {
		t.Errorf("ConnectUDPPorts = %v, want %v", capturedCfg.ConnectUDPPorts, want)
	}
}

func TestParsePortSpecsFullRange(t *testing.T) {
	got, err := parsePortSpecs([]string{"0-65535"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 65536 || got[0] != 0 || got[len(got)-1] != 65535 {
		t.Fatalf("got %d ports from %v to %v, want 65536 ports from 0 to 65535", len(got), got[0], got[len(got)-1])
	}
}

func TestFlagParsingInvalidPortRange(t *testing.T) {
	for _, flag := range []string{"--bind-tcp", "--connect-tcp", "--bind-udp", "--connect-udp"} {
		for _, value := range []string{"9000-8000", "65000-65536", "80-", "abc"} {
			t.Run(flag+"="+value, func(t *testing.T) {
				app := newApp()
				app.Writer, app.ErrWriter = io.Discard, io.Discard
				app.Action = func(ctx context.Context, c *cli.Command) error {
					t.Fatal("action should not run with an invalid port spec")
					return nil
				}
				args := []string{"landrun", flag, value, "true"}
				if err := app.Run(context.Background(), args); err == nil {
					t.Fatalf("expected error for %s %s", flag, value)
				}
			})
		}
	}
}

// Port ranges used by TestPortRangeEnforcement. The ports just outside each
// range must be denied by Landlock.
const (
	tcpBindLo, tcpBindHi       = 38700, 38702
	tcpConnectLo, tcpConnectHi = 38710, 38712
	udpBindLo, udpBindHi       = 38720, 38722
	udpConnectLo, udpConnectHi = 38730, 38732
)

// TestPortRangeEnforcement applies a sandbox built from range flags in a
// subprocess (Landlock restrictions cannot be undone) and checks that ports
// inside the ranges are allowed and ports just outside are denied.
func TestPortRangeEnforcement(t *testing.T) {
	if os.Getenv("LANDRUN_PORT_RANGE_HELPER") == "1" {
		portRangeEnforcementHelper(t)
		return
	}

	abi, err := llsyscall.LandlockGetABIVersion()
	if err != nil || abi < 4 {
		t.Skipf("Landlock network rules need ABI v4+ (got %d, err %v)", abi, err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestPortRangeEnforcement$", "-test.v")
	cmd.Env = append(os.Environ(), "LANDRUN_PORT_RANGE_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandboxed helper failed: %v\n%s", err, out)
	}
	t.Logf("helper output:\n%s", out)
}

func portRangeEnforcementHelper(t *testing.T) {
	abi, _ := llsyscall.LandlockGetABIVersion()
	checkUDP := abi >= 10

	app := newApp()
	app.Action = func(ctx context.Context, c *cli.Command) error {
		return sandbox.Apply(buildConfig(c, "", nil))
	}
	args := []string{
		"landrun",
		"--best-effort",
		"--unrestricted-filesystem",
		"--bind-tcp", fmt.Sprintf("%d-%d", tcpBindLo, tcpBindHi),
		"--connect-tcp", fmt.Sprintf("%d-%d", tcpConnectLo, tcpConnectHi),
		"--bind-udp", fmt.Sprintf("%d-%d", udpBindLo, udpBindHi),
		"--connect-udp", fmt.Sprintf("%d-%d", udpConnectLo, udpConnectHi),
		"true",
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("failed to apply sandbox: %v", err)
	}

	tcpBind := func(port int) error {
		// Go listeners default to MPTCP, which Landlock's TCP rights do not cover.
		var lc net.ListenConfig
		lc.SetMultipathTCP(false)
		l, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			l.Close()
		}
		return err
	}
	tcpConnect := func(port int) error {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			c.Close()
		}
		return err
	}
	udpBind := func(port int) error {
		c, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			c.Close()
		}
		return err
	}
	udpConnect := func(port int) error {
		c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			c.Close()
		}
		return err
	}

	checks := []struct {
		name   string
		op     func(int) error
		lo, hi int
		run    bool
	}{
		{"tcp bind", tcpBind, tcpBindLo, tcpBindHi, true},
		{"tcp connect", tcpConnect, tcpConnectLo, tcpConnectHi, true},
		{"udp bind", udpBind, udpBindLo, udpBindHi, checkUDP},
		{"udp connect", udpConnect, udpConnectLo, udpConnectHi, checkUDP},
	}

	for _, c := range checks {
		if !c.run {
			t.Logf("skipping %s checks: Landlock ABI v%d < v10", c.name, abi)
			continue
		}
		// Anything other than a permission error (e.g. ECONNREFUSED because
		// nothing is listening) means Landlock let the call through.
		for _, port := range []int{c.lo, (c.lo + c.hi) / 2, c.hi} {
			if err := c.op(port); errors.Is(err, os.ErrPermission) {
				t.Errorf("%s on port %d (inside %d-%d) was denied: %v", c.name, port, c.lo, c.hi, err)
			}
		}
		for _, port := range []int{c.lo - 1, c.hi + 1} {
			if err := c.op(port); !errors.Is(err, os.ErrPermission) {
				t.Errorf("%s on port %d (outside %d-%d) was not denied by Landlock, got err=%v", c.name, port, c.lo, c.hi, err)
			}
		}
	}
}

func TestFlagParsingUnrestrictedNetwork(t *testing.T) {
	app := newApp()
	var capturedCfg sandbox.Config
	app.Action = func(ctx context.Context, c *cli.Command) error {
		capturedCfg = buildConfig(c, "", nil)
		return nil
	}

	args := []string{
		"landrun",
		"--unrestricted-network",
		"true",
	}

	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("unexpected error running app: %v", err)
	}

	if !capturedCfg.UnrestrictedNetwork {
		t.Errorf("UnrestrictedNetwork = false, want true")
	}
}

func TestParseSandboxConfig(t *testing.T) {
	tests := []struct {
		name     string
		cliArgs  []string
		validate func(t *testing.T, cfg sandbox.Config)
	}{
		{
			name:    "unrestricted network with bind tcp decoupled",
			cliArgs: []string{"landrun", "--unrestricted-network", "--bind-tcp", "12345", "dummy"},
			validate: func(t *testing.T, cfg sandbox.Config) {
				if !cfg.UnrestrictedNetwork {
					t.Errorf("expected UnrestrictedNetwork to be true")
				}
				if !reflect.DeepEqual(cfg.BindTCPPorts, []int{12345}) {
					t.Errorf("expected BindTCPPorts [12345], got %v", cfg.BindTCPPorts)
				}
				if cfg.RestrictBindTCP {
					t.Errorf("expected RestrictBindTCP to be false")
				}
			},
		},
		{
			name:    "unrestricted egress alias with bind tcp",
			cliArgs: []string{"landrun", "--unrestricted-egress", "--bind-tcp", "8080,9090", "dummy"},
			validate: func(t *testing.T, cfg sandbox.Config) {
				if !cfg.UnrestrictedNetwork {
					t.Errorf("expected UnrestrictedNetwork to be true via --unrestricted-egress alias")
				}
				if !reflect.DeepEqual(cfg.BindTCPPorts, []int{8080, 9090}) {
					t.Errorf("expected BindTCPPorts [8080 9090], got %v", cfg.BindTCPPorts)
				}
			},
		},
		{
			name:    "unrestricted network with explicit restrict-bind-tcp",
			cliArgs: []string{"landrun", "--unrestricted-network", "--restrict-bind-tcp", "dummy"},
			validate: func(t *testing.T, cfg sandbox.Config) {
				if !cfg.UnrestrictedNetwork {
					t.Errorf("expected UnrestrictedNetwork to be true")
				}
				if !cfg.RestrictBindTCP {
					t.Errorf("expected RestrictBindTCP to be true")
				}
				if len(cfg.BindTCPPorts) != 0 {
					t.Errorf("expected empty BindTCPPorts, got %v", cfg.BindTCPPorts)
				}
			},
		},
		{
			name:    "restricted network with bind and connect ports",
			cliArgs: []string{"landrun", "--bind-tcp", "8080", "--connect-tcp", "443", "dummy"},
			validate: func(t *testing.T, cfg sandbox.Config) {
				if cfg.UnrestrictedNetwork {
					t.Errorf("expected UnrestrictedNetwork to be false")
				}
				if !reflect.DeepEqual(cfg.BindTCPPorts, []int{8080}) {
					t.Errorf("expected BindTCPPorts [8080], got %v", cfg.BindTCPPorts)
				}
				if !reflect.DeepEqual(cfg.ConnectTCPPorts, []int{443}) {
					t.Errorf("expected ConnectTCPPorts [443], got %v", cfg.ConnectTCPPorts)
				}
			},
		},
		{
			name:    "path permissions and options parsing",
			cliArgs: []string{"landrun", "--ro", "/etc", "--rox", "/usr/bin", "--rw", "/tmp", "--rwx", "/var/log", "--unix", "/run/app.sock", "--best-effort", "--ignore-missing", "dummy"},
			validate: func(t *testing.T, cfg sandbox.Config) {
				if !reflect.DeepEqual(cfg.ReadOnlyPaths, []string{"/etc", "/usr/bin"}) {
					t.Errorf("unexpected ReadOnlyPaths: %v", cfg.ReadOnlyPaths)
				}
				if !reflect.DeepEqual(cfg.ReadWritePaths, []string{"/tmp", "/var/log"}) {
					t.Errorf("unexpected ReadWritePaths: %v", cfg.ReadWritePaths)
				}
				if !reflect.DeepEqual(cfg.ReadOnlyExecutablePaths, []string{"/usr/bin"}) {
					t.Errorf("unexpected ReadOnlyExecutablePaths: %v", cfg.ReadOnlyExecutablePaths)
				}
				if !reflect.DeepEqual(cfg.ReadWriteExecutablePaths, []string{"/var/log"}) {
					t.Errorf("unexpected ReadWriteExecutablePaths: %v", cfg.ReadWriteExecutablePaths)
				}
				if !reflect.DeepEqual(cfg.UnixSocketPaths, []string{"/run/app.sock"}) {
					t.Errorf("unexpected UnixSocketPaths: %v", cfg.UnixSocketPaths)
				}
				if !cfg.BestEffort {
					t.Errorf("expected BestEffort to be true")
				}
				if !cfg.IgnoreMissingPaths {
					t.Errorf("expected IgnoreMissingPaths to be true")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newApp()
			var capturedCfg sandbox.Config
			app.Action = func(ctx context.Context, c *cli.Command) error {
				capturedCfg = parseSandboxConfig(c)
				return nil
			}

			if err := app.Run(context.Background(), tc.cliArgs); err != nil {
				t.Fatalf("app.Run failed: %v", err)
			}
			tc.validate(t, capturedCfg)
		})
	}
}
