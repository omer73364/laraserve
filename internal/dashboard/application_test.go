package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"laraserve/internal/appsettings"
	"laraserve/internal/projects"
)

func TestApplicationAndRemovalEndpoints(t *testing.T) {
	s, p := fixture(t)
	for _, path := range []string{"/api/application", "/api/application/updates", "/api/application/uninstall", "/api/projects/" + p.ID + "/startup", "/api/projects/" + p.ID + "/remove"} {
		if w := call(s, "POST", path, `{}`, false, false, s.origin); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
		if w := call(s, "POST", path, `{}`, true, false, s.origin); w.Code != 403 {
			t.Fatal(path, w.Code)
		}
	}
	if w := call(s, "POST", "/api/application/uninstall", `{"confirm":"UNINSTALL"}`, true, true, s.origin); w.Code != 400 {
		t.Fatal("uninstall did not require data acknowledgement", w.Code)
	}
	if w := call(s, "POST", "/api/projects/"+p.ID+"/remove", `{}`, true, true, s.origin); w.Code != 400 {
		t.Fatal("removal did not require confirmation", w.Code)
	}
	if w := call(s, "POST", "/api/projects/"+p.ID+"/startup", `{"enabled":true}`, true, true, s.origin); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	pref, err := appsettings.Read(s.home)
	if err != nil || !pref.Config.Projects[p.ID] {
		t.Fatal(pref, err)
	}
	w := call(s, "GET", "/api/projects", "", true, false, "")
	var views []View
	if json.Unmarshal(w.Body.Bytes(), &views) != nil || len(views) != 1 || !views[0].Startup {
		t.Fatal(w.Body.String())
	}
	if w = call(s, "POST", "/api/projects/"+p.ID+"/remove", `{"confirm":true}`, true, true, s.origin); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 100; i++ {
		state, _ := projects.New(s.home).Load(p.ID)
		if state.Removed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("registration did not become removed")
}
