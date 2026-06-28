package ci

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/goccy/go-yaml"
)

type Vessel struct {
	ConfsYaml  map[string][]byte
	ShardsYaml map[string]map[string][]byte // for each env, a map of shard name to its processed yaml
	VesVals    map[string]map[string]*VesselValues
	Additions  map[string]map[string]ShardsMap
	Final      []*FinalValues
}

type VesselValues struct {
	Fragments ShardsMap
}

type VesselTpl struct {
	Shards map[string]ShardsMap
	App    string
	Domain string
}

type FinalValues struct {
	Env    string                 `yaml:"env"`
	Values map[string][]ShardData `yaml:",inline"`
}

// ValuesRefs ...
// to be used for dynamic functions that need a context where the function was called in vessel template
// first key is env
// index determines call order
type ValuesRefs struct {
	SecretNames       map[string][]SecretInfo
	SecretFuncCounter int
	CurrentEnv        string
}

type SecretInfo struct {
	Name string
	Key  string
}

var CurrValues = ValuesRefs{SecretNames: make(map[string][]SecretInfo)}

// GetSecretNames records, per env, the ordered (Name, Key) pairs that secRand
// will fill in. secRand takes no arguments, so it recovers its context from the
// SecretFuncCounter index into this list; the order here must therefore match
// the order secRand is called during template execution. The YAML encoder emits
// map keys sorted (and slices in order), so we register data keys sorted too.
// Only secRand uses this list — secFile/secValue derive their name from their
// filename (see secNameFromFile), so they are not registered here.
func (v *ValuesRefs) GetSecretNames(secData []ShardData, env string) {
	for _, shardData := range secData {
		shardNameAny, ok := shardData["name"]
		if !ok {
			log.Fatal("missing name in secret shard")
		}
		shardName, ok := shardNameAny.(string)
		if !ok {
			log.Fatal("invalid secret shard name type. must be string")
		}
		data, ok := shardData["data"]
		if !ok {
			log.Fatal("missing secret data")
		}
		d, ok := data.(ShardData)
		if !ok {
			// non-map data (e.g. secFile) doesn't use the secRand counter
			continue
		}
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, secKey := range keys {
			sv, ok := d[secKey].(string)
			if !ok {
				continue
			}
			if strings.Contains(sv, "(( secRand ))") {
				v.SecretNames[env] = append(v.SecretNames[env], SecretInfo{Name: shardName, Key: secKey})
			}
		}
	}
}

func (v *Vessel) Merge() error {
	v.VesVals = make(map[string]map[string]*VesselValues, len(v.ShardsYaml))
	v.Final = make([]*FinalValues, len(v.ShardsYaml))

	envIdx := 0
	for env, ymls := range v.ShardsYaml {
		v.VesVals[env] = make(map[string]*VesselValues, len(ymls))
		v.Final[envIdx] = &FinalValues{Env: env, Values: make(map[string][]ShardData)}
		for shardName, yml := range ymls {
			var shard ShardsMap
			err := yaml.Unmarshal(yml, &shard)
			if err != nil {
				return fmt.Errorf("could not unmarshal shard yaml while merging vessel. %v", err)
			}

			v.VesVals[env][shardName] = &VesselValues{Fragments: shard}

			addition, ok := v.Additions[env][shardName]
			if ok {
				vv := v.VesVals[env][shardName]
				for k, val := range addition {
					maps.Copy(vv.Fragments[k], val)
				}
			}
			// after all the processing we can start collecting for final result
			for k, shardData := range v.VesVals[env][shardName].Fragments {
				cleanName, _, _ := strings.Cut(k, "_")
				v.Final[envIdx].Values[cleanName] = append(v.Final[envIdx].Values[cleanName], shardData)

			}
		}
		envIdx++
	}
	err := v.processFinals()
	if err != nil {
		log.Fatal(err)
	}
	PutEnvParams()
	return nil
}

