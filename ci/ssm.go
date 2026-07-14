package ci

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

type EnvFileMap map[string]map[string]string

type SSMParams struct {
	Envs    EnvFileMap
	Secrets EnvFileMap
}

type SSMData struct {
	Data   map[string]*SSMParams
	Client *ssm.Client
}

type EmptyParam struct {
	Param string
}

func (e *EmptyParam) Error() string {
	return fmt.Sprintf("ssm parameter %q not found. proceeding without it", e.Param)
}

type PullEnvParamsResult struct {
	Data *SSMParams
	Err  error
}

var SSMParameters = SSMData{Data: make(map[string]*SSMParams)}

// ssmDataMu guards concurrent access to SSMParameters.Data, which processFinals
// mutates from one goroutine per env (concurrent map writes panic otherwise).
// ssmClientOnce ensures the SSM client is initialized exactly once even when
// those same goroutines call LoadClient at the same time.
var (
	ssmDataMu     sync.Mutex
	ssmClientOnce sync.Once
)

func (s *SSMData) LoadClient() {
	ssmClientOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			log.Fatal("could not read AWS creds")
		}

		s.Client = ssm.NewFromConfig(cfg)
	})
}

func (p EnvFileMap) Put(repo, envir, key string) error {
	// no-op for not initialized
	if p == nil {
		return nil
	}
	param := fmt.Sprintf("/%s/%s/%s", repo, envir, key)
	paramJSON, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("could not encode %v to JSON", param)
	}
	err = PutParam(repo, envir, key, string(paramJSON))
	if err != nil {
		return err
	}
	return nil
}

// GetParam ...
// returns empty string and nil error if no parameter was found
func GetParam(arn string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	param, err := SSMParameters.Client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(arn),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.ParameterNotFound](err); ok {
			return "", nil
		} else {
			return "", fmt.Errorf("get ssm parameter error %q: %w", arn, err)
		}
	}

	return *param.Parameter.Value, nil
}

// only returns pointer to SSMParams when not using local, otherwise nil
func pullEnvParam(local bool, repo, envir, key string) *PullEnvParamsResult {
	ssmParams := &SSMParams{}
	param := fmt.Sprintf("/%s/%s/%s", repo, envir, key)
	pJSON, err := GetParam(param)
	if err != nil {
		log.Fatal(err)
	}
	if pJSON == "" {
		return &PullEnvParamsResult{Data: ssmParams, Err: &EmptyParam{Param: param}}
	}

	var data EnvFileMap
	err = json.Unmarshal([]byte(pJSON), &data)
	if err != nil {
		log.Fatalf("Could not decode JSON parameter. Error\n%v", err)
	}

	if local {
		for k, v := range data {
			f, err := os.Create(filepath.Join(k))
			if err != nil {
				log.Fatalf("Could not create a file %s. Error\n%v", k, err)
			}
			defer f.Close()

			var envLines []string
			for k, v := range v {
				envLines = append(envLines, k+"="+v)
			}
			env := strings.Join(envLines, "\n")
			f.WriteString(env)
		}
		return &PullEnvParamsResult{Data: nil, Err: nil}
	} else {
		ref := reflect.ValueOf(ssmParams).Elem()
		fieldStr := strings.ToUpper(key[:1]) + key[1:]
		field := ref.FieldByName(fieldStr)

		if field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(data))
		} else {
			return &PullEnvParamsResult{Data: nil, Err: fmt.Errorf("invalid ssm param. expecting either envs or secrets but got %q", key)}
		}

		return &PullEnvParamsResult{Data: ssmParams, Err: nil}
	}
}

