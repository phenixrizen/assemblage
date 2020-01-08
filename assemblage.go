/*
This is a simple Cluster using MDNS and gossip to maintain the cluster
*/
package assemblage

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/hashicorp/memberlist"
	"github.com/patrickmn/go-cache"
	"github.com/serialx/hashring"
	"github.com/sirupsen/logrus"
)

const (
	SCANFREQ = 15
	ADVFREQ  = 15
)

type Assemblage struct {
	port       int
	service    string
	instance   string
	domain     string
	iface      string
	memberlist *memberlist.Memberlist
	cache      *cache.Cache
	logger     *logrus.Logger
	events     chan memberlist.NodeEvent
	mdnsServer *mdns.Server
	hashring   *hashring.HashRing
}

func NewAssemblage(port int, iface, service, instance, domain string, log *logrus.Logger) (*Assemblage, error) {
	// check if valid port number
	if port < 1024 || port < 0 || port > 65535 {
		return nil, fmt.Errorf("not a valid service port: '%d' passed, must be between 1025-65535", port)
	}

	// check iface, service, instance, and domain
	if len(iface) == 0 {
		return nil, fmt.Errorf("must pass a valid interface name: '%s' passed", iface)
	} else {
		_, err := net.InterfaceByName(iface)
		if err != nil {
			return nil, fmt.Errorf("must pass a valid interface name: '%s' passed", err)
		}
	}
	if len(service) == 0 {
		return nil, fmt.Errorf("must pass a valid service name: '%s' passed", service)
	}
	if len(instance) == 0 {
		return nil, fmt.Errorf("must pass a valid instance name: '%s' passed", instance)
	}
	if len(domain) == 0 {
		return nil, fmt.Errorf("must pass a valid domain name: '%s' passed", domain)
	}

	assem := &Assemblage{
		port:     port,
		service:  service,
		instance: instance,
		domain:   domain,
		iface:    iface,
	}

	if log == nil {
		assem.logger = logrus.New()
	} else {
		assem.logger = log
	}

	return assem, nil
}

func (a *Assemblage) Run() error {
	c := cache.New(cache.NoExpiration, 5*time.Minute)
	a.cache = c

	go func(iface string, a *Assemblage) {
		// Check for hostname & IP then readvertise
		host, _ := os.Hostname()
		i, _ := net.InterfaceByName(iface)
		addrs, err := i.Addrs()
		if err != nil {
			a.logger.Fatalf("Failed to get interface IP[s]: %s", err)
		}
		var ips []net.IP
		for _, ip := range addrs {
			i, _, err := net.ParseCIDR(ip.String())
			if err != nil {
				a.logger.Fatalf("Cannot parse CIDR: %s", err)
			} else {
				ips = append(ips, i)
			}
		}
		a.logger.Infof("Advertizing on the following addresses: %v", ips)
		// Now that we know the IPs and it's safe we can advertise the service is running
		// Setup our service export
		service, err := mdns.NewMDNSService(a.instance, a.service, a.domain, host+".", a.port, ips, []string{host})
		if err != nil {
			a.logger.Fatalf("Failed to create MDNS service: %s", err)
		}
		// Create the mDNS server, defer shutdown
		a.mdnsServer, err = mdns.NewServer(&mdns.Config{Zone: service})
		//defer server.Shutdown()
		if err != nil {
			a.logger.Fatalf("Failed to create MDNS server: %s", err)
		}
	}(a.iface, a)

	// Create the initial member list from a default configuration
	conf := memberlist.DefaultLANConfig()
	ml, err := memberlist.Create(conf)
	a.memberlist = ml
	ring := hashring.New(a.getMembers())
	a.hashring = ring

	if err != nil {
		a.logger.Fatalf("Failed to create cluster server: %s", err)
	} else {
		a.logger.Infof("Advertizing self as %s via MDNS", a.service)
	}

	// Make a channel for mdns service results and start listening
	results := make(chan *mdns.ServiceEntry, 1)
	go func(a *Assemblage) {
		// Act on all entries we receive on the results channel
		for entry := range results {
			a.logger.Infof("Received MDNS service broadcast: %v", *entry)
			// Get our addresses and iterate over them
			addrs, err := net.InterfaceAddrs()
			if err != nil {
				a.logger.Fatalf("Failed to get interface addresses: %s", err)
			}
			for _, addr := range addrs {
				address, _, _ := net.ParseCIDR(addr.String())
				if address.String() == entry.AddrV4.String() || address.String() == entry.AddrV6.String() {
					// Do nothing
					a.logger.Infof("Ignoring self broadcast no need to add to cluster: %v@%v", entry.Host, entry.AddrV4.String())
				} else {
					// Check to see if we have seen this node on the last 300 seconds if we have then there is no need to add to cluster
					v, _ := a.cache.Get("/mdns/" + entry.AddrV4.String())
					if strings.Index(entry.Name, a.service) > -1 && entry.Port == a.port && !a.isMember(entry.AddrV4.String()) && v == nil {
						ipv4 := []string{entry.AddrV4.String()}
						a.logger.Infof("Discovered a new service entry: %v", *entry)
						// Join an existing cluster by mdns info received
						//TODO: add ipv6 clustering
						_, err := a.memberlist.Join(ipv4)
						if err != nil {
							a.logger.Warningf("Failed to add node to cluster server: %s", err)
						} else {
							a.logger.Infof("Added %s to cluster", ipv4)
							a.addNodesToRing()
						}
					}

					a.cache.Set("/mdns/"+entry.AddrV4.String(), entry, 300*time.Second)
				}
			}
		}
	}(a)

	go func(results chan *mdns.ServiceEntry, a *Assemblage) {
		for {
			// Start the lookup
			i, _ := net.InterfaceByName(a.iface)
			params := &mdns.QueryParam{
				Service:             a.service,
				Domain:              a.domain,
				Interface:           i,
				Timeout:             SCANFREQ * time.Second,
				Entries:             results,
				WantUnicastResponse: true,
			}
			mdns.Query(params)
			delay := SCANFREQ * time.Second
			time.Sleep(delay)
		}
	}(results, a)

	a.memberlist = ml

	return nil
}

func (a *Assemblage) Shutdown() error {
	err := a.mdnsServer.Shutdown()
	if err != nil {
		return err
	}
	err = a.memberlist.Shutdown()
	if err != nil {
		return err
	}

	return nil
}

func (a *Assemblage) GetNode(key string) string {
	node, ok := a.hashring.GetNode(key)
	logrus.New().Infof("%v", ok)
	return node
}

func (a *Assemblage) GetNodes(key string, size int) []string {
	nodes, ok := a.hashring.GetNodes(key, size)
	logrus.New().Infof("%v", ok)
	return nodes
}

func (a *Assemblage) isMember(ip string) bool {
	members := a.memberlist.Members()
	for _, mem := range members {
		if mem.Addr.String() == ip {
			return true
		}
	}
	return false
}

func (a *Assemblage) getMembers() []string {
	members := make([]string, a.memberlist.NumMembers())
	for i, mem := range a.memberlist.Members() {
		members[i] = mem.Addr.String()
	}
	return members
}

func (a *Assemblage) addNodesToRing() error {
	members := a.getMembers()
	for _, mem := range members {
		a.hashring = a.hashring.AddNode(mem)
	}
	return nil
}
