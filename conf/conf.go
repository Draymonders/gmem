package conf

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type Config struct {
	Port int `json:"port"`
}

func LoadConf(path string) (*Config, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	jsonBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	cf := &Config{}
	if err = json.Unmarshal(jsonBytes, &cf); err != nil {
		return nil, err
	}
	return cf, nil
}
