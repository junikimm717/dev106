package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/junikimm717/dev106/internal/cli"
	"github.com/spf13/cobra"
)

type App struct {
	Config        *cli.DevConfig
	Client        *cli.DevClient
	ContainerName string
	Root          string
	Binds         []string
}

// Function that generates a new app. It contains an option for whether it is
// strictly required that we are in some Git repository.
func newApp(allowNoRoot bool) (*App, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// The repo root is resolved before the config so that a .dev106.toml at
	// the root can override the global one. rootErr is deferred rather than
	// returned so commands that tolerate no repo still get a config.
	root, rootErr := cli.FindRoot(wd)
	if rootErr != nil {
		root = ""
	}

	config, err := cli.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	client, err := cli.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	if rootErr != nil {
		if allowNoRoot {
			return &App{
				Config: config,
				Client: client,
			}, nil
		} else {
			return nil, rootErr
		}
	}

	if warning := cli.WorkspaceWarning(root); warning != "" {
		fmt.Fprint(os.Stderr, warning)
	}

	// Only the warnings the user can act on repeat on every command. A host
	// that cannot pass USB through at all is a fact of the platform, not a
	// mistake, so it is said once at container creation instead of nagging
	// every time someone runs a simulation.
	if config.USB {
		if devices := cli.DetectUSB(); devices.Supported {
			if warning := cli.USBWarning(devices); warning != "" {
				fmt.Fprint(os.Stderr, warning)
			}
		}
	}

	binds, err := cli.BindMounts(config, root)
	if err != nil {
		if allowNoRoot {
			fmt.Fprintf(os.Stderr, "warning: not using bind mounts: %v\n", err)
			return &App{
				Config: config,
				Client: client,
			}, nil
		} else {
			return nil, err
		}
	}

	name, err := cli.ContainerName(root)
	if err != nil {
		return nil, err
	}

	return &App{
		Config:        config,
		Client:        client,
		ContainerName: name,
		Root:          root,
		Binds:         binds,
	}, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "dev106",
		Short: "Open a shell in the course container",
		Long:  "dev106 starts (or attaches to) a development container for the current git repo. With no arguments it opens an interactive shell.",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}
			return shell(app)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(pullCmd())
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(shellCmd())
	rootCmd.AddCommand(killCmd())
	rootCmd.AddCommand(restartCmd())
	rootCmd.AddCommand(execCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(nukeCmd())
	rootCmd.AddCommand(configCmd())

	if err := rootCmd.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			if msg := exitErr.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, msg)
			}
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
