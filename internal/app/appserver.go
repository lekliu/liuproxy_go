package app

import (
	"fmt"
	"github.com/google/uuid"
	"liuproxy_go/internal/core/dispatcher"
	"liuproxy_go/internal/core/gateway"
	"liuproxy_go/internal/core/health"
	"liuproxy_go/internal/service/web"
	"liuproxy_go/internal/shared/config"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/shared/settings"
	"liuproxy_go/internal/tunnel"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"liuproxy_go/internal/shared/types"
)

// AppState 包含 AppServer 的所有动态和半静态状态。
// 它现在只包含一个以 Server ID 为键的 ServerState map。
type AppState struct {
	Servers map[string]*types.ServerState
}

// AppServer is the application's main struct.
type AppServer struct {
	cfg         *types.Config
	iniPath     string
	serversPath string

	settingsManager *settings.SettingsManager

	serversFileLock sync.Mutex // NEW: Lock for servers.json read/write operations

	// configLock 保护 configState 的修改和后台操作
	configLock sync.RWMutex
	// configState is the "A Zone", for background configuration changes
	configState *AppState

	// workLock 保护 workState 指针的读取和交换
	workLock sync.RWMutex
	// workState is the "B Zone", for live traffic dispatching
	workState *AppState

	failureMutex    sync.Mutex
	failureCounters map[string]int

	dispatcher        types.Dispatcher
	gateway           *gateway.Gateway
	healthChecker     *health.Checker
	healthCheckTicker *time.Ticker // NEW

	waitGroup sync.WaitGroup
	stopOnce  sync.Once
}

// New creates a new AppServer instance
func New(cfg *types.Config, iniPath, serversPath string, _ []*types.ServerProfile) *AppServer {
	configDir := filepath.Dir(iniPath)
	settingsPath := filepath.Join(configDir, "settings.json")

	sm, err := settings.NewSettingsManager(settingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize settings manager: %v\n", err)
		os.Exit(1)
	}

	s := &AppServer{
		cfg:             cfg,
		iniPath:         iniPath,
		serversPath:     serversPath,
		settingsManager: sm,
		healthChecker:   health.New(),
		failureCounters: make(map[string]int),
		// 初始化 A 区和 B 区，防止空指针
		configState: &AppState{Servers: make(map[string]*types.ServerState)},
		workState:   &AppState{Servers: make(map[string]*types.ServerState)},
		// 降低健康检查频率，后续将改为配置驱动
		healthCheckTicker: time.NewTicker(30 * time.Second),
	}

	// 创建 Dispatcher，并注入初始配置
	initialSettings := sm.Get()
	disp := dispatcher.New(initialSettings.Gateway, s, s) // Pass AppServer as both StateProvider and FailureReporter

	// 将 Dispatcher 注册为相关模块的订阅者
	sm.Register("gateway", disp)
	sm.Register("routing", disp)

	s.dispatcher = disp
	s.gateway = gateway.New(cfg.LocalConf.UnifiedPort, disp, s)

	// 注意：完整的启动逻辑（加载配置、管理实例、首次重载）将在后续步骤中添加到此处
	// 按照V9方案，此处暂时不执行 s.ReloadStrategy()

	return s
}

// Run is the server's entry point.
func (s *AppServer) Run() {
	logger.Info().Msg("Starting server in 'local' mode...")

	if err := s.loadConfigAndBootstrap(); err != nil {
		logger.Fatal().Err(err).Msg("Server bootstrap failed")
	}

	s.dispatcher.(*dispatcher.Dispatcher).Start()

	s.waitGroup.Add(1)
	go s.healthCheckLoop()

	if s.cfg.LocalConf.UnifiedPort > 0 {
		s.waitGroup.Add(1)
		go func() {
			defer s.waitGroup.Done()
			if err := s.gateway.Start(); err != nil {
				logger.Fatal().Err(err).Msg("Gateway failed to start")
			}
		}()
	} else {
		logger.Warn().Msg("Gateway is disabled.")
	}
	web.StartServer(&s.waitGroup, s.cfg, s.serversPath, s.settingsManager, s)
	s.Wait()
}

