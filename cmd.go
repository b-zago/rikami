package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/b-zago/rikami/ci"
	"github.com/bitnami-labs/sealed-secrets/pkg/apis/sealedsecrets/v1alpha1"
	"github.com/bitnami-labs/sealed-secrets/pkg/kubeseal"
	"golang.org/x/term"
)

type ConfigFlags struct {
	Edit  string
	Value string
}

type SealFlags struct {
	Namespace string
	Name      string
}

type ParamsOptions struct {
	Env    string
	Params string // separated by ,
	Action string // push, pull
}

func ConfigCmd(flags *ConfigFlags, confPath string) {
	if flags.Edit == "" {
		MakeConfig(confPath)
	} else {
		config := LoadConfig(confPath)
		if flags.Value != "" {
			// pipe support
			if flags.Value == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					log.Fatalf("reading stdin: %v", err)
				}
				flags.Value = strings.TrimSpace(string(b))
			}
			config.EditConfig(confPath, flags.Edit, flags.Value)
		} else {
			fmt.Println("value not provided. provide value with '-value' or for sensitive data pipe it to '-value -'")
			os.Exit(2)
		}
	}
}

func ManifestCmd(config *Config) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("provide app name: ")
	appNameB, err := reader.ReadBytes('\n')
	if err != nil {
		log.Fatalf("error while reading value of appName: %v", err)
	}
	appName := string(bytes.TrimSpace(appNameB))
	ci.Run(config.BaseGHToken, config.BaseOwner, config.BaseRepo, config.Domain, config.URL, appName)
}

func ParamsCmd(opts *ParamsOptions) {
	if opts.Env == "" {
		fmt.Println("you need to specify '-env'")
		os.Exit(2)
	}
	switch opts.Action {
	case "push":
		// push
		ci.PutLocalEnvParams(opts.Env)
	case "pull":
		// pull
		if opts.Params == "" {
			fmt.Println("you need to specify '-params'")
			os.Exit(2)
		}
		ci.PullEnvParams(true, opts.Env, strings.Split(opts.Params, ",")...)
	default:
		fmt.Println("unknown params subcommand")
		os.Exit(2)
	}
}

func SealCmd(flags *SealFlags, config *Config) {
	if flags.Name == "" || flags.Namespace == "" {
		fmt.Println("missing mandatory flags: '-ns' '-name'")
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("reading stdin: %v", err)
	}
	b = bytes.TrimSpace(b)
	cert := APICert(config)
	var buf bytes.Buffer
	var scope v1alpha1.SealingScope
	scope.Set("strict")
	err = kubeseal.EncryptSecretItem(&buf, flags.Name, flags.Namespace, b, scope, cert)
	if err != nil {
		log.Fatalf("could not seal secret: %v", err)
	}
	fmt.Println(buf.String())
}

func LoginCmd(config *Config, credsPath string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Login: ")
	b, err := reader.ReadBytes('\n')
	if err != nil {
		log.Fatalf("error while reading value: %v", err)
	}
	login := string(bytes.TrimSpace(b))

	fmt.Print("Password: ")
	b, err = term.ReadPassword(int(os.Stderr.Fd()))
	if err != nil {
		log.Fatalf("error while reading value: %v", err)
	}
	password := string(bytes.TrimSpace(b))
	fmt.Print("\nLogging...\n")
	resp := APILogin(config, &ReqUserLogin{User: login, Password: password})

	f, err := os.Create(credsPath)
	if err != nil {
		log.Fatalf("failed to write credentials: %v", err)
	}
	defer f.Close()

	tokens := Tokens{Short: resp.Short, Refresh: resp.Refresh}
	tokensB, err := json.Marshal(&tokens)
	if err != nil {
		log.Fatalf("failed to save credentials as JSON: %v\n", err)
	}

	f.Write(tokensB)
}
