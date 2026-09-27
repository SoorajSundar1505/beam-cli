package discovery

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"

	"beam/internal/protocol"
)

type Remote struct {
	ID       string
	Name     string
	Type     string
	Host     string
	Port     int
	Pair     bool
	Addr     string
	Instance string
	TTL      uint32
}

type Advertiser struct {
	server  *zeroconf.Server
	info    Info
	pairing bool
}

type Info struct {
	ID   string
	Name string
	Type string
	Port int
}

func Advertise(info Info, pairing bool) (*Advertiser, error) {
	instance := info.ID
	if instance == "" {
		instance = info.Name
	}
	s, err := zeroconf.Register(instance, protocol.ServiceType, protocol.Domain, info.Port, txtRecords(info, pairing), nil)
	if err != nil {
		return nil, err
	}
	return &Advertiser{server: s, info: info, pairing: pairing}, nil
}

func (a *Advertiser) Close() {
	if a != nil && a.server != nil {
		a.server.Shutdown()
	}
}

// SetDisplayName updates the advertised label without changing the service
// instance. The instance stays the device ID, so a rename cannot leave a
// second peer record on the network.
func (a *Advertiser) SetDisplayName(name string) {
	if a == nil || a.server == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" || name == a.info.Name {
		return
	}
	a.info.Name = name
	a.server.SetText(txtRecords(a.info, a.pairing))
}

func txtRecords(info Info, pairing bool) []string {
	txt := []string{
		"id=" + info.ID,
		"name=" + info.Name,
		"type=" + info.Type,
		"proto=1",
	}
	if pairing {
		txt = append(txt, "pair=1")
	}
	return txt
}

// LocalAddresses returns active, non-loopback addresses that mDNS may
// advertise. It is intended for diagnostics; zeroconf still selects the
// actual interfaces used for DNS-SD.
func LocalAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil && !ip.IsLoopback() {
				out = append(out, ip.String())
			}
		}
	}
	sort.Strings(out)
	return out
}

func Browse(ctx context.Context, timeout time.Duration) ([]Remote, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}
	entries := make(chan *zeroconf.ServiceEntry, 16)
	bctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := resolver.Browse(bctx, protocol.ServiceType, protocol.Domain, entries); err != nil {
		return nil, err
	}

	byID := map[string]Remote{}
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			r, ok := fromEntry(e)
			if !ok {
				continue
			}
			mu.Lock()
			if existing, ok := byID[r.ID]; ok {
				r = keepRemote(existing, r)
			}
			byID[r.ID] = r
			mu.Unlock()
		}
	}()
	<-bctx.Done()
	// Allow in-flight entries to drain briefly.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	out := make([]Remote, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	return out, nil
}

func fromEntry(e *zeroconf.ServiceEntry) (Remote, bool) {
	txt := map[string]string{}
	for _, t := range e.Text {
		k, v, ok := strings.Cut(t, "=")
		if ok {
			txt[k] = v
		}
	}
	id := txt["id"]
	if id == "" {
		return Remote{}, false
	}
	name := txt["name"]
	if name == "" {
		name = e.Instance
	}
	port := e.Port
	host := pickAddr(e)
	if host == "" {
		host = strings.TrimSuffix(e.HostName, ".")
	}
	return Remote{
		ID:       id,
		Name:     name,
		Type:     txt["type"],
		Host:     host,
		Port:     port,
		Pair:     txt["pair"] == "1",
		Addr:     net.JoinHostPort(host, strconv.Itoa(port)),
		Instance: e.Instance,
		TTL:      e.TTL,
	}, true
}

// keepRemote chooses which advertisement represents one device ID.
// A registration whose instance is the device ID is the current daemon.
// An older registration that used the display name as its instance loses,
// so a cached "Windows-PC" record cannot keep that label after a rename.
func keepRemote(current, next Remote) Remote {
	if current.ID == "" {
		return next
	}
	currentStable := current.Instance != "" && current.Instance == current.ID
	nextStable := next.Instance != "" && next.Instance == next.ID
	if nextStable != currentStable {
		if nextStable {
			return next
		}
		return current
	}
	if next.TTL > current.TTL {
		return next
	}
	if next.TTL == current.TTL && next.Name != "" {
		return next
	}
	return current
}

func pickAddr(e *zeroconf.ServiceEntry) string {
	for _, ip := range e.AddrIPv4 {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		return ip.String()
	}
	for _, ip := range e.AddrIPv6 {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		return ip.String()
	}
	if len(e.AddrIPv4) > 0 && e.AddrIPv4[0] != nil {
		return e.AddrIPv4[0].String()
	}
	return ""
}

func FindID(ctx context.Context, id string) (*Remote, error) {
	list, err := Browse(ctx, 3*time.Second)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("device is offline or not advertising (is beam receive running on the other device?)")
}

func FindPairing(ctx context.Context) ([]Remote, error) {
	list, err := Browse(ctx, 3*time.Second)
	if err != nil {
		return nil, err
	}
	var out []Remote
	for _, r := range list {
		if r.Pair {
			out = append(out, r)
		}
	}
	return out, nil
}
