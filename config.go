package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Conf struct {
	BindLabels   []string
	ResourcePath string
	UserID       string
	Token        string
	URL          string
	Domain       string
}

func fieldValidation(name reflect.Value) bool {
	if !name.IsValid() {
		fmt.Println("Error in config. Generate a new one or ctrl+c to exit")
		return false
	}
	return true
}

func LoadConf(path string) *Conf {
	confFile, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error loading config...")
		return MakeConf(path)
	}

	conf := &Conf{}
	r := reflect.ValueOf(conf).Elem()
	confString := strings.TrimSpace(string(confFile))
	// fmt.Println(confString)

	for v := range strings.SplitSeq(confString, "\n") {
		split := strings.Split(v, "=")
		// fmt.Println(split, v)
		if len(split) != 2 {
			panic(fmt.Sprintf("Invalid config, please validate at %s", path))
		}
		if split[1] == "" {
			fmt.Printf("Missing value for %s in config, please make an adjustment\n", split[0])
			return MakeConf(path)
		}
		vList := strings.Split(split[1], ",")
		if len(vList) > 1 {
			fieldName := r.FieldByName(split[0])
			if fieldValidation(fieldName) {
				r.FieldByName(split[0]).Set(reflect.ValueOf(vList))
			} else {
				return MakeConf(path)
			}
		} else {
			fieldName := r.FieldByName(split[0])
			if fieldValidation(fieldName) {
				r.FieldByName(split[0]).SetString(split[1])
			} else {
				return MakeConf(path)
			}
		}
		// fmt.Println(conf)
	}
	return conf
}

func MakeConf(path string) *Conf {
	conf := &Conf{}
	r := reflect.ValueOf(conf).Elem()
	t := r.Type()
	fieldsNum := r.NumField()
	fmt.Println("Creating new config... Separate lists using ','. Spaces will be ignored")
	fmt.Println("Just enter to skip if config already exists. Really not recommended for first config generation as it can break things")
	for i := range fieldsNum {
		fieldName := t.Field(i).Name
		fmt.Printf("Please provide me %s:", fieldName)
		val, err := Reader.ReadString('\n')
		Check(err)
		cleanVal := strings.TrimSpace(val)
		if cleanVal == "" {
			continue
		}
		if strings.Contains(cleanVal, ",") {
			split := strings.Split(cleanVal, ",")
			r.FieldByName(fieldName).Set(reflect.ValueOf(split))
		} else {
			// fmt.Println(cleanVal)
			r.FieldByName(fieldName).SetString(cleanVal)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		err = os.MkdirAll(filepath.Dir(path), 0755)
		Check(err)
		f, err = os.Create(path)
		Check(err)
	}
	defer f.Close()

	for i := range fieldsNum {
		fieldName := t.Field(i).Name
		val := r.FieldByName(fieldName)
		var valToWrite string
		if val.Type() == reflect.TypeOf([]string{}) {
			s := val.Interface().([]string)
			valToWrite = strings.Join(s, ",")
		} else {
			valToWrite = val.String()
		}
		str := fmt.Sprintf("%s=%s\n", fieldName, valToWrite)
		_, err := f.WriteString(str)
		Check(err)
	}
	return conf
}
