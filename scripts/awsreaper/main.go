package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/credentials"
	"github.com/awslabs/aws-sdk-go/service/ec2"
)

func main() {
	tagName := flag.String("tagName", "garden-ci-test-instance", "Name of tags to terminate")
	lifeTime := flag.Duration("lifeTime", time.Second, "The allowed lifetime of an instance")
	dryRun := flag.Bool("dryRun", false, "Do not actually terminate instances")
	region := flag.String("region", "us-east-1", "The aws region")

	flag.Parse()
	creds := credentials.NewEnvCredentials()
	_, err := creds.Get()
	if err != nil {
		log.Fatal("Please ensure that the AWS_SECRET_KEY and AWS_ACCESS_KEY variables are set")
	}
	ec2Config := &aws.Config{
		Region:      *region,
		Credentials: creds,
	}

	ec2Client := ec2.New(ec2Config)

	tagValue := "tag-value"
	state := "instance-state-name"
	stateValue := "running"
	fmt.Println("Getting instances for termination...")
	output, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: &tagValue, Values: []*string{tagName},
			},
			&ec2.Filter{
				Name: &state, Values: []*string{&stateValue},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	instancesToKill := []*string{}
	for _, reservation := range output.Reservations {
		for _, instance := range reservation.Instances {
			uptime := time.Now().UTC().Sub(*instance.LaunchTime)
			if uptime < *lifeTime {
				continue
			}
			fmt.Println("*************************")
			fmt.Printf("Instance: %s\n", *instance.InstanceID)
			fmt.Printf("State: %v\n", *instance.State.Name)
			fmt.Printf("LaunchTime: %v\n", *instance.LaunchTime)
			fmt.Printf("Uptime: %v\n", uptime)
			fmt.Println("*************************")
			fmt.Println("")

			instancesToKill = append(instancesToKill, instance.InstanceID)
		}
	}

	if len(instancesToKill) == 0 {
		fmt.Println("Nothing to terminate")
		return
	}

	fmt.Println("Terminating instances...")
	_, err = ec2Client.TerminateInstances(&ec2.TerminateInstancesInput{
		DryRun:      dryRun,
		InstanceIDs: instancesToKill,
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Done")
}