// Stop gracefully shuts down the server.
func (s *AppServer) Stop() {
	s.stopOnce.Do(func() {
		logger.Info().Msg("Stopping server...")
		if s.healthCheckTicker != nil {
			s.healthCheckTicker.Stop()
		}
		s.configLock.Lock()
		defer s.configLock.Unlock()

		if s.configState != nil && s.configState.Servers != nil {
			for id, state := range s.configState.Servers {
				if state.Instance != nil {
					logger.Info().Str("server_id", id).Msg("Closing strategy instance.")
					state.Instance.CloseTunnel()
					state.Instance = nil
				}
			}
		}

		if s.gateway != nil {
			s.gateway.Close()
		}
		logger.Info().Msg("All strategies stopped.")
	})
}

// manageInstances is the core instance lifecycle manager.
// It creates, starts, stops, and cleans up strategy instances based on the desired state in configState.
// IMPORTANT: This function must be called under the protection of s.configLock (write lock).
func (s *AppServer) manageInstances() {
	logger.Debug().Msg("[LockTrace] manageInstances: Entered")
	logger.Debug().Msg("[AppServer] Managing instances in A-Zone...")

	// Stop instances that are removed or deactivated
	for _, state := range s.configState.Servers {
		if !state.Profile.Active && state.Instance != nil {
			logger.Info().Str("remarks", state.Profile.Remarks).Msg("Deactivating and closing instance.")
			state.Instance.CloseTunnel()
			state.Instance = nil
		}
	}

	// Start instances that are new or activated
	for _, state := range s.configState.Servers {
		if state.Profile.Active && state.Instance == nil {
			logger.Info().Str("remarks", state.Profile.Remarks).Msg("Activating and creating new strategy instance.")
			newInstance, err := tunnel.NewStrategy(s.cfg, []*types.ServerProfile{state.Profile})
			if err != nil {
				logger.Error().Err(err).Str("remarks", state.Profile.Remarks).Msg("Failed to create strategy")
				state.Profile.Active = false // Mark as inactive on creation failure
				continue
			}
			state.Instance = newInstance

			if err := state.Instance.Initialize(); err != nil {
				logger.Error().Err(err).Str("remarks", state.Profile.Remarks).Msg("Failed to initialize strategy")
				state.Instance.CloseTunnel()
				state.Instance = nil
				state.Profile.Active = false // Mark as inactive on init failure
				continue
			}
			state.Health = types.StatusUp
			logger.Info().Str("remarks", state.Profile.Remarks).Str("listen_addr", state.Instance.GetListenerInfo().Address).Int("port", state.Instance.GetListenerInfo().Port).Msg("Instance initialized successfully.")
		}
	}
	logger.Debug().Msg("[AppServer] Instance management complete.")
	logger.Debug().Msg("[LockTrace] manageInstances: Exiting")
}

// deepCopyAppState performs a deep copy of the AppState, suitable for creating a read-only snapshot.
// Pointers to Profile and Instance are copied by value, which is intentional to avoid re-creating them.
func deepCopyAppState(original *AppState) *AppState {
	if original == nil {
		return nil
	}

	newState := &AppState{
		Servers: make(map[string]*types.ServerState, len(original.Servers)),
	}

	for id, serverState := range original.Servers {
		// Create a copy of the ServerState struct.
		stateCopy := *serverState
		// The Profile and Instance pointers are copied by value, which is what we want.
		// Metrics, however, should be a deep copy to prevent race conditions on metric updates.
		if serverState.Metrics != nil {
			metricsCopy := *serverState.Metrics
			stateCopy.Metrics = &metricsCopy
		}
		stateCopy.Health = serverState.Health
		newState.Servers[id] = &stateCopy
	}

	return newState
}

// ReloadStrategy is the lightweight "publisher" that copies the state from A-Zone (configState) to B-Zone (workState).
// This operation is fast and read-locks A-Zone, then write-locks B-Zone for an atomic swap.
func (s *AppServer) ReloadStrategy() error {
	logger.Debug().Msg("[AppServer] Publishing A-Zone state to B-Zone...")

	s.configLock.RLock()
	newState := deepCopyAppState(s.configState)
	s.configLock.RUnlock()

	s.workLock.Lock()
	s.workState = newState
	s.workLock.Unlock()

	// Log the final state that was just published to the workState for dispatcher use.
	finalWorkStateForLog := make(map[string]map[string]interface{})
	for _, state := range newState.Servers {
		finalWorkStateForLog[state.Profile.Remarks] = map[string]interface{}{
			"Health":   int(state.Health),
			"Instance": state.Instance != nil,
		}
	}
	logger.Debug().
		Interface("published_workState_summary", finalWorkStateForLog).
		Msg("[AppServer] ReloadStrategy finished and updated workState.")

	// After publishing, notify the dispatcher that the routing table might need an update
	// because backend health/availability could have changed.
	go func() {
		currentRoutingSettings := s.settingsManager.Get().Routing
		if disp, ok := s.dispatcher.(settings.ConfigurableModule); ok {
			if err := disp.OnSettingsUpdate("routing", currentRoutingSettings); err != nil {
				logger.Error().Err(err).Msg("Error notifying dispatcher of routing update after reload")
			}
		}
	}()

	logger.Info().Int("count", len(newState.Servers)).Msg(">>> State published to dispatcher successfully.")
	return nil
}

