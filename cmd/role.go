package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/opsee/basic/schema"
	"github.com/opsee/boop/errors"
	"github.com/opsee/boop/svc"
	"github.com/opsee/boop/util"
	"github.com/opsee/spanx/roler"
	"github.com/spf13/cobra"
)

type opseePolicy struct {
	Name string
	Role string
}

var roleCmd = &cobra.Command{
	Use:   "role",
	Short: "customer role commands",
}

var updatePolicyCmd = &cobra.Command{
	Use:   "updatePolicy [customer email|customer UUID]",
	Short: "update customer role policy",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		user, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		userCreds, err := opseeServices.GetRoleCreds(user)
		if err != nil {
			return errors.NewSystemErrorF("cannot obtain AWS creds for user: %s", user.Id)
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
			return errors.NewSystemErrorF("no role policy for user: %s", user.Email)
		}

		err = pol.updateOpseeRolePolicy(iamClient)
		if err != nil {
			return err
		}
		fmt.Printf("policy updated: %s\n", pol.Name)

		return nil
	},
}

var roleCredsCommand = &cobra.Command{
	Use:   "creds [customer email|customer UUID]",
	Short: "print opsee-role cred vars for customer",
	RunE: func(cmd *cobra.Command, args []string) error {
		opseeServices := &svc.OpseeServices{}

		user, err := util.GetUserFromArgs(args, 0, opseeServices)
		if err != nil {
			return err
		}

		userCreds, err := opseeServices.GetRoleCreds(user)
		if err != nil {
			return err
		}

		fmt.Printf("%s:%s:%s\n", *userCreds.AccessKeyID, *userCreds.SecretAccessKey, *userCreds.SessionToken)

		return nil
	},
}

func (p opseePolicy) updateOpseeRolePolicy(client *iam.IAM) error {
	_, err := client.PutRolePolicy(&iam.PutRolePolicyInput{
		RoleName:       aws.String(p.Role),
		PolicyName:     aws.String(p.Name),
		PolicyDocument: aws.String(roler.Policy),
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
	BoopCmd.AddCommand(roleCmd)
	roleCmd.AddCommand(updatePolicyCmd)
	roleCmd.AddCommand(roleCredsCommand)
}
