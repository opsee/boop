package cmd

import (
	"github.com/spf13/cobra"
	"os"
)

var BoopCmd = &cobra.Command{
	Use: "boop",
}

func Execute() {
	BoopCmd.SilenceUsage = true

	if c, err := BoopCmd.ExecuteC(); err != nil {
		if IsUserError(err) {
			c.Println("")
			c.Println(c.UsageString())
		}

		os.Exit(-1)
	}
}
