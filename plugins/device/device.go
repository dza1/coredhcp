package device

import (
	"time"

	"github.com/coredhcp/coredhcp/logger"
)

var log = logger.GetLogger("plugins/range")

type Device struct {
	ID uint `gorm:"primarykey"`
	//gorm.Model
	HardwareAddr    string `gorm:"unique"`          // set member number to unique and not null
	IP              string `gorm:"unique;not null"` // set member number to unique and not null
	Host            string
	LeaseExpiration time.Time `gorm:"index"`
	//Feeder          string
}
