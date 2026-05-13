package main

import (
	"fmt"
	"slices"
	"strconv"
	s "strings"
)

type ShardPart struct {
	Values map[string]any `yaml:",inline"`
}

type Summon struct {
	ShardMap         map[string]map[string]ShardPart
	Shards           map[string][]string
	CurrentShardPart string
	CurrentShard     string
	ShardCounter     int
	BindLabels       []string
	Vessel           map[string][]ShardPart `yaml:",inline"`
}

func NewSummon() *Summon {
	summon := Summon{}
	summon.Shards = make(map[string][]string)
	summon.ShardCounter = 0
	summon.BindLabels = []string{"runs-on", "name"} // load from config later
	summon.Vessel = make(map[string][]ShardPart)
	summon.ShardMap = make(map[string]map[string]ShardPart)

	return &summon
}

func (smn *Summon) beginShard(key string) string {
	// get shards[key] as an global slice and loop over it for every op or MUCH SIMPLIER just have global counter and add {{end}} func (smn *Summon)tion to shards that will increment it
	shardKey, present := smn.Shards[key]
	if !present {
		smn.ShardCounter = 0
	}
	smn.CurrentShard = shardKey[smn.ShardCounter]
	fmt.Println("begin shard:", smn.CurrentShard)
	smn.ShardMap[smn.CurrentShard] = make(map[string]ShardPart)
	return key
}

func (smn *Summon) beginShardPart(key string) string {
	smn.CurrentShardPart = key
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart] = ShardPart{Values: make(map[string]any)}
	fmt.Printf("begin shard part: %s with value: %v\n", key, smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])

	return key
}

func (smn *Summon) setShardVal(key string, val any) string {
	switch v := val.(type) {
	case string:
		if slices.Contains(smn.BindLabels, key) && smn.ShardCounter > 0 {
			val = v + "-" + strconv.Itoa(smn.ShardCounter)
		}
	case int:
		val = v
	}

	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart].Values[key] = val
	fmt.Printf("setting value under %s at %s", smn.CurrentShard, smn.CurrentShardPart)
	fmt.Println("set the value as: ", smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])
	return fmt.Sprintf("%s: %v", key, val)
}

func (smn *Summon) collectShard(key string, name string) string {
	smn.Shards[key] = append(smn.Shards[key], name)
	fmt.Println(smn.Shards[key])
	return key
}

func (smn *Summon) receive(key string) string {
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart].Values[key] = ""
	return key
}

func (smn *Summon) override(shardPart ShardPart, key string, val any) string {
	switch v := val.(type) {
	case int:
		val = v
	}
	shardPart.Values[key] = val
	return ""
}

func (smn *Summon) endShard() string {
	smn.ShardCounter++
	return ""
}

func (smn *Summon) soulGen() string {
	for shardName, shardVals := range smn.ShardMap {
		fmt.Println(shardName, shardVals)
		for shardVal, shardFragment := range shardVals {
			fmt.Println(shardVal, shardFragment)
			vslGroupName := s.ToLower(shardVal)
			vslGroup := smn.Vessel[vslGroupName]
			smn.Vessel[vslGroupName] = append(vslGroup, shardFragment)
			for fragmentKey, fragmentVal := range shardFragment.Values {
				fmt.Println(fragmentKey, fragmentVal)
			}
		}
	}
	return ""
}
