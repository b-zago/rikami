package ci

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

type FetchReq struct {
	Client *http.Client
	Owner  string
	Repo   string
	Paths  map[string]string
	Token  string
}

type (
	ShardData map[string]any
	ConfsData map[string]map[string]string
	ShardsMap map[string]ShardData
)

// this is so that yaml will see map[string]any as ShardData according to given struct
func (s *ShardData) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*s = normalize(raw).(ShardData)
	return nil
}

func normalize(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(ShardData, len(val))
		for k, sub := range val {
			out[k] = normalize(sub)
		}
		return out
	case []any:
		for i, sub := range val {
			val[i] = normalize(sub)
		}
		return val
	default:
		return val
	}
}

type EnvData struct {
	Env    string
	Shards ShardsMap
}

type AppData struct {
	Envs  []EnvData
	Confs ConfsData
}

type ConfValues struct {
	Values map[string]string
}

type Blob struct {
	Text string `json:"text"`
}

type GithubData struct {
	Data struct {
		Repository map[string]Blob `json:"repository"`
	} `json:"data"`
}

type RawData struct {
	Confs ConfsData              `yaml:"confs"`
	Envs  []map[string]ShardsMap `yaml:"envs"`
}

var (
	AppName string
	Domain  string
)

func (data *RawData) processData(r *FetchReq) *AppData {
	app := AppData{Envs: make([]EnvData, len(data.Envs)), Confs: make(ConfsData, len(data.Confs))}
	for i, currentEnv := range data.Envs {
		for envName, envData := range currentEnv {
			env := EnvData{Env: envName, Shards: make(ShardsMap)}
			for k, v := range envData {
				name, definedName, tag, err := processKey(k)
				if err != nil {
					log.Fatal(err)
				}
				// support for multiple shards usage but with custom names after `_`
				r.Paths[definedName] = fmt.Sprintf("%s:shards/%s.yaml", tag, name)
				newShardData := make(ShardData)
				for key, shardVal := range v {
					run, _ := utf8.DecodeRuneInString(key)
					if unicode.IsUpper(run) {
						name, definedNameFrag, tag, err := processKey(key)
						if err != nil {
							log.Fatal(err)
						}
						r.Paths[definedName+"_"+definedNameFrag] = fmt.Sprintf("%s:fragments/%s.yaml", tag, name)
						newShardData[definedNameFrag] = shardVal
					} else {
						newShardData[key] = shardVal
					}
				}
				env.Shards[definedName] = newShardData
			}
			app.Envs[i] = env
		}
	}
	for confName, confData := range data.Confs {

		name, definedName, tag, err := processKey(confName)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("def name here", definedName, name)
		r.Paths[definedName] = fmt.Sprintf("%s:confs/%s.yaml", tag, name)
		app.Confs[definedName] = confData
	}

	app.MergeEnvs()
	return &app
}

func processKey(key string) (actualName string, definedName string, tag string, err error) {
	kSplit := strings.Split(key, "@")
	if len(kSplit) == 1 || len(kSplit) > 2 {
		return "", "", "", fmt.Errorf("could not read shard %s from rikami.yaml correctly", key)
	}
	nameSplit := strings.SplitN(kSplit[0], "_", 2)
	name := kSplit[0]
	if len(nameSplit) > 1 {
		name = nameSplit[0]
	}
	return name, kSplit[0], kSplit[1], nil
}

var (
	Client  *http.Client
	CertURL string
)

func Run(token, owner, repo, domain, rikamiURL, appName string) {
	if appName == "" {
		AppName = GetRepoName()
	} else {
		AppName = appName
	}
	Domain = domain
	CertURL = rikamiURL + "/cert"

	client := &http.Client{Timeout: 15 * time.Second}
	Client = client
	req := FetchReq{
		Token:  token,
		Owner:  owner,
		Repo:   repo,
		Client: client,
		Paths:  make(map[string]string),
	}

	// fmt.Println(string(data))
	appData, err := processYaml(&req)
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println(*appData)
	// fmt.Println("---------")
	ghData, err := req.fetchFiles()
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println(*ghData)
	// fmt.Println("---------")
	vessel := appData.assembleVes(ghData)
	// for k, v := range vessel.ShardsYaml {
	// 	fmt.Println(k)
	// 	for _, b := range v {
	// 		fmt.Println(string(b))
	// 	}
	// 	fmt.Println("---------")
	// }
	if !vessel.Validate() {
		log.Fatal("vessel validation failed")
	}
	fmt.Println("############")
	err = vessel.Merge()
	if err != nil {
		log.Fatal(err)
	}
}

