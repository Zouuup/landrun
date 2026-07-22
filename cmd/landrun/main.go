package main

import (
	"os"
	osexec "os/exec"
	"strings"

	"github.com/urfave/cli/v3"
	"github.com/zouuup/landrun/internal/elfdeps"
	"github.com/zouuup/landrun/internal/exec"
	"github.com/zouuup/landrun/internal/log"
	"github.com/zouuup/landrun/internal/sandbox"

	"context"
)

// Version is the current version of landrun
const Version = "0.1.17"

func main() {
	app := &cli.Command{
		Name:    "landrun",
		Usage:   "Run a command in a Landlock sandbox",
		Version: Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Set logging level (error, info, debug)",
				Value:   "error",
				Sources: cli.EnvVars("LANDRUN_LOG_LEVEL"),
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
			&cli.StringSliceFlag{
				Name:  "unix",
				Usage: "Allow connect(2)/sendmsg(2) on this pathname UNIX domain socket (Landlock ABI v9+)",
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
				Name:  "unrestricted-scoped",
				Usage: "Allow unrestricted IPC scoping (do not restrict abstract UNIX sockets and signals; Landlock ABI v6+)",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "ignore-missing",
				Usage: "Gracefully ignore paths that do not exist instead of failing",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "log-disable-originating",
				Usage: "Disable audit logging of denials from the originating process (Landlock ABI v7+)",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "log-enable-subprocesses",
				Usage: "Enable audit logging of denials after execve(2) in subprocesses (Landlock ABI v7+)",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "log-disable-subdomains",
				Usage: "Disable audit logging of denials from nested Landlock domains (Landlock ABI v7+)",
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
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			log.SetLevel(c.String("log-level"))
			return nil, nil
		},
		Action: func(ctx context.Context, c *cli.Command) error {
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

			// Add command to readOnlyExecutablePaths
			if c.Bool("add-exec") {
				readOnlyExecutablePaths = append(readOnlyExecutablePaths, binary)
				log.Debug("Added executable path: %v", binary)
			}

			// If --ldd flag is set, detect and add library dependencies
			if c.Bool("ldd") {
				libPaths, err := elfdeps.GetLibraryDependencies(binary)
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
				UnixSocketPaths:          c.StringSlice("unix"),
				BindTCPPorts:             c.IntSlice("bind-tcp"),
				ConnectTCPPorts:          c.IntSlice("connect-tcp"),
				BestEffort:               c.Bool("best-effort"),
				UnrestrictedFilesystem:   c.Bool("unrestricted-filesystem"),
				UnrestrictedNetwork:      c.Bool("unrestricted-network"),
				UnrestrictedScoped:       c.Bool("unrestricted-scoped"),
				IgnoreMissingPaths:       c.Bool("ignore-missing"),
				DisableLogOriginating:    c.Bool("log-disable-originating"),
				EnableLogSubprocesses:    c.Bool("log-enable-subprocesses"),
				DisableLogSubdomains:     c.Bool("log-disable-subdomains"),
			}

			// Process environment variables
			envVars := processEnvironmentVars(c.StringSlice("env"))

			if err := sandbox.Apply(cfg); err != nil {
				log.Fatal("Failed to apply sandbox: %v", err)
			}

			return exec.Run(args, envVars)
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
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
