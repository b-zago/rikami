package ci

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func GetParam(arn string) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Could not read AWS creds. Error:\n%v", err)
	}

	client := ssm.NewFromConfig(cfg)

	param, err := client.GetParameter(context.TODO(), &ssm.GetParameterInput{
		Name:           aws.String(arn),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		log.Fatalf("Could not get SSM. Error:\n%v", err)
	}

	fmt.Println(*param.Parameter.Value)
}
