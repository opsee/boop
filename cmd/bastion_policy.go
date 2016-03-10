package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	log "github.com/mborsuk/jwalterweatherman"
	"github.com/opsee/basic/schema"
	"github.com/opsee/basic/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

// XXX duplicated in Spanx
// TODO publish to and get from a canonical source on s3
const (
	PolicyDoc = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "autoscaling:DescribeLoadBalancers",
        "autoscaling:DescribeAutoScalingGroups",
        "autoscaling:CreateLaunchConfiguration",
        "autoscaling:DescribeLaunchConfigurations",
        "autoscaling:DeleteLaunchConfiguration",
        "autoscaling:UpdateAutoScalingGroup",
        "autoscaling:DescribeScalingActivities",
        "autoscaling:DescribeScheduledActions",
        "cloudformation:CreateStack",
        "cloudformation:DeleteStack",
        "cloudformation:DescribeStacks",
        "cloudformation:DescribeStackResources",
        "cloudformation:ListStackResources",
        "cloudformation:UpdateStack",
        "ec2:CreateTags",
        "ec2:DeleteTags",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupIngress",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:StartInstances",
        "ec2:RunInstances",
        "ec2:StopInstances",
        "ec2:RebootInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeAccountAttributes",
        "ec2:DescribeImages",
        "ec2:DescribeSecurityGroups",
        "ec2:CreateSecurityGroup",
        "ec2:DeleteSecurityGroup",
        "ec2:DescribeSubnets",
        "ec2:DescribeVpcs",
        "ec2:DescribeInstances",
        "ec2:DescribeInternetGateways",
        "ec2:DescribeRouteTables",
        "elasticloadbalancing:DescribeLoadBalancers",
        "sns:CreateTopic",
        "sns:DeleteTopic",
        "sns:Subscribe",
        "sns:Unsubscribe",
        "sns:Publish",
        "sqs:CreateQueue",
        "sqs:DeleteQueue",
        "sqs:DeleteMessage",
        "sqs:ReceiveMessage",
        "sqs:GetQueueAttributes",
        "sqs:SetQueueAttributes",
        "rds:DescribeDBInstances",
        "rds:DescribeDBSecurityGroups"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "iam:*"
      ],
      "Resource": "arn:aws:iam::*:role/opsee-role-*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": "arn:aws:s3:::opsee-bastion-cf/*"
    }
  ]
}`
)

type opseePolicy struct {
	Name string
	Role string
}

var bastionPolicy = &cobra.Command{
	Use:   "policy",
	Short: "bastion account policy commands",
}

var bastionPolicyList = &cobra.Command{
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
		user := userResp.User

		spanxResp, err := svcs.Spanx.GetCredentials(context.Background(), &service.GetCredentialsRequest{
			User: user,
		})
		if err != nil {
			return err
		}
		userCreds := spanxResp.GetCredentials()

		if userCreds == nil {
			return NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
		}
		staticCreds := credentials.NewStaticCredentials(
			*userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

		iamClient := iam.New(session.New(&aws.Config{
			Credentials: staticCreds,
			MaxRetries:  aws.Int(5),
			Region:      aws.String("us-west-1"),
		}))

		pol, err := findOpseeRolePolicy(iamClient, user)
		if err != nil {
			return err
		}
		if pol == nil {
			return NewSystemErrorF("no role policy for user: %s", user.Email)
		}

		err = pol.updateOpseeRolePolicy(iamClient)
		if err != nil {
			return err
		}
		fmt.Printf("policy updated: %s\n", pol.Name)

		return nil
	},
}

func (p opseePolicy) updateOpseeRolePolicy(client *iam.IAM) error {
	_, err := client.PutRolePolicy(&iam.PutRolePolicyInput{
		RoleName:       aws.String(p.Role),
		PolicyName:     aws.String(p.Name),
		PolicyDocument: aws.String(PolicyDoc),
	})

	return err
}

func findOpseeRolePolicy(iamClient *iam.IAM, user *schema.User) (*opseePolicy, error) {
	pol := &opseePolicy{}

	resp, err := iamClient.GetRolePolicy(&iam.GetRolePolicyInput{
		PolicyName: aws.String(fmt.Sprintf("opsee-policy-%s", user.CustomerId)),
		RoleName:   aws.String(fmt.Sprintf("opsee-role-%s", user.CustomerId)),
	})
	if err != nil {
		return nil, err
	}
	if resp != nil {
		pol.Name = *resp.PolicyName
		pol.Role = *resp.RoleName
	}

	return pol, nil
}

func init() {
	bastionCmd.AddCommand(bastionPolicy)
	bastionPolicy.AddCommand(bastionPolicyList)
}
