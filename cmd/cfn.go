package cmd

import (
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/fatih/color"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/basic/schema"
	"github.com/opsee/boop/errors"
	"github.com/opsee/boop/svc"
	"github.com/opsee/boop/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"text/tabwriter"
)

const (
	secGrpTemplate = "bastion-ingress-cf.template"
	cfnTemplate    = "bastion-cf.template"
	cfnS3BucketURL = "https://s3%s%s.amazonaws.com/opsee-bastion-cf-%s/beta"
	badUserdata    = `        - name: "10-cgroupfs.conf"
          content: |
            [Service]
            Environment="DOCKER_OPTS=--exec-opt=native.cgroupdriver=cgroupfs"
`
)

type cfnStack struct {
	Creds  *credentials.Credentials
	Region string
	Stack  *cloudformation.Stack
}

func (s cfnStack) getStackParams(amiId string) ([]*cloudformation.Parameter, error) {
	params := []*cloudformation.Parameter{
		{
			ParameterKey:     aws.String("InstanceType"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("VpcId"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("SubnetId"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("AssociatePublicIpAddress"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("CustomerId"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("BastionId"),
			UsePreviousValue: aws.Bool(true),
		},
	}

	if amiId != "" {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String("ImageId"),
			ParameterValue: aws.String(amiId),
		})
	} else {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:     aws.String("ImageId"),
			UsePreviousValue: aws.Bool(true),
		})
	}

	if viper.IsSet("cfnup-allow-ssh") {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String("AllowSSH"),
			ParameterValue: aws.String("True"),
		})
	} else {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String("AllowSSH"),
			ParameterValue: aws.String("False"),
		})
	}

	if viper.GetBool("userdata") {
		userdata, err := s.getUserdata()
		if err != nil {
			return nil, err
		}

		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String("UserData"),
			ParameterValue: aws.String(base64.StdEncoding.EncodeToString([]byte(userdata))),
		})
	} else {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:     aws.String("UserData"),
			UsePreviousValue: aws.Bool(true),
		})
	}

	return params, nil
}

func (s cfnStack) getCFNTemplate() ([]byte, error) {
	resp, err := http.Get(s.getS3URL(cfnTemplate))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func (s cfnStack) getS3URL(template string) string {
	sep := "-"
	reg := s.Region

	bucketUrl := fmt.Sprintf(cfnS3BucketURL, sep, reg, reg)
	// p cool exception
	if s.Region == "us-east-1" {
		bucketUrl = fmt.Sprintf(cfnS3BucketURL, "", "", reg)
	}

	u, err := url.Parse(bucketUrl)
	if err != nil {
		log.WARN.Printf("failed to parse: %s\n", bucketUrl)
		return ""
	}

	u.Path = path.Join(u.Path, template)

	return u.String()
}

func (s cfnStack) getUserdata() (string, error) {
	if s.Stack != nil {
		for _, p := range s.Stack.Parameters {
			if aws.StringValue(p.ParameterKey) == "UserData" {
				data, err := base64.StdEncoding.DecodeString(aws.StringValue(p.ParameterValue))
				if err != nil {
					return "", err
				}

				return strings.Replace(string(data), badUserdata, "", 1), nil
			}
		}
	}

	return "", fmt.Errorf("no stack found")

}

var cfnCommand = &cobra.Command{
	Use:   "cfn",
	Short: "bastion cloud formation commands",
}

var cfnUserdata = &cobra.Command{
	Use:   "userdata [customer email|customer UUID]",
	Short: "show a cloudformation stack's userdata",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		stackName := "opsee-stack-" + u.CustomerId
		stack, err := findStack(u, stackName, opseeServices)
		if err != nil {
			return err
		}

		ud, err := stack.getUserdata()
		if err != nil {
			return err
		}

		fmt.Println(ud)
		return nil
	},
}

