package device

import (
	"encoding/binary"
	"net"
	"os"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	logDb "gorm.io/gorm/logger"
)

const dbName = ".test.db"
const startIP = "192.168.0.1"
const endtIP = "192.168.0.254"
const LeaseTime = 60
const testLength = 200

type sqliteTestState struct {
	db *Sqlite3Service
}

func TestDevice(t *testing.T) {
	p := setup()
	var i uint32
	for ; i < testLength; i++ {
		go func(i uint32) {
			recIP, err := p.db.UpdateHWAddr(int2mac(i), int2ip(i), dhcpv4.MessageTypeDiscover, "Test Host")
			if err != nil {
				log.Fatalf("Error during update IP %s: %v", recIP.String(), err)
			}
		}(i)
	}
	//os.Remove(dbName)

}

func setup() sqliteTestState {
	var s sqliteTestState
	ipRangeStart := net.ParseIP(startIP)
	if ipRangeStart.To4() == nil {
		log.Fatalf("invalid IPv4 address: %v", startIP)
	}
	ipRangeEnd := net.ParseIP(endtIP)
	if ipRangeStart.To4() == nil {
		log.Fatalf("invalid IPv4 address: %v", endtIP)
	}

	os.Remove(dbName)

	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{
		Logger: logDb.Default.LogMode(logDb.Silent),
	})
	if err != nil {
		log.Fatalf("%v", err)
	}

	s.db, err = NewSqlite3Service(db, LeaseTime, ipRangeStart, ipRangeEnd)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Info("setup")
	return s
}

func int2ip(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}

func int2mac(nn uint32) net.HardwareAddr {
	mac := make(net.HardwareAddr, 6)
	binary.BigEndian.PutUint32(mac, nn)
	return mac
}
