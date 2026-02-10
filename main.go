package main

import (
	"context"
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
	client := cli.NewClient(ctx)

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
			return &App{
				Config: config,
				Client: client,
			}, nil
		} else {
			return nil, err
		}
	}

	return &App{
		Config:        config,
		Client:        client,
		ContainerName: cli.ContainerName(root),
		Binds:         binds,
	}, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "dev106",
		Short: "dev106 container runtime",
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

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
