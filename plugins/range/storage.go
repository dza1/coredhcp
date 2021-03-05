// Copyright 2018-present the CoreDHCP Authors. All rights reserved
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rangeplugin

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coredhcp/coredhcp/plugins/allocators"
	"github.com/coredhcp/coredhcp/plugins/allocators/bitmap"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

//Record holds an IP lease record
type Record struct {
	IP      net.IP
	expires time.Time
}

// PluginState is the data held by an instance of the range plugin
type Storage struct {
	// Rough lock for the whole plugin, we'll get better performance once we use leasestorage
	sync.Mutex
	// Recordsv4 holds a MAC -> IP address and lease time mapping
	Recordsv4 map[string]*Record
	//leasefile *os.File
	allocator allocators.Allocator
	filename  string
	LeaseTime time.Duration
}

func SetupStorage(filename string, LeaseTime time.Duration, ipRangeStart net.IP, ipRangeEnd net.IP) (*Storage, error) {
	var err error
	s := &Storage{
		filename:  filename,
		LeaseTime: LeaseTime,
	}
	s.allocator, err = bitmap.NewIPv4Allocator(ipRangeStart, ipRangeEnd)
	if err != nil {
		return nil, fmt.Errorf("could not create an allocator: %w", err)
	}

	s.Recordsv4, err = s.loadRecordsFromFile(filename)
	if err != nil {
		s.Recordsv4 = make(map[string]*Record)
		log.Warnf("Could not load %s: %v", filename, LeaseTime)
	}

	s.initGarbColl(&s.Recordsv4, 5)
	return s, nil

}
func (s *Storage) Update(HWAddr net.HardwareAddr, reqIP net.IP, MsgTyp dhcpv4.MessageType) (net.IP, error) {
	s.Lock()
	defer s.Unlock()
	record, ok := s.Recordsv4[HWAddr.String()]
	if !ok {
		if MsgTyp == dhcpv4.MessageTypeRequest {
			//If an IP address is requested, that we didn't offer, we answer with a NAK
			return nil, fmt.Errorf("HWAddr not found in storage: %s", HWAddr.String())
		}
		// Allocating new address since there isn't one allocated
		log.Printf("MAC address %s is new, leasing new IPv4 address", HWAddr.String())
		ip, err := s.allocator.Allocate(net.IPNet{IP: reqIP})
		if err != nil {
			return nil, fmt.Errorf("Could not allocate IP for MAC %s: %v", HWAddr.String(), err)
		}
		rec := Record{
			IP:      ip.IP.To4(),
			expires: time.Now().Add(s.LeaseTime),
		}
		s.Recordsv4[HWAddr.String()] = &rec
		record = &rec
	} else {
		// Ensure we extend the existing lease at least past when the one we're giving expires
		if record.expires.Before(time.Now().Add(s.LeaseTime)) {
			record.expires = time.Now().Add(s.LeaseTime).Round(time.Second)
			if MsgTyp == dhcpv4.MessageTypeRequest { //just write the leasefile the client sends a valid request
				err := s.writeLeaseFile(s.filename)
				if err != nil {
					log.Errorf("Could not persist lease for MAC %s: %v", HWAddr.String(), err)
				}
			}
		}
	}
	return record.IP, nil
}

func (s *Storage) Delete(HWAddr net.HardwareAddr) error {
	s.Lock()
	defer s.Unlock()
	record, ok := s.Recordsv4[HWAddr.String()]
	if !ok {
		log.Errorf("Range: Release for unknown HW ID requested")
		return fmt.Errorf("Range: Release for unknown HW ID requested")
	}
	err := s.allocator.Free(net.IPNet{IP: record.IP})
	if err != nil {
		return fmt.Errorf("Release IP : %v", err)
	}
	delete(s.Recordsv4, HWAddr.String())
	err = s.writeLeaseFile(s.filename)
	if err != nil {
		return fmt.Errorf("Could not write the leasfile %s: %v", s.filename, err.Error())
	}
	log.Infof("IP %s released", record.IP.String())
	return nil
}

// loadRecords loads the DHCPv6/v4 Records global map with records stored on
// the specified file. The records have to be one per line, a mac address and an
// IP address.
func loadRecords(r io.Reader) (map[string]*Record, error) {
	sc := bufio.NewScanner(r)
	records := make(map[string]*Record)
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 {
			continue
		}
		tokens := strings.Fields(line)
		if len(tokens) != 3 {
			return nil, fmt.Errorf("malformed line, want 3 fields, got %d: %s", len(tokens), line)
		}
		hwaddr, err := net.ParseMAC(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("malformed hardware address: %s", tokens[0])
		}
		ipaddr := net.ParseIP(tokens[1])
		if ipaddr.To4() == nil {
			return nil, fmt.Errorf("expected an IPv4 address, got: %v", ipaddr)
		}
		expires, err := time.Parse(time.RFC3339, tokens[2])
		if err != nil {
			return nil, fmt.Errorf("expected time of exipry in RFC3339 format, got: %v", tokens[2])
		}

		if expires.After(time.Now()) {
			records[hwaddr.String()] = &Record{IP: ipaddr, expires: expires}
		}
	}
	return records, nil
}

