package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aditip149209/Orion/pkg/cli"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get [resource]",
	Short: "Display one or many resources (e.g., tasks)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resource := args[0]

		if resource != "tasks" && resource != "task" {
			fmt.Printf("Error: unknown resource '%s'. Valid resources: tasks\n", resource)
			os.Exit(1)
		}

		client := cli.NewClient(managerAddr)
		tasks, err := client.GetTasks()
		if err != nil {
			fmt.Printf("Error fetching tasks: %v\n", err)
			os.Exit(1)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found in the cluster.")
			return
		}

		// Initialize tabwriter for clean column formatting
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "TASK ID\tNAME\tSTATE\tIMAGE")

		for _, t := range tasks {
			// Shorten the UUID for cleaner display
			idShort := t.ID
			if len(idShort) > 8 {
				idShort = idShort[:8]
			}

			// Print t.State directly! No mapping required.
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", idShort, t.Name, t.State, t.Image)
		}
		w.Flush() // Execute the write
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
