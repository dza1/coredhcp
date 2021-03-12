package device

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
)

type Config struct {
	LoggingConfig LoggingConfig `mapstructure:"logging" json:"logging"`
	NATSConfig    NATSConfig    `mapstructure:"nats" json:"nats"`
	SQLiteConfig  SQLiteConfig  `mapstructure:"sqlite" json:"sqlite"`
}

type LoggingConfig struct {
	LogLevel   string
	Filename   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

type NATSConfig struct {
	Host string
}

type SQLiteConfig struct {
	Filename string
}

type configManager struct {
	kv            *api.KV
	key           string
	filename      string
	configPath    string
	consulAddress string
	deviceID      string
}

func NewConfigManager(kv *api.KV, configPath string, consulAddress string, deviceID string) *configManager {
	return &configManager{
		kv:            kv,
		key:           strings.Join([]string{deviceID, configKey}, "/"),
		filename:      strings.Join([]string{configPath, configFile}, "/"),
		configPath:    configPath,
		consulAddress: consulAddress,
		deviceID:      deviceID,
	}
}

func (c *configManager) ReadLocalConfig() error {
	viper.SetConfigType(configType)
	viper.SetConfigName(configName)
	viper.AddConfigPath(c.configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read local config file: %w", err)
	}
	return nil
}

func (c *configManager) ReadRemoteConfig() error {
	viper.SetConfigType(configType)
	err := viper.AddRemoteProvider(configProvider, c.consulAddress, c.key)
	if err != nil {
		return fmt.Errorf("add remote provider %s: %w", c.consulAddress, err)
	}
	if err := viper.ReadRemoteConfig(); err != nil {
		return fmt.Errorf("read remote config: %w", err)
	}
	return nil
}

func (c *configManager) WriteRemoteConfig() error {
	file, err := ioutil.ReadFile(c.filename)
	if err != nil {
		return fmt.Errorf("read file %s: %w", c.filename, err)
	}
	p := &api.KVPair{Key: c.key, Value: file}
	_, err = c.kv.Put(p, nil)
	if err != nil {
		return fmt.Errorf("put key %s with value of file %s: %w", c.key, c.filename, err)
	}
	return nil
}

func (c *configManager) WatchRemoteConfig() {
	for {
		time.Sleep(configSyncInterval)
		if err := viper.WatchRemoteConfig(); err != nil {
			log.Errorf("watch remote config: %v", err)
			continue
		}
		if err := c.WriteLocalConfig(); err != nil {
			log.Errorf("write local config: %v", err)
		}
	}
}

func (c *configManager) WriteLocalConfig() error {
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	configJSON, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := ioutil.WriteFile(c.filename, configJSON, 0644); err != nil {
		return fmt.Errorf("write config as config.json: %w", err)
	}
	return nil
}