func processYaml(fReq *FetchReq) (*AppData, error) {
	confYaml := "rikami.yaml"
	f, err := os.ReadFile(confYaml)
	if err != nil {
		return nil, fmt.Errorf("could not read rikami config yaml at %s. Error\n%v", confYaml, err)
	}

	var doc RawData
	err = yaml.Unmarshal(f, &doc)
	if err != nil {
		return nil, fmt.Errorf("could not decode yaml at %s. Error\n%v", confYaml, err)
	}

	app := doc.processData(fReq)

	// fmt.Println(doc)
	return app, nil
}

func (r *FetchReq) fetchFiles() (*GithubData, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `query { repository(owner:"%s", name:"%s") {`, r.Owner, r.Repo)
	for k, v := range r.Paths {
		fmt.Fprintf(&buf, `%s: object(expression: "%s"){... on Blob{text}}`, k, v)
	}
	buf.WriteString("}}")

	reqBody, err := json.Marshal(map[string]string{
		"query": buf.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("error making query.\n%v", err)
	}

	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error making query.\n%v", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.Token)

	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making a request.\n%v", err)
	}
	defer resp.Body.Close()

	respRead, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response.\n%v", err)
	}

	var data GithubData
	err = json.Unmarshal(respRead, &data)
	if err != nil {
		return nil, fmt.Errorf("error reading response as JSON.\n%v", err)
	}

	return &data, nil
}

func (app *AppData) assembleVes(ghData *GithubData) *Vessel {
	vessel := Vessel{ShardsYaml: make(map[string]map[string][]byte, len(app.Envs)), Additions: make(map[string]map[string]ShardsMap), ConfsYaml: make(map[string][]byte)}
	for _, envData := range app.Envs {
		vessel.ShardsYaml[envData.Env] = make(map[string][]byte, len(envData.Shards))
		for k, shardData := range envData.Shards {
			name := getShardResourceName(k)
			shardStr := ghData.Data.Repository[k].Text
			var builder strings.Builder
			builder.WriteString(shardStr)

			for key, val := range shardData {
				fBlob, ok := ghData.Data.Repository[k+"_"+key]
				fStr := fBlob.Text
				if ok {
					_, fragDefName, found := strings.Cut(key, "_")
					var fragName string
					if found {
						fragName = fmt.Sprintf("%s-%s", name, fragDefName)
						_, rest, found := strings.Cut(fStr, ":")
						if found {
							fStr = fmt.Sprintf("%s:%s", strings.ToLower(key), rest)
						} else {
							log.Fatalf("error processing fragment %s. is top-level fragment name defined correctly?", key)
						}
					} else {
						fragName = name
					}

					fmt.Printf("%T %s\n", val, envData.Env)

					fragMap, ok := val.(ShardData)
					if !ok {
						log.Fatalf("could not process fragment %s with provided values", key)
					}

					fragment := Fragment{Fragments: FragmentData(fragMap), Name: FragmentName{Shard: name, Fragment: fragName}}
					tpl := template.Must(template.New(key).Funcs(GetTplFuncs()).Parse(fStr))
					var buf bytes.Buffer
					tpl.Execute(&buf, fragment)
					fmt.Fprintf(&builder, "%s", buf.String())

					keys := getFragKeys(fStr)
					for fragK, fragV := range fragMap {
						if !slices.Contains(keys, fragK) {
							var buf bytes.Buffer
							enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
							err := enc.Encode(ShardData{fragK: fragV})
							if err != nil {
								log.Fatalf("could not marshal to yaml %s in fragment. Error\n%v", fragK, err)
							}
							enc.Close()

							sc := bufio.NewScanner(&buf)
							for sc.Scan() {
								fmt.Fprintf(&builder, "  %s\n", sc.Text())
							}
						}
					}
				} else {
					frags := getShardFrags(shardStr)
					actual, _, _ := strings.Cut(key, "_")

					if slices.Contains(frags, actual) {
						fragMap, ok := val.(ShardData)
						if !ok {
							log.Fatalf("could not process fragment %s with provided values", key)
						}
						_, ok = vessel.Additions[envData.Env][k]
						if !ok {
							vessel.Additions[envData.Env] = make(map[string]ShardsMap)
							vessel.Additions[envData.Env][k] = make(ShardsMap)
						}
						vessel.Additions[envData.Env][k][actual] = fragMap
					}
				}
			}
			shardTplString := builder.String()
			shard := NewShard(AppName, Domain, name, shardData)

			if !shard.Validate() {
				log.Fatalf("validation failed for shard %s in env %s", k, envData.Env)
			}
			tpl := template.Must(template.New(k).Funcs(GetTplFuncs()).Parse(shardTplString))

			var buf bytes.Buffer
			tpl.Execute(&buf, shard)
			vessel.ShardsYaml[envData.Env][k] = buf.Bytes()
		}
	}
	// for confs
	for k, confData := range app.Confs {
		fmt.Println("mmmm", k)
		confStr := ghData.Data.Repository[k].Text
		confValues := &ConfValues{Values: confData}
		tpl := template.Must(template.New(k).Funcs(GetTplFuncs()).Parse(confStr))
		var buf bytes.Buffer
		tpl.Execute(&buf, confValues)
		vessel.ConfsYaml[k] = buf.Bytes()
	}
	return &vessel
}

