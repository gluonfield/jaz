package storage

import "testing"

func TestRuntimeCapabilitiesPersistenceStripsNegotiable(t *testing.T) {
	raw, err := MarshalRuntimeCapabilities(&RuntimeCapabilities{NativeGoalNegotiable: true})
	if err != nil {
		t.Fatal(err)
	}
	if raw != "{}" {
		t.Fatalf("raw = %q, want empty persisted capabilities", raw)
	}
	caps, err := UnmarshalRuntimeCapabilities(`{"native_goal_negotiable":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if caps != nil {
		t.Fatalf("capabilities = %#v, want nil persisted capabilities", caps)
	}
}
