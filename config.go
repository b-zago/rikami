package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/term"
)

type Config struct {
	URL         string `json:"URL"`
	HMAC        string `json:"HMAC" sensitive:"true"`
	BaseGHToken string `json:"base_gh_token"`
	BaseOwner   string `json:"base_owner"`
	BaseRepo    string `json:"base_repo"`
	Domain      string `json:"domain"`
}

func LoadConfig(path string) *Config {
	conf, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("config file not found under path: %s\nrun `rika config`", path)
		} else {
			log.Fatalf("could not load config: %v", err)
		}
	}
	var config Config
	err = json.Unmarshal(conf, &config)
	if err != nil {
		log.Fatalf("could not read config: %v", err)
	}
	return &config
}

func MakeConfig(path string) {
	var config Config
	ref := reflect.ValueOf(&config).Elem()

	reader := bufio.NewReader(os.Stdin)
	for k := range ref.Fields() {
		fmt.Printf("%s: ", k.Name)
		sensitive := k.Tag.Get("sensitive") == "true"
		if sensitive {
			// when sig killed while in this mode it breaks the user's teminal, maybe capture sigkill somehow
			v, err := term.ReadPassword(int(os.Stderr.Fd()))
			if err != nil {
				log.Fatalf("error while reading value: %v", err)
			}
			ref.FieldByName(k.Name).SetString(string(v))
		}
		v, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("error while reading value: %v", err)
		}
		v = strings.TrimSpace(v)
		ref.FieldByName(k.Name).SetString(v)
	}

	config.WriteConfig(path)
}

func (c *Config) WriteConfig(path string) {
	b, err := json.Marshal(c)
	if err != nil {
		log.Fatalf("could not parse provided config: %v", err)
	}
	err = os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		log.Fatalf("error creating config: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("error creating config: %v", err)
	}
	defer f.Close()
	f.Write(b)
}

func (c *Config) EditConfig(path, field, value string) {
	ref := reflect.ValueOf(c).Elem()
	ref.FieldByName(field).SetString(value)
	c.WriteConfig(path)
}

func (c *Config) Validate() {
	ref := reflect.ValueOf(c).Elem()
	for k := range ref.Fields() {
		if ref.FieldByName(k.Name).String() == "" {
			log.Fatalf("could not find %q field in config or it's empty", k.Name)
		}
	}
}
