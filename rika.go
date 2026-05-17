package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	s "strings"
	"text/template"

	"github.com/goccy/go-yaml"
)

func Check(e error) {
	if e != nil {
		panic(e)
	}
}

var Reader = bufio.NewReader(os.Stdin)

func main() {
	homePath, err := os.UserHomeDir()
	Check(err)
	confPath := filepath.Join(homePath, ".config", "rikami", "conf")

	conf := LoadConf(confPath)

	fmt.Println(conf, "CONFIGGGG")

	summon := NewSummon(conf.BindLabels)
	shardFuncMap := template.FuncMap{
		"set":     summon.setShardVal,
		"begin":   summon.beginShardPart,
		"shard":   summon.beginShard,
		"receive": summon.receive,
		"seal":    summon.endShard,
		"list":    summon.makeList,
		"map":     MakeMap,
		"envMake": EnvMake,
		"bind":    summon.bindParts,
	}
	vesselFuncMap := template.FuncMap{
		"cast":     summon.collectShard,
		"summon":   summon.soulGen,
		"override": summon.override,
		"append":   summon.appendObj,
		"list":     summon.makeList,
		"map":      MakeMap,
		"partMake": summon.partMake,
		"envMake":  EnvMake,
		"envGen":   summon.envGen,
		"global":   summon.globalSet,
		"conf":     summon.confAdd,
		"secMake":  SecMake,
		"vessel":   summon.bindAll,
		"request":  summon.request,
		"target":   summon.setTarget,
	}

	vessel, err := os.ReadFile("vessels/scorevault.vesl")
	Check(err)

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
	Check(err)

	err = vesselShardsTmpl.Execute(io.Discard, nil)
	Check(err)

	for key, val := range summon.Shards {
		shard, err := os.ReadFile(fmt.Sprintf("shards/%s.shard", key))
		Check(err)
		shardString := string(shard)
		for _, definedName := range val {
			shardTmpl := template.Must(template.New(definedName).Funcs(shardFuncMap).Parse(shardString))
			err = shardTmpl.Execute(io.Discard, nil)
			Check(err)
		}
	}
	// fmt.Println("shards map:", summon.Shards)
	// fmt.Println("-----------------------")
	err = vesselTraitsTmpl.Execute(os.Stdout, summon.ShardMap)
	Check(err)

	fmt.Println("-----------------------")
	fmt.Println(summon.ShardMap)

	fmt.Println("-----------------------")
	fmt.Println(summon.Vessel)

	fmt.Println(len(summon.Vessel))

	for _, conf := range summon.ConfShards {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
		Check(enc.Encode(summon.ShardMap[conf]["Main"]))
		enc.Close()
		filename := fmt.Sprintf("%s.yaml", conf)
		writePath := filepath.Join(summon.TargetPath, filename)
		Check(os.WriteFile(writePath, buf.Bytes(), 0644))
	}
}

func ExecuteVessel(smn *Summon, outputfile string) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	smn.Globals["deps"] = []string{}
	for part := range smn.Vessel {
		smn.Globals["deps"] = append(smn.Globals["deps"].([]string), part)
	}

	Check(enc.Encode(smn.Globals))
	Check(enc.Encode(smn.Vessel))
	enc.Close()

	err := os.MkdirAll(smn.TargetPath, 0755)
	Check(err)
	addExt := fmt.Sprintf("%s.yaml", outputfile)
	writePath := filepath.Join(smn.TargetPath, addExt)
	Check(os.WriteFile(writePath, buf.Bytes(), 0644))
	buf.Reset()
}
