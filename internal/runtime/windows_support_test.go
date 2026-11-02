package runtime

import "testing"

func TestWindowsPHPCatalog(t *testing.T) {
	for _, version := range []string{"7.4.33", "8.0.30", "8.1.34", "8.2.35", "8.3.36", "8.4.27", "8.5.12"} {
		source, err := windowsPHPSource(version)
		if err != nil {
			t.Fatal(err)
		}
		if source.SHA256 != "publisher" {
			t.Fatal("compiled checksum", source)
		}
	}
}
