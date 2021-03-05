package device

import (
	"time"

	"github.com/jinzhu/gorm"
)

type Device struct {
	gorm.Model
	HardwareAddr    string `gorm:"unique;not null"` // set member number to unique and not null
	IP              string `gorm:"unique;not null"` // set member number to unique and not null
	Host            string
	LeaseExpiration time.Time
	Feeder          string
}
