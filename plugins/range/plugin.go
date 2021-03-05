// Copyright 2018-present the CoreDHCP Authors. All rights reserved
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rangeplugin

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

var log = logger.GetLogger("plugins/range")

// Plugin wraps plugin registration information
var Plugin = plugins.Plugin{
	Name:   "range",
	Setup4: setupRange,
}

// PluginState is the data held by an instance of the range plugin
type PluginState struct {
	// Rough lock for the whole plugin, we'll get better performance once we use leasestorage
	sync.Mutex
	// Recordsv4 holds a MAC -> IP address and lease time mapping
	Recordsv4 map[string]*Record
	storage   *Storage
}

// Handler4 handles DHCPv4 packets for the range plugin
func (p *PluginState) Handler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	var recIP net.IP
	var err error
	p.Lock()
	defer p.Unlock()
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		recIP, err = p.storage.Update(req.ClientHWAddr, req.RequestedIPAddress(), req.MessageType())
		if err != nil {
			log.Infof("Range: %v", err)
			resp = replaceWithNak(req, resp, "No available IPs")
			return resp, true
		}
		log.Printf("Offer IP address %s for MAC %s", recIP, req.ClientHWAddr.String())
	case dhcpv4.MessageTypeRequest:
		recIP, err = p.storage.Update(req.ClientHWAddr, req.RequestedIPAddress(), req.MessageType())
		if err != nil {
			log.Infof("Range: %v", err)
			resp = replaceWithNak(req, resp, "No lease")
			return resp, true
		}
		log.Printf("ACK IP address %s for MAC %s", recIP, req.ClientHWAddr.String())
	case dhcpv4.MessageTypeRelease:
		err = p.storage.Delete(req.ClientHWAddr)
		if err != nil {
			log.Errorf("Range: Could not delete %s from map: %v", req.ClientIPAddr.String(), err)
		}
		return nil, true
	default:
		log.Errorf("Request Message Type not supporte: %v", req.MessageType())
		return nil, true
	}

	resp.YourIPAddr = recIP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(p.storage.LeaseTime.Round(time.Second)))
	return resp, false
}

func setupRange(args ...string) (handler.Handler4, error) {
	var (
		err error
		p   PluginState
	)

	if len(args) < 4 {
		return nil, fmt.Errorf("invalid number of arguments, want: 4 (file name, start IP, end IP, lease time), got: %d", len(args))
	}
	filename := args[0]
	if filename == "" {
		return nil, errors.New("file name cannot be empty")
	}
	ipRangeStart := net.ParseIP(args[1])
	if ipRangeStart.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", args[1])
	}
	ipRangeEnd := net.ParseIP(args[2])
	if ipRangeEnd.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", args[2])
	}
	if binary.BigEndian.Uint32(ipRangeStart.To4()) >= binary.BigEndian.Uint32(ipRangeEnd.To4()) {
		return nil, errors.New("start of IP range has to be lower than the end of an IP range")
	}

	LeaseTime, err := time.ParseDuration(args[3])
	if err != nil {
		return nil, fmt.Errorf("invalid lease duration: %v", args[3])
	}

	// p.Recordsv4, err = loadRecordsFromFile(p.filename)
	// if err != nil {
	// 	return nil, fmt.Errorf("could not load records from file: %v", err)
	// }

	p.storage, err = SetupStorage(filename, LeaseTime, ipRangeStart, ipRangeEnd)
	if err != nil {
		return nil, fmt.Errorf("Could not setup Storage: %v", err)
	}

	log.Printf("Loaded %d DHCPv4 leases from %s", len(p.Recordsv4), filename)

	// if err := p.registerBackingFile(p.filename); err != nil {
	// 	return nil, fmt.Errorf("could not setup lease storage: %w", err)
	// }

	return p.Handler4, nil
}

// Replaces 'resp' with an NAK response to the 'req' message,
// keeps only the ServerIPAddr from the original 'resp' and puts
// an error msg at 'resp'
func replaceWithNak(req, resp *dhcpv4.DHCPv4, msg string) *dhcpv4.DHCPv4 {
	serverID := resp.ServerIPAddr
	if serverID.IsUnspecified() {
		log.Warn("Server ID is unspecified, 'server_id' must be before 'range' in config file")
	}
	tResp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		log.Errorf("MainHandler4: failed to build NAK reply: %v", err)
	}
	tResp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
	tResp.UpdateOption(dhcpv4.OptMessage(msg))
	tResp.UpdateOption(dhcpv4.OptServerIdentifier(serverID))
	return tResp
}
