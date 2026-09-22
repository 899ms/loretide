package diagnostics

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func factAt(component configComponent, state configState, at time.Time) componentConfigFact {
	return componentConfigFact{Component: component, State: state, ObservedAt: at, Version: "test"}
}

func bootSnapshot(generation uint64, at time.Time) sourceSnapshot {
	return sourceSnapshot{
		Source:     sourceServerBoot,
		Generation: generation,
		Components: []componentConfigFact{
			factAt(componentDatabase, configConfigured, at),
			factAt(componentAPI, configConfigured, at),
		},
	}
}

func storageSnapshot(generation uint64, state configState, at time.Time) sourceSnapshot {
	return sourceSnapshot{
		Source:     sourceRouterStorage,
		Generation: generation,
		Components: []componentConfigFact{factAt(componentFiles, state, at)},
	}
}

func TestComponentFactRegistryRejectsInvalidDomains(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		snapshot sourceSnapshot
	}{
		{"unknown source", sourceSnapshot{Source: "other", Generation: 1}},
		{"zero generation", sourceSnapshot{Source: sourceRouterStorage, Components: []componentConfigFact{factAt(componentFiles, configConfigured, now)}}},
		{"missing boot component", sourceSnapshot{Source: sourceServerBoot, Generation: 1, Components: []componentConfigFact{factAt(componentAPI, configConfigured, now)}}},
		{"duplicate component", sourceSnapshot{Source: sourceServerBoot, Generation: 1, Components: []componentConfigFact{factAt(componentAPI, configConfigured, now), factAt(componentAPI, configConfigured, now)}}},
		{"wrong component", sourceSnapshot{Source: sourceRouterStorage, Generation: 1, Components: []componentConfigFact{factAt(componentSearch, configConfigured, now)}}},
		{"policy must have execution", sourceSnapshot{Source: sourceExecutionPolicy, Generation: 1}},
		{"policy cannot have component", sourceSnapshot{Source: sourceExecutionPolicy, Generation: 1, Components: []componentConfigFact{factAt(componentExecutor, configUnknown, now)}, Execution: &executionFact{Component: componentExecutor, State: executionDisabled, ObservedAt: now}}},
		{"policy cannot enable", sourceSnapshot{Source: sourceExecutionPolicy, Generation: 1, Execution: &executionFact{Component: componentExecutor, State: executionState("enabled"), ObservedAt: now}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := newComponentFactRegistry().registerSourceSnapshot(tt.snapshot); !errors.Is(err, errInvalidSourceSnapshot) {
				t.Fatalf("registerSourceSnapshot() error = %v, want invalid snapshot", err)
			}
		})
	}
}

func TestComponentFactRegistryAcceptsDeclaredDomains(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		source    configSource
		component configComponent
	}{
		{"server boot", sourceServerBoot, ""},
		{"router storage", sourceRouterStorage, componentFiles},
		{"web capability", sourceWebCapability, componentWeb},
		{"runtime registry", sourceRuntimeRegistry, componentDaemon},
		{"executor host", sourceExecutorHost, componentExecutor},
		{"search host", sourceSearchHost, componentSearch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newComponentFactRegistry()
			var snapshot sourceSnapshot
			if tt.source == sourceServerBoot {
				snapshot = bootSnapshot(1, now)
			} else {
				snapshot = sourceSnapshot{Source: tt.source, Generation: 1, Components: []componentConfigFact{factAt(tt.component, configUnknown, now)}}
			}
			if err := registry.registerSourceSnapshot(snapshot); err != nil {
				t.Fatalf("registerSourceSnapshot() error = %v", err)
			}
		})
	}

	policy := sourceSnapshot{Source: sourceExecutionPolicy, Generation: 1, Execution: &executionFact{Component: componentExecutor, State: executionDisabled, ObservedAt: now}}
	if err := newComponentFactRegistry().registerSourceSnapshot(policy); err != nil {
		t.Fatalf("execution policy snapshot error = %v", err)
	}
}

