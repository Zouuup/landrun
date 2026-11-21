package main

import (
	"fmt"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
	"github.com/zouuup/landrun/internal/elfdeps"
	"github.com/zouuup/landrun/internal/exec"
	"github.com/zouuup/landrun/internal/log"
	"github.com/zouuup/landrun/internal/sandbox"

	"context"
)

// Version is the current version of landrun
const Version = "0.1.18"

func newApp() *cli.Command {
	return &cli.Command{
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
			&cli.StringSliceFlag{
				Name:      "bind-tcp",
				Usage:     "Allow binding to these TCP ports (single port or range, e.g. 8080 or 32000-32768; restricts TCP binding even if network egress is unrestricted)",
				Validator: validatePortSpecs,
			},
			&cli.StringSliceFlag{
				Name:      "connect-tcp",
				Usage:     "Allow connecting to these TCP ports (single port or range, e.g. 443 or 32000-32768)",
				Validator: validatePortSpecs,
			},
			&cli.StringSliceFlag{
				Name:      "bind-udp",
				Usage:     "Allow binding to these UDP ports (single port or range; Landlock ABI v10+)",
				Validator: validatePortSpecs,
			},
			&cli.StringSliceFlag{
				Name:      "connect-udp",
				Usage:     "Allow connecting/sending to these UDP ports (single port or range; Landlock ABI v10+)",
				Validator: validatePortSpecs,
			},
			&cli.BoolFlag{
				Name:  "restrict-bind-tcp",
				Usage: "Restrict TCP port binding independently of outgoing network access",
				Value: false,
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
				Name:    "unrestricted-network",
				Aliases: []string{"unrestricted-egress"},
				Usage:   "Allow unrestricted network access (or unrestricted egress when combined with bind-tcp restrictions)",
				Value:   false,
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

			binary, err := osexec.LookPath(args[0])
			if err != nil {
				log.Fatal("Failed to find binary: %v", err)
			}

			var libPaths []string
			if c.Bool("ldd") {
				libPaths, err = elfdeps.GetLibraryDependencies(binary)
				if err != nil {
					log.Fatal("Failed to detect library dependencies: %v", err)
				}
			}

			cfg := buildConfig(c, binary, libPaths)
			envVars := processEnvironmentVars(c.StringSlice("env"))

			if err := sandbox.Apply(cfg); err != nil {
				log.Fatal("Failed to apply sandbox: %v", err)
			}

			return exec.Run(args, envVars)
		},
	}
}

func buildConfig(c *cli.Command, binary string, libPaths []string) sandbox.Config {
	readOnlyPaths := append([]string{}, c.StringSlice("ro")...)
	readOnlyPaths = append(readOnlyPaths, c.StringSlice("rox")...)

	readWritePaths := append([]string{}, c.StringSlice("rw")...)
	readWritePaths = append(readWritePaths, c.StringSlice("rwx")...)

	readOnlyExecutablePaths := append([]string{}, c.StringSlice("rox")...)
	readWriteExecutablePaths := append([]string{}, c.StringSlice("rwx")...)

	if c.Bool("add-exec") && binary != "" {
		readOnlyExecutablePaths = append(readOnlyExecutablePaths, binary)
		log.Debug("Added executable path: %v", binary)
	}

	if len(libPaths) > 0 {
		readOnlyExecutablePaths = append(readOnlyExecutablePaths, libPaths...)
		log.Debug("Added library paths: %v", libPaths)
	}

	return sandbox.Config{
		ReadOnlyPaths:            readOnlyPaths,
		ReadWritePaths:           readWritePaths,
		ReadOnlyExecutablePaths:  readOnlyExecutablePaths,
		ReadWriteExecutablePaths: readWriteExecutablePaths,
		UnixSocketPaths:          c.StringSlice("unix"),
		BindTCPPorts:             portsFlag(c, "bind-tcp"),
		ConnectTCPPorts:          portsFlag(c, "connect-tcp"),
		BindUDPPorts:             portsFlag(c, "bind-udp"),
		ConnectUDPPorts:          portsFlag(c, "connect-udp"),
		BestEffort:               c.Bool("best-effort"),
		UnrestrictedFilesystem:   c.Bool("unrestricted-filesystem"),
		UnrestrictedNetwork:      c.Bool("unrestricted-network"),
		UnrestrictedScoped:       c.Bool("unrestricted-scoped"),
		RestrictBindTCP:          c.Bool("restrict-bind-tcp"),
		IgnoreMissingPaths:       c.Bool("ignore-missing"),
		DisableLogOriginating:    c.Bool("log-disable-originating"),
		EnableLogSubprocesses:    c.Bool("log-enable-subprocesses"),
		DisableLogSubdomains:     c.Bool("log-disable-subdomains"),
	}
}

func parseSandboxConfig(c *cli.Command) sandbox.Config {
	return buildConfig(c, "", nil)
}

// parsePortSpecs expands port specs such as "8080" or "32000-32768" into
// the list of individual ports they cover.
func parsePortSpecs(specs []string) ([]int, error) {
	var ports []int
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		startStr, endStr, isRange := strings.Cut(spec, "-")

		start, err := parsePort(startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %w", spec, err)
		}
		end := start
		if isRange {
			end, err = parsePort(endStr)
			if err != nil {
				return nil, fmt.Errorf("invalid port range %q: %w", spec, err)
			}
			if start > end {
				return nil, fmt.Errorf("invalid port range %q: start is greater than end", spec)
			}
		}

		for p := start; p <= end; p++ {
			ports = append(ports, p)
		}
	}
	return ports, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("not a number")
	}
	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("must be between 0 and 65535")
	}
	return port, nil
}

func validatePortSpecs(specs []string) error {
	_, err := parsePortSpecs(specs)
	return err
}

// portsFlag returns the expanded ports of a flag already checked by
// validatePortSpecs.
func portsFlag(c *cli.Command, name string) []int {
	ports, err := parsePortSpecs(c.StringSlice(name))
	if err != nil {
		log.Fatal("--%s: %v", name, err)
	}
	return ports
}
func main() {
	app := newApp()
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
