package cmd

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/fatih/color"
	"sort"
	//log "github.com/mborsuk/jwalterweatherman"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ImageList []*ec2.Image

func (l ImageList) Len() int           { return len(l) }
func (l ImageList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l ImageList) Less(i, j int) bool { return *l[i].Name > *l[j].Name }

func init() {
	bastionCmd.AddCommand(bastionAMI)
	bastionAMI.AddCommand(bastionAMIList)
	flags := bastionAMI.Flags()
	flags.StringP("region", "r", "us-west-1", "region")
	viper.BindPFlag("ami-list-region", flags.Lookup("region"))
}

var bastionAMI = &cobra.Command{
	Use:   "ami",
	Short: "bastion AMI commands",
}

var bastionAMIList = &cobra.Command{
	Use:   "list",
	Short: "list available bastion AMIs",
	RunE: func(cmd *cobra.Command, args []string) error {

		amiList, err := getAMIList()
		if err != nil {
			return err
		}

		yellow := color.New(color.FgYellow).SprintFunc()
		for _, ami := range amiList {
			fmt.Printf("%s %s\n", yellow(*ami.ImageId), *ami.Name)
		}

		return nil
	},
}

func getAMIList() ([]*ec2.Image, error) {
	ownerID := "933693344490"
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&ec2rolecreds.EC2RoleProvider{
				Client: ec2metadata.New(session.New()),
			},
			&credentials.EnvProvider{},
		},
	)

	ec2client := ec2.New(session.New(&aws.Config{
		Credentials: creds,
		MaxRetries:  aws.Int(3),
		Region:      aws.String(viper.GetString("ami-list-region")),
	}))

	imageOutput, err := ec2client.DescribeImages(&ec2.DescribeImagesInput{
		Owners: []*string{
			aws.String(ownerID),
		},
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:release"),
				Values: []*string{aws.String("stable")},
			},
			{
				Name:   aws.String("is-public"),
				Values: []*string{aws.String("true")},
			},
		},
	})

	if err != nil {
		return nil, err
	}

	sort.Sort(ImageList(imageOutput.Images))

	return imageOutput.Images, nil
}
