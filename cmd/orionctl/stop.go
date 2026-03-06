package main

import (
	"fmt"
	"os"

	"github.com/aditip149209/Orion/pkg/cli"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <task-id>",
	Short: "Gracefully stop a running or pending task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]

		client := cli.NewClient(managerAddr)
		err := client.StopTask(taskID)

		if err != nil {
			fmt.Printf("Failed to stop task %s: %v\n", taskID, err)
			os.Exit(1)
		}

		fmt.Printf("Task %s successfully scheduled for termination.\n", taskID)
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
