package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/junikimm717/dev106/internal/config"
	"github.com/junikimm717/dev106/internal/docker"
	"github.com/junikimm717/dev106/internal/host"
	"github.com/junikimm717/dev106/internal/repo"
	"github.com/spf13/cobra"
)

type App struct {
	Config        *config.DevConfig
	Client        *docker.DevClient
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

	// Root first, so a .dev106.toml there can override the global config.
	// rootErr is deferred so commands tolerating no repo still get a config.
	root, rootErr := repo.FindRoot(wd)
	if rootErr != nil {
		root = ""
	}

	config, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	client, err := docker.New(ctx, config)
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

	if warning := host.WorkspaceWarning(root); warning != "" {
		fmt.Fprint(os.Stderr, warning)
	}

	// Only actionable warnings repeat; an unusable daemon is said once, at
	// container creation.
	if config.USB {
		if devices := client.USB(); devices.Supported {
			if warning := host.USBWarning(devices); warning != "" {
				fmt.Fprint(os.Stderr, warning)
			}
		}
	}

	binds, err := repo.BindMounts(config, root)
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

	name, err := repo.ContainerName(root)
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
		var exitErr *docker.ExitError
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
