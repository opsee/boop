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
)

type bastionStack struct {
	Creds  *credentials.Credentials
	Region string
	Stack  *cloudformation.Stack
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

	var cfnClient *cloudformation.CloudFormation
	var stack *bastionStack

	for _, region := range regionList {
		log.INFO.Printf("checking %s\n", region)
		cfnClient = cloudformation.New(session.New(), aws.NewConfig().WithCredentials(staticCreds).WithRegion(region))
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
