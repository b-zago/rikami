package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var templateMap = map[string]string{
	"cast":     "{{cast \"%s\" \"%s\"}}",
	"override": "{{override %s \"%s\" %s}}",
	"append":   "{{append %s \"%s\" %s}}",
	"version":  "{{append .Chart.Main \"dependencies[0]\" (map \"version\" \"%s\")}}",
	"secMake":  "{{override %s \"data\" (secMake \".env.secret\" .Globals.Values.env .%s.Secrets.name)}}",
	"envMake":  "{{override %s \"envVars\" (envMake \".env\")}}",
	"secRand":  "{{override %s \"data\" (secRand %s %s)}}",
	"appendSec": `{{override %s "envSecretRefs" (list %s.Secrets.name)}}
{{append %s.Secrets "runsOnList" (list %s.name)}}`,
}

type Forgery struct {
	ConfBlock     []string
	CastBlock     []string
	TraitsBlock   []string
	StandardClose string
	StandardOpen  string
	Version       string
	DryRun        []string
}

func StartForgery(vesselName string) {
	forge := Forgery{
		ConfBlock:     []string{"{{conf \"Chart\"}}"},
		CastBlock:     []string{"{{cast \"Chart\" \"Chart\"}}"},
		TraitsBlock:   []string{},
		Version:       "",
		StandardClose: "{{summon \"values-staging\"}}",
		StandardOpen: `{{- global "env" "staging" -}}

{{override .Chart.Main "description" "App description"}}
{{request .Chart.Main "name" "Chart name"}}
{{$prefix := print .Chart.Main.name "-"}}

{{target .Chart.Main.name}}
		`,
	}

	fmt.Println("Forging", vesselName)
	// fmt.Println("Type help for usage info")

	isDone := true

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Rikami library chart version: ")
	scanner.Scan()
	libVer := scanner.Text()
	forge.Version = fmt.Sprintf(templateMap["version"], libVer)
	for isDone {

		scanner.Scan()
		args := strings.Fields(scanner.Text())

		argsNum := len(args)
		if strings.HasPrefix(args[0], "!") {
			forge.execFuncs(args, argsNum)
			continue
		}
		// here customExec
		switch args[0] {
		case "shard":
			if argsNum == 2 || argsNum == 3 {
				var definedName string
				if argsNum == 3 {
					definedName = args[2]
				} else {
					definedName = strings.ToLower(args[1])
				}
				extractedReq, ok := extractRequired(args[1])
				if !ok {
					break
				}
				for k, v := range extractedReq {
					for i := 0; i < len(v); i++ {

						fmt.Printf("%s %s: ", k, v[i])
						scanner.Scan()
						val := scanner.Text()
						keyPath := fmt.Sprintf(".%s.%s", definedName, k)

						if strings.HasPrefix(val, "!") {
							ok := forge.inputFuncs(val, keyPath, definedName, scanner)
							if !ok {
								// decrement loop
								i--
								continue
							}
						} else {
							override := fmt.Sprintf(templateMap["override"], keyPath, v[i], val)
							forge.TraitsBlock = append(forge.TraitsBlock, override)
						}
					}
				}
				cast := fmt.Sprintf(templateMap["cast"], args[1], definedName)
				forge.CastBlock = append(forge.CastBlock, cast)
			} else {
				fmt.Println("I'm taking only one arg for this which is shard filename (without extenstion)")
				// os.Exit(2)
			}
		case "done":
			forge.createVessel(vesselName)
			fmt.Println("Vessel forged")
			isDone = false
		case "exit":
			fmt.Println("Leaving forgery...")
			isDone = false
		default:
			fmt.Println("I don't understand what you mean. If you wanna exit just type it")
		}
	}
}

