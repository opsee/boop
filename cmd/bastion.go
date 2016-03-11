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
	"os"
	"regexp"
	"text/tabwriter"
	"time"
)

const uuidFormat = `^[a-z0-9]{8}-[a-z0-9]{4}-[1-5][a-z0-9]{3}-[a-z0-9]{4}-[a-z0-9]{12}$`
const emailFormat = `^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`
const tcpTimeout = time.Duration(3) * time.Second

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

type bastionInstance struct {
	Instance *ec2.Instance
	Creds    *credentials.Credentials
	Region   string
}

var svcs *bastionServices

// bastionCmd represents the bastion command
var bastionCmd = &cobra.Command{
	Use:   "bastion",
	Short: "opsee bastion managment commands",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
	},
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

		u := userResp.User
		log.INFO.Printf("user info: %s, %s, %s\n", u.Email, u.CustomerId, u.Name)

		bastionStates, err := getBastions(u.CustomerId)
		if err != nil {
			return err
		}

		yellow := color.New(color.FgYellow).SprintFunc()
		blue := color.New(color.FgBlue).SprintFunc()
		header := color.New(color.FgWhite).SprintFunc()

		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 1, 0, 2, ' ', 0)

		if len(bastionStates) > 0 {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", header("id"), "status",
				header("last seen"), header("region"))
		}

		for _, b := range bastionStates {
			reg := ""
			if b.Status == "active" {
				inst, err := findBastionInstance(u, b.Id)
				if err != nil {
					log.WARN.Printf("error finding bastion instance for %s\n", b.Id)
				} else {
					reg = inst.Region
				}
			}

			lastSeenDur := time.Since(time.Unix(b.LastSeen.Seconds, 0))
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", yellow(b.Id), b.Status, blue(roundDuration(lastSeenDur, time.Second)), reg)
		}
		w.Flush()

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

		bastionInstance, err := findBastionInstance(userResp.User, bastionID)
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

		bastionInstance, err := findBastionInstance(userResp.User, bastionID)
		if err != nil {
			return err
		}

		if bastionInstance != nil {
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

func findBastionInstance(user *schema.User, bastionID string) (*bastionInstance, error) {
	bastionStates, err := getBastions(user.CustomerId)
	if err != nil {
		return nil, err
	}

	var userCreds *opsee_aws_credentials.Value
	var foundBast bool
	for _, b := range bastionStates {
		if b.Id == bastionID {
			spanxResp, err := svcs.Spanx.GetCredentials(context.Background(), &service.GetCredentialsRequest{
				User: user,
			})
			if err != nil {
				return nil, err
			}
			userCreds = spanxResp.GetCredentials()
			foundBast = true
			break
		}
	}

	if !foundBast {
		return nil, NewSystemErrorF("cannot find bastion: %s", bastionID)
	}
	if userCreds == nil {
		return nil, NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
	}
	staticCreds := credentials.NewStaticCredentials(
		*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

	var instance *ec2.Instance
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
			return nil, err
		}
		for _, r := range descResponse.Reservations {
			for _, i := range r.Instances {
				for _, tag := range i.Tags {
					if *tag.Key == "opsee:id" {
						if *tag.Value == bastionID {
							instance = i
							bastionRegion = region
							break RegionLoop
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
	flags := BoopCmd.PersistentFlags()
	flags.BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag("verbose", flags.Lookup("verbose"))

	bastionCmd.AddCommand(bastionListCmd)
	flags := BoopCmd.Flags()
	flags.BoolP("active", "a", false, "list active only")
	viper.BindPFlag("list-active", flags.Lookup("active"))

	bastionCmd.AddCommand(bastionRestartCmd)

	bastionCmd.AddCommand(bastionTermCmd)
	flags = bastionTermCmd.Flags()
	flags.BoolP("dry-run", "n", false, "dry run")
	viper.BindPFlag("term-dry-run", flags.Lookup("dry-run"))
}
