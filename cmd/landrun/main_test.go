package main

import (
	"context"
	"os"
	"reflect"
	"testing"

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
