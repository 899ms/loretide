package diagnostics

import (
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"
)

type configComponent string

const (
	componentAPI      configComponent = "api"
	componentDatabase configComponent = "database"
	componentWeb      configComponent = "web"
	componentFiles    configComponent = "files"
	componentDaemon   configComponent = "daemon"
	componentExecutor configComponent = "executor"
	componentSearch   configComponent = "search"
)

type configSource string

const (
	sourceServerBoot      configSource = "server_boot"
	sourceRouterStorage   configSource = "router_storage"
	sourceWebCapability   configSource = "web_capability"
	sourceRuntimeRegistry configSource = "runtime_registry"
	sourceExecutionPolicy configSource = "execution_policy"
	sourceExecutorHost    configSource = "executor_host"
	sourceSearchHost      configSource = "search_host"
)

type configState string

const (
	configConfigured   configState = "configured"
	configUnconfigured configState = "unconfigured"
	configUnknown      configState = "unknown"
)

type healthState string

const (
	healthHealthy     healthState = "healthy"
	healthUnverified  healthState = "unverified"
	healthUnavailable healthState = "unavailable"
	healthUnknown     healthState = "unknown"
)

type executionState string

const (
	executionDisabled executionState = "disabled"
	executionUnknown  executionState = "unknown"
)

const (
	componentHeartbeatTTL = 30 * time.Second
	maximumClockSkew      = 5 * time.Second
)

var (
	errInvalidSourceSnapshot = errors.New("invalid component fact source snapshot")
	errStaleSourceSnapshot   = errors.New("stale component fact source snapshot")
	errConflictingGeneration = errors.New("conflicting component fact generation")
)

type componentConfigFact struct {
	Component  configComponent
	State      configState
	ObservedAt time.Time
	Version    string
	Reason     string
}

type livenessFact struct {
	State    healthState
	LastSeen time.Time
	Version  string
	Reason   string
}

type executionFact struct {
	Component  configComponent
	State      executionState
	ObservedAt time.Time
	Reason     string
}

type sourceSnapshot struct {
	Source     configSource
	Generation uint64
	Components []componentConfigFact
	Execution  *executionFact
}

type componentAssessment struct {
	Config    configState
	Health    healthState
	Execution executionState
	Status    string
	Reason    string
	LastSeen  *time.Time
}

type componentFactRegistry struct {
	mu        sync.RWMutex
	snapshots map[configSource]sourceSnapshot
}

func newComponentFactRegistry() *componentFactRegistry {
	return &componentFactRegistry{snapshots: make(map[configSource]sourceSnapshot)}
}

func (r *componentFactRegistry) registerSourceSnapshot(input sourceSnapshot) error {
	snapshot, err := normalizeSourceSnapshot(input)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, ok := r.snapshots[snapshot.Source]; ok {
		switch {
		case snapshot.Generation < previous.Generation:
			return errStaleSourceSnapshot
		case snapshot.Generation == previous.Generation:
			if reflect.DeepEqual(snapshot, previous) {
				return nil
			}
			return errConflictingGeneration
		}
	}
	r.snapshots[snapshot.Source] = cloneSourceSnapshot(snapshot)
	return nil
}

func (r *componentFactRegistry) sourceSnapshot(source configSource) (sourceSnapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot, ok := r.snapshots[source]
	return cloneSourceSnapshot(snapshot), ok
}

func (r *componentFactRegistry) snapshotsCopy() map[configSource]sourceSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[configSource]sourceSnapshot, len(r.snapshots))
	for source, snapshot := range r.snapshots {
		result[source] = cloneSourceSnapshot(snapshot)
	}
	return result
}

