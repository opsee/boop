package cmd

import (
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
	"text/tabwriter"
	"time"
)

const cfnS3BucketURL = "https://s3%s%s.amazonaws.com/opsee-bastion-cf-%s/beta"
const cfnTemplate = "bastion-cf.template"
const secGrpTemplate = "bastion-ingress-cf.template"

type cfnStack struct {
	Creds  *credentials.Credentials
	Region string
	Stack  *cloudformation.Stack
}

func (s cfnStack) getStackParams() []*cloudformation.Parameter {
	params := []*cloudformation.Parameter{
		{
			ParameterKey:     aws.String("InstanceType"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("UserData"),
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
		{
			ParameterKey:     aws.String("OpseeRole"),
			UsePreviousValue: aws.Bool(true),
		},
	}

	if viper.IsSet("cfnup-ami-id") {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String("ImageId"),
			ParameterValue: aws.String(viper.GetString("cfnup-ami-id")),
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

	return params
}

func (s cfnStack) makeStackTags(user *schema.User) []*cloudformation.Tag {
	tags := []*cloudformation.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String("Opsee Stack"),
		},
		{
			Key:   aws.String("vendor"),
			Value: aws.String("opsee"),
		},
		{
			Key:   aws.String("opsee:customer-id"),
			Value: aws.String(user.CustomerId),
		},
	}

	return tags
}

func (s cfnStack) makeStackParams(user *schema.User) []*cloudformation.Parameter {
	assocIP := "False"
	if viper.GetBool("cfncreate-assoc-ip") {
		assocIP = "True"
	}
	allowSSH := "False"
	if viper.GetBool("cfncreate-allow-ssh") {
		allowSSH = "True"
	}
	params := []*cloudformation.Parameter{
		{
			ParameterKey:   aws.String("InstanceType"),
			ParameterValue: aws.String(viper.GetString("cfncreate-inst-type")),
		},
		{
			// TODO get userdata from file
			ParameterKey:   aws.String("UserData"),
			ParameterValue: aws.String(""),
		},
		{
			ParameterKey:   aws.String("VpcId"),
			ParameterValue: aws.String(viper.GetString("cfncreate-vpc")),
		},
		{
			ParameterKey:   aws.String("SubnetId"),
			ParameterValue: aws.String(viper.GetString("cfncreate-subnet")),
		},
		{
			ParameterKey:   aws.String("AssociatePublicIpAddress"),
			ParameterValue: aws.String(assocIP),
		},
		{
			ParameterKey:   aws.String("ImageId"),
			ParameterValue: aws.String(viper.GetString("cfncreate-ami")),
		},
		{
			ParameterKey:   aws.String("AllowSSH"),
			ParameterValue: aws.String(allowSSH),
		},
		{
			ParameterKey:   aws.String("CustomerId"),
			ParameterValue: aws.String(user.CustomerId),
		},
		{
			ParameterKey:   aws.String("BastionId"),
			ParameterValue: aws.String(viper.GetString("cfncreate-bastion")),
		},
		{
			ParameterKey:   aws.String("OpseeRole"),
			ParameterValue: aws.String(fmt.Sprintf("opsee-role-%s", user.CustomerId)),
		},
	}

	return params
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

var cfnCommand = &cobra.Command{
	Use:   "cfn",
	Short: "bastion cloud formation commands",
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

			cfnClient := cloudformation.New(session.New(),
				aws.NewConfig().WithCredentials(stack.Creds).WithRegion(stack.Region).WithMaxRetries(10))
			_, err = cfnClient.UpdateStack(&cloudformation.UpdateStackInput{
				StackName:    aws.String(stackName),
				TemplateBody: aws.String(string(templateBytes)),
				Capabilities: []*string{
					aws.String("CAPABILITY_IAM"),
				},
				Parameters: stack.getStackParams(),
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

var cfnCreate = &cobra.Command{
	Use:   "create [customer email|customer UUID]",
	Short: "create new CFN stack from current template for registered user",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		u, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		// TODO find better way to set required options
		if !viper.IsSet("cfncreate-region") {
			return errors.NewUserError("required option not set: region")
		}
		region := viper.GetString("cfncreate-region")
		if !viper.IsSet("cfncreate-vpc") {
			return errors.NewUserError("required option not set: vpc")
		}
		if !viper.IsSet("cfncreate-subnet") {
			return errors.NewUserError("required option not set: subnet")
		}
		if !viper.IsSet("cfncreate-ami") {
			return errors.NewUserError("required option not set: ami")
		}
		if !viper.IsSet("cfncreate-bastion") {
			return errors.NewUserError("required option not set: bastion")
		}

		stackName := "opsee-stack-" + u.CustomerId
		stack, err := findStack(u, stackName, opseeServices)
		if err != nil {
			return err
		}

		if stack.Stack != nil {
			return errors.NewUserError(fmt.Sprintf("stack %s already exists in %s", stackName, stack.Region))
		}

		stack.Region = region
		templateBytes, err := stack.getCFNTemplate()
		if err != nil {
			return err
		}
		_ = templateBytes

		cfnClient := cloudformation.New(session.New(),
			aws.NewConfig().WithCredentials(stack.Creds).WithRegion(stack.Region).WithMaxRetries(10))

		params := stack.makeStackParams(u)
		tags := stack.makeStackTags(u)

		_, err = cfnClient.CreateStack(&cloudformation.CreateStackInput{
			StackName:    aws.String(stackName),
			TemplateBody: aws.String(string(templateBytes)),
			Capabilities: []*string{
				aws.String("CAPABILITY_IAM"),
			},
			Parameters: params,
			Tags:       tags,
		})
		if err != nil {
			return err
		}

		if viper.GetBool("cfncreate-wait") {
			for {
				descResp, err := cfnClient.DescribeStacks(&cloudformation.DescribeStacksInput{
					StackName: aws.String(stackName),
				})
				if err != nil {
					return err
				}
				createdStack := descResp.Stacks[0]
				if createdStack == nil {
					log.WARN.Printf("unable to describe stack %s\n", stackName)
					break
				}

				createStatus := ptos(createdStack.StackStatus)
				fmt.Printf("%s status: %s\n", *createdStack.StackName, createStatus)
				if createStatus == "CREATE_COMPLETE" || createStatus == "ROLLBACK_COMPLETE" {
					break
				}

				time.Sleep(5 * time.Second)
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

	/* TODO finish impl
	cfnCommand.AddCommand(cfnCreate)
	flags = cfnCreate.Flags()
	// start of many create stack params
	flags.StringP("region", "r", "", "create stack in region")
	viper.BindPFlag("cfncreate-region", flags.Lookup("region"))

	flags.StringP("bastion", "b", "", "bastion id")
	viper.BindPFlag("cfncreate-bastion", flags.Lookup("bastion"))

	flags.StringP("subnet", "s", "", "subnet id")
	viper.BindPFlag("cfncreate-subnet", flags.Lookup("subnet"))

	flags.StringP("vpc", "p", "", "vpc id")
	viper.BindPFlag("cfncreate-vpc", flags.Lookup("vpc"))

	flags.StringP("instance-type", "t", "t2.micro", "instance type")
	viper.BindPFlag("cfncreate-inst-type", flags.Lookup("instance-type"))

	flags.BoolP("assoc-ip", "", false, "associate public ip")
	viper.BindPFlag("cfncreate-assoc-ip", flags.Lookup("assoc-ip"))

	flags.StringP("ami", "i", "", "AMI id")
	viper.BindPFlag("cfncreate-ami", flags.Lookup("ami"))

	flags.BoolP("allow-ssh", "", false, "allow ssh")
	viper.BindPFlag("cfncreate-allow-ssh", flags.Lookup("allow-ssh"))

	flags.BoolP("wait", "w", false, "wait for create to complate")
	viper.BindPFlag("cfncreate-wait", flags.Lookup("wait"))

	flags.StringP("userdata-file", "u", "", "file containing userdata")
	viper.BindPFlag("cfncreate-userdatafn", flags.Lookup("userdata-file"))
	// end create stack params
	*/
}
