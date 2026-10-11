package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testServer(sendFile func(context.Context, string, string, string, func(int64, int64)) error, sendText func(context.Context, string, string, func(int64, int64)) error) *httptest.Server {
	s := &server{opt: Options{
		Self: "Suraj-Mac",
		List: func(context.Context) ([]Device, error) {
			return []Device{{ID: "win", Name: "Suraj-Windows", Status: "online"}}, nil
		},
		SendFile: sendFile,
		SendText: sendText,
		ReadClip: func() (string, error) { return "", errors.New("clipboard is empty") },
	}, jobs: map[string]*job{}}
	return httptest.NewServer(s.routes())
}

func TestDevicesDoNotExposeAddresses(t *testing.T) {
	srv := testServer(nil, nil)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if bytes.Contains(raw, []byte("192.168")) || bytes.Contains(raw, []byte("addr")) {
		t.Fatalf("device payload leaked an address: %s", raw)
	}
	if body["self"] != "Suraj-Mac" {
		t.Fatalf("self: %v", body["self"])
	}
}

func TestSendTextReportsRealResult(t *testing.T) {
	var got string
	srv := testServer(nil, func(_ context.Context, device, text string, report func(int64, int64)) error {
		if device != "win" {
			t.Fatalf("device %s", device)
		}
		got = text
		report(4, 8)
		report(8, 8)
		return nil
	})
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/send/text", "application/json", strings.NewReader(`{"device":"win","text":"https://example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	var started job
	if err := json.NewDecoder(res.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	final := waitJob(t, srv.URL, started.ID)
	if final.Status != "done" || final.Percent != 100 || got != "https://example.com" {
		t.Fatalf("job %+v text %q", final, got)
	}
}

func TestSendFailureIsVisible(t *testing.T) {
	srv := testServer(func(context.Context, string, string, string, func(int64, int64)) error {
		return errors.New("Suraj-Windows is currently unreachable.")
	}, nil)
	defer srv.Close()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	_ = form.WriteField("device", "win")
	part, err := form.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("hello"))
	_ = form.Close()
	res, err := http.Post(srv.URL+"/api/send/file", form.FormDataContentType(), body)
	if err != nil {
		t.Fatal(err)
	}
	var started job
	if err := json.NewDecoder(res.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	final := waitJob(t, srv.URL, started.ID)
	if final.Status != "error" || !strings.Contains(final.Error, "unreachable") {
		t.Fatalf("job %+v", final)
	}
}

func TestCancelStopsTheTransfer(t *testing.T) {
	startedSend := make(chan struct{})
	srv := testServer(nil, func(ctx context.Context, _, _ string, _ func(int64, int64)) error {
		close(startedSend)
		<-ctx.Done()
		return ctx.Err()
	})
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/send/text", "application/json", strings.NewReader(`{"device":"win","text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	var started job
	if err := json.NewDecoder(res.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	select {
	case <-startedSend:
	case <-time.After(2 * time.Second):
		t.Fatal("transfer did not start")
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/jobs/"+started.ID+"/cancel", nil)
	cres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cres.Body.Close()
	final := waitJob(t, srv.URL, started.ID)
	if final.Status != "cancelled" {
		t.Fatalf("job %+v", final)
	}
}

func TestMissingDeviceAndEmptyClipboard(t *testing.T) {
	srv := testServer(nil, nil)
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/send/text", "application/json", strings.NewReader(`{"device":"","text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
	res.Body.Close()
	res, err = http.Post(srv.URL+"/api/send/clipboard", "application/json", strings.NewReader(`{"device":"win"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]string
	_ = json.NewDecoder(res.Body).Decode(&body)
	if !strings.Contains(body["error"], "clipboard is empty") {
		t.Fatalf("clipboard error: %v", body)
	}
}

func waitJob(t *testing.T, base, id string) job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/api/jobs/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var job job
		err = json.NewDecoder(res.Body).Decode(&job)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != "running" {
			return job
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return job{}
}

func TestGestureReleaseStartsOneTransfer(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/note.txt"
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	s := &server{opt: Options{
		SendFile: func(context.Context, string, string, string, func(int64, int64)) error {
			calls.Add(1)
			once.Do(func() { close(started) })
			<-release
			return nil
		},
	}, jobs: map[string]*job{}, hand: NewMachine(), tray: []TrayItem{{
		ID: "note", Name: "note.txt", Kind: "file", Path: path,
	}}}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	post := func(frame Frame) View {
		t.Helper()
		raw, _ := json.Marshal(frame)
		res, err := http.Post(srv.URL+"/api/gesture", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var view View
		if err := json.NewDecoder(res.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	for i := 0; i <= 40; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/40
		post(Frame{
			Present: true, Confidence: 1, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.14*math.Cos(ang), Y: 0.45 + 0.14*math.Sin(ang),
		})
	}
	for i := 0; i < pinchHold; i++ {
		post(Frame{Present: true, Confidence: 1, X: 0.8, Y: 0.3, Pinch: 0.02, OverFile: -1, OverDevice: 0})
	}
	for i := 0; i < pinchHold; i++ {
		post(Frame{Present: true, Confidence: 1, X: 0.8, Y: 0.3, Pinch: 0.2, OverFile: -1, OverDevice: -1})
	}
	for i := 0; i < pinchHold; i++ {
		post(Frame{Present: true, Confidence: 1, X: 0.1, Y: 0.2, Pinch: 0.02, OverFile: 0, OverDevice: -1})
	}
	var view View
	for i := 0; i < pinchHold; i++ {
		view = post(Frame{Present: true, Confidence: 1, X: 0.5, Y: 0.4, Pinch: 0.2, OverFile: -1, OverDevice: -1, OverPortal: true, ToPortalX: 0.1, ToPortalY: 0.1})
		if view.Send {
			break
		}
	}
	if !view.Send || view.Destination != 0 || view.Held != 0 {
		t.Fatalf("gesture did not send: %+v", view)
	}

	res, err := http.Post(srv.URL+"/api/send/tray", "application/json", strings.NewReader(`{"device":"win","item":"note"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	res.Body.Close()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("SendFile was not called")
	}
	again, err := http.Post(srv.URL+"/api/send/tray", "application/json", strings.NewReader(`{"device":"win","item":"note"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer again.Body.Close()
	var errBody map[string]string
	json.NewDecoder(again.Body).Decode(&errBody)
	if again.StatusCode == http.StatusOK || errBody["error"] != "a transfer is already running" {
		t.Fatalf("duplicate: %d %v", again.StatusCode, errBody)
	}
	close(release)
	if calls.Load() != 1 {
		t.Fatalf("SendFile calls %d", calls.Load())
	}
}
