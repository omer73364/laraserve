//go:build !linux

package metrics

func (s *Sampler) Measure(id, generation string, pid int) Sample {
	return Sample{Note: "Resource measurement is currently available on Linux"}
}