// loadConfigAndBootstrap orchestrates the full startup sequence.
func (s *AppServer) loadConfigAndBootstrap() error {
	logger.Info().Msg("[AppServer] Starting bootstrap sequence...")

	// 1. Load profiles from servers.json into A-Zone, create instances
	s.configLock.Lock()
	if err := s.loadConfigFromFile(); err != nil {
		s.configLock.Unlock()
		return err
	}
	s.manageInstances()
	s.configLock.Unlock() // Release lock before (potentially slow) health checks

	// 2. Run initial health checks which will update A-Zone internally
	logger.Info().Msg("[Bootstrap] Running initial health checks...")
	s.runHealthChecks()

	// 3. Perform the first publication to B-Zone. runHealthChecks might have already done this
	// if a state change was detected, but this call ensures B-Zone is populated regardless.
	logger.Info().Msg("[Bootstrap] Performing initial state publication...")
	if err := s.ReloadStrategy(); err != nil {
		return fmt.Errorf("initial state publication failed: %w", err)
	}

	logger.Info().Msg("[AppServer] Bootstrap sequence completed.")
	return nil
}

// loadConfigFromFile reads servers.json and populates the initial configState (A-Zone).
// This must be called under a write lock on configLock.
func (s *AppServer) loadConfigFromFile() error {
	logger.Info().Msg("[AppServer] Loading server profiles from file...")
	profiles, err := config.LoadServers(s.serversPath)
	if err != nil {
		return fmt.Errorf("failed to load server profiles from %s: %w", s.serversPath, err)
	}

	// Reset the current server map in configState
	s.configState.Servers = make(map[string]*types.ServerState)

	for _, profile := range profiles {
		if profile.ID == "" {
			profile.ID = uuid.New().String()
		}
		serverState := &types.ServerState{
			Profile: profile,
			Health:  types.StatusUnknown,
			Metrics: &types.Metrics{ActiveConnections: -1, Latency: -1},
		}
		s.configState.Servers[profile.ID] = serverState
	}

	// Save back immediately if any new IDs were generated
	// This avoids race conditions with subsequent UI operations.
	s.serversFileLock.Lock()
	defer s.serversFileLock.Unlock()
	if err := config.SaveServers(s.serversPath, profiles); err != nil {
		logger.Error().Err(err).Msg("Failed to save profiles after assigning new IDs")
	}

	logger.Info().Int("count", len(s.configState.Servers)).Msg("Server profiles loaded into A-Zone.")
	return nil
}

// UpdateServerActiveState is called by the web handler to change a server's active status.
// It modifies the configState (A-Zone) and triggers instance management.
func (s *AppServer) UpdateServerActiveState(id string, active bool) error {
	s.configLock.RLock()
	state, ok := s.configState.Servers[id]
	if !ok {
		s.configLock.RUnlock()
		return fmt.Errorf("server with id %s not found in config state", id)
	}
	s.configLock.RUnlock()
	s.configLock.Lock()
	if state.Profile.Active == active {
		logger.Error().Bool("active", active).Msg("修改前后值是相同的")
		s.configLock.Unlock()
		return nil // No change needed
	}
	//无论运行时，是什么状态，都要更新json文件中的状态！
	state.Profile.Active = active

	logger.Info().Str("server", state.Profile.Remarks).Bool("active", active).Msg("Updating server active state in A-Zone.")

	// Apply the change immediately by managing instances
	s.manageInstances()
	s.configLock.Unlock()

	s.ReloadStrategy()
	go func() {
		s.SaveConfigToFile()
	}()

	return nil
}

