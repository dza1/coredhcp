package device

import (
	"net"
)

type IDeviceService interface {
	Create(device *Device) (*Device, error)
	Update(device *Device) (*Device, error)
	Delete(device *Device) error
	FindByID(id int) (*Device, error)
	FindByFeeder(id string) ([]*Device, error)
	FindByHardwareAddr(hardwareAddr net.HardwareAddr) (*Device, error)
	FindAll() ([]*Device, error)
}
