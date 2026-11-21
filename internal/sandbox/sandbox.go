package sandbox

import (
	"fmt"
	"os"

	"github.com/landlock-lsm/go-landlock/landlock"
	"github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/zouuup/landrun/internal/log"
)

type Config struct {
	ReadOnlyPaths            []string
	ReadWritePaths           []string
	ReadOnlyExecutablePaths  []string
	ReadWriteExecutablePaths []string
	UnixSocketPaths          []string
	BindTCPPorts             []int
	ConnectTCPPorts          []int
	BindUDPPorts             []int
	ConnectUDPPorts          []int
	BestEffort               bool
	UnrestrictedFilesystem   bool
	UnrestrictedNetwork      bool
	UnrestrictedScoped       bool
	RestrictBindTCP          bool
	RestrictBindUDP          bool
	IgnoreMissingPaths       bool
	// Audit logging configuration (Landlock ABI V7+).
	DisableLogOriginating bool
	EnableLogSubprocesses bool
	DisableLogSubdomains  bool
}

// fullFSAccess is the union of every filesystem access right supported by
// Landlock V10 (same FS rights as V9). It is used as the Config's handled access set
// so that every per-path rule we build stays within its bounds.
const fullFSAccess = landlock.AccessFSSet(
	syscall.AccessFSExecute |
		syscall.AccessFSWriteFile |
		syscall.AccessFSReadFile |
		syscall.AccessFSReadDir |
		syscall.AccessFSRemoveDir |
		syscall.AccessFSRemoveFile |
		syscall.AccessFSMakeChar |
		syscall.AccessFSMakeDir |
		syscall.AccessFSMakeReg |
		syscall.AccessFSMakeSock |
		syscall.AccessFSMakeFifo |
		syscall.AccessFSMakeBlock |
		syscall.AccessFSMakeSym |
		syscall.AccessFSRefer |
		syscall.AccessFSTruncate |
		syscall.AccessFSIoctlDev |
		syscall.AccessFSResolveUnix,
)

// fullNetAccess is the union of every network access right supported by
// Landlock V10 (available since V4 for TCP, V10 for UDP).
const fullNetAccess = landlock.AccessNetSet(
	syscall.AccessNetBindTCP |
		syscall.AccessNetConnectTCP |
		syscall.AccessNetBindUDP |
		syscall.AccessNetConnectSendUDP,
)

// fullScoped is the union of every IPC scope supported by Landlock V10
// (available since V6).
const fullScoped = landlock.ScopedSet(
	syscall.ScopeAbstractUnixSocket | syscall.ScopeSignal,
)

// getReadWriteExecutableRights returns a full set of permissions including execution
func getReadWriteExecutableRights(dir bool) landlock.AccessFSSet {
	accessRights := landlock.AccessFSSet(0)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSExecute)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSReadFile)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSWriteFile)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSTruncate)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSIoctlDev)

	if dir {
		accessRights |= landlock.AccessFSSet(syscall.AccessFSReadDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRemoveDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRemoveFile)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeChar)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeReg)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeSock)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeFifo)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeBlock)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeSym)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRefer)
	}

	return accessRights
}

func getReadOnlyExecutableRights(dir bool) landlock.AccessFSSet {
	accessRights := landlock.AccessFSSet(0)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSExecute)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSReadFile)
	if dir {
		accessRights |= landlock.AccessFSSet(syscall.AccessFSReadDir)
	}
	return accessRights
}

// getReadOnlyRights returns permissions for read-only access
func getReadOnlyRights(dir bool) landlock.AccessFSSet {
	accessRights := landlock.AccessFSSet(0)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSReadFile)
	if dir {
		accessRights |= landlock.AccessFSSet(syscall.AccessFSReadDir)
	}
	return accessRights
}