// AddServerProfile adds a new server to the configState (A-Zone).
func (s *AppServer) AddServerProfile(profile *types.ServerProfile) error {
	s.configLock.Lock()
	defer s.configLock.Unlock()

	if profile.ID == "" {
		return fmt.Errorf("profile must have an ID")
	}

	if _, exists := s.configState.Servers[profile.ID]; exists {
		return fmt.Errorf("server with id %s already exists", profile.ID)
	}

	serverState := &types.ServerState{
		Profile: profile,
		Health:  types.StatusUnknown,
		Metrics: &types.Metrics{ActiveConnections: -1, Latency: -1},
	}
	s.configState.Servers[profile.ID] = serverState
	go func() {
		s.ReloadStrategy()
		s.SaveConfigToFile()
	}()
	return nil
}

// UpdateServerProfile updates an existing server in the configState (A-Zone).
func (s *AppServer) UpdateServerProfile(id string, updatedProfile *types.ServerProfile) error {
	s.configLock.Lock()
	defer s.configLock.Unlock()

	state, ok := s.configState.Servers[id]
	if !ok {
		return fmt.Errorf("server with id %s not found", id)
	}

	// Preserve runtime state
	updatedProfile.Active = state.Profile.Active // Active status is managed separately
	state.Profile = updatedProfile

	// If the instance is active, it needs to be updated or restarted.
	// The simplest robust way is to shut down the old one. manageInstances will recreate it.
	if state.Profile.Active && state.Instance != nil {
		logger.Info().Str("id", id).Msg("Closing active instance for profile update.")
		state.Instance.CloseTunnel()
		state.Instance = nil
		s.manageInstances() // Re-create with new profile data
	}

	go func() {
		s.ReloadStrategy()
		s.SaveConfigToFile()
	}()

	return nil
}

// DeleteServerProfile removes a server from the configState (A-Zone).
func (s *AppServer) DeleteServerProfile(id string) error {
	s.configLock.Lock()
	defer s.configLock.Unlock()

	state, ok := s.configState.Servers[id]
	if !ok {
		return fmt.Errorf("server with id %s not found", id)
	}

	// Ensure the instance is stopped before deleting
	if state.Instance != nil {
		logger.Info().Str("id", id).Msg("Closing instance before deleting profile.")
		state.Instance.CloseTunnel()
		state.Instance = nil
	}

	delete(s.configState.Servers, id)

	go func() {
		s.ReloadStrategy()
		s.SaveConfigToFile()
	}()

	return nil
}

// GetAllServerProfilesSorted returns a sorted slice of all server profiles from the configState.
func (s *AppServer) GetAllServerProfilesSorted() []*types.ServerProfile {
	s.configLock.RLock()
	defer s.configLock.RUnlock()

	profiles := make([]*types.ServerProfile, 0, len(s.configState.Servers))
	for _, state := range s.configState.Servers {
		profiles = append(profiles, state.Profile)
	}

	// Sort by remarks to ensure stable order for the UI
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Remarks < profiles[j].Remarks
	})

	return profiles
}

// SaveConfigToFile persists the current configuration from A-Zone to servers.json.
func (s *AppServer) SaveConfigToFile() error {
	s.serversFileLock.Lock()
	defer s.serversFileLock.Unlock()

	s.configLock.RLock()
	profiles := make([]*types.ServerProfile, 0, len(s.configState.Servers))
	for _, state := range s.configState.Servers {
		profiles = append(profiles, state.Profile)
	}
	s.configLock.RUnlock()

	logger.Debug().Msg("Persisting configuration to servers.json directly from memory...")
	return config.SaveServers(s.serversPath, profiles)
}

// ReportSuccess implements the FailureReporter interface.
func (s *AppServer) ReportSuccess(serverID string) {
	s.failureMutex.Lock()
	defer s.failureMutex.Unlock()
	if _, ok := s.failureCounters[serverID]; ok {
		s.failureCounters[serverID] = 0
	}
}

// ReportFailure implements the FailureReporter interface.
func (s *AppServer) ReportFailure(serverID string) {
	s.failureMutex.Lock()
	defer s.failureMutex.Unlock()

	s.failureCounters[serverID]++
	count := s.failureCounters[serverID]

	logger.Warn().Str("server_id", serverID).Int("count", count).Msg("Failure reported for instance.")

	if count >= 5 {
		logger.Warn().Str("server_id", serverID).Msg("Failure threshold reached. Triggering health check.")
		s.failureCounters[serverID] = 0 // Reset counter
		go s.triggerSingleHealthCheck(serverID)
	}
}

