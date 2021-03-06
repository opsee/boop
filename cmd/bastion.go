package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/fatih/color"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/basic/schema"
	"github.com/opsee/basic/service"
	"github.com/opsee/boop/errors"
	"github.com/opsee/boop/svc"
	"github.com/opsee/boop/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"text/tabwriter"
	"time"
)

type bastionInstance struct {
	Instance *ec2.Instance
	Creds    *credentials.Credentials
	Region   string
}

// bastionCmd represents the bastion command
var bastionCmd = &cobra.Command{
	Use:   "bastion",
	Short: "opsee bastion managment commands",
}

var bastionListCmd = &cobra.Command{
	Use:   "list [customer email|UUID]",
	Short: "list customer's bastions and their status",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		var (
			bastionStates []*schema.BastionState
			err           error
		)

		if viper.GetBool("list-active") {
			bastionStates, err = opseeServices.GetBastionStates([]string{}, &service.Filter{
				Key:   "status",
				Value: "active",
			})
			if err != nil {
				return err
			}

		} else {
			u, err := util.GetUserFromArgs(args, 0, opseeServices)
			if err != nil {
				return err
			}

			bastionStates, err = opseeServices.GetBastionStates([]string{u.CustomerId})
			if err != nil {
				return err
			}

			log.INFO.Printf("user info: %s, %s, %s\n", u.Email, u.CustomerId, u.Name)
		}

		yellow := color.New(color.FgYellow).SprintFunc()
		blue := color.New(color.FgBlue).SprintFunc()
		header := color.New(color.FgWhite).SprintFunc()

		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 1, 0, 2, ' ', 0)

		if len(bastionStates) > 0 && !viper.GetBool("quiet") {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", header("customer_id"), header("id"), header("status"),
				header("last seen"), header("region"))
		}

		for _, b := range bastionStates {
			lastSeenDur := time.Since(time.Unix(b.LastSeen.Seconds, 0))
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", b.CustomerId, yellow(b.Id), b.Status,
				blue(roundDuration(lastSeenDur, time.Second)), b.Region)
		}
		w.Flush()

		return nil
	},
}

var bastionRestartCmd = &cobra.Command{
	Use:   "restart [customer email|customer UUID] [bastion UUID]",
	Short: "restart a customer bastion",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		bastionID, err := util.GetUUIDFromArgs(args, 1)
		if err != nil {
			return err
		}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		bastionInstance, err := findBastionInstance(u, *bastionID, opseeServices)
		if err != nil {
			return err
		}

		if bastionInstance != nil {
			log.INFO.Printf("found bastion instance: %s in %s\n", *bastionInstance.Instance.InstanceId, bastionInstance.Region)
			ec2client := ec2.New(session.New(&aws.Config{
				Credentials: bastionInstance.Creds,
				MaxRetries:  aws.Int(3),
				Region:      &bastionInstance.Region,
			}))
			// REBOOT THIS MOTHER
			_, err := ec2client.RebootInstances(&ec2.RebootInstancesInput{
				InstanceIds: []*string{bastionInstance.Instance.InstanceId},
			})
			if err != nil {
				return err
			}
			fmt.Printf("instance restart requested for: %s in %s\n", *bastionInstance.Instance.InstanceId, bastionInstance.Region)
		}
		return nil
	},
}

var bastionTermCmd = &cobra.Command{
	Use:   "terminate [customer email|customer UUID] [bastion UUID]",
	Short: "terminate a customer bastion",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		bastionID, err := util.GetUUIDFromArgs(args, 1)
		if err != nil {
			return err
		}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		bastionInstance, err := findBastionInstance(u, *bastionID, opseeServices)
		if err != nil {
			return err
		}

		if bastionInstance.Instance != nil {
			log.INFO.Printf("found bastion instance: %s in %s\n", *bastionInstance.Instance.InstanceId, bastionInstance.Region)
			var err error
			ec2client := ec2.New(session.New(&aws.Config{
				Credentials: bastionInstance.Creds,
				MaxRetries:  aws.Int(3),
				Region:      &bastionInstance.Region,
			}))
			if !viper.GetBool("term-dry-run") {
				// TERM THIS MOTHER
				_, err = ec2client.TerminateInstances(&ec2.TerminateInstancesInput{InstanceIds: []*string{bastionInstance.Instance.InstanceId}})
			}
			if err != nil {
				return err
			}
			fmt.Printf("instance termination requested for: %s in %s\n", *bastionInstance.Instance.InstanceId, bastionInstance.Region)
			if viper.GetBool("term-dry-run") {
				fmt.Println("(but not really bc dry-run)")
			}
		}
		return nil
	},
}

func findBastionInstance(user *schema.User, bastionID string, opseeServices *svc.OpseeServices) (*bastionInstance, error) {
	bastionStates, err := opseeServices.GetBastionStates([]string{user.CustomerId})
	if err != nil {
		return nil, err
	}

	var foundBast bool
	for _, b := range bastionStates {
		if b.Id == bastionID {
			foundBast = true
			break
		}
	}
	if !foundBast {
		return nil, errors.NewSystemErrorF("cannot find bastion: %s", bastionID)
	}

	userCreds, err := opseeServices.GetRoleCreds(user)
	if err != nil {
		return nil, errors.NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
	}
	staticCreds := credentials.NewStaticCredentials(
		*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

	var instance *ec2.Instance
	var bastionRegion string
	var ec2client *ec2.EC2

	if viper.GetBool("verbose") {
		log.SetStdoutThreshold(log.LevelInfo)
	}

	// TODO store bastion's region somewhere to avoid scanning
RegionLoop:
	for _, region := range regionList {
		log.INFO.Printf("checking %s\n", region)
		ec2client = ec2.New(session.New(&aws.Config{
			Credentials: staticCreds,
			MaxRetries:  aws.Int(3),
			Region:      &region,
		}))

		descResponse, err := ec2client.DescribeInstances(&ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []*string{aws.String("opsee:id")},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, r := range descResponse.Reservations {
			for _, i := range r.Instances {
				for _, tag := range i.Tags {
					if *tag.Key == "opsee:id" {
						if *tag.Value == bastionID {
							instance = i
							bastionRegion = region
							if *i.State.Name == "running" {
								break RegionLoop
							}
						}
					}
				}
			}
		}
	}

	return &bastionInstance{
		Instance: instance,
		Creds:    staticCreds,
		Region:   bastionRegion,
	}, nil
}

func init() {
	log.SetLogFlag(log.SFILE)

	BoopCmd.AddCommand(bastionCmd)

	bastionCmd.AddCommand(bastionListCmd)
	flags := bastionListCmd.Flags()
	flags.BoolP("active", "a", false, "list all active bastions")
	viper.BindPFlag("list-active", flags.Lookup("active"))
	flags.BoolP("quiet", "q", false, "silent output")
	viper.BindPFlag("quiet", flags.Lookup("quiet"))

	bastionCmd.AddCommand(bastionRestartCmd)

	bastionCmd.AddCommand(bastionTermCmd)
	flags = bastionTermCmd.Flags()
	flags.BoolP("dry-run", "n", false, "dry run")
	viper.BindPFlag("term-dry-run", flags.Lookup("dry-run"))
}
