package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
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
	IndexMap   map[int]int
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
	Globals          map[string]any
	ConfShards       []string
	TargetPath       string
}

func NewSummon() *Summon {
	summon := Summon{}
	summon.Shards = make(map[string][]string)
	summon.ShardCounter = 0
	summon.BindLabels = Config.BindLabels
	summon.Vessel = make(map[string][]map[string]any)
	summon.ShardMap = make(map[string]map[string]map[string]any)
	summon.EnvVars = make(map[string]string)
	summon.Globals = make(map[string]any)

	return &summon
}

func (smn *Summon) beginShard(key string) string {
	// fmt.Println("trying to begin")
	if smn.CurrentShardDef != key {
		smn.ShardCounter = 0
	}
	smn.CurrentShardDef = key
	shardKey := smn.Shards[key]

	smn.CurrentShard = shardKey[smn.ShardCounter]
	// fmt.Println("begin shard:", smn.CurrentShard)
	smn.ShardMap[smn.CurrentShard] = make(map[string]map[string]any)
	return key
}

func (smn *Summon) beginShardPart(key string) string {
	smn.CurrentShardPart = key
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart] = make(map[string]any)
	// fmt.Printf("begin shard part: %s with value: %v\n", key, smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])

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
	// fmt.Printf("setting value under %s at %s", smn.CurrentShard, smn.CurrentShardPart)
	// fmt.Println("set the value as: ", smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart])
	return fmt.Sprintf("%s: %v", key, val)
}

func (smn *Summon) collectShard(key string, name string) string {
	smn.Shards[key] = append(smn.Shards[key], name)
	return key
}

func (smn *Summon) receive(key string) string {
	smn.ShardMap[smn.CurrentShard][smn.CurrentShardPart][key] = ShardReceivePtr
	return key
}

func (smn *Summon) override(shardPart map[string]any, key string, val any) string {
	shardPart[key] = val
	// check here if we should even start binds, or maybe just do precise binds instaed of all
	smn.bindAll()
	return ""
}

func (smn *Summon) request(shardPart map[string]any, key string, name string) bool {
	fmt.Printf("Please provide me %s:", name)
	val, err := Reader.ReadString('\n')
	Check(err)
	cleanVal := strings.TrimSpace(val)
	shardPart[key] = cleanVal
	return true
}

func (smn *Summon) setTarget(path string) bool {
	smn.TargetPath = path
	return true
}

func (smn *Summon) endShard() string {
	smn.ShardCounter++
	return ""
}

func (smn *Summon) bindAll() string {
	for _, ref := range smn.BindRefs {
		shard := smn.ShardMap[ref.Shard]
		firstKey := ref.TargetPath[0]
		part, ok := shard[firstKey]
		if !ok {
			panic("Something went wrong while binding")
		}
		var sender any = part

		for i, p := range ref.TargetPath[1 : len(ref.TargetPath)-1] {

			v, ok := sender.(map[string]any)
			if !ok {
				panic("stop right there citizen")
			}
			// fmt.Println(sender, "sender")
			inx, ok := ref.IndexMap[i+1]

			if ok {
				list := v[p]
				n, ok := list.([]any)
				if !ok {
					panic("stop right there citizen")
				}
				sender = n[inx]
			} else {

				next, ok := v[p]
				if !ok {
					panic("stop right there citizen")
				}
				sender = next

				// fmt.Println(sender, "sender end")
			}
		}

		last := ref.TargetPath[len(ref.TargetPath)-1]
		if strings.HasSuffix(last, "]") {

			index, lastClean, noIndex := extractIndex(last)
			if noIndex {

				finalVal := []any{sender.(map[string]any)[lastClean]}
				smn.ShardMap[ref.Shard][ref.Part][ref.Key] = finalVal
			} else {

				finalList := sender.(map[string]any)[lastClean]
				assertVal, ok := finalList.([]any)
				if !ok {
					// fmt.Println(last, sender)
					panic("Something is wrong with list bind on type assertion")
				}
				finalVal := assertVal[index]
				smn.ShardMap[ref.Shard][ref.Part][ref.Key] = finalVal
			}
		} else {
			// fmt.Println("did it for", sender)
			// fmt.Println(smn.ShardMap[ref.Shard][ref.Part][ref.Key])
			smn.ShardMap[ref.Shard][ref.Part][ref.Key] = sender.(map[string]any)[last]

			// fmt.Println(smn.ShardMap[ref.Shard][ref.Part][ref.Key])
		}
	}
	return ""
}

