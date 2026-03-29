package main

import (
	"fmt"
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
			fmt.Println("Error: please provide a task manifest file using -f")
			os.Exit(1)
		}

		payload, kind, err := parseSubmissionFile(taskFile)
		if err != nil {
			fmt.Printf("Error parsing manifest: %v\n", err)
			os.Exit(1)
		}

		client := cli.NewClient(managerAddr)
		switch kind {
		case submissionKindTask:
			err = client.SubmitTask(payload)
			if err != nil {
				fmt.Printf("Failed to submit task: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Task submitted successfully!")
		case submissionKindApp:
			err = client.SubmitApp(payload)
			if err != nil {
				fmt.Printf("Failed to submit app: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("App submitted successfully!")
		default:
			fmt.Printf("Error: unsupported manifest kind %q\n", kind)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	// Bind the -f flag to the taskFile variable
	runCmd.Flags().StringVarP(&taskFile, "file", "f", "", "Path to task manifest file (.json/.yaml/.yml) (required)")
	runCmd.MarkFlagRequired("file")
}
