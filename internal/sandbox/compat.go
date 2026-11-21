package sandbox

import (
	"fmt"
	"strings"

	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/zouuup/landrun/internal/log"
)

// Landlock ABI versions that introduced the features landrun relies on.
const (
	abiFS          = 1  // basic filesystem restrictions
	abiRefer       = 2  // linking/renaming files across directories
	abiTruncate    = 3  // file truncation
	abiTCP         = 4  // TCP bind/connect
	abiIoctlDev    = 5  // device ioctl
	abiScoped      = 6  // abstract UNIX socket and signal scoping
	abiAuditFlags  = 7  // audit logging configuration
	abiResolveUnix = 9  // pathname UNIX socket connect
	abiUDP         = 10 // UDP bind/connect/send
)

// requirement is a Landlock feature that cfg explicitly asks for.
type requirement struct {
	abi    int
	reason string
}

// abiFeature is a restriction landrun applies by default, which older
// kernels may not be able to enforce.
type abiFeature struct {
	abi  int
	what string
}

// errataSignalScoping is the Landlock erratum fixing signal scoping (ABI v6).
const errataSignalScoping = 0x2

// detectABIVersion returns the Landlock ABI version that go-landlock will
// actually enforce on the running kernel, or 0 if Landlock is unavailable.
//
// It mirrors go-landlock's internal detection: kernels reporting ABI v6+
// without the signal-scoping erratum fix are treated as v5, and go-landlock
// then enforces only v5. Using the raw kernel version here would let strict
// mode accept options that go-landlock silently drops.
var detectABIVersion = func() int {
	v, err := llsys.LandlockGetABIVersion()
	if err != nil || v < 0 {
		return 0
	}
	if v >= abiScoped {
		errata, err := llsys.LandlockGetErrata()
		if err != nil || errata&errataSignalScoping == 0 {
			log.Debug("Kernel reports Landlock ABI v%d without the signal-scoping fix; treating it as v5", v)
			v = abiScoped - 1
		}
	}
	return v
}

// usesRefer reports whether any rule grants the "refer" right (rename or
// link across directories), which --rw/--rwx grant on directories.
func usesRefer(cfg Config) bool {
	for _, p := range append(append([]string{}, cfg.ReadWritePaths...), cfg.ReadWriteExecutablePaths...) {
		if isDirectory(p) {
			return true
		}
	}
	return false
}

// requirements lists the Landlock features that cfg explicitly asks for.
// In strict mode (no --best-effort), the kernel must support all of them,
// so a restriction the user asked for is never silently dropped.
func requirements(cfg Config) []requirement {
	var reqs []requirement
	add := func(abi int, reason string) { reqs = append(reqs, requirement{abi, reason}) }

	if !cfg.UnrestrictedFilesystem {
		add(abiFS, "filesystem restrictions")
		if usesRefer(cfg) {
			// On ABI v1, go-landlock cannot grant "refer" and would
			// disable the whole sandbox instead.
			add(abiRefer, "--rw/--rwx on a directory (rename/link across directories)")
		}
		if len(cfg.UnixSocketPaths) > 0 {
			add(abiResolveUnix, "--unix")
		}
	}

	if !cfg.UnrestrictedNetwork {
		if len(cfg.BindTCPPorts) > 0 || len(cfg.ConnectTCPPorts) > 0 {
			add(abiTCP, "--bind-tcp/--connect-tcp")
		}
		if len(cfg.BindUDPPorts) > 0 || len(cfg.ConnectUDPPorts) > 0 {
			add(abiUDP, "--bind-udp/--connect-udp")
		}
	} else {
		if restrictsBindTCP(cfg) {
			add(abiTCP, "TCP bind restrictions with --unrestricted-network")
		}
		if restrictsBindUDP(cfg) {
			add(abiUDP, "UDP bind restrictions with --unrestricted-network")
		}
	}

	if cfg.DisableLogOriginating || cfg.EnableLogSubprocesses || cfg.DisableLogSubdomains {
		add(abiAuditFlags, "--log-disable-originating/--log-enable-subprocesses/--log-disable-subdomains")
	}
	return reqs
}

// defaultFeatures lists the restrictions landrun applies for cfg without
// the user asking for them specifically (the "deny everything else" part).
func defaultFeatures(cfg Config) []abiFeature {
	var fs []abiFeature
	if !cfg.UnrestrictedFilesystem {
		fs = append(fs,
			abiFeature{abiTruncate, "file truncation"},
			abiFeature{abiIoctlDev, "device ioctl"},
			abiFeature{abiResolveUnix, "pathname UNIX socket connections"},
		)
	}
	if !cfg.UnrestrictedNetwork {
		fs = append(fs,
			abiFeature{abiTCP, "TCP bind/connect"},
			abiFeature{abiUDP, "UDP bind/connect/send"},
		)
	}
	if !cfg.UnrestrictedScoped {
		fs = append(fs, abiFeature{abiScoped, "abstract UNIX socket and signal scoping"})
	}
	return fs
}

// compatReport is the result of checking cfg against the kernel's ABI.
type compatReport struct {
	// skipped lists explicitly requested restrictions the kernel cannot
	// enforce. Only possible with --best-effort; strict mode errors instead.
	skipped []string
	// warnings lists everything else the user must know about: default
	// restrictions that are not enforced, or the sandbox being disabled.
	// These are shown at every log level, with or without --best-effort.
	warnings []string
}

// checkCompatibility compares cfg against the kernel's Landlock ABI.
//
// It returns an error if strict mode is used and the kernel cannot enforce
// something the user explicitly asked for.
func checkCompatibility(cfg Config, abi int) (compatReport, error) {
	var rep compatReport
	var missing []string
	for _, r := range requirements(cfg) {
		if abi < r.abi {
			missing = append(missing, fmt.Sprintf("%s needs ABI v%d", r.reason, r.abi))
		}
	}
	if len(missing) > 0 {
		if !cfg.BestEffort {
			return compatReport{}, fmt.Errorf("kernel supports Landlock ABI v%d, but %s; upgrade the kernel, drop these options, or pass --best-effort to run with weaker restrictions",
				abi, strings.Join(missing, "; "))
		}
		for _, m := range missing {
			rep.skipped = append(rep.skipped, fmt.Sprintf("not enforced: %s (kernel has v%d)", m, abi))
		}
		if abi == 0 {
			rep.warnings = append(rep.warnings, "Landlock is not available on this kernel: the sandbox is disabled entirely")
			return rep, nil
		}
		if abi < abiRefer && usesRefer(cfg) && !cfg.UnrestrictedFilesystem {
			rep.warnings = append(rep.warnings, "Landlock ABI v1 cannot grant rename/link across directories: the sandbox is disabled entirely")
			return rep, nil
		}
	}

	var unenforced []string
	for _, f := range defaultFeatures(cfg) {
		if abi < f.abi {
			unenforced = append(unenforced, fmt.Sprintf("%s (ABI v%d)", f.what, f.abi))
		}
	}
	if len(unenforced) > 0 {
		rep.warnings = append(rep.warnings, fmt.Sprintf("kernel supports Landlock ABI v%d, so these default restrictions are not enforced: %s",
			abi, strings.Join(unenforced, ", ")))
	}
	return rep, nil
}
