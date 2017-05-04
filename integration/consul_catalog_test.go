package main

import (
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"

	"github.com/go-check/check"
	"github.com/hashicorp/consul/api"

	"bytes"
	"errors"
	"github.com/containous/traefik/integration/utils"
	checker "github.com/vdemeester/shakers"
	"strings"
)

// Consul catalog test suites
type ConsulCatalogSuite struct {
	BaseSuite
	consulIP     string
	consulClient *api.Client
}

func (s *ConsulCatalogSuite) SetUpSuite(c *check.C) {

	s.createComposeProject(c, "consul_catalog")
	s.composeProject.Start(c)

	consul := s.composeProject.Container(c, "consul")

	s.consulIP = consul.NetworkSettings.IPAddress
	config := api.DefaultConfig()
	config.Address = s.consulIP + ":8500"
	consulClient, err := api.NewClient(config)
	if err != nil {
		c.Fatalf("Error creating consul client")
	}
	s.consulClient = consulClient

	// Wait for consul to elect itself leader
	time.Sleep(2000 * time.Millisecond)
}

func (s *ConsulCatalogSuite) registerService(name string, address string, port int, tags []string) error {
	catalog := s.consulClient.Catalog()
	_, err := catalog.Register(
		&api.CatalogRegistration{
			Node:    address,
			Address: address,
			Service: &api.AgentService{
				ID:      name,
				Service: name,
				Address: address,
				Port:    port,
				Tags:    tags,
			},
		},
		&api.WriteOptions{},
	)
	return err
}

func (s *ConsulCatalogSuite) registerServiceAndCheck(name string, address string, port int, tags []string) error {
	catalog := s.consulClient.Catalog()
	_, err := catalog.Register(
		&api.CatalogRegistration{
			Node:    address,
			Address: address,
			Service: &api.AgentService{
				ID:      name,
				Service: name,
				Address: address,
				Port:    port,
				Tags:    tags,
			},
			Check: &api.AgentCheck{
				Node:      address,
				CheckID:   name,
				ServiceID: name,
				Name:      name,
			},
		},
		&api.WriteOptions{},
	)
	return err
}

func (s *ConsulCatalogSuite) deregisterService(name string, address string) error {
	catalog := s.consulClient.Catalog()
	_, err := catalog.Deregister(
		&api.CatalogDeregistration{
			Node:      address,
			Address:   address,
			ServiceID: name,
		},
		&api.WriteOptions{},
	)
	return err
}

func (s *ConsulCatalogSuite) TestSimpleConfiguration(c *check.C) {
	cmd := exec.Command(traefikBinary, "--consulCatalog", "--consulCatalog.endpoint="+s.consulIP+":8500", "--configFile=fixtures/consul_catalog/simple.toml")
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer cmd.Process.Kill()

	time.Sleep(500 * time.Millisecond)
	// TODO validate : run on 80
	resp, err := http.Get("http://127.0.0.1:8000/")

	// Expected a 404 as we did not configure anything
	c.Assert(err, checker.IsNil)
	c.Assert(resp.StatusCode, checker.Equals, 404)
}

func (s *ConsulCatalogSuite) TestSingleService(c *check.C) {
	cmd := exec.Command(traefikBinary, "--consulCatalog", "--consulCatalog.endpoint="+s.consulIP+":8500", "--consulCatalog.domain=consul.localhost", "--configFile=fixtures/consul_catalog/simple.toml")
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer cmd.Process.Kill()

	nginx := s.composeProject.Container(c, "nginx")

	err = s.registerService("test", nginx.NetworkSettings.IPAddress, 80, []string{})
	c.Assert(err, checker.IsNil, check.Commentf("Error registering service"))
	defer s.deregisterService("test", nginx.NetworkSettings.IPAddress)

	// wait for traefik
	err = utils.TryRequest("http://127.0.0.1:8081/api/providers", 60*time.Second, func(res *http.Response) error {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), "Host:test.consul.localhost") {
			return errors.New("Incorrect traefik config")
		}
		return nil
	})
	c.Assert(err, checker.IsNil)

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8000/", nil)
	c.Assert(err, checker.IsNil)
	req.Host = "test.consul.localhost"
	resp, err := client.Do(req)

	c.Assert(err, checker.IsNil)
	c.Assert(resp.StatusCode, checker.Equals, 200)

	_, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, checker.IsNil)
}

func (s *ConsulCatalogSuite) NoTestSingleHealthcheckService(c *check.C) {
	cmd := exec.Command(traefikBinary, "--consulCatalog", "--consulCatalog.endpoint="+s.consulIP+":8500", "--consulCatalog.domain=consul.localhost", "--configFile=fixtures/consul_catalog/simple.toml")
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer cmd.Process.Kill()

	whoami := s.composeProject.Container(c, "whoami")

	err = s.registerService("test", whoami.NetworkSettings.IPAddress, 80, []string{})
	c.Assert(err, checker.IsNil, check.Commentf("Error registering service"))
	defer s.deregisterService("test", whoami.NetworkSettings.IPAddress)

	agent := s.consulClient.Agent()
	err = agent.CheckRegister(&api.AgentCheckRegistration{
		ID:        "test",
		Name:      "test",
		ServiceID: "test",
		AgentServiceCheck: api.AgentServiceCheck{
			HTTP:     "http://" + whoami.NetworkSettings.IPAddress + ":80/health",
			Interval: "5s",
			Timeout:  "60s",
		},
	})
	c.Assert(err, checker.IsNil)

	client := &http.Client{}
	healthReq, err := http.NewRequest("POST", "http://"+whoami.NetworkSettings.IPAddress+"/health", bytes.NewBuffer([]byte("500")))
	c.Assert(err, checker.IsNil)
	_, err = client.Do(healthReq)
	c.Assert(err, checker.IsNil)

	// wait for traefik
	err = utils.TryRequest("http://127.0.0.1:8081/api/providers", 15*time.Second, func(res *http.Response) error {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), "Host:test.consul.localhost") {
			return errors.New("Incorrect traefik config")
		}
		return nil
	})
	c.Assert(err, checker.Not(checker.IsNil))

	healthReq, err = http.NewRequest("POST", "http://"+whoami.NetworkSettings.IPAddress+"/health", bytes.NewBuffer([]byte("200")))
	c.Assert(err, checker.IsNil)
	_, err = client.Do(healthReq)
	c.Assert(err, checker.IsNil)

	// wait for traefik
	err = utils.TryRequest("http://127.0.0.1:8081/api/providers", 15*time.Second, func(res *http.Response) error {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), "Host:test.consul.localhost") {
			return errors.New("Incorrect traefik config")
		}
		return nil
	})
	c.Assert(err, checker.IsNil)

	//client := &http.Client{}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8000/", nil)
	c.Assert(err, checker.IsNil)
	req.Host = "test.consul.localhost"
	resp, err := client.Do(req)

	c.Assert(err, checker.IsNil)
	c.Assert(resp.StatusCode, checker.Equals, 200)

	_, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, checker.IsNil)
}
