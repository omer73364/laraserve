//go:build linux

package metrics

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type proc struct {
	pid, ppid         int
	start, ticks, rss uint64
}

func readProc(pid int) (proc, error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return proc{}, err
	}
	return parseProc(pid, string(b))
}
func parseProc(pid int, raw string) (proc, error) {
	i := strings.LastIndex(raw, ")")
	if i < 0 {
		return proc{}, strconv.ErrSyntax
	}
	f := strings.Fields(raw[i+1:])
	if len(f) < 22 {
		return proc{}, strconv.ErrSyntax
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return proc{}, err
	}
	numbers := make([]uint64, 4)
	for i, n := range []int{11, 12, 19, 21} {
		v, e := strconv.ParseUint(f[n], 10, 64)
		if e != nil {
			return proc{}, e
		}
		numbers[i] = v
	}
	return proc{pid: pid, ppid: ppid, ticks: numbers[0] + numbers[1], start: numbers[2], rss: numbers[3] * uint64(os.Getpagesize())}, nil
}
func systemTicks() (uint64, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(f) < 9 || f[0] != "cpu" {
		return 0, strconv.ErrSyntax
	}
	var ticks uint64
	// Guest ticks are already included in user/nice and must not be counted twice.
	for _, raw := range f[1:9] {
		v, e := strconv.ParseUint(raw, 10, 64)
		if e != nil {
			return 0, e
		}
		ticks += v
	}
	return ticks, nil
}
func (s *Sampler) Measure(id, generation string, pid int) Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := readProc(pid)
	if err != nil {
		return Sample{Note: "Process counters unavailable"}
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return Sample{Note: "Process counters unavailable"}
	}
	all := map[int]proc{pid: root}
	for _, entry := range entries {
		n, e := strconv.Atoi(entry.Name())
		if e != nil {
			continue
		}
		p, e := readProc(n)
		if e == nil {
			all[n] = p
		}
	}
	owned := map[int]bool{pid: true}
	for changed := true; changed; {
		changed = false
		for n, p := range all {
			if !owned[n] && owned[p.ppid] {
				owned[n] = true
				changed = true
			}
		}
	}
	var ticks, rss uint64
	for n := range owned {
		p := all[n]
		ticks += p.ticks
		rss += p.rss
	}
	system, err := systemTicks()
	if err != nil {
		return Sample{Note: "CPU counters unavailable"}
	}
	check, err := readProc(pid)
	if err != nil || check.start != root.start {
		return Sample{Note: "Process changed while sampling"}
	}
	result := Sample{Available: true, RSS: rss, Processes: len(owned), Note: "Owned process tree RSS; shared pages may be counted more than once"}
	previous, ok := s.previous[id]
	if ok && previous.generation == generation && previous.start == root.start && system > previous.system && ticks >= previous.ticks {
		cpu := 100 * float64(runtime.NumCPU()) * float64(ticks-previous.ticks) / float64(system-previous.system)
		result.CPU = &cpu
	}
	s.previous[id] = reading{generation: generation, start: root.start, ticks: ticks, system: system}
	return result
}
