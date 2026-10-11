package portal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Device is a paired peer shown around the portal. Addresses are omitted.
type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Options connects the portal to the existing device list and transfer APIs.
type Options struct {
	Self      string
	List      func(context.Context) ([]Device, error)
	SendFile  func(ctx context.Context, deviceID, path, name string, report func(sent, total int64)) error
	SendText  func(ctx context.Context, deviceID, text string, report func(sent, total int64)) error
	SendClip  func(ctx context.Context, deviceID string, report func(sent, total int64)) error
	ReadClip  func() (string, error)
	Open      func(url string) error
	Addr      string
	NoBrowser bool
	Tray      string
}

type job struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Sent    int64  `json:"sent"`
	Total   int64  `json:"total"`
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"`
	cancel  context.CancelFunc
}

// Run serves the portal on localhost until ctx is cancelled.
func Run(ctx context.Context, opt Options) error {
	if opt.List == nil || opt.SendFile == nil || opt.SendText == nil {
		return errors.New("portal is missing transfer hooks")
	}
	addr := opt.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	s := &server{opt: opt, jobs: map[string]*job{}, hand: NewMachine(), tray: loadTray(opt.Tray)}
	httpSrv := &http.Server{Handler: s.routes()}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()
	url := "http://" + ln.Addr().String()
	fmt.Println(url)
	if !opt.NoBrowser && opt.Open != nil {
		_ = opt.Open(url)
	}
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shut)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

type server struct {
	opt  Options
	mu   sync.Mutex
	jobs map[string]*job
	hand *Machine
	tray []TrayItem
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/api/devices", s.devices)
	mux.HandleFunc("/api/gesture", s.gesture)
	mux.HandleFunc("/api/gesture/reset", s.gestureReset)
	mux.HandleFunc("/api/tray", s.trayList)
	mux.HandleFunc("/api/tray/import", s.importTray)
	mux.HandleFunc("/api/send/tray", s.sendTray)
	mux.HandleFunc("/api/send/text", s.sendText)
	mux.HandleFunc("/api/send/file", s.sendFile)
	mux.HandleFunc("/api/send/clipboard", s.sendClipboard)
	mux.HandleFunc("/api/jobs/", s.job)
	return mux
}

func (s *server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(pageHTML)
}

func (s *server) devices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	list, err := s.opt.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []Device{}
	}
	writeJSON(w, map[string]any{"self": s.opt.Self, "devices": list})
}

func (s *server) gesture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var frame Frame
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&frame); err != nil {
		writeErr(w, err)
		return
	}
	s.mu.Lock()
	view := s.hand.Step(frame)
	s.mu.Unlock()
	writeJSON(w, view)
}

func (s *server) gestureReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.hand.Reset()
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) trayList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	items := s.tray
	if items == nil {
		items = []TrayItem{}
	}
	writeJSON(w, map[string]any{"items": items})
}

func (s *server) importTray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, err)
		return
	}
	for _, hdr := range r.MultipartForm.File["file"] {
		src, err := hdr.Open()
		if err != nil {
			writeErr(w, err)
			return
		}
		tmp, err := os.CreateTemp("", "beam-tray-*")
		if err != nil {
			src.Close()
			writeErr(w, err)
			return
		}
		if _, err := io.Copy(tmp, src); err != nil {
			src.Close()
			tmp.Close()
			writeErr(w, err)
			return
		}
		src.Close()
		tmp.Close()
		name := filepath.Base(hdr.Filename)
		s.mu.Lock()
		s.tray = append(s.tray, TrayItem{ID: newID(), Name: name, Kind: "file", Path: tmp.Name()})
		s.mu.Unlock()
	}
	s.mu.Lock()
	items := append([]TrayItem(nil), s.tray...)
	s.mu.Unlock()
	if items == nil {
		items = []TrayItem{}
	}
	writeJSON(w, map[string]any{"items": items})
}

