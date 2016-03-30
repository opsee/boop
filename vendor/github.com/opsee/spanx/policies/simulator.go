package policies

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

type SimulateResponse struct {
	Action   string
	Decision string
}

func Simulate(actions []string) ([]*SimulateResponse, error) {
	return SimulateOnResources(actions, []string{"*"})
}

func SimulateOnResources(actions, resources []string) ([]*SimulateResponse, error) {
	var err error

	actionList := make([]*string, len(actions))
	for i, a := range actions {
		actionList[i] = aws.String(a)
	}

	resourceList := make([]*string, len(resources))
	for i, r := range resources {
		resourceList[i] = aws.String(r)
	}

	client := iam.New(session.New(aws.NewConfig().WithRegion("us-west-2")))

	resp, err := client.SimulateCustomPolicy(&iam.SimulateCustomPolicyInput{
		PolicyInputList: []*string{
			aws.String(GetPolicy()),
		},
		ResourceArns: resourceList,
		ActionNames:  actionList,
	})

	if err != nil {
		return nil, err
	}

	responses := make([]*SimulateResponse, len(resp.EvaluationResults))
	for i, result := range resp.EvaluationResults {
		responses[i] = &SimulateResponse{
			aws.StringValue(result.EvalActionName),
			aws.StringValue(result.EvalDecision),
		}

		if responses[i].Decision != "allowed" {
			err = errors.New("at least one action failed")
		}
	}

	return responses, err
}
