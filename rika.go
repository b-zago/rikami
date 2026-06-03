package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/b-zago/rikami/app"
	"github.com/b-zago/rikami/ci"
	"github.com/b-zago/rikami/summon"
)

func Check(e error) {
	if e != nil {
		panic(e)
	}
}

var (
	Reader = bufio.NewReader(os.Stdin)
	Config *Conf
)

func main() {
	userDefaultConf, err := os.UserConfigDir()
	Check(err)
	confPath := filepath.Join(userDefaultConf, "rikami", "conf")

	argsNum := len(os.Args)
	if argsNum < 2 {
		// ci.GetParam("/nyanwatch/endpoints")
		// ci.PutParam()
		s := ci.GetParam("/rikami/envs")
		fmt.Println(s)
		fmt.Println("Well you kinda have to tell me what to do")
		os.Exit(2)
	}

	summonCmd := flag.NewFlagSet("summon", flag.ExitOnError)
	summonLocal := summonCmd.Bool("local", false, "Determines if chart will be created locally")
	summonEnvs := summonCmd.String("envs", "", "Separate with ',' .env files to send over to rikami controller")
	summonTarget := summonCmd.String("target", "", "Overrides output target path of the chart")
	summonConf := summonCmd.String("conf", "", "Specify the config for this specific command")

	appCmd := flag.NewFlagSet("app", flag.ExitOnError)
	appLocal := appCmd.Bool("local", false, "Determines if app subcommand will be run locally")
	appParam := appCmd.String("p", "", "App subcommand parameter")
	appConf := appCmd.String("conf", "", "Specify the config for this specific command")

	certCmd := flag.NewFlagSet("fetch-cert", flag.ExitOnError)
	certTarget := certCmd.String("target", "./", "Determines where to save the cert")
	certConf := certCmd.String("conf", "", "Specify the config for this specific command")

	switch os.Args[1] {
	case "summon":
		if argsNum < 3 {
			fmt.Println("You need to specify the vessel for me")
			fmt.Println("Usage: rika summon <vessel>")
			os.Exit(2)
		}
		summonCmd.Parse(os.Args[3:])
		if *summonConf != "" {
			Config = LoadConf(*summonConf)
		} else {
			Config = LoadConf(confPath)
		}
		if *summonLocal {
			summon.StartSummonLocal(os.Args[2], *summonTarget, Config)
		} else {
			startSummon(os.Args[2], *summonEnvs)
		}
		fmt.Println("Summon done")
	case "forge":

		Config = LoadConf(confPath)
		if argsNum < 3 {
			fmt.Println("You need to specify what we will be forging")
			fmt.Println("Usage: rika forge <new vessel>")
			os.Exit(2)
		}
		StartForgery(os.Args[2])
	case "app":
		if argsNum < 4 {
			fmt.Println("You need to specify the app action and pattern.")
			os.Exit(2)
		}

		appCmd.Parse(os.Args[4:])
		if *appConf != "" {
			Config = LoadConf(*appConf)
		} else {
			Config = LoadConf(confPath)
		}

		if *appLocal {
			app.AppCallLocal(os.Args[2], os.Args[3], *appParam)
		} else {
			req := NewAppRequest(os.Args[2], os.Args[3], *appParam)
			err := req.Send()
			Check(err)
		}

	case "fetch-cert":
		certCmd.Parse(os.Args[2:])

		if *certConf != "" {
			Config = LoadConf(*certConf)
		} else {
			Config = LoadConf(confPath)
		}

		GetFreshCert(*certTarget)
	case "config":
		MakeConf(confPath)

	default:
		fmt.Println("Unknown command")
		os.Exit(2)
	}
}

func startSummon(vsl string, envs string) {
	var envSlice []EnvEntry
	if envs != "" {
		for p := range strings.SplitSeq(envs, ",") {
			envPath := filepath.Join(p)
			f, err := os.ReadFile(envPath)
			Check(err)
			newEntry := EnvEntry{
				EnvName: filepath.Base(envPath),
				EnvVals: summon.ParseEnvFile(string(f)),
			}
			envSlice = append(envSlice, newEntry)
		}
	}
	req := ForgeRequest(vsl, envSlice)
	fmt.Println(req)
	err := req.Send()
	Check(err)
}