func (forge *Forgery) execFuncs(args []string, argsNum int) bool {
	// maybe refacotr to somtehing nicer later
	switch args[0] {
	case "!appendSec":
		// usage !appendSec <defined shard name wiht Secrets part> <shard part path to append>
		// example: !appendSec db webserver.Deployments
		if argsNum < 3 {
			fmt.Println("Invalid number of args")
			return false
		}
		execFunc := fmt.Sprintf(templateMap["appendSec"], args[2], args[1], args[1], args[2])
		forge.TraitsBlock = append(forge.TraitsBlock, execFunc)
	case "!override":
		// usage: !override <shard part path> <key> <value>
		// example: !override webserver.Deployments envVars (envMake ".env")
		if argsNum < 4 {
			fmt.Println("Invalid number of args")
			return false
		}
		execFunc := fmt.Sprintf(templateMap["override"], args[1], args[2], args[3])
		forge.TraitsBlock = append(forge.TraitsBlock, execFunc)
	case "!append":
		// usage: !append <shard part path> <key/path> <value>
		// example: !append webserver.Deployments envVars[0] (map "value" "newValue")
		if argsNum < 4 {
			fmt.Println("Invalid number of args")
			return false
		}
		execFunc := fmt.Sprintf(templateMap["append"], args[1], args[2], args[3])
		forge.TraitsBlock = append(forge.TraitsBlock, execFunc)
	case "!ls":
		shardsPath := filepath.Join(Config.ResourcePath, "shards")
		shards, err := os.ReadDir(shardsPath)
		Check(err)
		for _, e := range shards {
			fmt.Println(e.Name())
		}
	case "!dryrun":
		forge.dryRun()
	}
	return true
}

func (forge *Forgery) inputFuncs(cmd string, keyPath string, definedName string, scanner *bufio.Scanner) bool {
	switch cmd {
	case "!secMake":
		sMake := fmt.Sprintf(templateMap["secMake"], keyPath, definedName)
		forge.TraitsBlock = append(forge.TraitsBlock, sMake)
	case "!envMake":
		eMake := fmt.Sprintf(templateMap["envMake"], keyPath)
		forge.TraitsBlock = append(forge.TraitsBlock, eMake)
	case "!secRand":
		fmt.Println("Provide keys separated by spaces. No need for quotes")
		scanner.Scan()
		args := strings.Fields(scanner.Text())
		for i, s := range args {
			args[i] = fmt.Sprintf("\"%s\"", s)
		}
		arg := strings.Join(args, " ")
		sRand := fmt.Sprintf(templateMap["secRand"], keyPath, keyPath+".name", arg)
		forge.TraitsBlock = append(forge.TraitsBlock, sRand)
	default:
		fmt.Println("No idea what this function is")
		return false
	}
	return true
}

func (forge *Forgery) createVessel(vesselName string) {
	vesselName += ".ves"
	vesselPath := filepath.Join(Config.ResourcePath, "vessels", vesselName)
	forge.dryRun()
	final := strings.Join(forge.DryRun, "\n")
	os.WriteFile(vesselPath, []byte(final), 0644)
}

func (forge *Forgery) dryRun() {
	forge.DryRun = []string{}

	forge.DryRun = append(forge.ConfBlock, "---")
	forge.DryRun = append(forge.DryRun, forge.CastBlock...)
	forge.DryRun = append(forge.DryRun, "---")
	forge.DryRun = append(forge.DryRun, forge.Version)
	forge.DryRun = append(forge.DryRun, forge.StandardOpen)
	forge.DryRun = append(forge.DryRun, forge.TraitsBlock...)
	forge.DryRun = append(forge.DryRun, forge.StandardClose)
	forge.DryRun = append(forge.DryRun, forge.copyMake())

	final := strings.Join(forge.DryRun, "\n")
	fmt.Println(final)
}

func extractRequired(shardName string) (map[string][]string, bool) {
	shardName += ".shard"
	shardPath := filepath.Join(Config.ResourcePath, "shards", shardName)
	shardRaw, err := os.ReadFile(shardPath)
	if err != nil {
		fmt.Println("Couldn't find shard")
		return nil, false
	}
	shardStr := string(shardRaw)
	shardLines := strings.Split(shardStr, "\n")

	receiveMap := make(map[string][]string)
	var lastPart string
	for _, line := range shardLines {
		if strings.Contains(line, "begin") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			lastPart = line[start+1 : end]
			continue
		}
		if strings.Contains(line, "receive") {

			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			receiveMap[lastPart] = append(receiveMap[lastPart], line[start+1:end])
		}
	}
	return receiveMap, true
}

func (forge *Forgery) copyMake() string {
	globalsPresent := []string{}
	for _, v := range forge.TraitsBlock {
		if strings.Contains(v, ".Globals") || strings.Contains(v, "secMake") || strings.Contains(v, "secRand") {
			globalsPresent = append(globalsPresent, v)
		}
	}
	formatStr := `{{global "env" "prod"}}
%s
{{summon "values-prod"}}`

	globalsStr := strings.Join(globalsPresent, "\n")
	return fmt.Sprintf(formatStr, globalsStr)
}
