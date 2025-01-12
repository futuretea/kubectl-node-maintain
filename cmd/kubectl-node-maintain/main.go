package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/futuretea/kubectl-node-maintain/pkg/plugin"
)

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	configFlags := genericclioptions.NewConfigFlags(true)

	cmd := &cobra.Command{
		Use:   "node-maintain",
		Short: "Node maintenance plugin",
		Long:  `A kubectl plugin for managing Kubernetes node maintenance, supporting node pod management and cleanup operations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := configFlags.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("failed to get kubeconfig: %v", err)
			}

			p, err := plugin.NewPlugin(config)
			if err != nil {
				return fmt.Errorf("failed to create plugin: %v", err)
			}

			return p.Run()
		},
	}

	configFlags.AddFlags(cmd.Flags())
	return cmd
}
