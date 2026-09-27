package device

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"beam/internal/crypto"
	"beam/internal/storage"
)

type Type string

const (
	TypeMac     Type = "mac"
	TypeWindows Type = "windows"
	TypeIOS     Type = "ios"
	TypeLinux   Type = "linux"
)

func CurrentType() Type {
	switch runtime.GOOS {
	case "darwin":
		return TypeMac
	case "windows":
		return TypeWindows
	case "ios":
		return TypeIOS
	default:
		return TypeLinux
	}
}

type Config struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Type       Type   `json:"type"`
	ListenPort int    `json:"listen_port"`
}

type Identity struct {
	Config     Config
	PrivateKey []byte
	PublicKey  []byte
}

type Peer struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          Type      `json:"type"`
	PublicKey     string    `json:"public_key"`
	PairedAt      time.Time `json:"paired_at"`
	Endpoint      string    `json:"endpoint,omitempty"`
	LastSeen      time.Time `json:"last_seen,omitempty"`
	LastReachable time.Time `json:"last_reachable,omitempty"`
	BadEndpoint   string    `json:"bad_endpoint,omitempty"`
}

const DefaultPort = 47821

func DefaultName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "BEAM-device"
	}
	return h
}

// Ensure loads an existing identity, or creates one when this device has not
// been initialized. A requested name updates only the display name of an
// existing device and never replaces its keys.
func Ensure(name string) (*Identity, bool, error) {
	if ident, err := Load(); err == nil {
		if name != "" && ident.Config.Name != name {
			ident.Config.Name = name
			if err := SaveConfig(ident.Config); err != nil {
				return nil, false, err
			}
		}
		return ident, false, nil
	}
	keyPath, err := storage.KeyPath()
	if err != nil {
		return nil, false, err
	}
	if _, err := os.Stat(keyPath); err == nil {
		return nil, false, fmt.Errorf("identity key exists but the device configuration is unreadable")
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}
	ident, err := Init(name)
	return ident, true, err
}

func Init(name string) (*Identity, error) {
	if name == "" {
		name = DefaultName()
	}
	keyPath, err := storage.KeyPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(keyPath); err == nil {
		return nil, fmt.Errorf("device already initialized (%s)", keyPath)
	}

	pub, priv, err := crypto.GenerateIdentity()
	if err != nil {
		return nil, err
	}
	if err := crypto.SavePrivateKey(keyPath, priv); err != nil {
		return nil, err
	}

	cfg := Config{
		DeviceID:   crypto.DeviceID(pub),
		Name:       name,
		Type:       CurrentType(),
		ListenPort: DefaultPort,
	}
	if err := SaveConfig(cfg); err != nil {
		return nil, err
	}
	if err := SavePeers(nil); err != nil {
		return nil, err
	}
	return &Identity{Config: cfg, PrivateKey: priv, PublicKey: pub}, nil
}

func Load() (*Identity, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	keyPath, err := storage.KeyPath()
	if err != nil {
		return nil, err
	}
	priv, err := crypto.LoadPrivateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load identity key: %w (run beam init)", err)
	}
	pub := crypto.PublicFromPrivate(priv)
	if crypto.DeviceID(pub) != cfg.DeviceID {
		return nil, fmt.Errorf("config device id does not match key file")
	}
	return &Identity{Config: cfg, PrivateKey: priv, PublicKey: pub}, nil
}

func SaveConfig(cfg Config) error {
	path, err := storage.ConfigPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func LoadConfig() (Config, error) {
	path, err := storage.ConfigPath()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w (run beam init)", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadPeers() ([]Peer, error) {
	path, err := storage.PeersPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var peers []Peer
	if err := json.Unmarshal(b, &peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func SavePeers(peers []Peer) error {
	path, err := storage.PeersPath()
	if err != nil {
		return err
	}
	if peers == nil {
		peers = []Peer{}
	}
	b, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func UpsertPeer(p Peer) error {
	peers, err := LoadPeers()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range peers {
		if existing.ID == p.ID {
			peers[i] = p
			found = true
			break
		}
	}
	if !found {
		peers = append(peers, p)
	}
	return SavePeers(peers)
}

func FindPeer(query string) (*Peer, error) {
	peers, err := LoadPeers()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var matches []Peer
	for _, p := range peers {
		if strings.EqualFold(p.ID, query) || strings.ToLower(p.Name) == q {
			matches = append(matches, p)
			continue
		}
		if strings.Contains(strings.ToLower(p.Name), q) {
			matches = append(matches, p)
		}
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous device %q", query)
	}
	return nil, fmt.Errorf("unknown device %q; pair it first with beam pair", query)
}

func PeerByID(id string) (*Peer, error) {
	peers, err := LoadPeers()
	if err != nil {
		return nil, err
	}
	for _, p := range peers {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("unknown paired device")
}
