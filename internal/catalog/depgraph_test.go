package catalog

import (
	"testing"
)

func TestResolveDeps_NoDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "ollama"},
		{Name: "qdrant"},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"ollama"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "ollama" {
		t.Errorf("resolved = %v, want [ollama]", resolved)
	}
	if len(autoAdded) != 0 {
		t.Errorf("autoAdded = %v, want empty", autoAdded)
	}
}

func TestResolveDeps_DirectDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "prometheus-stack"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"postgresql"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("resolved = %v, want 3 entries", resolved)
	}
	if resolved[len(resolved)-1] != "postgresql" {
		t.Errorf("last resolved = %q, want postgresql", resolved[len(resolved)-1])
	}
	if len(autoAdded) != 2 {
		t.Errorf("autoAdded = %v, want 2 entries", autoAdded)
	}
}

func TestResolveDeps_TransitiveDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "prometheus-stack"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
		{Name: "langfuse", DependsOn: []string{"cnpg-operator", "postgresql"}},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"langfuse"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 4 {
		t.Fatalf("resolved = %v, want 4 entries", resolved)
	}
	if resolved[len(resolved)-1] != "langfuse" {
		t.Errorf("last resolved = %q, want langfuse", resolved[len(resolved)-1])
	}
	if len(autoAdded) != 3 {
		t.Errorf("autoAdded = %v, want 3 entries", autoAdded)
	}
}

func TestResolveDeps_AlreadyRequested(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator"}},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"cnpg-operator", "postgresql"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %v, want 2 entries", resolved)
	}
	if len(autoAdded) != 0 {
		t.Errorf("autoAdded = %v, want empty (both were requested)", autoAdded)
	}
}

func TestResolveDeps_CycleDetection(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}
	_, _, err := ResolveDeps([]string{"a"}, all)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestResolveDeps_UnknownDep(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "postgresql", DependsOn: []string{"nonexistent"}},
	}
	_, _, err := ResolveDeps([]string{"postgresql"}, all)
	if err == nil {
		t.Fatal("expected error for unknown dep, got nil")
	}
}

func TestDependents_Direct(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator", Enabled: true},
		{Name: "postgresql", Enabled: true, DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
		{Name: "langfuse", Enabled: true, DependsOn: []string{"cnpg-operator", "postgresql"}},
		{Name: "ollama", Enabled: true},
	}
	deps := Dependents("cnpg-operator", all)
	if len(deps) != 2 {
		t.Fatalf("Dependents = %v, want 2 entries (postgresql, langfuse)", deps)
	}
}

func TestDependents_OnlyEnabled(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator", Enabled: true},
		{Name: "postgresql", Enabled: false, DependsOn: []string{"cnpg-operator"}},
	}
	deps := Dependents("cnpg-operator", all)
	if len(deps) != 0 {
		t.Errorf("Dependents = %v, want empty (postgresql is disabled)", deps)
	}
}

func TestDependents_NoDependents(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "ollama", Enabled: true},
		{Name: "qdrant", Enabled: true},
	}
	deps := Dependents("ollama", all)
	if len(deps) != 0 {
		t.Errorf("Dependents = %v, want empty", deps)
	}
}
