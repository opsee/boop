package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/basic/schema"
	"github.com/opsee/basic/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
)

const cfnS3BucketURL = "https://s3%s%s.amazonaws.com/opsee-bastion-cf-%s/beta"
const cfnTemplate = "bastion-cf.template"
const secGrpTemplate = "bastion-ingress-cf.template"

type bastionStack struct {
	Creds  *credentials.Credentials
	Region string
	Stack  *cloudformation.Stack
}

func (s bastionStack) getStackParams() []*cloudformation.Parameter {
	return []*cloudformation.Parameter{
		{
			ParameterKey: aws.String("ImageId"),
			// TODO optionally get this from cmdline
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:     aws.String("BastionIngressTemplateUrl"),
			UsePreviousValue: aws.Bool(true),
		},
		{
			ParameterKey:   aws.String("AllowSSH"),
			ParameterValue: aws.String("True"),
		},
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
}

func (s bastionStack) getCFNTemplate() ([]byte, error) {
	resp, err := http.Get(s.getS3URL(cfnTemplate))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func (s bastionStack) getS3URL(template string) string {
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

var bastionCFN = &cobra.Command{
	Use:   "cfn",
	Short: "bastion cloud formation commands",
}

var bastionCFNUpdate = &cobra.Command{
	Use:   "update [customer email|customer UUID]",
	Short: "update CFN template on a customer bastion",
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

		stack, err := findBastionStack(userResp.User)
		if err != nil {
			return err
		}

		if stack != nil {
			log.INFO.Printf("found bastion stack: %s in %s\n", *stack.Stack.StackId, stack.Region)
			log.INFO.Printf("requesting stack update with: %s\n", stack.getS3URL(cfnTemplate))

			templateBytes, err := stack.getCFNTemplate()
			if err != nil {
				return err
			}

			cfnClient := cloudformation.New(session.New(), aws.NewConfig().WithCredentials(stack.Creds).WithRegion(stack.Region))
			_, err = cfnClient.UpdateStack(&cloudformation.UpdateStackInput{
				StackName:    aws.String("opsee-stack-" + userResp.User.CustomerId),
				TemplateBody: aws.String(string(templateBytes)),
				Capabilities: []*string{
					aws.String("CAPABILITY_IAM"),
				},
				Parameters: stack.getStackParams(),
			})
			if err != nil {
				return err
			}
		}
		return nil
	},
}

func findBastionStack(user *schema.User) (*bastionStack, error) {
	spanxResp, err := svcs.Spanx.GetCredentials(context.Background(), &service.GetCredentialsRequest{
		User: user,
	})
	if err != nil {
		return nil, err
	}
	userCreds := spanxResp.GetCredentials()

	if userCreds == nil {
		return nil, NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
	}
	staticCreds := credentials.NewStaticCredentials(
		*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

	var stack *bastionStack

	for _, region := range regionList {
		log.INFO.Printf("checking %s\n", region)
		cfnClient := cloudformation.New(session.New(), aws.NewConfig().WithCredentials(staticCreds).WithRegion(region))
		stackname := fmt.Sprintf("opsee-stack-%s", user.CustomerId)
		descResponse, err := cfnClient.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: &stackname,
		})
		if err != nil {
			return nil, err
		}

		if len(descResponse.Stacks) > 1 {
			return nil, NewSystemErrorF("multiple opsee stacks found for cust %s in ", user.CustomerId)
		}

		if len(descResponse.Stacks) == 0 {
			return nil, NewSystemErrorF("cannot find opsee stack for cust: %s", user.CustomerId)
		} else {
			stack = &bastionStack{
				Stack:  descResponse.Stacks[0],
				Region: region,
				Creds:  staticCreds,
			}
			break
		}
	}

	return stack, nil
}

func init() {
	bastionCmd.AddCommand(bastionCFN)
	bastionCFN.AddCommand(bastionCFNUpdate)
	flags := bastionCFNUpdate.Flags()
	flags.BoolP("dry-run", "n", false, "dry run")
	viper.BindPFlag("cfnup-dry-run", flags.Lookup("dry-run"))
}
