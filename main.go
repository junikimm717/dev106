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
	Binds         []string
}

// Function that generates a new app. It contains an option for whether it is
// strictly required that we are in some Git repository.
func newApp(allowNoRoot bool) (*App, error) {
	config, err := cli.LoadConfig()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	client, err := cli.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	root, err := cli.FindRoot(wd)
	if err != nil {
		if allowNoRoot {
			return &App{
				Config: config,
				Client: client,
			}, nil
		} else {
			return nil, err
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