func (app *AppData) MergeEnvs() {
	for i := range app.Envs[1:] {
		for shardName, shardData := range app.Envs[i].Shards {
			dataCopy := copyShardData(shardData)
			for key := range shardData {
				_, ok := app.Envs[i+1].Shards[shardName][key]
				if !ok {
					_, ok := app.Envs[i+1].Shards[shardName]
					if !ok {
						app.Envs[i+1].Shards[shardName] = make(ShardData)
					}
					app.Envs[i+1].Shards[shardName][key] = dataCopy[key]
				}
			}
		}
	}
}

func copyShardData(shardData ShardData) ShardData {
	newData := make(ShardData)
	for key, val := range shardData {
		switch v := val.(type) {
		case map[string]any:
			newData[key] = copyShardData(v)
		default:
			newData[key] = v
		}
	}
	return newData
}

func getShardResourceName(name string) string {
	var builder strings.Builder
	var prevRune rune
	builder.WriteRune(unicode.ToLower(rune(name[0])))

	for _, r := range name[1:] {
		if r >= 'A' && r <= 'Z' && prevRune != '-' {
			builder.WriteRune('-')
			builder.WriteRune(unicode.ToLower(r))
		} else if r == '_' {
			builder.WriteRune('-')
		} else {
			builder.WriteRune(r)
		}
		prevRune = r
	}
	return builder.String()
}

func getShardFrags(shardStr string) []string {
	var frags []string
	for line := range strings.SplitSeq(shardStr, "\n") {
		if !strings.HasPrefix(line, " ") {
			frags = append(frags, strings.TrimSuffix(line, ":"))
		}
	}
	return frags
}

func getFragKeys(shardStr string) []string {
	var keys []string

	scanner := bufio.NewScanner(strings.NewReader(shardStr))

	sep := []byte(".Fragments.")

	for scanner.Scan() {
		b := scanner.Bytes()
		if bytes.Contains(b, sep) {
			split := bytes.Split(b, sep)
			for _, ln := range split[1:] {
				bf, _, _ := bytes.Cut(ln, []byte(" "))
				keys = append(keys, string(bf))
			}
		}
	}

	return keys
}
