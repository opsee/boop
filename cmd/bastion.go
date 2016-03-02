package cmd

import (
	"crypto/tls"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/fatih/color"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/basic/schema"
	opsee_aws_credentials "github.com/opsee/basic/schema/aws/credentials"
	"github.com/opsee/basic/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	grpc_credentials "google.golang.org/grpc/credentials"
	"regexp"
	"time"
)

const uuidFormat = `^[a-z0-9]{8}-[a-z0-9]{4}-[1-5][a-z0-9]{3}-[a-z0-9]{4}-[a-z0-9]{12}$`
const emailFormat = `^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`
const tcpTimeout = time.Duration(1) * time.Second

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

type bastionServices struct {
	Vape     service.VapeClient
	Spanx    service.SpanxClient
	Keelhaul service.KeelhaulClient
}

var svcs *bastionServices

// bastionCmd represents the bastion command
var bastionCmd = &cobra.Command{
	Use:   "bastion",
	Short: "opsee bastion managment commands",
}

var bastionListCmd = &cobra.Command{
	Use:   "list [customer email|UUID]",
	Short: "list customer's bastions and their status",
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) < 1 {
			return NewUserError("missing argument")
		}

		email, uuid, err := parseUserID(args[0])
		if err != nil {
			return err
		}

		initServices()

		userResp, err := svcs.Vape.GetUser(context.Background(), &service.GetUserRequest{
			Email:      email,
			CustomerId: uuid,
		})
		if err != nil {
			return err
		}

		bastionStates, err := getBastions(userResp.User.CustomerId)
		if err != nil {
			return err
		}

		yellow := color.New(color.FgYellow).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		blue := color.New(color.FgBlue).SprintFunc()
		for _, b := range bastionStates {
			lastSeenDur := time.Since(time.Unix(b.LastSeen.Seconds, 0))
			fmt.Printf("%s %s %s\n", yellow(b.Id), blue(b.Status), red(roundDuration(lastSeenDur, time.Second)))
		}

		return nil
	},
}

var bastionRestartCmd = &cobra.Command{
	Use:   "restart [customer email|customer UUID] [bastion UUID]",
	Short: "restart a customer bastion",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return NewUserError("missing argument")
		}

		email, uuid, err := parseUserID(args[0])
		if err != nil {
			return err
		}

		uuidExp := regexp.MustCompile(uuidFormat)
		if !uuidExp.MatchString(args[1]) {
			return NewUserErrorF("invalid bastion ID: %s", args[1])
		}
		bastionID := args[1]

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		initServices()

		userResp, err := svcs.Vape.GetUser(context.Background(), &service.GetUserRequest{
			Email:      email,
			CustomerId: uuid,
		})
		if err != nil {
			return err
		}

		bastionStates, err := getBastions(userResp.User.CustomerId)
		if err != nil {
			return err
		}

		var userCreds *opsee_aws_credentials.Value
		for _, b := range bastionStates {
			if b.Id == bastionID {
				spanxResp, err := svcs.Spanx.GetCredentials(context.Background(), &service.GetCredentialsRequest{
					User: userResp.User,
				})
				if err != nil {
					return err
				}
				userCreds = spanxResp.GetCredentials()
			}
		}

		if userCreds == nil {
			return NewSystemErrorF("cannot obtain AWS creds for user: %s", userResp.User.Id)
		}
		staticCreds := credentials.NewStaticCredentials(
			*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

		var bastionInstance *ec2.Instance
		var bastionRegion string
		var ec2client *ec2.EC2

		// TODO lookup bastion's region somewhere to avoid scanning all
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
				return err
			}
			for _, r := range descResponse.Reservations {
				for _, i := range r.Instances {
					for _, tag := range i.Tags {
						if *tag.Key == "opsee:id" {
							if *tag.Value == bastionID {
								bastionInstance = i
								bastionRegion = region
								break RegionLoop
							}
						}
					}
				}
			}
		}

		if bastionInstance != nil {
			log.INFO.Printf("found bastion instance: %s in %s\n", *bastionInstance.InstanceId, bastionRegion)
			// REBOOT THIS MOTHER
			rebootResponse, err := ec2client.RebootInstances(&ec2.RebootInstancesInput{
				InstanceIds: []*string{bastionInstance.InstanceId},
			})
			if err != nil {
				return err
			}
			fmt.Printf("instance restart requested for: %s in %s\n", *bastionInstance.InstanceId, bastionRegion)
		}
		return nil
	},
}

func getBastions(custID string) (states []*schema.BastionState, err error) {
	keelResp, err := svcs.Keelhaul.ListBastionStates(context.Background(), &service.ListBastionStatesRequest{
		CustomerIds: []string{custID},
	})
	if err != nil {
		return nil, err
	}

	return keelResp.GetBastionStates(), nil
}

func roundDuration(d, r time.Duration) time.Duration {
	if r <= 0 {
		return d
	}
	neg := d < 0
	if neg {
		d = -d
	}
	if m := d % r; m+m < r {
		d = d - m
	} else {
		d = d + r - m
	}
	if neg {
		return -d
	}
	return d
}

func parseUserID(id string) (email string, uuid string, err error) {
	emailExp := regexp.MustCompile(emailFormat)
	uuidExp := regexp.MustCompile(uuidFormat)

	if emailExp.MatchString(id) {
		return id, "", nil
	}

	if uuidExp.MatchString(id) {
		return "", id, nil
	}

	return "", "", NewUserError("no email or UUID found in string")
}

func initServices() {
	conn, err := grpc.Dial("vape.in.opsee.com:443",
		grpc.WithTransportCredentials(grpc_credentials.NewTLS(&tls.Config{})),
		grpc.WithTimeout(tcpTimeout),
		grpc.WithBlock())
	if err != nil {
		log.ERROR.Fatal(err)
	}
	vape := service.NewVapeClient(conn)

	conn, err = grpc.Dial("spanx.in.opsee.com:8443",
		grpc.WithTransportCredentials(grpc_credentials.NewTLS(&tls.Config{})),
		grpc.WithTimeout(tcpTimeout))
	if err != nil {
		panic(err)
	}
	spanx := service.NewSpanxClient(conn)

	conn, err = grpc.Dial("keelhaul.in.opsee.com:443",
		grpc.WithTransportCredentials(grpc_credentials.NewTLS(&tls.Config{})),
		grpc.WithTimeout(tcpTimeout))
	if err != nil {
		panic(err)
	}
	keelhaul := service.NewKeelhaulClient(conn)

	svcs = &bastionServices{
		Vape:     vape,
		Spanx:    spanx,
		Keelhaul: keelhaul,
	}
}

func init() {
	log.SetLogFlag(log.SFILE)

	BoopCmd.AddCommand(bastionCmd)

	bastionCmd.AddCommand(bastionListCmd)
	bastionCmd.AddCommand(bastionRestartCmd)

	flags := BoopCmd.PersistentFlags()
	flags.BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag("verbose", flags.Lookup("verbose"))
}