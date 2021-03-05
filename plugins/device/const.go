package device

import "time"

const (
	CreateTopic       = "device.create"
	UpdateTopic       = "device.update"
	DeleteTopic       = "device.delete"
	FindAllTopic      = "device.findAll"
	FindByIDTopic     = "device.findByID"
	FindByFeederTopic = "device.findByFeeder"
)

const (
	configType         = "json"
	configProvider     = "consul"
	configName         = "config"
	configFile         = "config.json"
	configKey          = "device_config"
	configSyncInterval = 5 * time.Second
)
