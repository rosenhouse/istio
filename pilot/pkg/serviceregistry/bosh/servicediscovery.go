package bosh

import (
	"fmt"

	"istio.io/istio/pilot/pkg/model"
)

// ServiceDiscovery implements the model.ServiceDiscovery interface for Cloud Foundry
type ServiceDiscovery struct {
	Client *Client
}

// Services implements a service catalog operation
func (sd *ServiceDiscovery) Services() ([]*model.Service, error) {
	servicesMap, err := sd.Client.GetAllServiceInstances()
	if err != nil {
		return nil, err
	}

	services := []*model.Service{}
	for _, service := range servicesMap {
		services = append(services, service.Service)
	}
	return services, nil
}

// GetService implements a service catalog operation
func (sd *ServiceDiscovery) GetService(hostname string) (*model.Service, error) {
	services, err := sd.Services()
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		if svc.Hostname == hostname {
			return svc, nil
		}
	}
	return nil, nil
}

// Instances implements a service catalog operation
func (sd *ServiceDiscovery) Instances(hostname string, ports []string, tagsList model.LabelsCollection) ([]*model.ServiceInstance, error) {
	servicesMap, err := sd.Client.GetAllServiceInstances()
	if err != nil {
		return nil, fmt.Errorf("getting all instances: %s", err)
	}

	svcWithInstances := servicesMap[Hostname(hostname)]

	svcPorts := make(map[string]*model.Port)
	for _, portName := range ports {
		if svcPort, exists := svcWithInstances.Service.Ports.Get(portName); exists {
			svcPorts[portName] = svcPort
		}
	}

	instances := []*model.ServiceInstance{}
	for _, instance := range svcWithInstances.Instances {
		if !tagsList.HasSubsetOf(instance.Labels) {
			continue
		}
		if _, exists := svcPorts[instance.Endpoint.ServicePort.Name]; exists {
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

// HostInstances implements a service catalog operation
func (sd *ServiceDiscovery) HostInstances(addrs map[string]*model.Node) ([]*model.ServiceInstance, error) {
	servicesMap, err := sd.Client.GetAllServiceInstances()
	if err != nil {
		return nil, fmt.Errorf("getting all instances: %s", err)
	}

	instances := []*model.ServiceInstance{}

	for _, svcWithInstances := range servicesMap {
		for _, instance := range svcWithInstances.Instances {
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

// ManagementPorts is not currently implemented for Cloud Foundry
func (sd *ServiceDiscovery) ManagementPorts(addr string) model.PortList {
	return nil
}
