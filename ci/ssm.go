package ci

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/b-zago/rikami/summon"
)

func GetParam(arn string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Could not read AWS creds. Error:\n%v", err)
	}

	client := ssm.NewFromConfig(cfg)

	param, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(arn),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		log.Fatalf("Could not get SSM. Error:\n%v", err)
	}

	// fmt.Println(*param.Parameter.Value)
	return *param.Parameter.Value
}

func PutParam() {
	repo, ok := os.LookupEnv("REPO_NAME")
	if !ok {
		// for local use, searches .git/config for origin url and determines from that
		repo = GetRepoName()
	}

	envPaths, _ := filepath.Glob("*.env*")
	var secretPaths []string

	for i, p := range envPaths {
		if ok, _ := filepath.Match("*.env*.secret", p); ok {
			secretPaths = append(secretPaths, p)
			envPaths[i] = envPaths[len(envPaths)-1]
			envPaths = envPaths[:len(envPaths)-1]
		}
	}

	var envs []map[string]map[string]string
	var secrets []map[string]map[string]string
	for _, p := range envPaths {
		f, err := os.ReadFile(p)
		if err != nil {
			log.Fatalf("Could not read .env file %s. Error\n%v", p, err)
		}
		data := strings.TrimSpace(string(f))
		envs = append(envs, map[string]map[string]string{p: summon.ParseEnvFile(data)})
	}

	for _, p := range secretPaths {
		f, err := os.ReadFile(p)
		if err != nil {
			log.Fatalf("Could not read .env file %s. Error\n%v", p, err)
		}
		data := strings.TrimSpace(string(f))
		secrets = append(secrets, map[string]map[string]string{p: summon.ParseEnvFile(data)})
	}

	fmt.Println(repo)

	envsJSON, err := json.Marshal(envs)
	if err != err {
		log.Fatalf("Could not encode to JSON. Error\n%v", err)
	}
	secretsJSON, err := json.Marshal(secrets)
	if err != err {
		log.Fatalf("Could not encode to JSON. Error\n%v", err)
	}

	// fmt.Println(string(envsJSON))
	// fmt.Println(string(secretsJSON))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Could not read AWS creds. Error:\n%v", err)
	}

	client := ssm.NewFromConfig(cfg)

	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String("/" + repo + "/envs"),
		Overwrite: aws.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Value:     aws.String(string(envsJSON)),
	})
	if err != nil {
		log.Fatalf("Could not put env parameters. Error\n%v", err)
	}
	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String("/" + repo + "/secrets"),
		Overwrite: aws.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Value:     aws.String(string(secretsJSON)),
	})
	if err != nil {
		log.Fatalf("Could not put secret parameters. Error\n%v", err)
	}
}

func GetRepoName() string {
	confPath := filepath.Join(".git", "config")
	f, err := os.Open(confPath)
	if err != nil {
		log.Fatalf("Could not read %s. Error\n%v", confPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	var repo string
	inRemote := false
	regex := regexp.MustCompile(`\[remote ".*"\]`)
	for scanner.Scan() {
		b := scanner.Bytes()
		if inRemote {
			if bytes.Contains(b, []byte("url")) {
				split := bytes.Split(b, []byte("/"))
				org := split[len(split)-1]
				repo = strings.TrimSuffix(string(org), ".git")
			}
		} else {
			if regex.Match(scanner.Bytes()) {
				inRemote = true
			}
		}
	}
	return repo
}