func (s *server) sendTray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Device string `json:"device"`
		Item   string `json:"item"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	var item TrayItem
	found := false
	for _, candidate := range s.tray {
		if candidate.ID == body.Item {
			item = candidate
			found = true
			break
		}
	}
	if !found {
		writeErr(w, errors.New("file is not in the tray"))
		return
	}
	if s.transferring() {
		writeErr(w, errors.New("a transfer is already running"))
		return
	}
	paths := item.files()
	if len(paths) == 0 {
		writeErr(w, errors.New("folder is empty"))
		return
	}
	s.start(w, r, body.Device, item.Name, func(ctx context.Context, report func(int64, int64)) error {
		for i, path := range paths {
			name := filepath.Base(path)
			if err := s.opt.SendFile(ctx, body.Device, path, name, func(sent, total int64) {
				report(int64(i)*total+sent, int64(len(paths))*total)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *server) sendText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Device string `json:"device"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, errors.New("nothing to send"))
		return
	}
	name := "text"
	if strings.Contains(text, "://") {
		name = "link"
	}
	s.start(w, r, body.Device, name, func(ctx context.Context, report func(int64, int64)) error {
		return s.opt.SendText(ctx, body.Device, text, report)
	})
}

func (s *server) sendClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Device string `json:"device"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if s.opt.SendClip != nil {
		s.start(w, r, body.Device, "clipboard", func(ctx context.Context, report func(int64, int64)) error {
			return s.opt.SendClip(ctx, body.Device, report)
		})
		return
	}
	if s.opt.ReadClip == nil {
		writeErr(w, errors.New("clipboard is not available"))
		return
	}
	text, err := s.opt.ReadClip()
	if err != nil {
		writeErr(w, err)
		return
	}
	s.start(w, r, body.Device, "clipboard", func(ctx context.Context, report func(int64, int64)) error {
		return s.opt.SendText(ctx, body.Device, text, report)
	})
}

func (s *server) sendFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, err)
		return
	}
	deviceID := r.FormValue("device")
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, errors.New("choose a file"))
		return
	}
	defer file.Close()
	name := filepath.Base(hdr.Filename)
	if name == "." || name == string(filepath.Separator) || name == "" {
		writeErr(w, errors.New("invalid filename"))
		return
	}
	tmp, err := os.CreateTemp("", "beam-portal-*")
	if err != nil {
		writeErr(w, err)
		return
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		writeErr(w, err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		writeErr(w, err)
		return
	}
	s.start(w, r, deviceID, name, func(ctx context.Context, report func(int64, int64)) error {
		defer os.Remove(tmpName)
		return s.opt.SendFile(ctx, deviceID, tmpName, name, report)
	})
}

func (s *server) start(w http.ResponseWriter, r *http.Request, deviceID, name string, run func(context.Context, func(int64, int64)) error) {
	if strings.TrimSpace(deviceID) == "" {
		writeErr(w, errors.New("select a device"))
		return
	}
	if s.transferring() {
		writeErr(w, errors.New("a transfer is already running"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{ID: newID(), Name: name, Status: "running", cancel: cancel}
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()
	go func() {
		err := run(ctx, func(sent, total int64) {
			s.mu.Lock()
			j.Sent = sent
			j.Total = total
			if total > 0 {
				j.Percent = int(sent * 100 / total)
				if j.Percent > 100 {
					j.Percent = 100
				}
			}
			s.mu.Unlock()
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		if j.Status == "cancelled" {
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				j.Status = "cancelled"
				return
			}
			j.Status = "error"
			j.Error = err.Error()
			return
		}
		j.Status = "done"
		j.Percent = 100
	}()
	s.mu.Lock()
	snap := j.view()
	s.mu.Unlock()
	writeJSON(w, snap)
}

func (s *server) job(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	id, cancel := strings.CutSuffix(id, "/cancel")
	s.mu.Lock()
	j := s.jobs[id]
	s.mu.Unlock()
	if j == nil {
		http.NotFound(w, r)
		return
	}
	if cancel {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.mu.Lock()
		if j.Status == "running" {
			j.Status = "cancelled"
			j.cancel()
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, j.view())
}

func (j *job) view() job {
	return job{ID: j.ID, Name: j.Name, Status: j.Status, Sent: j.Sent, Total: j.Total, Percent: j.Percent, Error: j.Error}
}

func (s *server) transferring() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.Status == "running" {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
