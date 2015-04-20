package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/ec2"
)

var script string = `
sudo su --login root -c '
mkdir -p ~/go/src/github.com/cloudfoundry-incubator &&
	cd ~/go/src/github.com/cloudfoundry-incubator &&
	git clone git://github.com/cloudfoundry-incubator/garden-linux.git &&
	cd garden-linux &&
	git reset --hard %s &&
	scripts/drone-test
'
`

func main() {
	commit := flag.String("commit", "", "Commit SHA to run against")
	imageId := flag.String("imageID", "", "The ami imageID")
	user := flag.String("user", "ubuntu", "ssh user")
	instanceType := flag.String("instanceType", "m3.large", "The aws instance type")
	keyName := flag.String("keyName", "ci_aws_key", "The aws key name")
	region := flag.String("region", "us-east-1", "The aws region")

	flag.Parse()

	if *commit == "" {
		log.Fatal("The commit SHA is missing.")
	}

	if *imageId == "" {
		log.Fatal("The imageID is missing.")
	}

	creds, err := aws.EnvCreds()
	if err != nil {
		log.Fatal("Please ensure that the AWS_SECRET_KEY and AWS_ACCESS_KEY variables are set")
	}
	ec2Config := &aws.Config{
		Region:      *region,
		Credentials: creds,
	}

	ec2Client := ec2.New(ec2Config)

	maxCount := new(int64)
	*maxCount = 1
	minCount := new(int64)
	*minCount = 1
	instanceSpec := &ec2.RunInstancesInput{
		ImageID:      imageId,
		InstanceType: instanceType,
		KeyName:      keyName,
		MaxCount:     maxCount,
		MinCount:     minCount,
	}
	instance := EC2Instance{
		ec2Client:    ec2Client,
		instanceSpec: instanceSpec,
	}

	signer, err := ssh.ParsePrivateKey([]byte(os.Getenv("SSH_PRIVATE_KEY")))
	if err != nil {
		log.Fatal(err)
	}

	auths := []ssh.AuthMethod{ssh.PublicKeys(signer)}

	sshConfig := &ssh.ClientConfig{
		User: *user,
		Auth: auths,
	}

	sshClient := SSHClient{
		sshConfig: sshConfig,
		protocol:  "tcp",
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}

	exitStatus, err := runCommand(sshClient, instance, fmt.Sprintf(script, *commit))
	if err != nil {
		log.Fatal(err)
	}

	if exitStatus != 0 {
		log.Printf("Remote command exited non zero: %d\n", exitStatus)
	}

	os.Exit(exitStatus)
}

func runCommand(sshClient SSHClient, instance EC2Instance, command string) (int, error) {
	err := instance.Start()
	if err != nil {
		return 1, err
	}
	fmt.Printf("Instance Started: %s\n", instance.InstanceId())
	defer terminate(instance)

	address, err := instance.PublicDNS()
	if err != nil {
		return 1, err
	}
	fmt.Printf("Instance available at: %s\n", address)

	sshClient.host = address
	sshClient.port = 22

	fmt.Println("Running Command")
	exitStatus, err := sshClient.Run(command)
	if err != nil {
		return 1, err
	}

	return exitStatus, nil
}

func terminate(instance EC2Instance) {
	fmt.Printf("Terminating instance: %s\n", instance.InstanceId())
	err := instance.Terminate()
	if err != nil {
		panic(err)
	}
}

type EC2Instance struct {
	ec2Client    *ec2.EC2
	instanceSpec *ec2.RunInstancesInput
	instanceInfo *ec2.Instance
}

func (inst *EC2Instance) InstanceId() string {
	if inst.instanceInfo != nil {
		return *inst.instanceInfo.InstanceID
	}
	return ""
}

func (inst *EC2Instance) Start() error {
	startResp, err := inst.ec2Client.RunInstances(inst.instanceSpec)
	if err != nil {
		return err
	}

	instance := startResp.Instances[0]
	inst.instanceInfo = instance

	key := "Name"
	value := "garden-ci-test-instance"
	_, err = inst.ec2Client.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{instance.InstanceID},
		Tags: []*ec2.Tag{
			&ec2.Tag{Key: &key, Value: &value},
		},
	})

	return err
}

func (inst *EC2Instance) PublicDNS() (string, error) {
	if name := *inst.instanceInfo.PublicDNSName; name != "" {
		return name, nil
	}

	for attempt := 1; attempt <= 100; attempt++ {
		time.Sleep(500 * time.Millisecond)

		r, err := inst.ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIDs: []*string{inst.instanceInfo.InstanceID},
		})
		if err != nil {
			return "", err
		}

		if len(r.Reservations) == 0 || len(r.Reservations[0].Instances) == 0 {
			return "", fmt.Errorf("instance not found: %s", inst.instanceInfo.InstanceID)
		}

		rinstance := r.Reservations[0].Instances[0]
		if host := *rinstance.PublicDNSName; host != "" {
			inst.instanceInfo = rinstance
			return host, nil
		}
	}

	return "", errors.New("couldn't determine IP address for instance")
}

func (inst *EC2Instance) Terminate() error {
	_, err := inst.ec2Client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIDs: []*string{inst.instanceInfo.InstanceID},
	})
	return err
}

type SSHClient struct {
	sshConfig *ssh.ClientConfig
	protocol  string
	host      string
	port      int
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
}

func (cl *SSHClient) Run(command string) (int, error) {
	address := fmt.Sprintf("%s:%d", cl.host, cl.port)

	var client *ssh.Client
	var err error
	for attempt := 1; attempt <= 100; attempt++ {
		time.Sleep(500 * time.Millisecond)
		client, err = ssh.Dial(cl.protocol, address, cl.sshConfig)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0, fmt.Errorf("Failed to dial: %s", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return 0, fmt.Errorf("Failed to create session: %s", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return 0, fmt.Errorf("request for pseudo terminal failed: %s", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("Unable to setup stdin for session: %v\n", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("Unable to setup stdout for session: %v\n", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("Unable to setup stderr for session: %v\n", err)
	}

	go io.Copy(stdin, cl.stdin)
	go io.Copy(cl.stdout, stdout)
	go io.Copy(cl.stderr, stderr)

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		} else {
			return 0, err
		}
	}

	return 0, nil
}
