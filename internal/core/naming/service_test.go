package naming

import "testing"

func TestServiceDisplayNameIsIdempotent(t *testing.T) {
	cases := []struct{ service, input, want string }{
		{"youtube", "Node A", "YT_Node A"},
		{"youtube", "YT_Node A", "YT_Node A"},
		{"telegram", "YT_Node A", "TG_Node A"},
		{"discord", "TG_Node A", "DC_Node A"},
		{"unknown", "Node A", "Node A"},
	}
	for _, tc := range cases {
		if got := ServiceDisplayName(tc.service, tc.input); got != tc.want {
			t.Errorf("ServiceDisplayName(%q,%q)=%q, want %q", tc.service, tc.input, got, tc.want)
		}
	}
	if got := OriginalDisplayName("TG_Node A"); got != "Node A" {
		t.Fatalf("original display name=%q", got)
	}
}
