package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/knadh/koanf"
	jsonkoanf "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

type CombinedStorage struct {
	// store env. variable
	kEnv *koanf.Koanf
	// store flag values
	flagSet *pflag.FlagSet

	// protect value stores
	storeLock *sync.Mutex

	configFile string
}

func NewCombinedStorage(configFile, envPrefix string) (*CombinedStorage, error) {
	kBase := koanf.New(".")
	if err := kBase.Load(env.Provider(envPrefix, ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, envPrefix)), "_", "")
	}), nil); err != nil {
		return nil, err
	}
	storage := &CombinedStorage{
		kEnv:       kBase,
		storeLock:  &sync.Mutex{},
		configFile: configFile,
	}
	return storage, nil
}

func (c *CombinedStorage) Get(key string) interface{} {
	c.storeLock.Lock()
	defer c.storeLock.Unlock()
	return c.merged().Get(key)
}

func (c *CombinedStorage) Set(key string, value interface{}) error {
	c.storeLock.Lock()
	defer c.storeLock.Unlock()
	if err := ensureConfigFileExists(c.configFile); err != nil {
		return err
	}
	in, err := ioutil.ReadFile(c.configFile)
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(in, &cfg); err != nil {
		return err
	}
	cfg[key] = value
	bin, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(c.configFile, bin, 0600)
}

func (c *CombinedStorage) Unset(key string) error {
	c.storeLock.Lock()
	defer c.storeLock.Unlock()
	if err := ensureConfigFileExists(c.configFile); err != nil {
		return err
	}
	in, err := ioutil.ReadFile(c.configFile)
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(in, &cfg); err != nil {
		return err
	}
	delete(cfg, key)
	bin, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(c.configFile, bin, 0600)
}

// BindFlagset binds a flagset to their respective config properties
func (c *CombinedStorage) LoadFlagSet(flagSet *pflag.FlagSet) {
	c.storeLock.Lock()
	defer c.storeLock.Unlock()
	c.flagSet = flagSet
}

// Print prints a key -> value string representation
// of the config map with keys sorted alphabetically.
func (c *CombinedStorage) Print() {
	c.storeLock.Lock()
	defer c.storeLock.Unlock()
	c.merged().Print()
}

// ensureConfigFileExists creates the config file if it does not exists
func ensureConfigFileExists(file string) error {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		return ioutil.WriteFile(file, []byte("{}\n"), 0600)
	}
	return err
}

func (c *CombinedStorage) merged() *koanf.Koanf {
	if err := ensureConfigFileExists(c.configFile); err != nil {
		logrus.Errorf("cannot create config file %s: %v", c.configFile, err)
		return nil
	}
	combined := koanf.New(".")
	if err := combined.Load(file.Provider(c.configFile), jsonkoanf.Parser()); err != nil {
		logrus.Errorf("cannot create config file %s: %v", c.configFile, err)
		return nil
	}
	combined.Merge(c.kEnv)
	if c.flagSet != nil {
		_ = combined.Load(posflag.Provider(c.flagSet, ".", combined), nil)
	}
	return combined
}
