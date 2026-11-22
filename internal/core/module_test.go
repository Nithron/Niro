package core

import "testing"

func TestNewRegistry(t *testing.T) {
	r, err := NewRegistry(DefaultModules()...)
	if err != nil {
		t.Fatalf("default modules: %v", err)
	}
	if got := len(r.All()); got != 7 {
		t.Fatalf("len(All()) = %d, want 7", got)
	}
	if m, ok := r.Get(ModuleCore); !ok || !m.AlwaysOn {
		t.Fatalf("core module must be registered and always on")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatalf("unknown module reported as registered")
	}

	if _, err := NewRegistry(Module{ID: "a"}, Module{ID: "a"}); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	if _, err := NewRegistry(Module{ID: ""}); err == nil {
		t.Fatal("empty ID accepted")
	}
	if _, err := NewRegistry(Module{ID: "x", AlwaysOn: true}); err == nil {
		t.Fatal("always-on module that is disabled by default accepted")
	}
}

func TestRegistryAllIsACopy(t *testing.T) {
	r, _ := NewRegistry(DefaultModules()...)
	all := r.All()
	all[0].ID = "mutated"
	if r.All()[0].ID != ModuleCore {
		t.Fatal("All() exposed internal slice")
	}
}