func PullEnvParams(local bool, envir string, params ...string) {
	SSMParameters.LoadClient()
	repo := GetRepoName()

	paramsChan := make(chan *PullEnvParamsResult, len(params))

	for _, p := range params {
		go func(param string) {
			paramsChan <- pullEnvParam(local, repo, envir, param)
		}(p)
	}

	// wait for all pulls to finish and behave accordingly
	for range len(params) {
		result := <-paramsChan
		if result.Err != nil {
			if err, ok := errors.AsType[*EmptyParam](result.Err); ok {
				log.Println(err.Error())
				continue
			} else {
				log.Fatal(result.Err.Error())
			}
		}
		if !local {
			// Merge the pulled field into the per-env struct instead of replacing
			// it. Each pullEnvParam returns a fresh SSMParams with only one field
			// set (Envs or Secrets), so assigning result.Data directly would let a
			// later param clobber an earlier one when several are pulled together.
			// The lock guards SSMParameters.Data because processFinals fans this
			// out across envs concurrently.
			ssmDataMu.Lock()
			existing := SSMParameters.Data[envir]
			if existing == nil {
				existing = &SSMParams{}
				SSMParameters.Data[envir] = existing
			}
			if result.Data.Envs != nil {
				existing.Envs = result.Data.Envs
			}
			if result.Data.Secrets != nil {
				existing.Secrets = result.Data.Secrets
			}
			ssmDataMu.Unlock()
		}
	}
}

func putLocalEnvParam(repo, envir, key string, paths []string) error {
	envs := make(EnvFileMap)
	for _, p := range paths {
		f, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("could not read .env file %s. error\n%v", p, err)
		}
		data := strings.TrimSpace(string(f))
		envs[filepath.Base(p)] = ParseEnvFile(data)
	}
	err := envs.Put(repo, envir, key)
	if err != nil {
		return err
	}
	return nil
}

// PutLocalEnvParams ...
// for now just hardcoding it that way is fine, no need for anything else atm
// perhaps will change it later if in need on additional parameters or more flexibility for this
func PutLocalEnvParams(envir string) {
	SSMParameters.LoadClient()
	repo := GetRepoName()
	envPathsAll, _ := filepath.Glob(".env*")
	var secretPaths []string
	var envPaths []string

	for _, p := range envPathsAll {
		if ok, _ := filepath.Match(".env*.secret", p); ok {
			secretPaths = append(secretPaths, p)
		} else {
			envPaths = append(envPaths, p)
		}
	}
	errorsChan := make(chan error, 2)
	go func() {
		errorsChan <- putLocalEnvParam(repo, envir, "envs", envPaths)
	}()
	go func() {
		errorsChan <- putLocalEnvParam(repo, envir, "secrets", secretPaths)
	}()
	for range 2 {
		if err := <-errorsChan; err != nil {
			log.Fatal(err)
		}
	}
}

func PutEnvParams() {
	errChan := make(chan error, len(SSMParameters.Data)*2) // x2 since we have only secrets and envs for now
	repo := GetRepoName()
	for envir, param := range SSMParameters.Data {
		// for now just hardcoded because i don't really need anything else atm
		go func(p *SSMParams) {
			errChan <- param.Envs.Put(repo, envir, "envs")
		}(param)
		go func(p *SSMParams) {
			errChan <- param.Secrets.Put(repo, envir, "secrets")
		}(param)
	}
	for range len(SSMParameters.Data) * 2 {
		if err := <-errChan; err != nil {
			log.Fatal(err)
		}
	}
}

func PutParam(repo, envir, key, valJSON string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	param := fmt.Sprintf("/%s/%s/%s", repo, envir, key)
	_, err := SSMParameters.Client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(param),
		Overwrite: aws.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Value:     aws.String(valJSON),
	})
	if err != nil {
		if e, ok := errors.AsType[smithy.APIError](err); ok {
			// so it wont log something it shouldnt by accident
			return fmt.Errorf("could not put secret parameter %q\ncode=%s fault=%s", param, e.ErrorCode(), e.ErrorFault())
		} else {
			return fmt.Errorf("unknown error for parameter %q", param)
		}
	}
	return nil
}

func GetRepoName() string {
	repo, ok := os.LookupEnv("REPO_NAME")
	if ok {
		return repo
	}
	confPath := filepath.Join(".git", "config")
	fmt.Printf("no REPO_NAME set. trying to read from %s instead\n", confPath)
	f, err := os.Open(confPath)
	if err != nil {
		log.Fatalf("Could not read %s. Error\n%v", confPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

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

func ParseEnvFile(data string) map[string]string {
	newMap := make(map[string]string)
	for line := range strings.SplitSeq(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		newMap[strings.TrimSpace(key)] = value
	}
	return newMap
}