func (s *Storage) loadRecordsFromFile(filename string) (map[string]*Record, error) {
	reader, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	defer func() {
		if err := reader.Close(); err != nil {
			log.Warningf("Failed to close file %s: %v", filename, err)
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("cannot open lease file %s: %w", filename, err)
	}

	rec, err := loadRecords(reader)
	if err != nil {
		return nil, err
	}
	//allocate IP addresses from lease file
	for _, v := range rec {
		ip, err := s.allocator.Allocate(net.IPNet{IP: v.IP})
		if err != nil {
			return nil, fmt.Errorf("Could not allocate IP %s form lease file: %v", v.IP, err.Error())
		}
		if !ip.IP.Equal(v.IP) {
			return nil, fmt.Errorf("Allocate IP %s, but it should be IP %s form lease file (wrong lease file?)", ip.IP.String(), v.IP)
		}

	}
	return rec, nil
}

// // saveIPAddress writes out a lease to storage
// func (s *Storage) saveIPAddress(mac net.HardwareAddr, record *Record) error {
// 	_, err := s.leasefile.WriteString(mac.String() + " " + record.IP.String() + " " + record.expires.Format(time.RFC3339) + "\n")
// 	if err != nil {
// 		return err
// 	}
// 	err = s.leasefile.Sync()
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// saveIPAddress writes out a lease to storage
func (s *Storage) writeLeaseFile(filename string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Warningf("Failed to open file %s: %v", filename, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Warningf("Failed to close file %s: %v", filename, err)
		}
	}()
	for key, v := range s.Recordsv4 {
		_, err := file.WriteString(key + " " + v.IP.String() + " " + v.expires.Format(time.RFC3339) + "\n")
		if err != nil {
			return err
		}
	}
	return nil
}

// func (p *PluginState) writeLeaseFileBin() error {
// 	file, err := os.OpenFile(leaseBin, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
// 	if err != nil {
// 		return err
// 	}
// 	defer file.Close()
// 	log.Info("Write lease.bin file")
// 	err = gob.NewEncoder(file).Encode(p.Recordsv4)
// 	return err
// }

// func (p *PluginState) readLeaseFile(keymap interface{}) error {
// 	file, err := os.OpenFile(leaseBin, os.O_RDWR|os.O_CREATE, 0644)
// 	if err != nil {
// 		return err
// 	}
// 	defer file.Close()
// 	err = gob.NewDecoder(file).Decode(keymap)
// 	return err
// }

// registerBackingFile installs a file as the backing store for leases
// func (p *PluginState) registerBackingFile(filename string) error {
// 	if p.leasefile != nil {
// 		// This is TODO; swapping the file out is easy
// 		// but maintaining consistency with the in-memory state isn't
// 		return errors.New("cannot swap out a lease storage file while running")
// 	}
// 	// We never close this, but that's ok because plugins are never stopped/unregistered
// 	newLeasefile, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	if err != nil {
// 		return fmt.Errorf("failed to open lease file %s: %w", filename, err)
// 	}
// 	p.leasefile = newLeasefile
// 	return nil
// }

func (s *Storage) initGarbColl(keymap *map[string]*Record, duration_s int) {
	ticker := time.NewTicker(time.Duration(duration_s) * time.Second)
	go func() {
		for range ticker.C {
			log.Debug("garbage collector")
			err := s.cleanupRecord(&s.Recordsv4)
			if err != nil {
				log.Errorf("Garbage collector error: %v", err.Error())

			}
		}
	}()
}

func (s *Storage) cleanupRecord(keymap *map[string]*Record) error {
	var del bool
	del = false
	s.Lock()
	defer s.Unlock()
	for key, v := range *keymap {
		if v.expires.Before(time.Now()) {
			err := s.allocator.Free(net.IPNet{IP: v.IP})
			if err != nil {
				return fmt.Errorf("Could not free IP %s: %v", v.IP, err.Error())
			}
			del = true
			log.Infof("Delete: %s", v.IP)
			delete(*keymap, key)
		}

	}
	if del {
		err := s.writeLeaseFile(s.filename)
		if err != nil {
			return fmt.Errorf("Could not write the leasfile %s: %v", s.filename, err.Error())
		}
	}
	return nil
}
