package web

import (
	"encoding/json"
	"io"
	"liuproxy_go/internal/shared/globalstate"
	"liuproxy_go/internal/shared/logger"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"liuproxy_go/internal/shared/settings"
	"liuproxy_go/internal/shared/types"
)

// ServerController defines the interface that the web handler uses to interact with the AppServer.
// This decouples the web package from the server package.
type ServerController interface {
	GetServerStates() map[string]*types.ServerState
	UpdateServerActiveState(id string, active bool) error
	ReloadStrategy() error
	SaveConfigToFile() error
	GetAllServerProfilesSorted() []*types.ServerProfile
	AddServerProfile(profile *types.ServerProfile) error
	UpdateServerProfile(id string, updatedProfile *types.ServerProfile) error
	DeleteServerProfile(id string) error
	GetRecentClientIPs() []string
}

type Handler struct {
	serversPath     string
	settingsManager *settings.SettingsManager // 新增
	controller      ServerController
	mu              sync.Mutex
}

func NewHandler(
	cfg *types.Config,
	serversPath string,
	settingsManager *settings.SettingsManager,
	controller ServerController,
) *Handler {
	return &Handler{
		serversPath:     serversPath,
		settingsManager: settingsManager,
		controller:      controller,
	}
}

// --- 新的统一配置 API ---

// HandleGetSettings 处理 GET /api/settings 请求
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	currentSettings := h.settingsManager.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentSettings)
}

// HandleUpdateSettings 处理 POST /api/settings/{module} 请求
func (h *Handler) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 路径中提取模块名
	moduleKey := strings.TrimPrefix(r.URL.Path, "/api/settings/")
	if moduleKey == "" {
		http.Error(w, "Module key is missing in URL path", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	// 将更新请求委托给 SettingsManager
	if err := h.settingsManager.Update(moduleKey, body); err != nil {
		// 根据错误类型返回不同的状态码
		if strings.Contains(err.Error(), "unknown settings module") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if strings.Contains(err.Error(), "failed to parse JSON") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Settings updated successfully"}`))
}

// HandleGetClients 处理 GET /api/clients 请求，返回可用的客户端IP列表。
func (h *Handler) HandleGetClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. 获取所有已配置的 source_ip 规则中的IP
	currentSettings := h.settingsManager.Get()
	configuredIPs := make(map[string]struct{})
	for _, rule := range currentSettings.Routing.Rules {
		if rule.Type == string(settings.RuleTypeSourceIP) {
			for _, val := range rule.Value {
				configuredIPs[val] = struct{}{}
			}
		}
	}

	// 2. 获取最近在线的IP
	recentIPs := h.controller.GetRecentClientIPs()

	// 3. 过滤掉已经配置过的IP
	availableIPs := make([]string, 0)
	for _, ip := range recentIPs {
		if _, exists := configuredIPs[ip]; !exists {
			availableIPs = append(availableIPs, ip)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(availableIPs)
}

// --- 旧的/现有的 API ---

// HandleStatus 保持不变
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	type StatusResponse struct {
		GlobalStatus string                         `json:"globalStatus"`
		RuntimeInfo  map[string]*types.ListenerInfo `json:"runtimeInfo"`
		HealthStatus map[string]types.HealthStatus  `json:"healthStatus"`
		Metrics      map[string]*types.Metrics      `json:"metrics"`
	}

	// 从统一的状态源获取所有服务器状态
	serverStates := h.controller.GetServerStates()

	// 手动构建前端需要的三个独立的 map
	runtimeInfo := make(map[string]*types.ListenerInfo)
	healthStatus := make(map[string]types.HealthStatus)
	metrics := make(map[string]*types.Metrics)

	for id, state := range serverStates {
		if state.Instance != nil {
			runtimeInfo[id] = state.Instance.GetListenerInfo()
		}
		healthStatus[id] = state.Health
		metrics[id] = state.Metrics
	}

	response := StatusResponse{
		GlobalStatus: globalstate.GlobalStatus.Get(),
		RuntimeInfo:  runtimeInfo,
		HealthStatus: healthStatus,
		Metrics:      metrics,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleSetServerActiveState 保持不变
func (h *Handler) HandleSetServerActiveState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	activeStr := r.URL.Query().Get("active")
	active, err := strconv.ParseBool(activeStr)
	if err != nil || id == "" {
		http.Error(w, "Invalid parameters", http.StatusBadRequest)
		return
	}

	logger.Info().Str("id", id).Bool("active", active).Msg("[Handler] Received request to set server active state")

	// 1. Update state in A-Zone and manage instance
	if err := h.controller.UpdateServerActiveState(id, active); err != nil {
		logger.Error().Err(err).Msg("Failed to update server active state")
		http.Error(w, "Failed to update server active state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Immediately respond to the client.
	w.WriteHeader(http.StatusOK)
}

// HandleServers (CRUD) 保持不变
func (h *Handler) HandleServers(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		h.getServers(w, r)
	case http.MethodPost:
		h.addServer(w, r)
	case http.MethodPut:
		h.updateServer(w, r)
	case http.MethodDelete:
		h.deleteServer(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ... (getServers, addServer, updateServer, deleteServer 的实现保持不变) ...
func (h *Handler) getServers(w http.ResponseWriter, r *http.Request) {
	profiles := h.controller.GetAllServerProfilesSorted()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

func (h *Handler) addServer(w http.ResponseWriter, r *http.Request) {
	var newProfile types.ServerProfile
	if err := json.NewDecoder(r.Body).Decode(&newProfile); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	newProfile.ID = uuid.New().String()
	newProfile.Active = false

	if err := h.controller.AddServerProfile(&newProfile); err != nil {
		http.Error(w, "Failed to add server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) updateServer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing server ID", http.StatusBadRequest)
		return
	}
	var updatedProfile types.ServerProfile
	if err := json.NewDecoder(r.Body).Decode(&updatedProfile); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	updatedProfile.ID = id // Ensure ID is correct

	if err := h.controller.UpdateServerProfile(id, &updatedProfile); err != nil {
		http.Error(w, "Failed to update server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing server ID", http.StatusBadRequest)
		return
	}

	if err := h.controller.DeleteServerProfile(id); err != nil {
		http.Error(w, "Failed to delete server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
