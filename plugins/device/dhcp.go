package device

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type dhcpRecord struct {
	hw         net.HardwareAddr
	ip         net.IP
	host       string
	expiration time.Time
}

type dhcpSync struct {
	watcher       *fsnotify.Watcher
	deviceService IDeviceService
	leaseFile     string
	leaseFileDump string
}

func NewDHCPSync(deviceService IDeviceService, leaseFile string, leaseFileDump string) (*dhcpSync, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("new file watch: %w", err)
	}
	if err := watcher.Add(leaseFile); err != nil {
		return nil, fmt.Errorf("watch lease file %s: %w", leaseFile, err)
	}
	return &dhcpSync{
		watcher:       watcher,
		deviceService: deviceService,
		leaseFile:     leaseFile,
		leaseFileDump: leaseFileDump,
	}, nil
}

func (s *dhcpSync) Sync() {
	if err := s.dumpLeasesAndUpdate(); err != nil {
		log.Errorf("dump leases and update: %v", err)
	}
	go func() {
		for {
			select {
			case event := <-s.watcher.Events:
				log.Infof("Received event %s", event)
				if err := s.dumpLeasesAndUpdate(); err != nil {
					log.Errorf("dump leases and update: %v", err)
				}
			case err := <-s.watcher.Errors:
				log.Errorf("%v", err)
			}
		}
	}()
}

func (s *dhcpSync) dumpLeasesAndUpdate() error {
	log.Infof("Dump leases %s", s.leaseFile)
	cmd := exec.Command("dumpleases", "-f", s.leaseFile, "-a")
	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("dumpleases: %w", err)
	}

	log.Infof("Create or truncate dump file %s", s.leaseFileDump)
	f, err := os.Create(s.leaseFileDump)
	if err != nil {
		return fmt.Errorf("create %s: %w", s.leaseFileDump, err)
	}

	log.Infof("Write dump to file %s", s.leaseFileDump)
	_, err = f.WriteString(string(b))
	if err != nil {
		return fmt.Errorf("write dump: %w", err)
	}

	log.Infof("Updating devices from %s", s.leaseFileDump)
	if err := s.updateDevices(); err != nil {
		return fmt.Errorf("update devices: %w", err)
	}
	return nil
}

func (s *dhcpSync) updateDevices() error {
	records, err := LoadDHCPv4Records(s.leaseFileDump)
	if err != nil {
		return fmt.Errorf("load DHCPv4 records from %s: %w", s.leaseFileDump, err)
	}

	for _, record := range records {
		device := &Device{
			HardwareAddr:    record.hw.String(),
			IP:              record.ip.String(),
			Host:            record.host,
			LeaseExpiration: record.expiration,
			Feeder:          "",
		}

		saved, err := s.deviceService.FindByHardwareAddr(record.hw)
		if err == gorm.ErrRecordNotFound {
			_, err := s.deviceService.Create(&Device{
				HardwareAddr:    record.hw.String(),
				IP:              record.ip.String(),
				Host:            record.host,
				LeaseExpiration: record.expiration,
				Feeder:          "",
			})
			if err != nil {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("find by hardware addr %s: %w", record.hw.String(), err)
		} else {
			device.ID = saved.ID
			_, err = s.deviceService.Update(device)
			if err != nil {
				return fmt.Errorf("update device %d: %w", device.ID, err)
			}
		}
	}
	return nil
}

func (s *dhcpSync) Close() error {
	return s.watcher.Close()
}

func LoadDHCPv4Records(filename string) (map[string]dhcpRecord, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	records := make(map[string]dhcpRecord)
	for _, lineBytes := range bytes.Split(data, []byte{'\n'})[1:] {
		line := string(lineBytes)
		if len(line) == 0 {
			continue
		}
		tokens := strings.Fields(line)
		if len(tokens) != 8 {
			return nil, fmt.Errorf("malformed line, want 2 fields, got %d: %s", len(tokens), line)
		}
		hwAddr, err := net.ParseMAC(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("malformed hardware address: %s", tokens[0])
		}
		ipAddr := net.ParseIP(tokens[1])
		if ipAddr.To4() == nil {
			return nil, fmt.Errorf("expected an IPv4 address, got: %v", ipAddr)
		}
		expirationToken := strings.Join(tokens[3:], " ")
		expiration, err := time.Parse(time.ANSIC, expirationToken)
		if err != nil {
			return nil, fmt.Errorf("malformed expiration time: %s", expirationToken)
		}
		records[hwAddr.String()] = dhcpRecord{
			hw:         hwAddr,
			ip:         ipAddr,
			host:       tokens[2],
			expiration: expiration,
		}
	}

	return records, nil
}