func (v *Vessel) processFinals() error {
	tplDirPath := filepath.Join(AppName, "templates")
	err := os.MkdirAll(tplDirPath, 0755)
	if err != nil {
		log.Fatalf("could not create app directory %q. err:\n%v", AppName, err)
	}
	finalTpls := make([][]byte, len(v.Final))
	doneChan := make(chan bool, len(v.Final))
	for i, final := range v.Final {
		go func(i int) {
			finalTpls[i] = final.ConvertFinalEnv()
			doneChan <- true
		}(i)
	}
	for range len(v.Final) {
		<-doneChan
	}
	seenFrags := make(map[string]bool)
	for i, final := range v.Final {
		for k := range final.Values {
			seenFrags[k] = true
		}
		// for reference in sec functions
		secretData, ok := final.Values["secret"]
		if ok {
			CurrValues.GetSecretNames(secretData, final.Env)
		}
		CurrValues.CurrentEnv = final.Env

		valFile := fmt.Sprintf("values-%s.yaml", final.Env)
		path := filepath.Join(AppName, valFile)
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("could not create the output file %s. error:\n%v", path, err)
		}
		defer f.Close()

		tpl := template.Must(template.New(final.Env).Funcs(GetVesFuncs()).Parse(string(finalTpls[i])))
		// just so i have nice templating in rikami.yaml
		vslTpl := VesselTpl{Shards: make(map[string]ShardsMap), App: AppName, Domain: Domain}
		for k, v := range v.VesVals[final.Env] {
			vslTpl.Shards[k] = v.Fragments
		}

		var buf bytes.Buffer
		err = tpl.Execute(&buf, vslTpl)
		if err != nil {
			log.Fatalf("error executing template for env %q: %v", final.Env, err)
		}
		f.Write(buf.Bytes())
		CurrValues.SecretFuncCounter = 0
	}
	// confs
	for k, yml := range v.ConfsYaml {
		confFile := fmt.Sprintf("%s.yaml", k)
		path := filepath.Join(AppName, confFile)
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("could not create the config file %q. error:\n%v", path, err)
		}
		defer f.Close()
		f.Write(yml)
	}
	// Main.yaml generation
	mainPath := filepath.Join(tplDirPath, "main.yaml")
	f, err := os.Create(mainPath)
	if err != nil {
		log.Fatalf("could not create the main.yaml file. error:\n%v", err)
	}
	defer f.Close()
	for k := range seenFrags {
		inc := fmt.Sprintf("{{- include \"lib.%s\" . -}}\n", k)
		f.WriteString(inc)
	}
	return nil
}

func (v *FinalValues) ConvertFinalEnv() []byte {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	err := enc.Encode(v)
	if err != nil {
		log.Fatalf("could not marshal final output to yaml for env %s. Error:\n%v", v.Env, err)
	}
	enc.Close()

	b := buf.Bytes()
	checkPullEnvs(b, v.Env)
	asTpl := convertToTpl(b)
	return asTpl
}

func checkPullEnvs(yml []byte, env string) {
	var params []string
	if bytes.Contains(yml, []byte("(( secRand ))")) || bytes.Contains(yml, []byte("(( secFile ")) {
		params = append(params, "secrets")
	}
	if bytes.Contains(yml, []byte("(( envFile ")) {
		params = append(params, "envs")
	}
	if len(params) > 0 {
		PullEnvParams(false, env, params...)
	}
}

func convertToTpl(yml []byte) []byte {
	tpl := bytes.ReplaceAll(yml, []byte(`"((`), []byte(`{{`))
	tpl = bytes.ReplaceAll(tpl, []byte(`))"`), []byte(`}}`))
	tpl = bytes.ReplaceAll(tpl, []byte(`((`), []byte(`{{`))
	tpl = bytes.ReplaceAll(tpl, []byte(`))`), []byte(`}}`))
	return tpl
}

func (v *Vessel) Validate() bool {
	isValid := true
	for env, shards := range v.ShardsYaml {
		for _, shardB := range shards {
			scanner := bufio.NewScanner(bytes.NewReader(shardB))
			for scanner.Scan() {
				b := scanner.Bytes()
				if bytes.Contains(b, []byte("<no value>")) {
					fmt.Printf("missing value in %s env! %s\n", env, string(b))
					isValid = false
				}
			}
			if err := scanner.Err(); err != nil {
				fmt.Printf("scan error in %s env: %v\n", env, err)
				isValid = false
			}
		}
	}
	return isValid
}

type FragmentName struct {
	Shard    string
	Fragment string
}

// FragmentData holds a single fragment's values. It mirrors ShardData but is a
// distinct type so one fragment's values aren't conflated with a whole shard's.
type FragmentData ShardData

type Fragment struct {
	// Fragments holds one fragment's values; named to match the .Fragments field
	// referenced in fragment templates.
	Fragments FragmentData
	Name      FragmentName
}

type Shard struct {
	App    string
	Domain string
	Name   string
	Values ShardData
}

func NewShard(app, domain, name string, vals ShardData) *Shard {
	return &Shard{
		App:    app,
		Domain: domain,
		Name:   name,
		Values: vals,
	}
}

func (s *Shard) Validate() bool {
	isValid := true
	if s.App == "" {
		fmt.Println("missing App name in a shard!")
		isValid = false
	}
	if s.Domain == "" {
		fmt.Println("missing Domain in a shard!")
		isValid = false
	}
	if s.Name == "" {
		fmt.Println("missing Name in a shard!")
		isValid = false
	}
	return isValid
}

// helpers

type FetchCertRes struct {
	Message string `json:"Message"`
}

func FetchCert(url string) (string, error) {
	var buf bytes.Buffer
	req, err := http.NewRequest("GET", url, &buf)
	if err != nil {
		return "", fmt.Errorf("fetch cert request failed with error:\n%v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch cert request failed with error:\n%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fetch cert request failed with error:\n%v", err)
	}

	var res FetchCertRes
	err = json.Unmarshal(body, &res)
	if err != nil {
		return "", fmt.Errorf("fetch cert request failed with error:\n%v", err)
	}
	return res.Message, nil
}
