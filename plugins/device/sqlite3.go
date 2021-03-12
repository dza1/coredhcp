package device

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/coredhcp/coredhcp/plugins/allocators"
	"github.com/coredhcp/coredhcp/plugins/allocators/bitmap"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"gorm.io/gorm"
)

type Sqlite3Service struct {
	db        *gorm.DB
	LeaseTime time.Duration
	allocator allocators.Allocator
}

// type freeTable struct{

// }

func NewSqlite3Service(db *gorm.DB, LeaseTime time.Duration, ipRangeStart net.IP, ipRangeEnd net.IP) (*Sqlite3Service, error) {
	var err error
	db.AutoMigrate(&Device{})
	sql := &Sqlite3Service{
		db:        db,
		LeaseTime: LeaseTime}

	err = sql.db.Where("lease_expiration < ?", time.Now()).Delete(&Device{}).Error
	if err != nil {
		return nil, fmt.Errorf("Could not delete old entries on startup: %v", err)
	}
	sql.allocator, err = bitmap.NewIPv4Allocator(ipRangeStart, ipRangeEnd)
	if err != nil {
		return nil, fmt.Errorf("Could not create an allocator: %v", err)
	}
	var device []Device
	sql.db.Select("ip").Find(&device)
	for _, v := range device {
		reqIP := net.ParseIP(v.IP)
		if reqIP == nil {
			log.Errorf("Could not ParseIP IP %s", v.IP)
			continue
		}
		recIp, err := sql.allocator.Allocate(net.IPNet{IP: reqIP})
		if err != nil {
			return nil, fmt.Errorf("Could not allocate IP %s form db: %v", v.IP, err.Error())
		}
		if !reqIP.Equal(recIp.IP) {
			sql.allocator.Free(recIp)
			log.Warnf("Allocate IP %s, but it should be IP %s form lease file (network changed?)", recIp.IP.String(), v.IP)
			continue
		}
		log.Infof("Allocate IP: %s", reqIP.String())
	}

	sql.initGarbColl(5) //carbage Collector each 5s
	return sql, nil
}

func (service *Sqlite3Service) Create(device *Device) (*Device, error) {
	log.Info("Creating new device")
	return device, service.db.Create(device).Error
}

func (service *Sqlite3Service) Update(device *Device) (*Device, error) {
	log.Infof("Updating device %d", device.ID)
	var saved Device
	return &saved, service.db.Model(&saved).Where("id = ?", device.ID).Updates(device).Error
}

func (service *Sqlite3Service) UpdateHWAddr(HWAddr net.HardwareAddr, reqIP net.IP, MsgTyp dhcpv4.MessageType, host string) (net.IP, error) {
	var respIP net.IP
	dbDevice, err := service.FindByHardwareAddr(HWAddr)
	if err != nil { //need to create a new entry
		if MsgTyp == dhcpv4.MessageTypeRequest {
			//If an IP address is requested, that we didn't offer, we answer with a NAK
			return nil, fmt.Errorf("HWAddr not found in storage: %s", HWAddr.String())
		}
		log.Printf("MAC address %s is new, leasing new IPv4 address", HWAddr.String())
		var ipNet net.IPNet
		ipNet, err = service.allocator.Allocate(net.IPNet{IP: reqIP})
		respIP = ipNet.IP
		if err != nil {
			return nil, fmt.Errorf("Could not allocate IP for MAC %s: %v", HWAddr.String(), err)
		}
		device := Device{
			HardwareAddr:    HWAddr.String(),
			IP:              respIP.String(),
			Host:            host,
			LeaseExpiration: time.Now().Add(service.LeaseTime),
		}
		log.Infof("Create device %d", HWAddr)
		err = service.db.Create(&device).Error
		if err != nil {
			err1 := service.allocator.Free(net.IPNet{IP: respIP})
			if err1 != nil {
				log.Error(err1)
				return nil, fmt.Errorf("Could free IP after DB error %s: %v", respIP.String(), err)
			}
		}
	} else { //update entry
		service.db.First(dbDevice) //get data from database in device
		dbDevice.LeaseExpiration = time.Now().Add(service.LeaseTime)
		if err := service.db.Save(&dbDevice).Error; err != nil {
			log.Errorf("Can't update device %s", HWAddr.String())
			return nil, err
		}
		if reqIP.String() != dbDevice.IP && MsgTyp == dhcpv4.MessageTypeRequest { //Requested IP must be the same that we storred
			return nil, fmt.Errorf("Received for %s IP %s, but IP %s is stored", HWAddr.String(), reqIP.String(), dbDevice.IP)
		}
		respIP = net.ParseIP(dbDevice.IP)
		log.Infof("Update device %d", HWAddr)
	}
	return respIP, err
}

