package main

import (
	"os"
	osexec "os/exec"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/zouuup/landrun/internal/exec"
	"github.com/zouuup/landrun/internal/log"
	"github.com/zouuup/landrun/internal/sandbox"
)

// Version is the current version of landrun
const Version = "0.1.15"

// getLibraryDependencies returns a list of library paths that the given binary depends on
func getLibraryDependencies(binary string) ([]string, error) {
	cmd := osexec.Command("ldd", binary)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var libPaths []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Skip lines without shared information about shared objects
		if !strings.Contains(line, ".so") {
			continue
		}
		// Extract the library path
		parts := strings.Fields(line)

		// Add only shared objects which are a **path**, because sometimes
		// they are present as `<shared-object-absolute-path> (<addr>)`
		// instead of `<shared-object-name> => <absolute-path> (<addr>)`.
		// And `ld-linux` sometimes has that behaviour and not including it
		// in `libPaths` definitely results in an error in executing the
		// binary.
		//
		// For example, the line `/lib/ld-linux-aarch64.so.1 (<addr>)` will
		// match and the path `/lib/ld-linux-aarch64.so.1` will be added to
		// `libPaths`. The line `linux-vdso.so.1 (<addr>)` will not match as
		// `linux-vdso.so.1` is not a path so we have nothing to
		// "whitelist" per se.
		if len(parts) == 2 {
			libPath := strings.Trim(parts[0], "()")
			if strings.Contains(libPath, "/") {
				libPaths = append(libPaths, libPath)
			}
		}

		if len(parts) >= 3 {
			libPath := strings.Trim(parts[2], "()")
			if libPath != "" {
				libPaths = append(libPaths, libPath)
			}
		}
	}
	return libPaths, nil
}

func main() {
	app := &cli.App{
		Name:    "landrun",
		Usage:   "Run a command in a Landlock sandbox",
		Version: Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Set logging level (error, info, debug)",
				Value:   "error",
				EnvVars: []string{"LANDRUN_LOG_LEVEL"},
			},
			&cli.StringSliceFlag{
				Name:  "ro",
				Usage: "Allow read-only access to this path",
			},
			&cli.StringSliceFlag{
				Name:  "rox",
				Usage: "Allow read-only access with execution to this path",
			},
			&cli.StringSliceFlag{
				Name:  "rw",
				Usage: "Allow read-write access to this path",
			},
			&cli.StringSliceFlag{
				Name:  "rwx",
				Usage: "Allow read-write access with execution to this path",
			},
			&cli.IntSliceFlag{
				Name:   "bind-tcp",
				Usage:  "Allow binding to these TCP ports",
				Hidden: false,
			},
			&cli.IntSliceFlag{
				Name:   "connect-tcp",
				Usage:  "Allow connecting to these TCP ports",
				Hidden: false,
			},
			&cli.BoolFlag{
				Name:  "best-effort",
				Usage: "Use best effort mode (fall back to less restrictive sandbox if necessary)",
				Value: false,
			},
			&cli.StringSliceFlag{
				Name:  "env",
				Usage: "Environment variables to pass to the sandboxed command (KEY=VALUE or just KEY to pass current value)",
				Value: cli.NewStringSlice(),
			},
			&cli.BoolFlag{
				Name:  "unrestricted-filesystem",
				Usage: "Allow unrestricted filesystem access",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "unrestricted-network",
				Usage: "Allow unrestricted network access",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "ldd",
				Usage: "Automatically detect and add library dependencies to --rox",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "add-exec",
				Usage: "Automatically add the executable path to --rox",
				Value: false,
			},
		},
		Before: func(c *cli.Context) error {
			log.SetLevel(c.String("log-level"))
			return nil
		},
		Action: func(c *cli.Context) error {
			args := c.Args().Slice()
			if len(args) == 0 {
				log.Fatal("Missing command to run")
			}

			// Combine --ro and --rox paths for read-only access
			readOnlyPaths := append([]string{}, c.StringSlice("ro")...)
			readOnlyPaths = append(readOnlyPaths, c.StringSlice("rox")...)

			// Combine --rw and --rwx paths for read-write access
			readWritePaths := append([]string{}, c.StringSlice("rw")...)
			readWritePaths = append(readWritePaths, c.StringSlice("rwx")...)

			// Combine --rox and --rwx paths for executable permissions
			readOnlyExecutablePaths := append([]string{}, c.StringSlice("rox")...)
			readWriteExecutablePaths := append([]string{}, c.StringSlice("rwx")...)

			binary, err := osexec.LookPath(args[0])
			if err != nil {
				log.Fatal("Failed to find binary: %v", err)
			}

			// Add command's directory to readOnlyExecutablePaths
			if c.Bool("add-exec") {
				readOnlyExecutablePaths = append(readOnlyExecutablePaths, binary)
				log.Debug("Added executable path: %v", binary)
			}

			// If --ldd flag is set, detect and add library dependencies
			if c.Bool("ldd") {
				libPaths, err := getLibraryDependencies(binary)
				if err != nil {
					log.Fatal("Failed to detect library dependencies: %v", err)
				}
				// Add library directories to readOnlyExecutablePaths
				readOnlyExecutablePaths = append(readOnlyExecutablePaths, libPaths...)
				log.Debug("Added library paths: %v", libPaths)
			}

			cfg := sandbox.Config{
				ReadOnlyPaths:            readOnlyPaths,
				ReadWritePaths:           readWritePaths,
				ReadOnlyExecutablePaths:  readOnlyExecutablePaths,
				ReadWriteExecutablePaths: readWriteExecutablePaths,
				BindTCPPorts:             c.IntSlice("bind-tcp"),
				ConnectTCPPorts:          c.IntSlice("connect-tcp"),
				BestEffort:               c.Bool("best-effort"),
				UnrestrictedFilesystem:   c.Bool("unrestricted-filesystem"),
				UnrestrictedNetwork:      c.Bool("unrestricted-network"),
			}

			// Process environment variables
			envVars := processEnvironmentVars(c.StringSlice("env"))

			if err := sandbox.Apply(cfg); err != nil {
				log.Fatal("Failed to apply sandbox: %v", err)
			}

			return exec.Run(args, envVars)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal("%v", err)
	}
}

// processEnvironmentVars processes the env flag values
func processEnvironmentVars(envFlags []string) []string {
	result := []string{}

	for _, env := range envFlags {
		// If the flag is just a key (no = sign), get the value from the current environment
		if !strings.Contains(env, "=") {
			if val, exists := os.LookupEnv(env); exists {
				result = append(result, env+"="+val)
			}
		} else {
			// Flag already contains the value (KEY=VALUE format)
			result = append(result, env)
		}
	}

	return result
}
