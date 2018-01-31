package bosh

import "istio.io/istio/pilot/pkg/model"

type serviceAccounts struct {
}

// NewServiceAccounts instantiates the Cloud Foundry service account interface
func NewServiceAccounts() model.ServiceAccounts {
	return &serviceAccounts{}
}

func (sa *serviceAccounts) GetIstioServiceAccounts(hostname string, ports []string) []string {
	return nil
}
