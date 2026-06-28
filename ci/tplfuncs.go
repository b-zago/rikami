package ci

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/bitnami-labs/sealed-secrets/pkg/apis/sealedsecrets/v1alpha1"
	"github.com/bitnami-labs/sealed-secrets/pkg/kubeseal"
	"github.com/goccy/go-yaml"
)

var Cert *rsa.PublicKey

func GetTplFuncs() template.FuncMap {
	return template.FuncMap{
		"default": tplDefault,
		"toYAML":  toYAML,
		"indent":  indent,
		"nindent": nindent,
		"hindent": hindent,
		"quote":   quote,
	}
}

func tplDefault(def, current any) any {
	if current == nil {
		return def
	}
	return current
}

func toYAML(v any) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	err := enc.Encode(v)
	if err != nil {
		log.Fatalf("could not marshal to yaml with function toYAML. Error\n%v", err)
	}
	enc.Close()

	return strings.TrimSuffix(buf.String(), "\n")
}

func indent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.ReplaceAll(v, "\n", "\n"+pad)
}

func hindent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return strings.ReplaceAll(v, "\n", "\n"+pad)
}

func nindent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return "\n" + pad + strings.ReplaceAll(v, "\n", "\n"+pad)
}

func quote(v any) string {
	return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
}

func GetVesFuncs() template.FuncMap {
	return template.FuncMap{
		"secRand":  secRand,
		"secFile":  secFile,
		"secValue": secValue,
		"fromFile": fromFile,
		"nindent":  nindent,
		"toYAML":   toYAML,
		"indent":   indent,
		"hindent":  hindent,
		"quote":    quote,
	}
}

func fromFile(filename string) string {
	f, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("could not read file %q", filename)
	}
	return string(f)
}

// secrets stuff
func secRand() string {
	env := CurrValues.CurrentEnv

	// the checks here should be fine but it doesn't hurt to be sure
	params, ok := SSMParameters.Data[env]
	if !ok {
		PullEnvParams(false, env, "secrets")
		params, ok = SSMParameters.Data[env]
	}
	// PullEnvParams skips storing when the SSM param doesn't exist yet so we need to create a new persistent struct here
	if !ok || params == nil {
		params = &SSMParams{}
		SSMParameters.Data[env] = params
	}
	// this check neccesary i think
	if params.Secrets == nil {
		params.Secrets = make(EnvFileMap)
	}

	names := CurrValues.SecretNames[env]
	if CurrValues.SecretFuncCounter >= len(names) {
		log.Fatalf("secRand call #%d in env %q has no matching secret registration; "+
			"secRand may only be used inside a secret shard's data block", CurrValues.SecretFuncCounter, env)
	}
	secName := names[CurrValues.SecretFuncCounter].Name
	secKey := names[CurrValues.SecretFuncCounter].Key

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("error creating random secret value. error\n%v", err)
	}
	s := hex.EncodeToString(b)
	sealed := string(sealRaw(s, secName, env, "strict"))

	secNameConv := strings.ReplaceAll(secName, "-", ".")
	fileName := fmt.Sprintf(".env.%s.secret", secNameConv)

	secMap, ok := params.Secrets[fileName]
	if !ok {
		secMap = make(map[string]string)
		params.Secrets[fileName] = secMap
	}
	secMap[secKey] = s

	CurrValues.SecretFuncCounter++
	return sealed
}

func secFile(filename string) string {
	sealedMap := getSealedMap(filename)
	return toYAML(sealedMap)
}

func secValue(filename, key string) string {
	sealedMap := getSealedMap(filename)

	sealedVal, ok := sealedMap[key]
	if !ok {
		log.Fatalf("could not find %q in %q on secValue function", key, filename)
	}

	return sealedVal
}

func sealRaw(val, secName, env, sealingScope string) []byte {
	if Cert == nil {
		cert, err := FetchCert(CertURL)
		if err != nil {
			log.Fatal(err)
		}
		Cert, err = kubeseal.ParseKey(strings.NewReader(cert))
		if err != nil {
			log.Fatalf("could not parse sealed-secrets public key. error\n%v", err)
		}
	}
	secName = fmt.Sprintf("%s-%s", AppName, secName)

	var buf bytes.Buffer
	var scope v1alpha1.SealingScope
	scope.Set(sealingScope)

	kubeseal.EncryptSecretItem(&buf, secName, env, []byte(val), scope, Cert)

	return buf.Bytes()
}

// secNameFromFile reverses the `.env.<name>.secret` convention secRand uses when
// it stores generated values (see secRand), recovering the secret's name from its
// filename. This lets secValue/secFile derive their seal name directly instead of
// relying on the positional SecretFuncCounter, which only tracks secRand.
func secNameFromFile(filename string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(filename, ".env."), ".secret")
	return strings.ReplaceAll(name, ".", "-")
}

func getSealedMap(filename string) map[string]string {
	env := CurrValues.CurrentEnv

	params, ok := SSMParameters.Data[env]
	if !ok || params == nil {
		// secValue isn't detected by checkPullEnvs, so self-heal like secRand does.
		PullEnvParams(false, env, "secrets")
		params = SSMParameters.Data[env]
	}
	if params == nil || params.Secrets == nil {
		log.Fatalf("could not find %q data. secrets are empty in env %q", filename, env)
	}
	data, ok := params.Secrets[filename]
	if !ok {
		log.Fatalf("could not find %q data. no such secret in env %q", filename, env)
	}

	secName := secNameFromFile(filename)
	sealedMap := make(map[string]string, len(data))

	// potential goroutine here to seal all secrets in parallel
	for k, v := range data {
		sealed := sealRaw(v, secName, env, "strict")
		sealedMap[k] = string(sealed)
	}
	return sealedMap
}