func TestComponentFactRegistryKeepsSourcePartitionsAndGenerations(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	registry := newComponentFactRegistry()
	if err := registry.registerSourceSnapshot(bootSnapshot(7, now)); err != nil {
		t.Fatal(err)
	}
	if err := registry.registerSourceSnapshot(storageSnapshot(3, configUnconfigured, now)); err != nil {
		t.Fatal(err)
	}
	if err := registry.registerSourceSnapshot(storageSnapshot(4, configConfigured, now)); err != nil {
		t.Fatal(err)
	}

	boot, ok := registry.sourceSnapshot(sourceServerBoot)
	if !ok || boot.Generation != 7 || len(boot.Components) != 2 {
		t.Fatalf("boot partition = %#v, ok=%t; router update must not erase it", boot, ok)
	}
	storage, ok := registry.sourceSnapshot(sourceRouterStorage)
	if !ok || storage.Generation != 4 || storage.Components[0].State != configConfigured {
		t.Fatalf("storage partition = %#v, ok=%t", storage, ok)
	}
	if err := registry.registerSourceSnapshot(storageSnapshot(2, configConfigured, now)); !errors.Is(err, errStaleSourceSnapshot) {
		t.Fatalf("stale generation error = %v", err)
	}
	if err := registry.registerSourceSnapshot(storageSnapshot(4, configConfigured, now)); err != nil {
		t.Fatalf("identical generation must be idempotent: %v", err)
	}
	if err := registry.registerSourceSnapshot(storageSnapshot(4, configUnconfigured, now)); !errors.Is(err, errConflictingGeneration) {
		t.Fatalf("different equal-generation snapshot error = %v", err)
	}
}

func TestComponentFactRegistryNormalizesPolicyAndBootSnapshots(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	registry := newComponentFactRegistry()
	policy := sourceSnapshot{Source: sourceExecutionPolicy, Generation: 1, Components: []componentConfigFact{}, Execution: &executionFact{Component: componentExecutor, State: executionDisabled, ObservedAt: now.In(time.FixedZone("+01", 3600))}}
	if err := registry.registerSourceSnapshot(policy); err != nil {
		t.Fatal(err)
	}
	policy.Components = nil
	policy.Execution.ObservedAt = now
	if err := registry.registerSourceSnapshot(policy); err != nil {
		t.Fatalf("normalized policy replay must be idempotent: %v", err)
	}

	boot := bootSnapshot(2, now.In(time.FixedZone("+01", 3600)))
	if err := registry.registerSourceSnapshot(boot); err != nil {
		t.Fatal(err)
	}
	boot.Components[0], boot.Components[1] = boot.Components[1], boot.Components[0]
	boot.Components[0].ObservedAt = now
	boot.Components[1].ObservedAt = now
	if err := registry.registerSourceSnapshot(boot); err != nil {
		t.Fatalf("ordered UTC-equivalent boot replay must be idempotent: %v", err)
	}
}

func TestComponentFactRegistryCopiesInputAndOutput(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	registry := newComponentFactRegistry()
	input := storageSnapshot(1, configConfigured, now)
	if err := registry.registerSourceSnapshot(input); err != nil {
		t.Fatal(err)
	}
	input.Components[0].State = configUnconfigured

	first, ok := registry.sourceSnapshot(sourceRouterStorage)
	if !ok || first.Components[0].State != configConfigured {
		t.Fatalf("input mutation leaked into registry: %#v", first)
	}
	first.Components[0].State = configUnconfigured
	second, _ := registry.sourceSnapshot(sourceRouterStorage)
	if second.Components[0].State != configConfigured {
		t.Fatalf("output mutation leaked into registry: %#v", second)
	}

	all := registry.snapshotsCopy()
	all[sourceRouterStorage] = storageSnapshot(9, configUnconfigured, now)
	stored, _ := registry.sourceSnapshot(sourceRouterStorage)
	if stored.Generation != 1 {
		t.Fatalf("map mutation leaked into registry: %#v", stored)
	}
}

