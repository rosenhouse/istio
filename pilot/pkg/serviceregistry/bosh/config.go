package bosh

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

type Config struct {
	CACertPath     string
	caCertContents []byte
	Client         string
	ClientSecret   string
	Host           string
	PollInterval   time.Duration
}

// LoadConfig reads configuration data from the environment
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	cfg.CACertPath = os.Getenv("BOSH_CA_CERT")
	if strings.HasPrefix(cfg.CACertPath, "-----BEGIN CERTIFICATE-----") {
		cfg.caCertContents = []byte(cfg.CACertPath)
	} else {
		var err error
		cfg.caCertContents, err = ioutil.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("reading BOSH CA cert from %s: %s",
				cfg.CACertPath, err)
		}
	}
	cfg.Client = os.Getenv("BOSH_CLIENT")
	cfg.ClientSecret = os.Getenv("BOSH_CLIENT_SECRET")
	cfg.Host = os.Getenv("BOSH_ENVIRONMENT")
	if cfg.Host == "vbox" {
		cfg.Host = "192.168.50.6"
	}
	cfg.PollInterval = 10 * time.Second

	return cfg, nil
}
