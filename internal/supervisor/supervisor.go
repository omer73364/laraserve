// Package supervisor owns a project's lifetime and authenticated local controls.
// OS locks, rather than recorded PIDs, determine whether a generation is live.
package supervisor

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"laraserve/internal/filelock"
	"laraserve/internal/projects"
)

type State struct {
	ID         string `json:"id"`
	Generation string `json:"generation"`
	PID        int    `json:"pid"`
	Status     string `json:"status"`
	URL        string `json:"url,omitempty"`
	Updated    string `json:"updated_utc"`
	Address    string `json:"control_address,omitempty"`
	Token      string `json:"control_token,omitempty"`
}
type Session struct {
	mu      sync.Mutex
	state   State
	file    string
	ctx     context.Context
	cancel  context.CancelFunc
	server  *http.Server
	release func()
	once    sync.Once
}

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func directory(home string) string     { return filepath.Join(home, "control") }
func stateFile(home, id string) string { return filepath.Join(directory(home), id+".json") }
func lockFile(home, id string) string  { return filepath.Join(directory(home), id+".lock") }

var validID = regexp.MustCompile(`^[a-f0-9]{12}$`)

func Begin(parent context.Context, home, id, generation string) (*Session, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("invalid project ID")
	}
	if err := os.MkdirAll(directory(home), 0700); err != nil {
		return nil, err
	}
	release, ok, err := filelock.Try(lockFile(home, id))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("project %s already has a running or starting owner; use status or stop", id)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		release()
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	if generation == "" {
		generation = NewID()
	}
	s := &Session{file: stateFile(home, id), ctx: ctx, cancel: cancel, release: release, state: State{ID: id, Generation: generation, PID: os.Getpid(), Status: "starting", Address: listener.Addr().String(), Token: NewID() + NewID()}}
	s.server = &http.Server{Handler: http.HandlerFunc(s.handle), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 3 * time.Second}
	if err = s.update("starting", ""); err != nil {
		listener.Close()
		release()
		cancel()
		return nil, err
	}
	go s.server.Serve(listener)
	return s, nil
}
func (s *Session) Context() context.Context { return s.ctx }
func (s *Session) Ready(url string) error   { return s.update("running", url) }
func (s *Session) update(status, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Status = status
	if url != "" {
		s.state.URL = url
	}
	s.state.Updated = time.Now().UTC().Format(time.RFC3339Nano)
	return projects.WriteJSON(s.file, &s.state)
}
func (s *Session) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	if r.Host != state.Address || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+state.Token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method == "GET" && r.URL.Path == "/status":
		state.Token = ""
		state.Address = ""
		json.NewEncoder(w).Encode(state)
	case r.Method == "POST" && r.URL.Path == "/stop":
		if err := s.update("stopping", ""); err != nil {
			http.Error(w, "cannot record shutdown", 500)
			return
		}
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusAccepted)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		s.cancel()
	default:
		http.NotFound(w, r)
	}
}
func (s *Session) Close(runErr error) {
	s.once.Do(func() {
		s.cancel()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := s.server.Shutdown(shutdown); err != nil {
			_ = s.server.Close()
		}
		cancel()
		status := "stopped"
		if runErr != nil {
			status = "failed"
		}
		_ = s.update(status, "")
		s.release()
	})
}

func read(home, id string) (State, error) {
	var state State
	if !validID.MatchString(id) {
		return state, fmt.Errorf("invalid project ID")
	}
	b, err := os.ReadFile(stateFile(home, id))
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	if state.ID != id {
		return state, fmt.Errorf("control record identity mismatch")
	}
	host, _, err := net.SplitHostPort(state.Address)
	if err != nil || host != "127.0.0.1" {
		return state, fmt.Errorf("invalid local control endpoint")
	}
	_, tokenError := hex.DecodeString(state.Token)
	if len(state.Token) != 64 || tokenError != nil {
		return state, fmt.Errorf("invalid local control credential")
	}
	return state, nil
}

var client = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("control redirects are forbidden") }}

func request(ctx context.Context, state State, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+state.Address+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+state.Token)
	return client.Do(req)
}
func Status(home, id string) (State, error) {
	state, err := read(home, id)
	if os.IsNotExist(err) {
		return State{ID: id, Status: "stopped"}, nil
	}
	if err != nil {
		return state, err
	}
	release, available, lockErr := filelock.Try(lockFile(home, id))
	if lockErr != nil {
		return state, lockErr
	}
	if available {
		release()
		if state.Status == "starting" || state.Status == "running" || state.Status == "stopping" {
			state.Status = "crashed"
		}
		state.Token = ""
		state.Address = ""
		return state, nil
	}
	response, err := request(context.Background(), state, "GET", "/status")
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return State{}, fmt.Errorf("project control rejected authentication (HTTP %d)", response.StatusCode)
		}
		var live State
		if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&live); err != nil {
			return State{}, err
		}
		if live.ID != id || live.Generation != state.Generation {
			return State{}, fmt.Errorf("control generation mismatch")
		}
		return live, nil
	}
	state.Status = "unresponsive"
	state.Token = ""
	state.Address = ""
	return state, nil
}
func Stop(ctx context.Context, home, id string) error {
	state, err := read(home, id)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	live, err := Status(home, id)
	if err != nil {
		return err
	}
	if live.Status == "stopped" || live.Status == "failed" || live.Status == "crashed" {
		return nil
	}
	response, err := request(ctx, state, "POST", "/stop")
	if err != nil {
		return fmt.Errorf("cannot reach this project's owner; no PID was killed: %w", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("stop rejected: HTTP %d", response.StatusCode)
	}
	for {
		if current, e := read(home, id); e == nil && current.Generation != state.Generation {
			return nil
		}
		release, ok, err := filelock.Try(lockFile(home, id))
		if err != nil {
			return err
		}
		if ok {
			release()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("project is still stopping: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// States includes owners that are still preparing their project registration.
func States(home string) ([]State, error) {
	entries, err := os.ReadDir(directory(home))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var states []State
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !validID.MatchString(id) {
			continue
		}
		state, err := Status(home, id)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}
