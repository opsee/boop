package policies

const (
	UserPolicy = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "iam:CreateRole",
                "iam:GetUser",
                "iam:PutRolePolicy"
            ],
            "Resource": "*"
        }
    ]
}`

	AssumeRolePolicy = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "933693344490"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "%s"
        }
      }
    },
    {
      "Effect": "Allow",
      "Principal": {
        "Service": ["ec2.amazonaws.com"]
      },
      "Action": [ "sts:AssumeRole" ]
    }
  ]
}
`
	Policy = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "autoscaling:*",
        "cloudformation:*",
        "ec2:*",
        "cloudwatch:*",
        "rds:Describe*",
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
        "elasticloadbalancing:DescribeLoadBalancers"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "iam:*"
      ],
      "Resource": [
        "arn:aws:iam::*:role/opsee-role-*",
        "arn:aws:iam::*:instance-profile/opsee-stack-*"
      ]
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
