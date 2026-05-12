package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	s "strings"
	"text/template"

	"github.com/goccy/go-yaml"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type ShardPart struct {
	Values map[string]any `yaml:",inline"`
}

type ShardMap struct {
	Values map[string]map[string]ShardPart
}

type Vessel struct {
	Values map[string][]ShardPart `yaml:",inline"`
}

var (
	shardMap         ShardMap
	shards           = make(map[string][]string)
	currentShardPart string
	currentShard     string
	shardCounter     = 0
	bindLabels       = []string{"runs-on", "name"}
	vesselFinal      = Vessel{make(map[string][]ShardPart)}
	buf              bytes.Buffer
)

func main() {
	shardMap.Values = make(map[string]map[string]ShardPart)

	shardFuncMap := template.FuncMap{
		"set":     setShardVal,
		"begin":   beginShardPart,
		"shard":   beginShard,
		"receive": receive,
		"seal":    endShard,
	}
	vesselFuncMap := template.FuncMap{
		"cast":     collectShard,
		"summon":   soulGen,
		"override": override,
	}

	vessel, err := os.ReadFile("vessels/scorevault.vesl")
	check(err)

	vesselSlice := s.Split(string(vessel), "---")
	vesselShards := vesselSlice[0]
	vesselTraits := vesselSlice[1]
	vesselShardsTmpl := template.Must(template.New("scorevault-shards").Funcs(vesselFuncMap).Parse(vesselShards))
	vesselTraitsTmpl := template.Must(template.New("scorevault-traits").Funcs(vesselFuncMap).Parse(vesselTraits))

	err = vesselShardsTmpl.Execute(io.Discard, nil)
	check(err)

	for key, val := range shards {
		shard, err := os.ReadFile(fmt.Sprintf("shards/%s.shard", key))
		check(err)
		shardString := string(shard)
		for _, definedName := range val {
			fmt.Println(key, definedName)
			shardTmpl := template.Must(template.New(definedName).Funcs(shardFuncMap).Parse(shardString))
			err = shardTmpl.Execute(io.Discard, nil)
			check(err)
		}
	}
	fmt.Println("shards map:", shards)
	fmt.Println("-----------------------")
	err = vesselTraitsTmpl.Execute(os.Stdout, shardMap.Values)
	check(err)

	fmt.Println("-----------------------")
	fmt.Println(shardMap.Values)

	fmt.Println("-----------------------")
	fmt.Println(vesselFinal)

	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	check(enc.Encode(vesselFinal))
	enc.Close()
	check(os.WriteFile("values.yaml", buf.Bytes(), 0644))
}

func beginShard(key string) string {
	// get shards[key] as an global slice and loop over it for every op or MUCH SIMPLIER just have global counter and add {{end}} function to shards that will increment it
	shardKey, present := shards[key]
	if !present {
		shardCounter = 0
	}
	currentShard = shardKey[shardCounter]
	fmt.Println("begin shard:", currentShard)
	shardMap.Values[currentShard] = make(map[string]ShardPart)
	return key
}

func beginShardPart(key string) string {
	currentShardPart = key
	shardMap.Values[currentShard][currentShardPart] = ShardPart{Values: make(map[string]any)}
	fmt.Printf("begin shard part: %s with value: %v\n", key, shardMap.Values[currentShard][currentShardPart])

	return key
}

func setShardVal(key string, val any) string {
	if slices.Contains(bindLabels, key) && shardCounter > 0 {
		val = val.(string) + "-" + strconv.Itoa(shardCounter)
	}
	shardMap.Values[currentShard][currentShardPart].Values[key] = val
	fmt.Printf("setting value under %s at %s", currentShard, currentShardPart)
	fmt.Println("set the value as: ", shardMap.Values[currentShard][currentShardPart])
	return fmt.Sprintf("%s: %v", key, val)
}

func collectShard(key string, name string) string {
	shards[key] = append(shards[key], name)
	fmt.Println(shards[key])
	return key
}

func receive(key string) string {
	shardMap.Values[currentShard][currentShardPart].Values[key] = ""
	return key
}

func override(shardPart ShardPart, key string, val string) string {
	shardPart.Values[key] = val
	return ""
}

func endShard() string {
	shardCounter++
	return ""
}

func soulGen() string {
	for shardName, shardVals := range shardMap.Values {
		fmt.Println(shardName, shardVals)
		for shardVal, shardFragment := range shardVals {
			fmt.Println(shardVal, shardFragment)
			vslGroupName := s.ToLower(shardVal)
			vslGroup := vesselFinal.Values[vslGroupName]
			vesselFinal.Values[vslGroupName] = append(vslGroup, shardFragment)
			for fragmentKey, fragmentVal := range shardFragment.Values {
				fmt.Println(fragmentKey, fragmentVal)
			}
		}
	}
	return ""
}