var cfnUpdate = &cobra.Command{
	Use:   "update [customer email|customer UUID]",
	Short: "update CFN template for a customer bastion stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		stackName := "opsee-stack-" + u.CustomerId
		stack, err := findStack(u, stackName, opseeServices)
		if err != nil {
			return err
		}

		if stack.Stack != nil {
			log.INFO.Printf("found stack: %s in %s\n", *stack.Stack.StackId, stack.Region)

			templateBytes, err := stack.getCFNTemplate()
			if err != nil {
				return err
			}

			var amiId string
			if viper.GetBool("latest") {
				log.INFO.Printf("requesting latest image in region: %s", stack.Region)
				imageList, err := getAMIList(stack.Region, "stable")
				if err != nil {
					return err
				}

				if len(imageList) == 0 {
					return fmt.Errorf("no images found")
				}

				amiId = aws.StringValue(imageList[0].ImageId)
			} else {
				amiId = viper.GetString("cfnup-ami-id")
			}

			log.INFO.Printf("updating with image id: %s", amiId)

			params, err := stack.getStackParams(amiId)
			if err != nil {
				return err
			}

			cfnClient := cloudformation.New(session.New(),
				aws.NewConfig().WithCredentials(stack.Creds).WithRegion(stack.Region).WithMaxRetries(10))

			_, err = cfnClient.UpdateStack(&cloudformation.UpdateStackInput{
				StackName:    aws.String(stackName),
				TemplateBody: aws.String(string(templateBytes)),
				Capabilities: []*string{
					aws.String("CAPABILITY_IAM"),
				},
				Parameters: params,
			})
			if err != nil {
				return err
			}

			fmt.Printf("requested stack update\n")
			if viper.GetBool("cfnup-wait") {
				// TODO replace with better waiter
				err = cfnClient.WaitUntilStackUpdateComplete(&cloudformation.DescribeStacksInput{
					StackName: aws.String(stackName),
				})
				if err != nil {
					return err
				}

				descResponse, err := cfnClient.DescribeStacks(&cloudformation.DescribeStacksInput{
					StackName: aws.String(stackName),
				})
				if err != nil {
					return err
				}

				if len(descResponse.Stacks) > 1 {
					return errors.NewSystemErrorF("multiple opsee stacks found for cust %s in ", u.CustomerId)
				}

				if len(descResponse.Stacks) == 0 {
					return errors.NewSystemErrorF("cannot find opsee stack for cust: %s", u.CustomerId)
				}

				stk := descResponse.Stacks[0]
				fmt.Printf("update complete: %s, %s\n", *stk.StackStatus, *stk.StackStatusReason)
			}
		}

		return nil
	},
}

var cfnPrint = &cobra.Command{
	Use:   "print [customer email|customer UUID]",
	Short: "print CFN info for customer bastion stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		stackName := "opsee-stack-" + u.CustomerId
		stack, err := findStack(u, stackName, opseeServices)
		if err != nil {
			return err
		}

		if stack.Stack == nil {
			return errors.NewUserErrorF("stack %s not found", stackName)
		}

		fmt.Printf("requesting stack deletion for %s\n", *stack.Stack.StackName)
		fmt.Printf("name: %s\n", *stack.Stack.StackName)
		fmt.Printf("region: %s\n", stack.Region)
		fmt.Println("stack params: ")
		for _, p := range stack.Stack.Parameters {
			fmt.Printf("   %s: %s\n", *p.ParameterKey, *p.ParameterValue)
		}
		fmt.Println("stack tags: ")
		for _, t := range stack.Stack.Tags {
			fmt.Printf("   %s: %s\n", *t.Key, *t.Value)
		}
		return nil
	},
}