func (service *Sqlite3Service) Release(HWAddr net.HardwareAddr, reqIP net.IP) error {
	err := service.db.Where("hardware_addr = ? AND ip = ?", HWAddr.String(), reqIP.String()).Delete(&Device{}).Error
	if err != nil {
		log.Errorf("Could not Delete IP %s from DB %v", reqIP.String(), err)
	}
	err = service.allocator.Free(net.IPNet{IP: reqIP})
	if err != nil {
		log.Errorf("Could not free IP %s", reqIP.String())
	}
	return err
}

func (service *Sqlite3Service) initGarbColl(duration_s int) {
	ticker := time.NewTicker(time.Duration(duration_s) * time.Second)
	go func() {
		for range ticker.C {
			log.Debug("garbage collector")
			err := service.DeleteOld()
			if err != nil {
				log.Errorf("Garbage collector error: %v", err)
			}
		}
	}()
}

// func (service *Sqlite3Service) Delete(device *Device) error {
// 	log.Infof("Deleting device %d", device.ID)
// 	return service.db.Model(&Device{}).Delete(device).Error
// }

func (service *Sqlite3Service) DeleteOld() error {
	log.Info("Deleting old device")
	var device []Device

	function := func(tx *gorm.DB) error {
		if r := recover(); r != nil { //cache panic
			tx.Rollback()
		}
		time := time.Now()

		err := tx.Model(Device{}).Where("lease_expiration < ?", time).Find(&device).Error
		if err != nil {
			return err
		}
		result := tx.Where("lease_expiration < ?", time).Delete(&Device{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(device)) {
			return errors.New("Entries have been modified during delete transaction")
		}
		return nil
	}

	var err error
	for i := 0; i < 3; i++ { //Try 3 times to delete the old Entries
		err = service.db.Transaction(function)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	for _, v := range device {
		ip := net.ParseIP(v.IP)
		if ip == nil {
			log.Errorf("Could not ParseIP IP %s", v.IP)
			continue
		}
		err := service.allocator.Free(net.IPNet{IP: ip})
		if err != nil {
			log.Errorf("Could not free IP %s, %v", v.IP, err)
		}
	}

	return err
}

// func (service *Sqlite3Service) DeleteOld() error {
// 	log.Info("Deleting old device")
// 	tx := service.db.Begin()
// 	curTime := time.Now()
// 	// defer func() {
// 	// 	if r := recover(); r != nil { //cache panic
// 	// 		log.Error("Rollback")
// 	// 		tx.Rollback()
// 	// 	}
// 	// }()

// 	if err := tx.Error; err != nil {
// 		return err
// 	}

// 	var device []Device
// 	err := tx.Model(Device{}).Where("lease_expiration < ?", curTime).Find(&device).Error
// 	if err != nil {
// 		tx.Rollback()
// 		return err
// 	}
// 	log.Warn(device)
// 	result := service.db.Model(Device{}).Where("lease_expiration < ?", curTime).Delete(&Device{})
// 	if result.Error != nil {
// 		tx.Rollback()
// 		return result.Error
// 	}
// 	log.Warn(int64(len(device)))
// 	if result.RowsAffected != int64(len(device)) {
// 		tx.Rollback()
// 		return errors.New("Entries have been modified during delete transaction")
// 	}

// 	err = tx.Commit().Error
// 	if err != nil {
// 		tx.Rollback()
// 		return err
// 	}

// 	return nil

// }

// func (d *Device) AfterDelete(tx *gorm.DB) (err error) {
// 	// 	if d.ID == 1 {
// 	// 		return errors.New("Delete not possible")
// 	// 	}
// 	log.Warn("After: ", d)
// 	return
// }

func (service *Sqlite3Service) FindByID(id int) (*Device, error) {
	log.Infof("Finding device by id %d", id)
	device := &Device{}
	if err := service.db.First(&device, id).Error; err != nil {
		return nil, err
	}
	return device, nil
}

func (service *Sqlite3Service) FindByFeeder(feeder string) ([]*Device, error) {
	log.Infof("Finding devices by feeder %s", feeder)
	devices := make([]*Device, 0)
	if err := service.db.Where("feeder = ?", feeder).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (service *Sqlite3Service) FindByHardwareAddr(hardwareAddr net.HardwareAddr) (*Device, error) {
	log.Infof("Finding device by hardware address %s", hardwareAddr)
	var device Device
	if err := service.db.Where("hardware_addr = ?", hardwareAddr.String()).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (service *Sqlite3Service) FindAll() ([]*Device, error) {
	log.Infof("Finding all devices")
	devices := make([]*Device, 0)
	if err := service.db.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}
