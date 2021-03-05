package device

import (
	"net"

	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"
)

type sqlite3Service struct {
	db *gorm.DB
}

func NewSqlite3Service(db *gorm.DB) *sqlite3Service {
	return &sqlite3Service{db: db}
}

func (service *sqlite3Service) Create(device *Device) (*Device, error) {
	log.Info("Creating new device")
	return device, service.db.Create(device).Error
}

func (service *sqlite3Service) Update(device *Device) (*Device, error) {
	log.Infof("Updating device %d", device.ID)
	var saved Device
	return &saved, service.db.Model(&saved).Where("id = ?", device.ID).Updates(device).Error
}

func (service *sqlite3Service) UpdateHWAddr(device *Device) error {
	log.Infof("Updating device %d", device.HardwareAddr)
	var saved Device
	if err := service.db.Model(&saved).Where("hardware_addr = ?", device.HardwareAddr).Updates(device).Error; err != nil {
		_, err2 := service.Create(device)
		return err2
	}
	return nil
}

func (service *sqlite3Service) Delete(device *Device) error {
	log.Infof("Deleting device %d", device.ID)
	return service.db.Delete(device).Error
}

func (service *sqlite3Service) FindByID(id int) (*Device, error) {
	log.Infof("Finding device by id %d", id)
	device := &Device{}
	if err := service.db.First(&device, id).Error; err != nil {
		return nil, err
	}
	return device, nil
}

func (service *sqlite3Service) FindByFeeder(feeder string) ([]*Device, error) {
	log.Infof("Finding devices by feeder %s", feeder)
	devices := make([]*Device, 0)
	if err := service.db.Where("feeder = ?", feeder).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (service *sqlite3Service) FindByHardwareAddr(hardwareAddr net.HardwareAddr) (*Device, error) {
	log.Infof("Finding device by hardware address %s", hardwareAddr)
	var device Device
	if err := service.db.Where("hardware_addr = ?", hardwareAddr.String()).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (service *sqlite3Service) FindAll() ([]*Device, error) {
	log.Infof("Finding all devices")
	devices := make([]*Device, 0)
	if err := service.db.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}
