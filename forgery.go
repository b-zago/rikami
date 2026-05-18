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
	"version":  "{{append .Chart.Main \"dependencies[0]\" (map \"version\" \"%s\")}}",
	"secMake":  "{{override %s \"data\" (secMake \".env.secret\" .Globals.Values.env .%s.Secrets.name)}}",
}

type Forgery struct {
	ConfBlock    []string
	CastBlock    []string
	TraitsBlock  []string
	StandardOpen string
	Version      string
}

func StartForgery(vesselName string) {
	forge := Forgery{
		ConfBlock:   []string{"{{conf \"Chart\"}}"},
		CastBlock:   []string{"{{cast \"Chart\" \"Chart\"}}"},
		TraitsBlock: []string{},
		Version:     "",
		StandardOpen: `{{- global "env" "staging" -}}

{{override .Chart.Main "description" "App description"}}
{{request .Chart.Main "name" "Chart name"}}
{{$prefix := print .Chart.Main.name "-"}}

{{target .Chart.Main.name}}
		`,
	}

	fmt.Println("Forging", vesselName)

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
					for _, prop := range v {

						fmt.Printf("%s %s: ", k, prop)
						scanner.Scan()
						val := scanner.Text()
						keyPath := fmt.Sprintf(".%s.%s", definedName, k)

						if strings.HasPrefix(val, "!") {
							switch val {
							case "!secMake":
								sMake := fmt.Sprintf(templateMap["secMake"], keyPath, definedName)
								forge.TraitsBlock = append(forge.TraitsBlock, sMake)
								continue
							default:
								fmt.Println("No idea what this function is")
								// refactor to index based loop so that i can reset the iteration here
							}
						}

						override := fmt.Sprintf(templateMap["override"], keyPath, prop, val)
						forge.TraitsBlock = append(forge.TraitsBlock, override)
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

func (forge *Forgery) createVessel(vesselName string) {
	vesselName += ".ves"
	vesselPath := filepath.Join(Config.ResourcePath, "vessels", vesselName)
	forge.ConfBlock = append(forge.ConfBlock, "---")
	forge.ConfBlock = append(forge.ConfBlock, forge.CastBlock...)
	forge.ConfBlock = append(forge.ConfBlock, "---")
	forge.ConfBlock = append(forge.ConfBlock, forge.Version)
	forge.ConfBlock = append(forge.ConfBlock, forge.StandardOpen)
	forge.ConfBlock = append(forge.ConfBlock, forge.TraitsBlock...)

	final := strings.Join(forge.ConfBlock, "\n")
	os.WriteFile(vesselPath, []byte(final), 0644)
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
