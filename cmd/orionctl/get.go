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
	Short: "Display one or many resources (e.g., tasks, nodes)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resource := args[0]
		client := cli.NewClient(managerAddr)

		if resource == "tasks" || resource == "task" {
			tasks, err := client.GetTasks()
			if err != nil {
				fmt.Printf("Error fetching tasks: %v\n", err)
				os.Exit(1)
			}

			if len(tasks) == 0 {
				fmt.Println("No tasks found in the cluster.")
				return
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "TASK ID\tNAME\tSTATE\tIMAGE")

			for _, t := range tasks {
				idShort := t.ID
				if len(idShort) > 8 {
					idShort = idShort[:8]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", idShort, t.Name, t.State, t.Image)
			}
			w.Flush()

		} else if resource == "nodes" || resource == "node" {
			nodes, err := client.GetNodes()
			if err != nil {
				fmt.Printf("Error fetching nodes: %v\n", err)
				os.Exit(1)
			}

			if len(nodes) == 0 {
				fmt.Println("No active worker nodes found in the cluster.")
				return
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NODE ID\tADDRESS\tLAST HEARTBEAT")

			for _, n := range nodes {
				idShort := n.ID
				if len(idShort) > 8 {
					idShort = idShort[:8]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", idShort, n.Address, n.Heartbeat)
			}
			w.Flush()

		} else {
			fmt.Printf("Error: unknown resource '%s'. Valid resources: tasks, nodes\n", resource)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
