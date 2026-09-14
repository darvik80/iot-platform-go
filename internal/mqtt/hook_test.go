package mqtt

import (
	"testing"
)

func TestExtPluginSlugAndID(t *testing.T) {
	cases := []struct {
		user    string
		wantSlug string
		wantID   string
		wantOK   bool
	}{
		{"extplg_def_py-ai-ctrl-center", "def", "py-ai-ctrl-center", true},
		{"extplg_acme_temp-alert", "acme", "temp-alert", true},
		{"extplg_def_simple", "def", "simple", true},
		{"extplg_under_score_id", "under", "score_id", true},
		{"extplg__nope", "", "", false},     // empty slug
		{"extplg_def_", "", "", false},      // empty id
		{"nope_def_x", "", "", false},       // missing prefix
		{"extplg_def", "", "", false},       // no separator
		{"", "", "", false},                 // empty
	}
	for _, c := range cases {
		gotSlug := extPluginSlug(c.user)
		gotID, gotOK := extPluginID(c.user)
		if gotSlug != c.wantSlug {
			t.Errorf("extPluginSlug(%q)=%q want %q", c.user, gotSlug, c.wantSlug)
		}
		if gotID != c.wantID || gotOK != c.wantOK {
			t.Errorf("extPluginID(%q)=(%q,%v) want (%q,%v)", c.user, gotID, gotOK, c.wantID, c.wantOK)
		}
	}
}
