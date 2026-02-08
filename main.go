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

func newApp() (*App, error) {
	config, err := cli.LoadConfig()
	if err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	root, err := cli.FindRoot(wd)
	if err != nil {
		return nil, err
	}

	binds, err := cli.BindMounts(config, root)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	return &App{
		Config:        config,
		Client:        cli.NewClient(ctx),
		ContainerName: cli.ContainerName(root),
		Binds:         binds,
	}, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "dev106",
		Short: "dev106 container runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
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