// getReadWriteRights returns permissions for read-write access
func getReadWriteRights(dir bool) landlock.AccessFSSet {
	accessRights := landlock.AccessFSSet(0)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSReadFile)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSWriteFile)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSTruncate)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSIoctlDev)
	if dir {
		accessRights |= landlock.AccessFSSet(syscall.AccessFSReadDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRemoveDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRemoveFile)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeChar)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeDir)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeReg)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeSock)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeFifo)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeBlock)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSMakeSym)
		accessRights |= landlock.AccessFSSet(syscall.AccessFSRefer)
	}

	return accessRights
}

// getUnixSocketRights returns permissions for connecting to a pathname UNIX
// domain socket (connect(2)/sendmsg(2)), available since Landlock ABI V9.
func getUnixSocketRights(dir bool) landlock.AccessFSSet {
	accessRights := landlock.AccessFSSet(0)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSReadFile)
	accessRights |= landlock.AccessFSSet(syscall.AccessFSResolveUnix)
	if dir {
		accessRights |= landlock.AccessFSSet(syscall.AccessFSReadDir)
	}
	return accessRights
}

// isDirectory checks if the given path is a directory
func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// pathRule builds a filesystem rule for the given access rights and path,
// optionally applying the IgnoreIfMissing modifier so that referencing a
// non-existing path does not lead to a runtime error.
func pathRule(rights landlock.AccessFSSet, path string, ignoreMissing bool) landlock.Rule {
	rule := landlock.PathAccess(rights, path)
	if ignoreMissing {
		return rule.IgnoreIfMissing()
	}
	return rule
}

func Apply(cfg Config) error {
	log.Info("Sandbox config: %+v", cfg)

	if cfg.UnrestrictedFilesystem {
		log.Info("Unrestricted filesystem access enabled.")
	}
	if cfg.UnrestrictedNetwork {
		tcp, udp := restrictsBindTCP(cfg), restrictsBindUDP(cfg)
		const tcpNote, udpNote = "TCP binding remains restricted (Landlock ABI v4+)", "UDP binding remains restricted except ephemeral ports (Landlock ABI v10+)"
		suffix := ""
		if cfg.BestEffort {
			suffix = "; with --best-effort, restrictions the kernel does not support are skipped"
		}
		switch {
		case tcp && udp:
			log.Info("Unrestricted outgoing network access enabled; %s; %s%s.", tcpNote, udpNote, suffix)
		case tcp:
			log.Info("Unrestricted outgoing network access enabled; %s%s.", tcpNote, suffix)
		case udp:
			log.Info("Unrestricted outgoing network access enabled; %s%s.", udpNote, suffix)
		default:
			log.Info("Unrestricted network access enabled.")
		}
	} else if cfg.RestrictBindTCP || cfg.RestrictBindUDP {
		log.Info("--restrict-bind-tcp/--restrict-bind-udp have no effect without --unrestricted-network: binding is already restricted.")
	}
	if cfg.UnrestrictedScoped {
		log.Info("Unrestricted IPC scoping enabled.")
	}

	llCfg, allRules, err := buildLandlockConfig(cfg)
	if err != nil {
		return err
	}

	// If every domain is unrestricted, there is nothing for Landlock to do.
	if llCfg == nil {
		log.Info("Unrestricted filesystem, network and IPC scoping enabled; no rules applied.")
		return nil
	}

	// Strict mode only insists on what the user explicitly asked for (see
	// checkCompatibility); everything else degrades to what the kernel
	// supports, with a warning. go-landlock's best-effort mode does the
	// downgrading once we've checked the explicit requirements ourselves.
	abi := detectABIVersion()
	log.Debug("Detected Landlock ABI v%d", abi)
	rep, err := checkCompatibility(cfg, abi)
	if err != nil {
		return err
	}
	for _, s := range rep.skipped {
		log.Warn("%s", s)
	}
	for _, w := range rep.warnings {
		log.Warn("%s", w)
	}

	if len(allRules) == 0 {
		log.Info("No rules provided; applying maximum restrictions for the handled domains.")
	}

	log.Debug("Applying Landlock restrictions: %s", llCfg.String())
	if err := llCfg.BestEffort().Restrict(allRules...); err != nil {
		return fmt.Errorf("failed to apply Landlock restrictions: %w", err)
	}

	log.Info("Landlock restrictions applied successfully")
	return nil
}

