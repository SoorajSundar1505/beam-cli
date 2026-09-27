package device

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"beam/internal/discovery"
)

type Listed struct {
	Index  int
	ID     string
	Name   string
	Type   Type
	Status string
	Addr   string
	Peer   *Peer
	Self   bool
}

func ListDevices(ident *Identity, online []discovery.Remote) ([]Listed, error) {
	peers, err := LoadPeers()
	if err != nil {
		return nil, err
	}
	on := map[string]discovery.Remote{}
	for _, r := range online {
		on[r.ID] = r
	}
	if err := syncAdvertisedNames(ident.Config.DeviceID, peers, online); err != nil {
		return nil, err
	}
	var out []Listed
	out = append(out, Listed{
		ID:     ident.Config.DeviceID,
		Name:   ident.Config.Name,
		Type:   ident.Config.Type,
		Status: "this device",
		Self:   true,
	})
	seen := map[string]bool{ident.Config.DeviceID: true}
	for _, p := range peers {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		st := "offline"
		addr := ""
		if r, ok := on[p.ID]; ok {
			addr = r.Addr
			if p.BadEndpoint != "" && p.BadEndpoint == r.Addr {
				st = "unreachable"
			} else {
				st = "online"
			}
		}
		out = append(out, Listed{
			ID:     p.ID,
			Name:   p.Name,
			Type:   p.Type,
			Status: st,
			Addr:   addr,
			Peer:   peerPtr(p),
		})
	}
	for _, r := range online {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, Listed{
			ID:     r.ID,
			Name:   r.Name,
			Type:   Type(r.Type),
			Status: "online (unpaired)",
			Addr:   r.Addr,
		})
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out, nil
}

// syncAdvertisedNames stores the latest mDNS name and endpoint for a paired
// device. The device ID stays the record key, so a rename or a new address
// updates that one peer and leaves the public key and pairing time alone.
func syncAdvertisedNames(selfID string, peers []Peer, online []discovery.Remote) error {
	index := make(map[string]int, len(peers))
	for i, peer := range peers {
		index[peer.ID] = i
	}
	changed := false
	now := time.Now().UTC()
	for _, remote := range online {
		i, ok := index[remote.ID]
		if !ok || remote.ID == "" || remote.ID == selfID {
			continue
		}
		name := strings.TrimSpace(remote.Name)
		if name != "" && peers[i].Name != name {
			peers[i].Name = name
			changed = true
		}
		if remote.Addr != "" && peers[i].Endpoint != remote.Addr {
			peers[i].Endpoint = remote.Addr
			if peers[i].BadEndpoint != "" && peers[i].BadEndpoint != remote.Addr {
				peers[i].BadEndpoint = ""
			}
			changed = true
		}
		if peers[i].LastSeen.IsZero() || now.Sub(peers[i].LastSeen) > time.Second {
			peers[i].LastSeen = now
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return SavePeers(peers)
}

// MarkReachable records that a handshake to this endpoint succeeded.
func MarkReachable(id, endpoint string) error {
	return noteEndpoint(id, endpoint, true)
}

// MarkUnreachable forgets an endpoint that could not complete a connection.
func MarkUnreachable(id, endpoint string) error {
	return noteEndpoint(id, endpoint, false)
}

func noteEndpoint(id, endpoint string, reachable bool) error {
	peers, err := LoadPeers()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	found := false
	for i := range peers {
		if peers[i].ID != id {
			continue
		}
		found = true
		if reachable {
			peers[i].Endpoint = endpoint
			peers[i].LastReachable = now
			peers[i].BadEndpoint = ""
		} else if endpoint != "" {
			peers[i].BadEndpoint = endpoint
			if peers[i].Endpoint == endpoint {
				peers[i].Endpoint = ""
			}
		}
	}
	if !found {
		return nil
	}
	return SavePeers(peers)
}

func peerPtr(p Peer) *Peer {
	cp := p
	return &cp
}

func FormatTable(items []Listed) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tSTATUS")
	for _, it := range items {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", it.Index, it.Name, it.Type, it.Status)
	}
	_ = w.Flush()
	return b.String()
}

func ResolveTarget(query string, items []Listed) (*Listed, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("no device specified")
	}
	var matches []Listed
	for _, it := range items {
		if fmt.Sprintf("%d", it.Index) == q || strings.EqualFold(it.Name, q) || it.ID == q {
			matches = append(matches, it)
			continue
		}
		if strings.Contains(strings.ToLower(it.Name), strings.ToLower(q)) {
			matches = append(matches, it)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("unknown device %q", query)
	}
	if len(matches) > 1 {
		exact := 0
		var only Listed
		for _, m := range matches {
			if strings.EqualFold(m.Name, q) || m.ID == q {
				exact++
				only = m
			}
		}
		if exact == 1 {
			return &only, nil
		}
		return nil, fmt.Errorf("ambiguous device %q", query)
	}
	return &matches[0], nil
}