func TestReduceComponentStatus(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 40, 0, time.UTC)
	configured := factAt(componentFiles, configConfigured, now)
	unconfigured := factAt(componentFiles, configUnconfigured, now)
	tests := []struct {
		name      string
		component configComponent
		config    *componentConfigFact
		liveness  *livenessFact
		execution *executionFact
		status    string
		health    healthState
		reason    string
	}{
		{"missing fact", componentFiles, nil, nil, nil, "unknown", healthUnverified, ""},
		{"configured unverified", componentFiles, &configured, nil, nil, "unverified", healthUnverified, ""},
		{"configured fresh", componentFiles, &configured, &livenessFact{LastSeen: now.Add(-componentHeartbeatTTL), Version: "browser"}, nil, "healthy", healthHealthy, ""},
		{"configured expired", componentFiles, &configured, &livenessFact{LastSeen: now.Add(-componentHeartbeatTTL - time.Nanosecond)}, nil, "unavailable", healthUnavailable, "heartbeat_expired"},
		{"unconfigured heartbeat conflict", componentFiles, &unconfigured, &livenessFact{LastSeen: now}, nil, "unconfigured", healthHealthy, "heartbeat_without_configuration"},
		{"unknown config keeps evidence", componentFiles, &componentConfigFact{Component: componentFiles, State: configUnknown, ObservedAt: now}, &livenessFact{LastSeen: now}, nil, "unknown", healthHealthy, ""},
		{"clock skew", componentFiles, &configured, &livenessFact{LastSeen: now.Add(maximumClockSkew + time.Nanosecond)}, nil, "unknown", healthUnknown, "clock_skew"},
		{"policy disabled", componentExecutor, nil, &livenessFact{LastSeen: now}, &executionFact{Component: componentExecutor, State: executionDisabled, ObservedAt: now}, "disabled", healthHealthy, "execution_disabled"},
		{"executor policy cannot disable files", componentFiles, &configured, &livenessFact{LastSeen: now}, &executionFact{Component: componentExecutor, State: executionDisabled, ObservedAt: now}, "healthy", healthHealthy, ""},
		{"mismatched config cannot promote", componentFiles, &componentConfigFact{Component: componentAPI, State: configConfigured, ObservedAt: now}, &livenessFact{LastSeen: now}, nil, "unknown", healthHealthy, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reduceComponentStatus(tt.component, tt.config, tt.liveness, tt.execution, now)
			if got.Status != tt.status || got.Health != tt.health || got.Reason != tt.reason {
				t.Fatalf("assessment = %#v, want status=%q health=%q reason=%q", got, tt.status, tt.health, tt.reason)
			}
		})
	}
}

func TestComponentFactRegistryConcurrentSnapshots(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	registry := newComponentFactRegistry()
	if err := registry.registerSourceSnapshot(bootSnapshot(1, now)); err != nil {
		t.Fatal(err)
	}
	const rounds = 64
	var writers sync.WaitGroup
	var readers sync.WaitGroup
	for i := 1; i <= rounds; i++ {
		generation := i
		writers.Go(func() {
			_ = registry.registerSourceSnapshot(storageSnapshot(uint64(generation), configConfigured, now))
		})
	}
	for range rounds {
		readers.Go(func() {
			for range rounds {
				snapshots := registry.snapshotsCopy()
				if boot, ok := snapshots[sourceServerBoot]; !ok || len(boot.Components) != 2 {
					t.Errorf("boot partition lost during concurrent update: %#v", boot)
					return
				}
			}
		})
	}
	writers.Wait()
	readers.Wait()
	if storage, ok := registry.sourceSnapshot(sourceRouterStorage); !ok || storage.Generation != rounds {
		t.Fatalf("storage partition missing after concurrent updates: %#v", storage)
	}
}

func TestNewServiceBuildsFactRegistryWithoutStore(t *testing.T) {
	service := NewService(nil, "test", false)
	if service.facts == nil {
		t.Fatal("NewService must initialize the in-memory fact registry")
	}
}
