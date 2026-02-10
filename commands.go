package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func pullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull container image",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(true)
			if err != nil {
				return err
			}
			return app.Client.Pull(app.Config)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start container",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}
			fmt.Printf("Starting new container %s\n", app.ContainerName)
			return app.Client.Run(app.Config, app.ContainerName, app.Binds)
		},
	}
}

func killCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill",
		Short: "Kill and delete container",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}
			fmt.Printf("Killing container %s\n", app.ContainerName)
			return app.Client.Delete(app.ContainerName)
		},
	}
}

func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Recreate container",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}

			fmt.Printf("Killing container %s\n", app.ContainerName)
			if err := app.Client.Delete(app.ContainerName); err != nil {
				return err
			}

			fmt.Printf("Starting new container %s\n", app.ContainerName)
			return app.Client.Run(app.Config, app.ContainerName, app.Binds)
		},
	}
}

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open shell in container",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}
			return shell(app)
		},
	}
}

func shell(app *App) error {
	exists, err := app.Client.ContainerExists(app.ContainerName)
	if err != nil {
		return err
	}

	if !exists {
		fmt.Printf("Starting new container %s\n", app.ContainerName)
		if err := app.Client.Run(app.Config, app.ContainerName, app.Binds); err != nil {
			return err
		}
	}

	return app.Client.Exec(app.ContainerName)
}