func (smn *Summon) soulGen(outputFile string) string {
	smn.Vessel = make(map[string][]map[string]any)
	for shardName, shardVals := range smn.ShardMap {
		for shardVal, shardFragment := range shardVals {
			if !slices.Contains(smn.ConfShards, shardName) && shardName != "Globals" {
				vslGroupName := s.ToLower(shardVal)
				vslGroup := smn.Vessel[vslGroupName]
				smn.Vessel[vslGroupName] = append(vslGroup, shardFragment)
			}
			for fragmentKey, fragmentVal := range shardFragment {
				if fragmentVal == ShardReceivePtr {
					panic(fmt.Sprintf("You need to set %s at %s in %s", fragmentKey, shardVal, shardName))
				}
			}
		}
	}
	smn.BindRefs = nil // disable binding for all overlays
	ExecuteVessel(smn, outputFile)
	return ""
}

func (smn *Summon) makeList(elements ...any) []any {
	var newList []any
	newList = append(newList, elements...)
	return newList
}

func MakeMap(elements ...any) map[string]any {
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
	var index int
	insideList := false
	if strings.HasSuffix(key, "]") {
		inx, rawKey, noIndex := extractIndex(key)
		if noIndex {
			panic("Need to provide an index to access a list")
		}
		key = rawKey
		index = inx
		insideList = true
	}
	// fmt.Println(shardPart[key], "hellooooo")
	switch v := shardPart[key].(type) {
	case map[string]any:
		newMap, ok := val.(map[string]any)
		if ok {
			maps.Copy(v, newMap)
		} else {
			panic("Wrong type") // specify error later
		}
	case []any:

		if insideList {

			// fmt.Println("yes")
			inner, ok := v[index].(map[string]any)
			if !ok {
				panic("Wrong type")
			}
			newMap, ok := val.(map[string]any)
			if !ok {
				panic("Wrong type")
			}
			maps.Copy(inner, newMap)
			shardPart[key] = v
		} else {
			vList, ok := val.([]any)
			if ok {
				shardPart[key] = append(v, vList...)
			} else {
				panic("Wrong type")
			}
		}

	}
	return ""
}

func extractIndex(rawStr string) (int, string, bool) {
	valSplit := strings.Split(rawStr, "[")
	cleanString := valSplit[0]
	if len(valSplit) == 1 || len(valSplit) >= 3 {
		panic("Something is wrong with list bind on checking len")
	}
	index := strings.TrimSuffix(valSplit[1], "]")
	if len(index) == 0 {
		return -1, cleanString, true
	} else {
		indexNum, err := strconv.Atoi(index)
		Check(err)
		return indexNum, cleanString, false
	}
}

func (smn *Summon) bindParts(key string, targetStr string) string {
	targetSplit := strings.Split(targetStr, "@")
	_, ok := smn.ShardMap[smn.CurrentShard][targetSplit[0]]
	if !ok {
		panic("Wrong target to bind to")
	}

	indexMap := make(map[int]int)

	var noIndex bool
	for i, v := range targetSplit[1 : len(targetSplit)-1] {
		if strings.HasSuffix(v, "]") {
			inx := i + 1
			indexMap[inx], targetSplit[inx], noIndex = extractIndex(v)
			if noIndex {
				panic("Index not provided in a list")
			}
		}
	}
	// fmt.Println(targetSplit, "targetto splitto firsto")
	// fmt.Println(indexMap)
	smn.BindRefs = append(smn.BindRefs, BindRef{Shard: smn.CurrentShard, Part: smn.CurrentShardPart, Key: key, TargetPath: targetSplit, IndexMap: indexMap})
	//smn.SgardMap[smn.CurrentShard][smn.CurrentShardPart][key] = targetVal
	return ""
}

