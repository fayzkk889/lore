package display

import (
	"strings"
	"testing"
)

func TestTokenMeterDisplayLabelsProviderBilledUsage(t *testing.T) {
	var m TokenMeter
	m.Add(1721, 30)

	out := m.Display()
	for _, want := range []string{"turn billed:", "1,751", "session billed:", "1,751", "in 1,721 / out 30"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Display() missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tokens:") || strings.Contains(out, "session:") {
		t.Fatalf("Display() used ambiguous token labels:\n%s", out)
	}
}
