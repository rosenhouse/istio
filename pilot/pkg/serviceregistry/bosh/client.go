package bosh

import (
	"fmt"
	"sync"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/director"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	yaml "gopkg.in/yaml.v2"

	"istio.io/istio/pilot/pkg/model"
)

type Client struct {
	boshdir.Director

	virtualIPs VIPStore

	mu                sync.Mutex
	cachedFrom        time.Time
	allInstancesCache map[Hostname]*ServiceWithInstances
}

func NewClient(cfg *Config) (*Client, error) {
	logger := boshlog.NewLogger(boshlog.LevelError)
	factory := boshdir.NewFactory(logger)

	var err error

	factoryCfg := boshdir.Config{
		Host: cfg.Host,
		Port: 25555,

		CACert:       string(cfg.caCertContents),
		Client:       cfg.Client,
		ClientSecret: cfg.ClientSecret,

		TokenFunc: nil,
	}
	directorClient, err := factory.New(factoryCfg,
		boshdir.NewNoopTaskReporter(),
		boshdir.NewNoopFileReporter(),
	)
	return &Client{
		Director:   directorClient,
		virtualIPs: NewVIPStore(),
	}, err
}

type DeploymentName string
type InstanceGroupName string
type JobName string

type InstanceGroup struct {
	VMs          []boshdir.VMInfo
	ExposedPorts map[JobName][]*model.Port
}

type Deployment map[InstanceGroupName]*InstanceGroup
type AllDeployments map[DeploymentName]Deployment

func (c *Client) GetDeployments() (AllDeployments, error) {
	directorDeployments, err := c.Deployments()
	if err != nil {
		return nil, fmt.Errorf("getting deployments: %s", err)
	}

	allDeployments := make(AllDeployments)
	for _, directorDeployment := range directorDeployments {
		deploymentName := DeploymentName(directorDeployment.Name())
		manifestWithPorts, err := c.GetManifestWithPortExposures(directorDeployment)
		if err != nil {
			return nil, fmt.Errorf("getting port exposures for %s: %s", deploymentName, err)
		}
		deployment := make(Deployment)

		vmInfos, err := directorDeployment.InstanceInfos()
		if err != nil {
			return nil, fmt.Errorf("getting instance infos for deployment %s: %s", deploymentName, err)
		}

		for _, vmInfo := range vmInfos {
			instanceGroupName := InstanceGroupName(vmInfo.JobName)

			instanceGroup := deployment[instanceGroupName]
			if instanceGroup == nil {
				instanceGroup = &InstanceGroup{
					ExposedPorts: make(map[JobName][]*model.Port),
				}
				for _, ig := range manifestWithPorts.InstanceGroups {
					if ig.Name != string(instanceGroupName) {
						continue
					}
					for _, job := range ig.Jobs {
						exposedPorts := []*model.Port{}
						for _, port := range job.ExposePorts {
							exposedPorts = append(exposedPorts, &model.Port{
								Port:     port.Number,
								Name:     port.Name,
								Protocol: model.ProtocolTCP,
							})
						}
						instanceGroup.ExposedPorts[JobName(job.Name)] = exposedPorts
					}
				}
			}
			instanceGroup.VMs = append(instanceGroup.VMs, vmInfo)
			deployment[instanceGroupName] = instanceGroup
		}

		allDeployments[deploymentName] = deployment
	}
	return allDeployments, nil
}

type Hostname string

type ServiceWithInstances struct {
	Service   *model.Service
	Instances []*model.ServiceInstance
}

/*

  instance_groups:
  - name: zookeeper-group
    jobs:
    - name: zookeeper
+     expose_ports:
+     - name: client_port
+       number: 2181
+     - name: quorum_port
+       number: 2888
  - name: nats-group
    jobs:
    - name: nats
+     expose_ports:
+     - name: client_port
+       number: 4222


*/

type ManifestWithExposedPorts struct {
	Name           string
	InstanceGroups []struct {
		Name string
		Jobs []struct {
			Name        string
			ExposePorts []struct {
				Name   string
				Number int
			} `yaml:"expose_ports"`
		}
	} `yaml:"instance_groups"`
}

func (c *Client) GetManifestWithPortExposures(deployment boshdir.Deployment) (*ManifestWithExposedPorts, error) {
	manifestString, err := deployment.Manifest()
	if err != nil {
		return nil, fmt.Errorf("getting manifest: %s", err)
	}

	var manifestStruct ManifestWithExposedPorts
	err = yaml.Unmarshal([]byte(manifestString), &manifestStruct)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest yaml: %s", err)
	}
	return &manifestStruct, nil
}

func (c *Client) GetServiceInstances(deploymentName DeploymentName,
	instanceGroupName InstanceGroupName,
	instanceGroup InstanceGroup,
) (map[Hostname]*ServiceWithInstances, error) {

	dnsSuffix := fmt.Sprintf("%s.%s.boshy", instanceGroupName, deploymentName)
	servicesMap := make(map[Hostname]*ServiceWithInstances)

	for _, vmInfo := range instanceGroup.VMs {
		for _, process := range vmInfo.Processes {
			hostname := Hostname(fmt.Sprintf("%s.%s", process.Name, dnsSuffix))
			servicePorts := instanceGroup.ExposedPorts[JobName(process.Name)]
			if servicesMap[hostname] == nil {
				virtualIP, err := c.virtualIPs.Get(hostname)
				if err != nil {
					return nil, err
				}

				servicesMap[hostname] = &ServiceWithInstances{
					Service: &model.Service{
						Hostname: string(hostname),
						Ports:    servicePorts,
						Address:  string(virtualIP),
					},
				}
			}
			serviceWithInstances := servicesMap[hostname]
			labels := map[string]string{
				"vm.state":      vmInfo.State,
				"vm.job_state":  vmInfo.ProcessState,
				"process.state": process.State,
			}
			if vmInfo.Index != nil {
				labels["index"] = fmt.Sprintf("%d", *vmInfo.Index)
			}
			for i, dnsName := range vmInfo.DNS {
				labels[fmt.Sprintf("dns-%d", i)] = dnsName
			}
			for _, ip := range vmInfo.IPs {
				for _, servicePort := range servicePorts {
					serviceWithInstances.Instances = append(serviceWithInstances.Instances, &model.ServiceInstance{
						Endpoint: model.NetworkEndpoint{
							Address:     ip,
							Port:        servicePort.Port,
							ServicePort: servicePort,
						},
						Service:          serviceWithInstances.Service,
						Labels:           labels,
						AvailabilityZone: vmInfo.AZ,
					})
				}
			}
		}
	}

	return servicesMap, nil
}

func (c *Client) cacheIsFresh() bool {
	return c.allInstancesCache != nil && time.Since(c.cachedFrom) < 10*time.Second
}

func (c *Client) GetAllServiceInstances() (map[Hostname]*ServiceWithInstances, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cacheIsFresh() {
		return c.allInstancesCache, nil
	}

	allDeployments, err := c.GetDeployments()
	if err != nil {
		return nil, fmt.Errorf("bosh client: %s", err)
	}

	servicesMap := make(map[Hostname]*ServiceWithInstances)

	for deploymentName, deployment := range allDeployments {
		for instanceGroupName, instanceGroup := range deployment {
			serviceInstances, err := c.GetServiceInstances(deploymentName, instanceGroupName, *instanceGroup)
			if err != nil {
				return nil, fmt.Errorf("get service instances: %s", err)
			}

			for hostname, set := range serviceInstances {
				servicesMap[hostname] = set
			}
		}
	}

	c.allInstancesCache = servicesMap
	c.cachedFrom = time.Now()

	return servicesMap, nil
}
