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

func (summon *Summon) beginShard(key string) string {
	// get shards[key] as an global slice and loop over it for every op or MUCH SIMPLIER just have global counter and add {{end}} func (summon *Summon)tion to shards that will increment it
	shardKey, present := summon.Shards[key]
	if !present {
		summon.ShardCounter = 0
	}
	summon.CurrentShard = shardKey[summon.ShardCounter]
	fmt.Println("begin shard:", summon.CurrentShard)
	summon.ShardMap[summon.CurrentShard] = make(map[string]ShardPart)
	return key
}

func (summon *Summon) beginShardPart(key string) string {
	summon.CurrentShardPart = key
	summon.ShardMap[summon.CurrentShard][summon.CurrentShardPart] = ShardPart{Values: make(map[string]any)}
	fmt.Printf("begin shard part: %s with value: %v\n", key, summon.ShardMap[summon.CurrentShard][summon.CurrentShardPart])

	return key
}

func (summon *Summon) setShardVal(key string, val any) string {
	if slices.Contains(summon.BindLabels, key) && summon.ShardCounter > 0 {
		val = val.(string) + "-" + strconv.Itoa(summon.ShardCounter)
	}
	summon.ShardMap[summon.CurrentShard][summon.CurrentShardPart].Values[key] = val
	fmt.Printf("setting value under %s at %s", summon.CurrentShard, summon.CurrentShardPart)
	fmt.Println("set the value as: ", summon.ShardMap[summon.CurrentShard][summon.CurrentShardPart])
	return fmt.Sprintf("%s: %v", key, val)
}

func (summon *Summon) collectShard(key string, name string) string {
	summon.Shards[key] = append(summon.Shards[key], name)
	fmt.Println(summon.Shards[key])
	return key
}

func (summon *Summon) receive(key string) string {
	summon.ShardMap[summon.CurrentShard][summon.CurrentShardPart].Values[key] = ""
	return key
}

func (summon *Summon) override(shardPart ShardPart, key string, val string) string {
	shardPart.Values[key] = val
	return ""
}

func (summon *Summon) endShard() string {
	summon.ShardCounter++
	return ""
}

func (summon *Summon) soulGen() string {
	for shardName, shardVals := range summon.ShardMap {
		fmt.Println(shardName, shardVals)
		for shardVal, shardFragment := range shardVals {
			fmt.Println(shardVal, shardFragment)
			vslGroupName := s.ToLower(shardVal)
			vslGroup := summon.Vessel[vslGroupName]
			summon.Vessel[vslGroupName] = append(vslGroup, shardFragment)
			for fragmentKey, fragmentVal := range shardFragment.Values {
				fmt.Println(fragmentKey, fragmentVal)
			}
		}
	}
	return ""
}
