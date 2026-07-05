package main

import (
	"fmt"
	"io"
	"os"

	"github.com/aditip149209/Orion/pkg/cli"

	"github.com/spf13/cobra"
)

var taskFile string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Submit a new task to the cluster",
	Run: func(cmd *cobra.Command, args []string) {
		if taskFile == "" {
			fmt.Println("Error: please provide a task JSON file using -f")
			os.Exit(1)
		}

		file, err := os.Open(taskFile)
		if err != nil {
			fmt.Printf("Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		client := cli.NewClient(managerAddr)
		err = client.SubmitTask(data)
		if err != nil {
			fmt.Printf("Failed to submit task: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Task submitted successfully!")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	// Bind the -f flag to the taskFile variable
	runCmd.Flags().StringVarP(&taskFile, "file", "f", "", "Path to task JSON file (required)")
	runCmd.MarkFlagRequired("file")
}
