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
	BindTCPPorts             []int
	ConnectTCPPorts          []int
	BestEffort               bool
	UnrestrictedFilesystem   bool
	UnrestrictedNetwork      bool
}

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

// isDirectory checks if the given path is a directory
func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func Apply(cfg Config) error {
	log.Info("Sandbox config: %+v", cfg)

	// Get the most advanced Landlock version available
	llCfg := landlock.V5
	if cfg.BestEffort {
		llCfg = llCfg.BestEffort()
	}

	// Collect our rules
	var file_rules []landlock.Rule
	var net_rules []landlock.Rule

	// Process executable paths
	for _, path := range cfg.ReadOnlyExecutablePaths {
		log.Debug("Adding read-only executable path: %s", path)
		file_rules = append(file_rules, landlock.PathAccess(getReadOnlyExecutableRights(isDirectory(path)), path))
	}

	for _, path := range cfg.ReadWriteExecutablePaths {
		log.Debug("Adding read-write executable path: %s", path)
		file_rules = append(file_rules, landlock.PathAccess(getReadWriteExecutableRights(isDirectory(path)), path))
	}

	// Process read-only paths
	for _, path := range cfg.ReadOnlyPaths {
		log.Debug("Adding read-only path: %s", path)
		file_rules = append(file_rules, landlock.PathAccess(getReadOnlyRights(isDirectory(path)), path))
	}

	// Process read-write paths
	for _, path := range cfg.ReadWritePaths {
		log.Debug("Adding read-write path: %s", path)
		file_rules = append(file_rules, landlock.PathAccess(getReadWriteRights(isDirectory(path)), path))
	}

	// Add rules for TCP port binding
	for _, port := range cfg.BindTCPPorts {
		log.Debug("Adding TCP bind port: %d", port)
		net_rules = append(net_rules, landlock.BindTCP(uint16(port)))
	}

	// Add rules for TCP connections
	for _, port := range cfg.ConnectTCPPorts {
		log.Debug("Adding TCP connect port: %d", port)
		net_rules = append(net_rules, landlock.ConnectTCP(uint16(port)))
	}

	if cfg.UnrestrictedFilesystem && cfg.UnrestrictedNetwork {
		log.Info("Unrestricted filesystem and network access enabled; no rules applied.")
		return nil
	}

	if cfg.UnrestrictedFilesystem {
		log.Info("Unrestricted filesystem access enabled.")
	}

	if cfg.UnrestrictedNetwork {
		log.Info("Unrestricted network access enabled")
	}

	// If we have no rules, just return
	if len(file_rules) == 0 && len(net_rules) == 0 && !cfg.UnrestrictedFilesystem && !cfg.UnrestrictedNetwork {
		log.Error("No rules provided, applying default restrictive rules, this will restrict anything landlock can do.")
		err := llCfg.Restrict()
		if err != nil {
			return fmt.Errorf("failed to apply default Landlock restrictions: %w", err)
		}
		log.Info("Default restrictive Landlock rules applied successfully")
		return nil
	}

	// Choose which go-landlock entry point to use:
	// - RestrictPaths clears handledAccessNet, so network stays unrestricted.
	// - RestrictNet clears handledAccessFS, so filesystem stays unrestricted.
	// - Restrict keeps both domains handled. Prefer that when both are
	//   restricted: separate RestrictPaths+RestrictNet stacks two rulesets,
	//   and the second clears handledAccessFS, which implicitly denies REFER
	//   ("always denied by default when not in handled_access_fs").
	// V5.Restrict with only FS rules would still handle net and deny all TCP
	// with no ConnectTCP/BindTCP allows — so unrestricted-network must use
	// RestrictPaths, not Restrict.
	log.Debug("Applying Landlock restrictions")
	var err error
	switch {
	case cfg.UnrestrictedNetwork && !cfg.UnrestrictedFilesystem:
		if len(file_rules) > 0 {
			err = llCfg.RestrictPaths(file_rules...)
		}
	case cfg.UnrestrictedFilesystem && !cfg.UnrestrictedNetwork:
		if len(net_rules) > 0 {
			err = llCfg.RestrictNet(net_rules...)
		}
	default:
		var allRules []landlock.Rule
		if !cfg.UnrestrictedFilesystem {
			allRules = append(allRules, file_rules...)
		}
		if !cfg.UnrestrictedNetwork {
			allRules = append(allRules, net_rules...)
		}
		if len(allRules) > 0 {
			err = llCfg.Restrict(allRules...)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to apply Landlock restrictions: %w", err)
	}

	log.Info("Landlock restrictions applied successfully")
	return nil
}
