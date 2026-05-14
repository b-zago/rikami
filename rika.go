package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	s "strings"
	"text/template"

	"github.com/goccy/go-yaml"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	summon := NewSummon()
	shardFuncMap := template.FuncMap{
		"set":     summon.setShardVal,
		"begin":   summon.beginShardPart,
		"shard":   summon.beginShard,
		"receive": summon.receive,
		"seal":    summon.endShard,
		"list":    summon.makeList,
		"map":     summon.makeMap,
		"envMake": EnvMake,
		"bind":    summon.bindParts,
	}
	vesselFuncMap := template.FuncMap{
		"cast":     summon.collectShard,
		"summon":   summon.soulGen,
		"override": summon.override,
		"append":   summon.appendObj,
		"list":     summon.makeList,
		"map":      summon.makeMap,
		"partMake": summon.partMake,
		"envMake":  EnvMake,
		"global":   summon.globalSet,
		"conf":     summon.confAdd,
	}

	vessel, err := os.ReadFile("vessels/scorevault.vesl")
	check(err)

	vesselSlice := s.Split(string(vessel), "---")
	var vesselConfs string
	vesselPartJump := 0
	if len(vesselSlice) > 2 {
		vesselConfs = vesselSlice[0]
		vesselPartJump = 1
	}
	vesselShards := vesselSlice[vesselPartJump]
	vesselTraits := vesselSlice[vesselPartJump+1]
	vesselShardsTmpl := template.Must(template.New("scorevault-shards").Funcs(vesselFuncMap).Parse(vesselShards))
	vesselTraitsTmpl := template.Must(template.New("scorevault-traits").Funcs(vesselFuncMap).Parse(vesselTraits))
	vesselConfsTmpl := template.Must(template.New("vessel-config").Funcs(vesselFuncMap).Parse(vesselConfs))

	err = vesselConfsTmpl.Execute(io.Discard, nil)
	check(err)

	err = vesselShardsTmpl.Execute(io.Discard, nil)
	check(err)

	for key, val := range summon.Shards {
		shard, err := os.ReadFile(fmt.Sprintf("shards/%s.shard", key))
		check(err)
		shardString := string(shard)
		for _, definedName := range val {
			shardTmpl := template.Must(template.New(definedName).Funcs(shardFuncMap).Parse(shardString))
			err = shardTmpl.Execute(io.Discard, nil)
			check(err)
		}
	}
	// fmt.Println("shards map:", summon.Shards)
	// fmt.Println("-----------------------")
	err = vesselTraitsTmpl.Execute(io.Discard, summon.ShardMap)
	check(err)

	fmt.Println("-----------------------")
	fmt.Println(summon.ShardMap)

	fmt.Println("-----------------------")
	fmt.Println(summon.Vessel)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	check(enc.Encode(summon.Globals))
	check(enc.Encode(summon.Vessel))
	enc.Close()
	check(os.WriteFile("values.yaml", buf.Bytes(), 0644))

	buf.Reset()

	for _, conf := range summon.ConfShards {
		check(enc.Encode(summon.ShardMap[conf]["Main"]))
		filename := fmt.Sprintf("%s.yaml", conf)
		check(os.WriteFile(filename, buf.Bytes(), 0644))
		buf.Reset()
	}
}
