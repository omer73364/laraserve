package supervisor

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestOwnerLockAuthenticationAndGracefulStop(t *testing.T) {
	home := t.TempDir()
	const id = "123456abcdef"
	session, err := Begin(context.Background(), home, id, "generation-one")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(nil)
	if _, err = Begin(context.Background(), home, id, "duplicate"); err == nil {
		t.Fatal("duplicate owner acquired lock")
	}
	private, err := read(home, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ token, host, origin string }{{"wrong", private.Address, ""}, {private.Token, "attacker.test", ""}, {private.Token, private.Address, "https://attacker.test"}} {
		req, _ := http.NewRequest("POST", "http://"+private.Address+"/stop", nil)
		req.Host = test.host
		req.Header.Set("Authorization", "Bearer "+test.token)
		req.Header.Set("Origin", test.origin)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatal("unauthorized stop accepted")
		}
	}
	if session.Context().Err() != nil {
		t.Fatal("unauthorized request canceled owner")
	}
	if err = session.Ready("http://localhost:8080"); err != nil {
		t.Fatal(err)
	}
	state, err := Status(home, id)
	if err != nil || state.Status != "running" || state.Token != "" || state.Address != "" {
		t.Fatal(state, err)
	}
	go func() { <-session.Context().Done(); session.Close(nil) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = Stop(ctx, home, id); err != nil {
		t.Fatal(err)
	}
	next, err := Begin(context.Background(), home, id, "generation-two")
	if err != nil {
		t.Fatal(err)
	}
	next.Close(nil)
}
func TestStaleRecordDoesNotBlockRecovery(t *testing.T) {
	home := t.TempDir()
	const id = "abcdef123456"
	owner, err := Begin(context.Background(), home, id, "old")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an unclean owner exit: OS lock released, stale running metadata.
	owner.server.Close()
	owner.cancel()
	owner.release()
	state, err := Status(home, id)
	if err != nil || state.Status != "crashed" {
		t.Fatal(state, err)
	}
	recovered, err := Begin(context.Background(), home, id, "new")
	if err != nil {
		t.Fatal(err)
	}
	recovered.Close(nil)
}
func TestServicesWaitForReadinessAndCancelSiblings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	readyFirst, allowSecond, failSecond, cleaned := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	first := ServiceFunc{"first", func(ctx context.Context, ready func()) error {
		ready()
		close(readyFirst)
		<-ctx.Done()
		close(cleaned)
		return nil
	}}
	second := ServiceFunc{"second", func(ctx context.Context, ready func()) error {
		select {
		case <-ctx.Done():
			return nil
		case <-allowSecond:
		}
		ready()
		select {
		case <-ctx.Done():
			return nil
		case <-failSecond:
			return context.DeadlineExceeded
		}
	}}
	announced := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- RunServices(ctx, []Service{first, second}, func() { close(announced) }) }()
	<-readyFirst
	select {
	case <-announced:
		t.Fatal("group announced before all services were ready")
	default:
	}
	close(allowSecond)
	select {
	case <-announced:
	case <-ctx.Done():
		t.Fatal("group never became ready")
	}
	close(failSecond)
	if err := <-done; err == nil {
		t.Fatal("service failure ignored")
	}
	<-cleaned
}