// restrictsBindTCP reports whether TCP binding stays restricted while the
// rest of the network is unrestricted (--unrestricted-network combined with
// --bind-tcp or --restrict-bind-tcp).
func restrictsBindTCP(cfg Config) bool {
	return cfg.RestrictBindTCP || len(cfg.BindTCPPorts) > 0
}

// restrictsBindUDP is the UDP counterpart of restrictsBindTCP.
func restrictsBindUDP(cfg Config) bool {
	return cfg.RestrictBindUDP || len(cfg.BindUDPPorts) > 0
}

// withEphemeralUDP returns ports with UDP port 0 added if not already there.
//
// The kernel implicitly binds ("autobinds") an unbound UDP socket to an
// ephemeral port when it connects or sends its first datagram, and Landlock
// checks that as a bind to port 0. Without it, any permitted outgoing UDP
// (e.g. DNS) would still fail on unbound sockets.
func withEphemeralUDP(ports []int) []int {
	for _, p := range ports {
		if p == 0 {
			return ports
		}
	}
	log.Debug("Allowing UDP bind to ephemeral port 0 so outgoing UDP sockets can autobind")
	return append([]int{0}, ports...)
}

// portRules validates ports and turns them into rules using mk.
func portRules(ports []int, kind string, mk func(uint16) landlock.NetRule) ([]landlock.Rule, error) {
	var rules []landlock.Rule
	for _, port := range ports {
		if port < 0 || port > 65535 {
			return nil, fmt.Errorf("invalid %s port %d: must be between 0 and 65535", kind, port)
		}
		log.Debug("Adding %s port: %d", kind, port)
		rules = append(rules, mk(uint16(port)))
	}
	return rules, nil
}

