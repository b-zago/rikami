package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	s "strings"
	"text/template"

	"github.com/goccy/go-yaml"
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

	Config = LoadConf(confPath)

	argsNum := len(os.Args)
	if argsNum < 2 {
		fmt.Println("Well you kinda have to tell me what to do")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "summon":
		if argsNum < 3 {
			fmt.Println("You need to specify the vessel for me")
			fmt.Println("Usage: rika summon <vessel>")
			os.Exit(2)
		}
		startSummon(os.Args[2])
		fmt.Println("Summon done")
	case "forge":

		if argsNum < 3 {
			fmt.Println("You need to specify what we will be forging")
			fmt.Println("Usage: rika forge <new vessel>")
			os.Exit(2)
		}
		StartForgery(os.Args[2])
	default:
		fmt.Println("Unknown command")
		os.Exit(2)
	}
}

func startSummon(vesselName string) {
	summon := NewSummon()
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
	vesselShardsTmpl := template.Must(template.New("shards").Funcs(vesselFuncMap).Parse(vesselShards))
	vesselTraitsTmpl := template.Must(template.New("traits").Funcs(vesselFuncMap).Parse(vesselTraits))
	vesselConfsTmpl := template.Must(template.New("config").Funcs(vesselFuncMap).Parse(vesselConfs))

	err = vesselConfsTmpl.Execute(io.Discard, nil)
	Check(err)

	err = vesselShardsTmpl.Execute(io.Discard, nil)
	Check(err)

	for key, val := range summon.Shards {
		shardFile := key + ".shard"
		shardPath := filepath.Join(shardsPath, shardFile)
		shard, err := os.ReadFile(shardPath)
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
	err = vesselTraitsTmpl.Execute(io.Discard, summon.ShardMap)
	Check(err)

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
	var deps strings.Builder
	for part := range smn.Vessel {
		dep := fmt.Sprintf("{{- include \"lib.%s\" . -}}\n", part)
		deps.WriteString(dep)
	}
	// fmt.Println(deps.String())

	Check(enc.Encode(smn.Globals))
	Check(enc.Encode(smn.Vessel))
	enc.Close()

	out := bytes.ReplaceAll(buf.Bytes(), []byte("---\n"), nil)

	err := os.MkdirAll(smn.TargetPath, 0755)
	Check(err)
	addExt := fmt.Sprintf("%s.yaml", outputfile)
	writePath := filepath.Join(smn.TargetPath, addExt)
	Check(os.WriteFile(writePath, out, 0644))

	// we create main.yaml only on the first run which means adding parts/casting shards wont work on overlays (which is fine)
	mainPath := filepath.Join(smn.TargetPath, "templates", "main.yaml")
	_, err = os.Stat(filepath.Dir(mainPath))
	if err != nil {
		err = os.Mkdir(filepath.Dir(mainPath), 0755)
		Check(err)
		err = os.WriteFile(mainPath, []byte(deps.String()), 0644)
		Check(err)
	}
}
