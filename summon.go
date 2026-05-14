package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	s "strings"
)

var ShardReceivePtr = &struct{}{}

type ShardMapType map[string]map[string]map[string]any

type BindRef struct {
	Shard      string
	Part       string
	Key        string
	TargetPath []string
}

type Summon struct {
	ShardMap         ShardMapType
	Shards           map[string][]string
	CurrentShardPart string
	CurrentShard     string
	CurrentShardDef  string
	ShardCounter     int
	BindLabels       []string
	Vessel           map[string][]map[string]any `yaml:",inline"`
	EnvVars          map[string]string
	BindRefs         []BindRef
}

func NewSummon() *Summon {
	summon := Summon{}
	summon.Shards = make(map[string][]string)
	summon.ShardCounter = 0
	summon.BindLabels = []string{"runs-on", "name"} // load from config later
	summon.Vessel = make(map[string][]map[string]any)
	summon.ShardMap = make(map[string]map[string]map[string]any)
	summon.EnvVars = make(map[string]string)

	return &summon
}

func (smn *Summon) beginShard(key string) string {
	// get shards[key] as an global slice and loop over it for every op or MUCH SIMPLIER just have global counter and add {{end}} func (smn *Summon)tion to shards that will increment it
	fmt.Println("trying to begin")
	fmt.Println(key)
	if smn.CurrentShardDef != key {
		smn.ShardCounter = 0
	}
	smn.CurrentShardDef = key
	shardKey := smn.Shards[key]

	fmt.Println(shardKey)
	smn.CurrentShard = shardKey[smn.ShardCounter]
	fmt.Println("begin shard:", smn.CurrentShard)
	smn.ShardMap[smn.CurrentShard] = make(map[string]map[string]any)
	return key
}

func (smn *Summon) beginShardPart(key string) string {
	smn.CurrentShardPart = key
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart] = make(map[string]any)
	fmt.Printf("begin shard part: %s with value: %v\n", key, smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])

	return key
}

func (smn *Summon) setShardVal(key string, val any) string {
	switch v := val.(type) {
	case string:
		if slices.Contains(smn.BindLabels, key) && smn.ShardCounter > 0 {
			val = v + "-" + strconv.Itoa(smn.ShardCounter)
		}
	}

	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart][key] = val
	fmt.Printf("setting value under %s at %s", smn.CurrentShard, smn.CurrentShardPart)
	fmt.Println("set the value as: ", smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])
	return fmt.Sprintf("%s: %v", key, val)
}

func (smn *Summon) collectShard(key string, name string) string {
	smn.Shards[key] = append(smn.Shards[key], name)
	fmt.Println("collecting")
	fmt.Println(smn.Shards)
	return key
}

func (smn *Summon) receive(key string) string {
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart][key] = ShardReceivePtr
	return key
}

func (smn *Summon) override(shardPart map[string]any, key string, val any) string {
	shardPart[key] = val
	return ""
}

func (smn *Summon) endShard() string {
	smn.ShardCounter++
	return ""
}

func (smn *Summon) soulGen() string {
	for _, ref := range smn.BindRefs {
		shard := smn.ShardMap[ref.Shard]
		firstKey := ref.TargetPath[0]
		part, ok := shard[firstKey]
		if !ok {
			panic("Something went wrong while binding")
		}
		var sender any = part

		for _, p := range ref.TargetPath[1 : len(ref.TargetPath)-1] {
			v, ok := sender.(map[string]any)
			if !ok {
				break
			}
			next, ok := v[p]
			if !ok {
				break
			}
			sender = next
		}

		last := ref.TargetPath[len(ref.TargetPath)-1]
		smn.ShardMap[ref.Shard][ref.Part][ref.Key] = sender.(map[string]any)[last]
	}
	for shardName, shardVals := range smn.ShardMap {
		for shardVal, shardFragment := range shardVals {
			vslGroupName := s.ToLower(shardVal)
			vslGroup := smn.Vessel[vslGroupName]
			smn.Vessel[vslGroupName] = append(vslGroup, shardFragment)
			for fragmentKey, fragmentVal := range shardFragment {
				fmt.Println(fragmentKey, fragmentVal)
				fmt.Println(shardName)

				// if fragmentVal == ShardReceivePtr {
				// 	panic(fmt.Sprintf("You need to set %s at %s in %s", fragmentKey, shardVal, shardName))
				// }
			}
		}
	}

	return ""
}

func (smn *Summon) makeList(elements ...any) []any {
	var newList []any
	newList = append(newList, elements...)
	return newList
}

func (smn *Summon) makeMap(elements ...any) map[string]any {
	newMap := make(map[string]any)
	var prevKey string
	for i, element := range elements {
		if i%2 == 0 {
			key, ok := element.(string)
			if ok {
				newMap[key] = nil
				prevKey = key
			} else {
				panic("Wrong type")
			}
			continue
		}
		_, ok := newMap[prevKey]
		if ok {
			newMap[prevKey] = element
		} else {
			panic("Mismatched key-values in a map generation")
		}
	}
	return newMap
}

func (smn *Summon) appendObj(shardPart map[string]any, key string, val any) string {
	switch v := shardPart[key].(type) {
	case map[string]any:
		newMap, ok := val.(map[string]any)
		if ok {
			maps.Copy(v, newMap)
		} else {
			panic("Wrong type") // specify error later
		}
	case []any:
		vList, ok := val.([]any)
		if ok {
			shardPart[key] = append(v, vList...)
		} else {
			panic("Wrong type")
		}
	}
	return ""
}

func (smn *Summon) bindParts(key string, targetStr string) string {
	targetSplit := strings.Split(targetStr, "@")
	_, ok := smn.ShardMap[smn.CurrentShard][targetSplit[0]]
	if !ok {
		panic("Wrong target to bind to")
	}
	smn.BindRefs = append(smn.BindRefs, BindRef{Shard: smn.CurrentShard, Part: smn.CurrentShardPart, Key: key, TargetPath: targetSplit})
	//smn.SgardMap[smn.CurrentShard][smn.CurrentShardPart][key] = targetVal
	return ""
}

func EnvMake(path string) []map[string]string {
	rawEnvs, err := os.ReadFile(path)
	check(err)
	envs := strings.TrimSpace(string(rawEnvs))
	fmt.Println(envs)
	lineSplit := strings.Split(envs, "\n")
	fmt.Println("LINE SPLIT TEST")
	var list []map[string]string
	for _, env := range lineSplit {
		valSplit := strings.Split(env, "=")
		m := make(map[string]string)
		m["name"] = valSplit[0]
		m["value"] = valSplit[1]
		list = append(list, m)
	}
	fmt.Println(list)
	return list
}
