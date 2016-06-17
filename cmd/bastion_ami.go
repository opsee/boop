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
	"os"
	"text/tabwriter"
)

var regions = map[string]bool{
	"ap-northeast-1": true,
	"ap-southeast-1": true,
	"ap-southeast-2": true,
	"eu-central-1":   true,
	"eu-west-1":      true,
	"sa-east-1":      true,
	"us-east-1":      true,
	"us-west-1":      true,
	"us-west-2":      true,
}

type ImageList []*ec2.Image

func (l ImageList) Len() int           { return len(l) }
func (l ImageList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l ImageList) Less(i, j int) bool { return *l[i].CreationDate > *l[j].CreationDate }

func init() {
	bastionCmd.AddCommand(bastionAMI)
	bastionAMI.AddCommand(bastionAMIList)
	flags := bastionAMI.PersistentFlags()
	flags.StringP("region", "r", "us-west-1", "region")
	viper.BindPFlag("ami-list-region", flags.Lookup("region"))
	flags.IntP("num-results", "n", 0, "max number of sorted results (default no limit)")
	viper.BindPFlag("ami-list-num", flags.Lookup("num-results"))
}

var bastionAMI = &cobra.Command{
	Use:   "ami",
	Short: "bastion AMI commands",
}

var bastionAMIList = &cobra.Command{
	Use:   "list",
	Short: "list available bastion AMIs",
	RunE: func(cmd *cobra.Command, args []string) error {

		amiList, err := getAMIList(viper.GetString("ami-list-region"), "")
		if err != nil {
			return err
		}

		yellow := color.New(color.FgYellow).SprintFunc()
		blue := color.New(color.FgBlue).SprintFunc()
		header := color.New(color.FgWhite).SprintFunc()

		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 1, 0, 2, ' ', 0)

		if len(amiList) > 0 {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", header("id"), "name",
				header("sha"), "create date", header("release"))
		}

		for _, ami := range amiList {
			tag := ""
			rel := ""
			for _, t := range ami.Tags {
				if *t.Key == "sha" {
					tag = *t.Value
					if len(tag) > 8 {
						tag = tag[:8]
					}
				} else if *t.Key == "release" {
					rel = *t.Value
				}

			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", yellow(*ami.ImageId), *ami.Name, blue(tag), *ami.CreationDate, blue(rel))
		}
		w.Flush()

		return nil
	},
}

func getAMIList(region, releaseTag string) ([]*ec2.Image, error) {
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
		Region:      aws.String(region),
	}))

	filters := []*ec2.Filter{
		{
			Name:   aws.String("tag:opsee"),
			Values: []*string{aws.String("bastion")},
		},
	}

	if releaseTag != "" {
		filters = append(filters, &ec2.Filter{
			Name:   aws.String("tag:release"),
			Values: []*string{aws.String(releaseTag)},
		})
	}

	imageOutput, err := ec2client.DescribeImages(&ec2.DescribeImagesInput{
		Owners: []*string{
			aws.String(ownerID),
		},
		Filters: filters,
	})

	if err != nil {
		return nil, err
	}

	sort.Sort(ImageList(imageOutput.Images))

	if viper.IsSet("ami-list-num") {
		n := viper.GetInt("ami-list-num")
		if n < len(imageOutput.Images) {
			return imageOutput.Images[0:n], nil
		}
	}

	return imageOutput.Images, nil
}
