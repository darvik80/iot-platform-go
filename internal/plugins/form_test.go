package plugins

import "testing"

func TestBuiltinForms(t *testing.T) {
	for _, id := range []string{PluginLEDGarland, PluginSchedule} {
		d, _, ok := Lookup(id)
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if len(d.Form) == 0 {
			t.Fatalf("%s: empty form", id)
		}
		if raw := FormJSON(d.Form); len(raw) < 20 {
			t.Fatalf("%s form json=%s", id, raw)
		}
	}
}