// 对单个实例执行健康检查，并在失败时触发全局状态更新。
func (s *AppServer) triggerSingleHealthCheck(serverID string) {
	// 1. 从配置区安全地获取实例引用。配置区拥有所有实例的权威列表。
	s.configLock.Lock() // Use write lock as we might modify health status
	state, ok := s.configState.Servers[serverID]
	if !ok || state.Instance == nil {
		s.configLock.Unlock()
		logger.Warn().Str("server_id", serverID).Msg("Instance not found for single health check.")
		return
	}

	oldHealth := state.Health
	err := state.Instance.CheckHealth()
	newHealth := types.StatusUp
	if err != nil {
		newHealth = types.StatusDown
	}

	state.Health = newHealth
	s.configLock.Unlock()

	// If health status has changed, trigger a reload to publish the change.
	if newHealth != oldHealth {
		logger.Info().
			Str("server_id", serverID).
			Interface("old_health", oldHealth).
			Interface("new_health", newHealth).
			Msg("Health status changed after single check, triggering state reload.")
		// We call the reload function which will handle publishing the new state
		if err := s.ReloadStrategy(); err != nil {
			logger.Error().Err(err).Msg("Failed to reload state after single health check")
		}
	} else {
		logger.Debug().Str("server_id", serverID).Msg("Single health check passed, status unchanged.")
	}
}

// GetServerStates implements the StateProvider interface.
func (s *AppServer) GetServerStates() map[string]*types.ServerState {
	s.workLock.RLock()
	newState := deepCopyAppState(s.workState)
	s.workLock.RUnlock()
	return newState.Servers
}

// GetRecentClientIPs implements the ServerController interface.
func (s *AppServer) GetRecentClientIPs() []string {
	if d, ok := s.dispatcher.(*dispatcher.Dispatcher); ok {
		return d.GetRecentClientIPs()
	}
	return []string{}
}

func (s *AppServer) Wait() {
	s.waitGroup.Wait()
}

func (s *AppServer) healthCheckLoop() {
	defer s.waitGroup.Done()

	// The initial check is now handled by loadConfigAndBootstrap.
	// The loop starts ticking immediately after its interval.
	for {
		select {
		case <-s.healthCheckTicker.C:
			s.runHealthChecks()
		case <-s.done(): // Use a proper stop channel pattern
			return
		}
	}
}

func (s *AppServer) done() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		s.stopOnce.Do(func() {
			close(ch)
		})
	}()
	return ch
}

func (s *AppServer) runHealthChecks() {
	logger.Debug().Msg("[HealthChecker] Starting periodic health check cycle...")
	// 1. Get a list of instances to check from the A-Zone (configState)
	s.configLock.RLock()
	instancesToCheck := make(map[string]types.TunnelStrategy)
	for id, state := range s.configState.Servers {
		if state.Profile.Active && state.Instance != nil {
			instancesToCheck[id] = state.Instance
		}
	}
	s.configLock.RUnlock()

	if len(instancesToCheck) == 0 {
		logger.Debug().Msg("[HealthChecker] No active instances to check.")
		return
	}

	// 2. Perform the actual checks (this can be slow)
	healthStatusMap, metricsCacheMap := s.healthChecker.Check(instancesToCheck)

	// 3. Lock A-Zone for writing and update the state
	s.configLock.Lock()
	var stateChanged bool
	for id, newHealth := range healthStatusMap {
		if state, ok := s.configState.Servers[id]; ok {
			if state.Health != newHealth {
				state.Health = newHealth
				stateChanged = true
				logger.Info().Str("server", state.Profile.Remarks).Interface("new_status", newHealth).Msg("Health status changed.")
			}
			// Always update metrics
			state.Metrics = metricsCacheMap[id]
		}
	}
	s.configLock.Unlock()

	// 4. Log summary and publish if needed
	logger.Debug().
		Int("checked_count", len(healthStatusMap)).
		Bool("state_changed", stateChanged).
		Msg("[HealthChecker] Cycle complete.")

	if stateChanged {
		logger.Info().Msg("[HealthChecker] Change detected, publishing updated state...")
		if err := s.ReloadStrategy(); err != nil {
			logger.Error().Err(err).Msg("Failed to reload state after health check cycle")
		}
	}
}
