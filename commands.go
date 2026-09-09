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

			exists, err := app.Client.ContainerExists(app.ContainerName)
			if err != nil {
				return err
			}
			if exists {
				fmt.Printf("Container %s is already running\n", app.ContainerName)
				return nil
			}

			fmt.Printf("Starting new container %s\n", app.ContainerName)
			return app.Client.Run(app.Config, app.ContainerName, app.Binds, app.Root)
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
			removed, err := app.Client.Delete(app.ContainerName)
			if err != nil {
				return err
			}
			if removed {
				fmt.Printf("Killed container %s\n", app.ContainerName)
			} else {
				fmt.Printf("No container %s to kill\n", app.ContainerName)
			}
			return nil
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

			removed, err := app.Client.Delete(app.ContainerName)
			if err != nil {
				return err
			}
			if removed {
				fmt.Printf("Killed container %s\n", app.ContainerName)
			} else {
				fmt.Printf("No existing container %s\n", app.ContainerName)
			}

			fmt.Printf("Starting new container %s\n", app.ContainerName)
			return app.Client.Run(app.Config, app.ContainerName, app.Binds, app.Root)
		},
	}
}

func execCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "exec [command] [args...]",
		Short:              "Execute a command in the container",
		Long:               "Execute a command in the container. Flags after exec are passed through (no `--` needed).",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 0 {
				return fmt.Errorf("exec requires a command\nExample: dev106 exec make -j4")
			}

			app, err := newApp(false)
			if err != nil {
				return err
			}

			if err := ensureContainer(app); err != nil {
				return err
			}

			return app.Client.ExecCmd(app.ContainerName, args)
		},
	}
}

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open shell in container (same as running dev106 with no args)",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(false)
			if err != nil {
				return err
			}
			return shell(app)
		},
	}
}

func ensureContainer(app *App) error {
	exists, err := app.Client.ContainerExists(app.ContainerName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	fmt.Printf("Starting new container %s\n", app.ContainerName)
	return app.Client.Run(app.Config, app.ContainerName, app.Binds, app.Root)
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all dev106-managed containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(true)
			if err != nil {
				return err
			}
			items, err := app.Client.ListManaged()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No dev106 containers.")
				return nil
			}
			fmt.Printf("%-36s  %-16s  %s\n", "NAME", "STATUS", "ROOT")
			for _, item := range items {
				root := item.Root
				if root == "" {
					root = "-"
				}
				fmt.Printf("%-36s  %-16s  %s\n", item.Name, item.Status, root)
			}
			return nil
		},
	}
}

func nukeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "nuke",
		Short: "Kill every container managed by dev106",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(true)
			if err != nil {
				return err
			}
			removed, err := app.Client.NukeManaged()
			if err != nil {
				return err
			}
			if len(removed) == 0 {
				fmt.Println("No dev106 containers to nuke.")
				return nil
			}
			for _, name := range removed {
				fmt.Printf("Killed %s\n", name)
			}
			fmt.Printf("Nuked %d container(s).\n", len(removed))
			return nil
		},
	}
}

func shell(app *App) error {
	if err := ensureContainer(app); err != nil {
		return err
	}
	return app.Client.Exec(app.ContainerName)
}
