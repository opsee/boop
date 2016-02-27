// Copyright © 2016 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"crypto/tls"
	"fmt"
	"github.com/opsee/basic/service"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type bastionServices struct {
	Vape  service.VapeClient
	Spanx service.SpanxClient
}

func NewBastionServices(vape service.VapeClient, spanx service.SpanxClient) *bastionServices {
	return &bastionServices{
		Vape:  vape,
		Spanx: spanx,
	}
}

var svcs *bastionServices

// bastionCmd represents the bastion command
var bastionCmd = &cobra.Command{
	Use:   "bastion",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		vresp, err := svcs.Vape.ListUsers(context.Background(), &service.ListUsersRequest{})
		if err != nil {
			panic(err)
		}
		for _, u := range vresp.Users {
			sresp, err := svcs.Spanx.GetCredentials(context.Background(), &service.GetCredentialsRequest{User: u})
			if err != nil {
				fmt.Println("error: ", err)
				continue
			}
			fmt.Println(sresp.Credentials)
		}
	},
}

func init() {
	RootCmd.AddCommand(bastionCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// bastionCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// bastionCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	conn, err := grpc.Dial("vape.in.opsee.com:443", grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		panic(err)
	}
	vape := service.NewVapeClient(conn)

	conn, err = grpc.Dial("spanx.in.opsee.com:8443", grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		panic(err)
	}
	spanx := service.NewSpanxClient(conn)

	svcs = NewBastionServices(vape, spanx)

}
