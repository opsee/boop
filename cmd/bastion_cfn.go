package cmd

import (
	"github.com/spf13/cobra"
)

var bastionCFN = &cobra.Command{
	Use:   "cfn",
	Short: "bastion cloud formation commands",
}

// TODO de-duplicate command code from commands
var bastionCFNUpdate = &cobra.Command{
	Use:   "update",
	Short: "update CFN template on a customer bastion",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
