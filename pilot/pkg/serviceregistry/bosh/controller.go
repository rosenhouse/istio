package bosh

import (
	"reflect"
	"time"

	_ "github.com/golang/glog"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/log"
)

type serviceHandler func(*model.Service, model.Event)
type instanceHandler func(*model.ServiceInstance, model.Event)

// Ticker acts like time.Ticker but is mockable for testing
type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTicker struct {
	*time.Ticker
}

func (r realTicker) Chan() <-chan time.Time {
	return r.C
}

// NewTicker returns a Ticker used to instantiate a Controller
func NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

// Controller communicates with Cloud Foundry and monitors for changes
type Controller struct {
	Client           *Client
	Ticker           Ticker
	serviceHandlers  []serviceHandler
	instanceHandlers []instanceHandler
}

// AppendServiceHandler implements a service catalog operation
func (c *Controller) AppendServiceHandler(f func(*model.Service, model.Event)) error {
	c.serviceHandlers = append(c.serviceHandlers, f)
	return nil
}

// AppendInstanceHandler implements a service catalog operation
func (c *Controller) AppendInstanceHandler(f func(*model.ServiceInstance, model.Event)) error {
	c.instanceHandlers = append(c.instanceHandlers, f)
	return nil
}

// Run will loop, calling handlers in response to changes, until a signal is received
func (c *Controller) Run(stop <-chan struct{}) {
	var cache map[Hostname]*ServiceWithInstances

	for {
		select {
		case <-c.Ticker.Chan():
			allServiceInstances, err := c.Client.GetAllServiceInstances()
			if err != nil {
				log.Warnf("periodic bosh poll failed: %s", err)
				continue
			}

			if !reflect.DeepEqual(allServiceInstances, cache) {
				cache = allServiceInstances
				for _, h := range c.serviceHandlers {
					go h(&model.Service{}, model.EventAdd)
				}
				for _, h := range c.instanceHandlers {
					go h(&model.ServiceInstance{}, model.EventAdd)
				}
			}
		case <-stop:
			c.Ticker.Stop()
			return
		}
	}
}