func normalizeSourceSnapshot(input sourceSnapshot) (sourceSnapshot, error) {
	domain, executionRequired, ok := sourceDomain(input.Source)
	if !ok || input.Generation == 0 || len(input.Components) != len(domain) {
		return sourceSnapshot{}, errInvalidSourceSnapshot
	}
	if executionRequired != (input.Execution != nil) {
		return sourceSnapshot{}, errInvalidSourceSnapshot
	}

	components := make([]componentConfigFact, len(input.Components))
	copy(components, input.Components)
	seen := make(map[configComponent]bool, len(components))
	for index := range components {
		fact := &components[index]
		if !containsComponent(domain, fact.Component) || seen[fact.Component] || !validConfigState(fact.State) || fact.ObservedAt.IsZero() {
			return sourceSnapshot{}, errInvalidSourceSnapshot
		}
		seen[fact.Component] = true
		fact.ObservedAt = fact.ObservedAt.UTC()
		fact.Version = safeToken(fact.Version)
		fact.Reason = safeToken(fact.Reason)
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Component < components[j].Component })
	if len(components) == 0 {
		components = nil
	}

	result := sourceSnapshot{Source: input.Source, Generation: input.Generation, Components: components}
	if input.Execution != nil {
		execution := *input.Execution
		if execution.Component != componentExecutor || execution.State != executionDisabled || execution.ObservedAt.IsZero() {
			return sourceSnapshot{}, errInvalidSourceSnapshot
		}
		execution.ObservedAt = execution.ObservedAt.UTC()
		execution.Reason = safeToken(execution.Reason)
		result.Execution = &execution
	}
	return result, nil
}

func sourceDomain(source configSource) ([]configComponent, bool, bool) {
	switch source {
	case sourceServerBoot:
		return []configComponent{componentAPI, componentDatabase}, false, true
	case sourceRouterStorage:
		return []configComponent{componentFiles}, false, true
	case sourceWebCapability:
		return []configComponent{componentWeb}, false, true
	case sourceRuntimeRegistry:
		return []configComponent{componentDaemon}, false, true
	case sourceExecutionPolicy:
		return []configComponent{}, true, true
	case sourceExecutorHost:
		return []configComponent{componentExecutor}, false, true
	case sourceSearchHost:
		return []configComponent{componentSearch}, false, true
	default:
		return nil, false, false
	}
}

func containsComponent(domain []configComponent, component configComponent) bool {
	for _, candidate := range domain {
		if candidate == component {
			return true
		}
	}
	return false
}

func validConfigState(state configState) bool {
	return state == configConfigured || state == configUnconfigured || state == configUnknown
}

func cloneSourceSnapshot(input sourceSnapshot) sourceSnapshot {
	result := input
	if len(input.Components) == 0 {
		result.Components = nil
	} else {
		result.Components = append([]componentConfigFact(nil), input.Components...)
	}
	if input.Execution != nil {
		execution := *input.Execution
		result.Execution = &execution
	}
	return result
}

func reduceComponentStatus(component configComponent, config *componentConfigFact, liveness *livenessFact, execution *executionFact, now time.Time) componentAssessment {
	health, reason, lastSeen := reduceHealth(liveness, now)
	assessment := componentAssessment{Config: configUnknown, Health: health, Execution: executionUnknown, Status: "unknown", Reason: reason, LastSeen: lastSeen}
	if config != nil && config.Component == component {
		assessment.Config = config.State
	}
	if component == componentExecutor && execution != nil && execution.Component == component && execution.State == executionDisabled {
		assessment.Execution = executionDisabled
		assessment.Status = "disabled"
		assessment.Reason = "execution_disabled"
		return assessment
	}
	if config == nil || config.Component != component || config.State == configUnknown {
		return assessment
	}
	if config.State == configUnconfigured {
		assessment.Status = "unconfigured"
		if liveness != nil {
			assessment.Reason = "heartbeat_without_configuration"
		}
		return assessment
	}
	switch health {
	case healthHealthy:
		assessment.Status = "healthy"
	case healthUnverified:
		assessment.Status = "unverified"
	case healthUnavailable:
		assessment.Status = "unavailable"
	default:
		assessment.Status = "unknown"
	}
	return assessment
}

func reduceHealth(liveness *livenessFact, now time.Time) (healthState, string, *time.Time) {
	if liveness == nil {
		return healthUnverified, "", nil
	}
	if liveness.LastSeen.IsZero() {
		return healthUnknown, "heartbeat_missing_timestamp", nil
	}
	lastSeen := liveness.LastSeen.UTC()
	if liveness.State == healthUnavailable {
		return healthUnavailable, safeToken(liveness.Reason), &lastSeen
	}
	if lastSeen.After(now.Add(maximumClockSkew)) {
		return healthUnknown, "clock_skew", &lastSeen
	}
	if now.Sub(lastSeen) > componentHeartbeatTTL {
		return healthUnavailable, "heartbeat_expired", &lastSeen
	}
	return healthHealthy, safeToken(liveness.Reason), &lastSeen
}
