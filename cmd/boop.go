package cmd

import (
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/boop/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

// TODO read from config
var regionList = []string{
	"us-west-1",
	"us-west-2",
	"us-east-1",
	"eu-west-1",
	"eu-central-1",
	"sa-east-1",
	"ap-southeast-1",
	"ap-southeast-2",
}

var BoopCmd = &cobra.Command{
	Use: "boop",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}
	},
}

func Execute() {
	BoopCmd.SilenceUsage = true

	flags := BoopCmd.PersistentFlags()
	flags.BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag("verbose", flags.Lookup("verbose"))

	if c, err := BoopCmd.ExecuteC(); err != nil {
		if errors.IsUserError(err) {
			c.Println(c.UsageString())
		}

		os.Exit(-1)
	}
}
