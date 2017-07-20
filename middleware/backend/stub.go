package etcd

import (
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/middleware/backend/msg"
	"github.com/coredns/coredns/middleware/proxy"

	"github.com/miekg/dns"
)

// UpdateStubZones checks etcd for an update on the stubzones.
func (b *Backend) UpdateStubZones() {
	go func() {
		for {
			b.updateStubZones()
			time.Sleep(15 * time.Second)
		}
	}()
}

// Look in .../dns/stub/<zone>/xx for msg.Services. Loop through them
// extract <zone> and add them as forwarders (ip:port-combos) for
// the stub zones. Only numeric (i.b. IP address) hosts are used.
// Only the first zone configured on b is used for the lookup.
func (b *Backend) updateStubZones() {
	zone := b.Zones[0]
	services, err := b.ServiceBackend.Records(stubDomain+"."+zone, false)
	if err != nil {
		return
	}

	stubmap := make(map[string]proxy.Proxy)
	// track the nameservers on a per domain basis, but allow a list on the domain.
	nameservers := map[string][]string{}

Services:
	for _, serv := range services {
		if serv.Port == 0 {
			serv.Port = 53
		}
		ip := net.ParseIP(serv.Host)
		if ip == nil {
			log.Printf("[WARNING] Non IP address stub nameserver: %s", serv.Host)
			continue
		}

		domain := msg.Domain(serv.Key)
		labels := dns.SplitDomainName(domain)

		// If the remaining name equals any of the zones we have, we ignore it.
		for _, z := range b.Zones {
			// Chop of left most label, because that is used as the nameserver place holder
			// and drop the right most labels that belong to zone.
			// We must *also* chop of dns.stub. which means cutting two more labels.
			domain = dns.Fqdn(strings.Join(labels[1:len(labels)-dns.CountLabel(z)-2], "."))
			if domain == z {
				log.Printf("[WARNING] Skipping nameserver for domain we are authoritative for: %s", domain)
				continue Services
			}
		}
		nameservers[domain] = append(nameservers[domain], net.JoinHostPort(serv.Host, strconv.Itoa(serv.Port)))
	}

	for domain, nss := range nameservers {
		stubmap[domain] = proxy.NewLookup(nss)
	}
	// atomic swap (at least that's what we hope it is)
	if len(stubmap) > 0 {
		b.Stubmap = &stubmap
	}
	return
}