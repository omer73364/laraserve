//go:build linux

package metrics

import (
	"os"
	"testing"
	"time"
)

func TestLiveMeasurementAndGenerationReset(t *testing.T) {
	s := New()
	first := s.Measure("id", "generation-one", os.Getpid())
	if !first.Available || first.RSS == 0 || first.Processes < 1 || first.CPU != nil {
		t.Fatal(first)
	}
	time.Sleep(20 * time.Millisecond)
	next := s.Measure("id", "generation-one", os.Getpid())
	if !next.Available || next.CPU == nil || *next.CPU < 0 {
		t.Fatal(next)
	}
	reset := s.Measure("id", "generation-two", os.Getpid())
	if !reset.Available || reset.CPU != nil {
		t.Fatal(reset)
	}
	unavailable := s.Measure("missing", "none", -1)
	if unavailable.Available {
		t.Fatal(unavailable)
	}
}
func TestProcNamesMayContainSpacesAndParentheses(t *testing.T) {
	p, err := readProc(os.Getpid())
	if err != nil || p.pid != os.Getpid() || p.start == 0 {
		t.Fatal(p, err)
	}
	if _, err = parseProc(1, "1 (broken) S 1"); err == nil {
		t.Fatal("short stat accepted")
	}
}
