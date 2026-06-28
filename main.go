package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("command not provided")
		os.Exit(2)
	}

	log.SetFlags(0)
	userConfDir, err := os.UserConfigDir()
	if err != nil {
		// propose alternative later
		log.Fatal("could not identify user conf dir")
	}
	confPath := filepath.Join(userConfDir, "rika", "rika.json")

	configCmd := flag.NewFlagSet("config", flag.ExitOnError)
	configEdit := configCmd.String("edit", "", "edit specific config field")
	configValue := configCmd.String("value", "", "value to be provided when used with '-edit'")

	sealCmd := flag.NewFlagSet("seal", flag.ExitOnError)
	sealNs := sealCmd.String("ns", "", "namespace for sealed secret")
	sealName := sealCmd.String("name", "", "secret name")

	paramsCmd := flag.NewFlagSet("params", flag.ExitOnError)
	paramsEnv := paramsCmd.String("env", "", "environment of params")
	paramsParams := paramsCmd.String("params", "", "parameters to get separated by ','")

	switch os.Args[1] {
	case "config":
		configCmd.Parse(os.Args[2:])
		ConfigCmd(&ConfigFlags{Edit: *configEdit, Value: *configValue}, confPath)
	case "manifest":
		config := LoadConfig(confPath)
		config.Validate()
		ManifestCmd(config)
	case "params":
		if len(os.Args) < 3 {
			fmt.Println("you need to specify an action via a subcommand")
			os.Exit(2)
		}
		paramsCmd.Parse(os.Args[3:])
		ParamsCmd(&ParamsOptions{Action: os.Args[2], Env: *paramsEnv, Params: *paramsParams})
	case "summon":
		config := LoadConfig(confPath)
		config.Validate()
		fmt.Println("summoning")
		fmt.Println(config.URL)
	case "login":
		config := LoadConfig(confPath)
		config.Validate()
		credsPath := filepath.Join(filepath.Dir(confPath), "creds.json")
		LoginCmd(config, credsPath)
	case "seal":
		config := LoadConfig(confPath)
		config.Validate()
		sealCmd.Parse(os.Args[2:])
		SealCmd(&SealFlags{Name: *sealName, Namespace: *sealNs}, config)
	default:
		fmt.Println("uknown command")
		os.Exit(2)
	}
}
