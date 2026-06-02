package summon

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	s "strings"
	"text/template"

	"github.com/b-zago/rikami/types"
	"github.com/goccy/go-yaml"
)

var Config *types.Conf

func StartSummonLocal(vesselName string, targetOverride string, rikaCfg *types.Conf) {
	Config = rikaCfg
	summon := NewSummon()
	summon.TargetOverride = targetOverride
	shardFuncMap := template.FuncMap{
		"set":     summon.setShardVal,
		"begin":   summon.beginShardPart,
		"shard":   summon.beginShard,
		"receive": summon.receive,
		"seal":    summon.endShard,
		"list":    summon.makeList,
		"map":     MakeMap,
		"envGen":  summon.envGen,
		"bind":    summon.bindParts,
		"suffix":  summon.suffix,
		"prefix":  summon.prefix,
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
		"secMake":  summon.SecMake,
		"request":  summon.request,
		"target":   summon.setTarget,
		"secRand":  summon.SecRand,
	}

	shardsPath := filepath.Join(Config.ResourcePath, "shards")
	vesselName += ".ves"
	vesselPath := filepath.Join(Config.ResourcePath, "vessels", vesselName)
	vessel, err := os.ReadFile(vesselPath)
	if err != nil {
		log.Fatalf("Could not read file %s. Error:\n%v", vesselPath, err)
	}

	vesselSlice := s.Split(string(vessel), "---")
	var vesselConfs string
	vesselPartJump := 0
	if len(vesselSlice) > 2 {
		vesselConfs = vesselSlice[0]
		vesselPartJump = 1
	}
	vesselShards := vesselSlice[vesselPartJump]
	vesselTraits := vesselSlice[vesselPartJump+1]
	vesselShardsTmpl := template.Must(template.New("shards").Funcs(vesselFuncMap).Parse(vesselShards))
	vesselTraitsTmpl := template.Must(template.New("traits").Funcs(vesselFuncMap).Parse(vesselTraits))
	vesselConfsTmpl := template.Must(template.New("config").Funcs(vesselFuncMap).Parse(vesselConfs))

	err = vesselConfsTmpl.Execute(io.Discard, nil)
	if err != nil {
		log.Fatalf("Could not execute template %s. Error:\n%v", vesselConfsTmpl.Name(), err)
	}

	err = vesselShardsTmpl.Execute(io.Discard, nil)
	if err != nil {
		log.Fatalf("Could not execute template %s. Error:\n%v", vesselShardsTmpl.Name(), err)
	}

	for key, val := range summon.Shards {
		shardFile := key + ".shard"
		shardPath := filepath.Join(shardsPath, shardFile)
		shard, err := os.ReadFile(shardPath)
		if err != nil {
			log.Fatalf("Could not read file %s. Error:\n%v", shardPath, err)
		}
		shardString := string(shard)
		for _, definedName := range val {
			shardTmpl := template.Must(template.New(definedName).Funcs(shardFuncMap).Parse(shardString))
			err = shardTmpl.Execute(io.Discard, nil)
			if err != nil {
				log.Fatalf("Could not execute template %s. Error:\n%v", shardTmpl.Name(), err)
			}
		}
	}
	// fmt.Println("shards map:", summon.Shards)
	// fmt.Println("-----------------------")
	err = vesselTraitsTmpl.Execute(io.Discard, summon.ShardMap)
	if err != nil {
		log.Fatalf("Could not execute template %s. Error:\n%v", vesselTraitsTmpl.Name(), err)
	}

	for _, conf := range summon.ConfShards {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
		err := enc.Encode(summon.ShardMap[conf]["Main"])
		if err != nil {
			log.Fatalf("Could not encode to yaml. Error:\n%v", err)
		}
		enc.Close()
		filename := fmt.Sprintf("%s.yaml", conf)
		writePath := filepath.Join(summon.TargetPath, filename)
		err = os.WriteFile(writePath, buf.Bytes(), 0644)
		if err != nil {
			log.Fatalf("Could not write to file %s. Error:\n%v", writePath, err)
		}
	}
}

func ExecuteVessel(smn *Summon, outputfile string) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	var deps strings.Builder
	for part := range smn.Vessel {
		dep := fmt.Sprintf("{{- include \"lib.%s\" . -}}\n", part)
		deps.WriteString(dep)
	}
	// fmt.Println(deps.String())

	err := enc.Encode(smn.Globals)
	if err != nil {
		log.Fatalf("Could not encode globals to yaml. Error:\n%v", err)
	}
	err = enc.Encode(smn.Vessel)
	if err != nil {
		log.Fatalf("Could not encode vessel to yaml. Error:\n%v", err)
	}
	enc.Close()

	out := bytes.ReplaceAll(buf.Bytes(), []byte("---\n"), nil)

	err = os.MkdirAll(smn.TargetPath, 0755)
	if err != nil {
		log.Fatalf("Could not made a directory %s. Error:\n%v", smn.TargetPath, err)
	}
	addExt := fmt.Sprintf("%s.yaml", outputfile)
	writePath := filepath.Join(smn.TargetPath, addExt)
	err = os.WriteFile(writePath, out, 0644)
	if err != nil {
		log.Fatalf("Could not write to file %s. Error:\n%v", writePath, err)
	}

	// we create main.yaml only on the first run which means adding parts/casting shards wont work on overlays (which is fine)
	mainPath := filepath.Join(smn.TargetPath, "templates", "main.yaml")
	_, err = os.Stat(filepath.Dir(mainPath))
	if err != nil {
		err = os.Mkdir(filepath.Dir(mainPath), 0755)
		if err != nil {
			log.Fatalf("Could not made a directory %s. Error:\n%v", filepath.Dir(mainPath), err)
		}
		err = os.WriteFile(mainPath, []byte(deps.String()), 0644)
		if err != nil {
			log.Fatalf("Could not write to file %s. Error:\n%v", mainPath, err)
		}
	}
}
