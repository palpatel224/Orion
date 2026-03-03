package main

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "orionctl",
	Short: "Orion CLI controls the Orion Orchestrator",
	Long: `orionctl is a command line interface for managing tasks 
and worker nodes in the Orion distributed orchestrator.`,
}

// We will use this variable to store the Manager's API address
var managerAddr string

func init() {
	// This flag allows users to point the CLI to a different manager address
	// Example: orionctl get tasks --manager-addr http://192.168.1.5:5550
	rootCmd.PersistentFlags().StringVar(&managerAddr, "manager-addr", "http://localhost:5556", "Address of the Orion Manager API")
}