func (smn *Summon) partMake(shard map[string]map[string]any, part string) string {
	shard[part] = make(map[string]any)
	return ""
}

func (smn *Summon) globalSet(key string, val any) string {
	_, ok := smn.ShardMap["Globals"]["Values"]
	if !ok {
		smn.ShardMap["Globals"] = make(map[string]map[string]any)
	}
	smn.Globals[key] = val
	smn.ShardMap["Globals"]["Values"] = smn.Globals
	return ""
}

func (smn *Summon) confAdd(name string) string {
	smn.ConfShards = append(smn.ConfShards, name)
	return ""
}

func (smn *Summon) envGen(elements ...string) []map[string]string {
	var finalVal []map[string]string
	for i := 0; i < len(elements)-1; i += 2 {
		finalVal = append(finalVal, map[string]string{
			"name":  elements[i],
			"value": elements[i+1],
		})
	}
	return finalVal
}

func (smn *Summon) SecMake(path string, name string) map[string]string {
	nsAny := smn.ShardMap["Globals"]["Values"]["env"]
	ns, ok := nsAny.(string)
	if !ok {
		panic("Global env must be a string")
	}
	envLoad := EnvMake(path)
	secMap := make(map[string]string)

	for _, env := range envLoad {
		ksCmd := exec.Command("kubeseal", "--raw", "--namespace", ns, "--name", name)
		ksIn, _ := ksCmd.StdinPipe()
		ksOut, _ := ksCmd.StdoutPipe()
		err := ksCmd.Start()
		Check(err)
		_, err = ksIn.Write([]byte(env["value"]))
		Check(err)
		ksIn.Close()
		ksBytes, _ := io.ReadAll(ksOut)
		err = ksCmd.Wait()
		Check(err)
		secMap[env["name"]] = string(ksBytes)
	}
	return secMap
}

func (smn *Summon) SecRand(keys ...string) map[string]string {
	// first arg is the name of the secret
	vals := make(map[string]string)
	sealedVals := make(map[string]string)

	nsAny := smn.ShardMap["Globals"]["Values"]["env"]
	ns, ok := nsAny.(string)
	if !ok {
		panic("Global env must be a string")
	}

	for _, s := range keys[1:] {
		bytes := make([]byte, 8)
		rand.Read(bytes)
		// val := hex.EncodeToString(bytes)
		val := hex.EncodeToString(bytes)
		vals[s] = val

		ksCmd := exec.Command("kubeseal", "--raw", "--namespace", ns, "--name", keys[0])
		ksIn, _ := ksCmd.StdinPipe()
		ksOut, _ := ksCmd.StdoutPipe()
		err := ksCmd.Start()
		Check(err)
		_, err = ksIn.Write([]byte(val))
		Check(err)
		ksIn.Close()
		ksBytes, _ := io.ReadAll(ksOut)
		err = ksCmd.Wait()
		Check(err)
		sealedVals[s] = string(ksBytes)
	}
	secFile := ".env." + ns + "." + keys[0] + ".secret"
	f, err := os.Create(secFile)
	Check(err)
	defer f.Close()

	for k, v := range vals {
		line := fmt.Sprintf("%s=%s\n", k, v)
		f.WriteString(line)
	}

	return sealedVals
}

func EnvMake(path string) []map[string]string {
	rawEnvs, err := os.ReadFile(path)
	Check(err)
	envs := strings.TrimSpace(string(rawEnvs))
	lineSplit := strings.Split(envs, "\n")
	var list []map[string]string
	for _, env := range lineSplit {
		valSplit := strings.Split(env, "=")
		m := make(map[string]string)
		m["name"] = valSplit[0]
		m["value"] = valSplit[1]
		list = append(list, m)
	}
	return list
}