// buildLandlockConfig constructs the landlock.Config and rule set for cfg.
// It returns nil, nil, nil if every domain is unrestricted.
func buildLandlockConfig(cfg Config) (*landlock.Config, []landlock.Rule, error) {
	// Determine which access domains should be handled (i.e. restricted).
	// A domain that is left out of the Config stays completely unrestricted.
	var configArgs []interface{}
	if !cfg.UnrestrictedFilesystem {
		configArgs = append(configArgs, fullFSAccess)
	}

	// With --unrestricted-network, egress (connect/send) is never handled,
	// but TCP and UDP binding can still be restricted independently.
	if !cfg.UnrestrictedNetwork {
		configArgs = append(configArgs, fullNetAccess)
	} else {
		var bindAccess landlock.AccessNetSet
		if restrictsBindTCP(cfg) {
			bindAccess |= landlock.AccessNetSet(syscall.AccessNetBindTCP)
		}
		if restrictsBindUDP(cfg) {
			bindAccess |= landlock.AccessNetSet(syscall.AccessNetBindUDP)
		}
		if bindAccess != 0 {
			configArgs = append(configArgs, bindAccess)
		}
	}

	if !cfg.UnrestrictedScoped {
		configArgs = append(configArgs, fullScoped)
	}

	// If every domain is unrestricted, there is nothing for Landlock to do.
	if len(configArgs) == 0 {
		return nil, nil, nil
	}

	// Collect our rules. Filesystem rules are only meaningful when the
	// filesystem domain is handled; network rules only when the network
	// domain is handled. Adding a rule for an unhandled domain is rejected
	// by Landlock with EINVAL.
	var allRules []landlock.Rule

	if !cfg.UnrestrictedFilesystem {
		for _, path := range cfg.ReadOnlyExecutablePaths {
			log.Debug("Adding read-only executable path: %s", path)
			allRules = append(allRules, pathRule(getReadOnlyExecutableRights(isDirectory(path)), path, cfg.IgnoreMissingPaths))
		}

		for _, path := range cfg.ReadWriteExecutablePaths {
			log.Debug("Adding read-write executable path: %s", path)
			allRules = append(allRules, pathRule(getReadWriteExecutableRights(isDirectory(path)), path, cfg.IgnoreMissingPaths))
		}

		for _, path := range cfg.ReadOnlyPaths {
			log.Debug("Adding read-only path: %s", path)
			allRules = append(allRules, pathRule(getReadOnlyRights(isDirectory(path)), path, cfg.IgnoreMissingPaths))
		}

		for _, path := range cfg.ReadWritePaths {
			log.Debug("Adding read-write path: %s", path)
			allRules = append(allRules, pathRule(getReadWriteRights(isDirectory(path)), path, cfg.IgnoreMissingPaths))
		}

		for _, path := range cfg.UnixSocketPaths {
			log.Debug("Adding UNIX socket (connect) path: %s", path)
			allRules = append(allRules, pathRule(getUnixSocketRights(isDirectory(path)), path, cfg.IgnoreMissingPaths))
		}
	} else if len(cfg.UnixSocketPaths) > 0 {
		log.Info("Ignoring --unix paths because filesystem access is unrestricted.")
	}

	type portSpec struct {
		ports []int
		kind  string
		mk    func(uint16) landlock.NetRule
	}
	var netSpecs []portSpec
	if !cfg.UnrestrictedNetwork {
		bindUDP := cfg.BindUDPPorts
		if len(cfg.ConnectUDPPorts) > 0 {
			bindUDP = withEphemeralUDP(bindUDP)
		}
		netSpecs = []portSpec{
			{cfg.BindTCPPorts, "TCP bind", landlock.BindTCP},
			{cfg.ConnectTCPPorts, "TCP connect", landlock.ConnectTCP},
			{bindUDP, "UDP bind", landlock.BindUDP},
			{cfg.ConnectUDPPorts, "UDP connect/send", landlock.ConnectSendUDP},
		}
	} else {
		if len(cfg.ConnectTCPPorts) > 0 {
			log.Info("Ignoring --connect-tcp ports because outgoing network access is unrestricted.")
		}
		if len(cfg.ConnectUDPPorts) > 0 {
			log.Info("Ignoring --connect-udp ports because outgoing network access is unrestricted.")
		}
		if restrictsBindTCP(cfg) {
			netSpecs = append(netSpecs, portSpec{cfg.BindTCPPorts, "TCP bind", landlock.BindTCP})
		}
		if restrictsBindUDP(cfg) {
			// Outgoing UDP is unrestricted here, so it must keep working.
			netSpecs = append(netSpecs, portSpec{withEphemeralUDP(cfg.BindUDPPorts), "UDP bind", landlock.BindUDP})
		}
	}
	for _, spec := range netSpecs {
		rules, err := portRules(spec.ports, spec.kind, spec.mk)
		if err != nil {
			return nil, nil, err
		}
		allRules = append(allRules, rules...)
	}

	// Build the Landlock configuration. Start from a custom config that
	// handles exactly the domains we want to restrict, so that all rules are
	// enforced in a single ruleset layer. This keeps the "refer" access
	// right working (it is implicitly denied whenever the filesystem domain
	// is not handled by a layer).
	baseCfg, err := landlock.NewConfig(configArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build Landlock config: %w", err)
	}
	llCfg := *baseCfg

	// Audit logging configuration (Landlock ABI V7+). checkCompatibility
	// rejects these on older kernels unless --best-effort is given.
	if cfg.DisableLogOriginating {
		llCfg = llCfg.DisableLoggingForOriginatingProcess()
	}
	if cfg.EnableLogSubprocesses {
		llCfg = llCfg.EnableLoggingForSubprocesses()
	}
	if cfg.DisableLogSubdomains {
		llCfg = llCfg.DisableLoggingForSubdomains()
	}

	return &llCfg, allRules, nil
}
