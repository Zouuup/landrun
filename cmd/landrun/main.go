package main

import (
	"os"

	"github.com/urfave/cli/v2"
	"github.com/zouuup/landrun/internal/exec"
	"github.com/zouuup/landrun/internal/log"
	"github.com/zouuup/landrun/internal/sandbox"
)

func main() {
	app := &cli.App{
		Name:  "landrun",
		Usage: "Run a command in a Landlock sandbox",
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
				Name:  "rw",
				Usage: "Allow read-write access to this path",
			},
			&cli.BoolFlag{
				Name:  "exec",
				Usage: "Allow executing files in allowed paths",
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
				Value: true,
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

			cfg := sandbox.Config{
				ReadOnlyPaths:   c.StringSlice("ro"),
				ReadWritePaths:  c.StringSlice("rw"),
				AllowExec:       c.Bool("exec"),
				BindTCPPorts:    c.IntSlice("bind-tcp"),
				ConnectTCPPorts: c.IntSlice("connect-tcp"),
				BestEffort:      c.Bool("best-effort"),
			}

			if err := sandbox.Apply(cfg); err != nil {
				log.Fatal("Failed to apply sandbox: %v", err)
			}

			return exec.Run(args)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal("%v", err)
	}
}
