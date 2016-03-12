package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/boop/errors"
	"github.com/opsee/boop/svc"
	"github.com/opsee/boop/util"
	"github.com/opsee/keelhaul/scanner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// this is necessary due to the aws api being wrong
	attachmentStatusAvailable = "available"
	theInternet               = "0.0.0.0/0"
)

var scanCmd = &cobra.Command{
	Use:   "scan [customer email|UUID]",
	Short: "scan a customer's env",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}
		log.INFO.Printf("user info: %s, %s, %s\n", u.Email, u.CustomerId, u.Name)

		if !viper.IsSet("scan-region") {
			return errors.NewUserError("required option not set: region")
		}
		region := viper.GetString("scan-region")

		userCreds, err := opseeServices.GetRoleCreds(u)
		if err != nil {
			return errors.NewSystemErrorF("cannot obtain AWS creds for user: %s", u.Id)
		}

		staticCreds := credentials.NewStaticCredentials(
			*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

		session := session.New(aws.NewConfig().
			WithCredentials(staticCreds).
			WithRegion(region).WithMaxRetries(5))

		regionScan, err := scanner.ScanRegion(region, session)
		if err != nil {
			return err
		}

		for _, v := range regionScan.VPCs {
			fmt.Printf("%s (%d instances, default=%t)\n", *v.VpcId, v.InstanceCount, *v.IsDefault)
			for _, s := range regionScan.Subnets {
				if *s.VpcId == *v.VpcId {
					fmt.Printf("  %s (%s, %d instances, %s)\n", *s.SubnetId, *s.AvailabilityZone, s.InstanceCount, s.Routing)
				}
			}
		}

		return nil
	},
}

func init() {
	BoopCmd.AddCommand(scanCmd)
	flags := scanCmd.Flags()
	flags.StringP("region", "r", "", "scan region")
	viper.BindPFlag("scan-region", flags.Lookup("region"))
}
