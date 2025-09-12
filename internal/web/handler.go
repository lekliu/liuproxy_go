// --- START OF NEW FILE internal/web/handler.go ---
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"liuproxy_go/internal/globalstate"
	"liuproxy_go/internal/types"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"liuproxy_go/internal/config"
)

type Handler struct {
	config     *types.Config
	configPath string
	onReload   func() error
	mu         sync.Mutex // Mutex to protect config file reads/writes
}

func NewHandler(cfg *types.Config, configPath string, onReload func() error) *Handler {
	return &Handler{
		config:     cfg,
		configPath: configPath,
		onReload:   onReload,
	}
}

// API Models
type ServerInfo struct {
	ID      string `json:"id"`
	Remarks string `json:"remarks"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Scheme  string `json:"scheme"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	EdgeIP  string `json:"edgeIP"`
	Active  bool   `json:"active"`
}

// HandleServers is the main handler for /api/servers endpoint
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

func (h *Handler) getServers(w http.ResponseWriter, r *http.Request) {
	var serverInfos []ServerInfo

	// 在加载时重新从文件读取，以确保状态最新
	if err := config.LoadIni(h.config, h.configPath); err != nil {
		http.Error(w, "Failed to reload config file for reading", http.StatusInternalServerError)
		return
	}

	for i, remoteCfg := range h.config.LocalConf.RemoteIPs {
		id := fmt.Sprintf("remote_ip_%02d", i+1)

		port, _ := strconv.Atoi(remoteCfg[2])

		serverInfos = append(serverInfos, ServerInfo{
			ID:      id,
			Remarks: remoteCfg[0],
			Address: remoteCfg[1],
			Port:    port,
			Scheme:  remoteCfg[3],
			Path:    remoteCfg[4],
			Type:    remoteCfg[5],
			EdgeIP:  remoteCfg[6],
			Active:  remoteCfg[7] == "true",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverInfos)
}

func (h *Handler) addServer(w http.ResponseWriter, r *http.Request) {
	log.Println("[DEBUG] addServer: Handler entered.") // 日志 1: 确认函数被调用

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[DEBUG] addServer: ERROR - Failed to read request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	log.Printf("[DEBUG] addServer: Received raw body: %s", string(body)) // 日志 2: 打印原始请求体

	var newServer ServerInfo
	if err := json.Unmarshal(body, &newServer); err != nil {
		log.Printf("[DEBUG] addServer: ERROR - Failed to unmarshal JSON: %v", err)
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	newServerConfig := []string{
		newServer.Remarks, newServer.Address, strconv.Itoa(newServer.Port), newServer.Scheme,
		newServer.Path, newServer.Type, newServer.EdgeIP, "false",
	}
	log.Printf("[DEBUG] addServer: Constructed new server config slice: %v", newServerConfig) // 日志 4: 打印准备写入的切片

	h.config.LocalConf.RemoteIPs = append(h.config.LocalConf.RemoteIPs, newServerConfig)
	log.Println("[DEBUG] addServer: In-memory config updated. Preparing to save to file...")

	if err := config.SaveIni(h.config, h.configPath); err != nil {
		log.Printf("[DEBUG] addServer: ERROR - config.SaveIni returned an error: %v", err) // 日志 5 (失败时)
		http.Error(w, "Failed to save config file", http.StatusInternalServerError)
		return
	}

	log.Println("[DEBUG] addServer: Successfully saved config to file. Sending 201 Created.") // 日志 5 (成功时)
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) updateServer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	index, err := serverIDToIndex(id)
	if err != nil || index >= len(h.config.LocalConf.RemoteIPs) {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var updatedServer ServerInfo
	if err := json.Unmarshal(body, &updatedServer); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// 保持现有的激活状态
	currentActiveState := h.config.LocalConf.RemoteIPs[index][7]
	h.config.LocalConf.RemoteIPs[index] = []string{
		updatedServer.Remarks, updatedServer.Address, strconv.Itoa(updatedServer.Port), updatedServer.Scheme,
		updatedServer.Path, updatedServer.Type, updatedServer.EdgeIP, currentActiveState,
	}

	if err := config.SaveIni(h.config, h.configPath); err != nil {
		http.Error(w, "Failed to save config file", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	index, err := serverIDToIndex(id)
	if err != nil || index >= len(h.config.LocalConf.RemoteIPs) {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	if len(h.config.LocalConf.RemoteIPs) <= 1 {
		http.Error(w, "Cannot delete the last server", http.StatusBadRequest)
		return
	}

	// 检查是否正在删除活动服务器
	wasActive := h.config.LocalConf.RemoteIPs[index][7] == "true"
	h.config.LocalConf.RemoteIPs = append(h.config.LocalConf.RemoteIPs[:index], h.config.LocalConf.RemoteIPs[index+1:]...)

	// 如果删除了活动服务器，则自动激活第一个
	if wasActive && len(h.config.LocalConf.RemoteIPs) > 0 {
		h.config.LocalConf.RemoteIPs[0][7] = "true"
	}

	if err := config.SaveIni(h.config, h.configPath); err != nil {
		http.Error(w, "Failed to save config file", http.StatusInternalServerError)
		return
	}

	// 如果删除了活动服务器，触发重载
	if wasActive {
		go h.onReload()
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleActivateServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	id := r.URL.Query().Get("id")
	index, err := serverIDToIndex(id)
	if err != nil || index >= len(h.config.LocalConf.RemoteIPs) {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	// Set all to inactive first
	for i := range h.config.LocalConf.RemoteIPs {
		h.config.LocalConf.RemoteIPs[i][7] = "false"
	}
	// Set the selected one to active
	h.config.LocalConf.RemoteIPs[index][7] = "true"

	if err := config.SaveIni(h.config, h.configPath); err != nil {
		http.Error(w, "Failed to save config file", http.StatusInternalServerError)
		return
	}

	// Trigger the reload callback. This is now an async operation.
	// The frontend will poll for the status update.
	go h.onReload()

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	statusMsg := globalstate.GlobalStatus.Get()

	status := map[string]string{"status": statusMsg}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func serverIDToIndex(id string) (int, error) {
	parts := strings.Split(id, "_")
	if len(parts) != 3 || parts[0] != "remote" || parts[1] != "ip" {
		return -1, fmt.Errorf("invalid id format")
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil {
		return -1, fmt.Errorf("invalid numeric index in id: %w", err)
	}
	return index - 1, nil
}
