package device

import (
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type NATSController struct {
	nc            *nats.EncodedConn
	deviceService IDeviceService
}

func NewNATSController(nc *nats.EncodedConn, deviceService IDeviceService) *NATSController {
	return &NATSController{nc: nc, deviceService: deviceService}
}

func (controller *NATSController) Subscribe() error {
	_, err := controller.nc.Subscribe(CreateTopic, controller.handleCreate)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", CreateTopic, err)
	}
	_, err = controller.nc.Subscribe(UpdateTopic, controller.handleUpdate)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", UpdateTopic, err)
	}
	_, err = controller.nc.Subscribe(DeleteTopic, controller.handleDelete)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", DeleteTopic, err)
	}
	_, err = controller.nc.Subscribe(FindAllTopic, controller.handleFindAll)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", FindAllTopic, err)
	}
	_, err = controller.nc.Subscribe(FindByIDTopic, controller.handleFindByID)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", FindByIDTopic, err)
	}
	_, err = controller.nc.Subscribe(FindByFeederTopic, controller.handleFindByFeeder)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", FindByFeederTopic, err)
	}
	return nil
}

func (controller *NATSController) handleCreate(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	var device Device
	err := json.Unmarshal(msg.Data, &device)
	if err != nil {
		log.Errorf("unmarshal msg data: %v", err)
		return
	}
	d, err := controller.deviceService.Create(&device)
	if err != nil {
		log.Errorf("created device %d: %v", d.ID, err)
		return
	}
	err = controller.nc.Publish(msg.Reply, device)
	if err != nil {
		log.Errorf("publish created device %d: %v", d.ID, err)
		return
	}
}

func (controller *NATSController) handleUpdate(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	var device Device
	err := json.Unmarshal(msg.Data, &device)
	if err != nil {
		log.Errorf("unmarshal msg data: %v", err)
		return
	}
	_, err = controller.deviceService.Update(&device)
	if err != nil {
		log.Errorf("update device %d: %v", device.ID, err)
		return
	}
	err = controller.nc.Publish(msg.Reply, device)
	if err != nil {
		log.Errorf("publish updated device %d: %v", device.ID, err)
		return
	}
}

func (controller *NATSController) handleDelete(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	var device Device
	err := json.Unmarshal(msg.Data, &device)
	if err != nil {
		log.Errorf("unmarshal msg data: %v", err)
		return
	}
	err = controller.deviceService.Delete(&device)
	if err != nil {
		log.Errorf("delete device %d: %v", device.ID, err)
		return
	}
	err = controller.nc.Publish(msg.Reply, device)
	if err != nil {
		log.Errorf("publish deleted device %d: %v", device.ID, err)
		return
	}
}

func (controller *NATSController) handleFindAll(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	devices, err := controller.deviceService.FindAll()
	if err != nil {
		log.Errorf("find all devices: %v", err)
		return
	}
	err = controller.nc.Publish(msg.Reply, devices)
	if err != nil {
		log.Errorf("publish all devices: %v", err)
		return
	}
}

func (controller *NATSController) handleFindByID(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	var device Device
	err := json.Unmarshal(msg.Data, &device)
	if err != nil {
		log.Errorf("unmarshal msg data: %v", err)
		return
	}
	found, err := controller.deviceService.FindByID(int(device.ID))
	if err != nil {
		log.Errorf("find device by id %d: %v", device.ID, err)
		return
	}
	err = controller.nc.Publish(msg.Reply, found)
	if err != nil {
		log.Errorf("publish found device %d: %v", device.ID, err)
		return
	}
}

func (controller *NATSController) handleFindByFeeder(msg *nats.Msg) {
	log.Debugf("Received message on subject %s", msg.Subject)
	var device Device
	err := json.Unmarshal(msg.Data, &device)
	if err != nil {
		log.Errorf("unmarshal msg data: %v", err)
		return
	}
	found, err := controller.deviceService.FindByFeeder(device.Feeder)
	if err != nil {
		log.Errorf("find by feeder %s: %v", device.Feeder, err)
		return
	}
	err = controller.nc.Publish(msg.Reply, found)
	if err != nil {
		log.Errorf("publish devices: %v", err)
	}
}