var cfnEvents = &cobra.Command{
	Use:   "events [customer email|customer UUID]",
	Short: "list recent CFN events for a customer's bastions",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		if viper.GetBool("verbose") {
			log.SetStdoutThreshold(log.LevelInfo)
		}

		stackName := "opsee-stack-" + u.CustomerId
		if viper.IsSet("list-events-stack-name") {
			stackName = viper.GetString("list-events-stack-name")
		}
		stack, err := findStack(u, stackName, opseeServices)
		if err != nil {
			return err
		}

		if stack.Stack != nil {
			log.INFO.Printf("found bastion stack: %s in %s\n", *stack.Stack.StackId, stack.Region)

			cfnClient := cloudformation.New(session.New(), aws.NewConfig().WithCredentials(stack.Creds).WithRegion(stack.Region))
			resp, err := cfnClient.DescribeStackEvents(&cloudformation.DescribeStackEventsInput{
				StackName: aws.String(stackName),
			})
			if err != nil {
				return err
			}

			w := new(tabwriter.Writer)
			w.Init(os.Stdout, 1, 0, 2, ' ', 0)
			yellow := color.New(color.FgYellow).SprintFunc()
			blue := color.New(color.FgBlue).SprintFunc()
			header := color.New(color.FgWhite).SprintFunc()

			vinfoHead := ""
			if viper.GetBool("verbose") {
				vinfoHead = "extra info"
			}
			if len(resp.StackEvents) > 0 {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", header("time"), "status", header("resource"), "reason", vinfoHead)
			}
			for i, e := range resp.StackEvents {
				if i > viper.GetInt("list-events-num") {
					break
				}
				t := *e.Timestamp
				event := *e.ResourceStatus
				eres := *e.LogicalResourceId
				ereason := ""
				vinfo := ""
				if e.ResourceStatusReason != nil {
					ereason = *e.ResourceStatusReason
				}
				if viper.GetBool("verbose") {
					vinfo = fmt.Sprintf("%s", *e.PhysicalResourceId)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", yellow(t), event, blue(eres), ereason, vinfo)
			}
			w.Flush()

		}
		return nil
	},
}

func updateStackParam(params []*cloudformation.Parameter, key string, newValue string) {
	for _, p := range params {
		if *p.ParameterKey == key {
			*p.ParameterValue = newValue
		}
	}
}

func findStack(user *schema.User, stackname string, opseeServices *svc.OpseeServices) (*cfnStack, error) {
	userCreds, err := opseeServices.GetRoleCreds(user)
	if err != nil {
		return nil, errors.NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
	}

	staticCreds := credentials.NewStaticCredentials(
		*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

	stack := &cfnStack{
		Creds: staticCreds,
	}

	for _, region := range regionList {
		log.INFO.Printf("checking %s\n", region)
		// TODO reuse existing client/session
		cfnClient := cloudformation.New(session.New(), aws.NewConfig().WithCredentials(staticCreds).WithRegion(region))
		descResponse, _ := cfnClient.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackname),
		})

		if len(descResponse.Stacks) > 1 {
			return nil, errors.NewSystemErrorF("multiple opsee stacks found for cust %s in %s", user.CustomerId, region)
		}

		if len(descResponse.Stacks) > 0 {
			stack.Stack = descResponse.Stacks[0]
			stack.Region = region
			break
		}
	}

	return stack, nil
}

func ptos(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func init() {
	BoopCmd.AddCommand(cfnCommand)

	cfnCommand.AddCommand(cfnPrint)

	cfnCommand.AddCommand(cfnEvents)
	flags := cfnEvents.Flags()
	flags.IntP("num", "n", 10, "max number of events to display")
	viper.BindPFlag("list-events-num", flags.Lookup("num"))
	flags.StringP("stack", "s", "", "stack name to display instead of default opsee-stack)")
	viper.BindPFlag("list-events-stack-name", flags.Lookup("stack"))

	cfnCommand.AddCommand(cfnUpdate)
	flags = cfnUpdate.Flags()
	flags.BoolP("allow-ssh", "s", false, "allow ssh to bastion")
	viper.BindPFlag("cfnup-allow-ssh", flags.Lookup("allow-ssh"))
	flags.StringP("ami-id", "i", "", "use this AMI instead of template default")
	viper.BindPFlag("cfnup-ami-id", flags.Lookup("ami-id"))
	flags.BoolP("wait", "w", false, "wait for update to complate")
	viper.BindPFlag("cfnup-wait", flags.Lookup("wait"))
	flags.BoolP("userdata", "u", false, "refresh userdata")
	viper.BindPFlag("userdata", flags.Lookup("userdata"))
	flags.BoolP("latest", "l", false, "use latest stable ami in this region")
	viper.BindPFlag("latest", flags.Lookup("latest"))

	cfnCommand.AddCommand(cfnUserdata)
}
