// This is the main entry point for the frontend application.
// It imports functions from other modules and sets up event listeners.

import { serversCache, clearLogMessages } from './state.js';
import { fetchServers, fetchStatus, saveSettings } from './api.js';
import { initializeSettingsPage, loadSettings, saveRuleToCache, getRoutingSettingsData } from './settings.js';
import { 
    form, 
    ruleForm, // Import ruleForm
    showDialog,
    closeDialog,
    closeRuleDialog, // Import closeRuleDialog
    getRuleFormData, // Import getRuleFormData
    updateFormVisibility,
    getFormServerData,
    updateStatusMessage,
    renderLogPanel,
} from './ui.js';

document.addEventListener('DOMContentLoaded', () => {
    // --- UI Element References ---
    const mainServers = document.getElementById('main-servers');
    const mainGateway = document.getElementById('main-gateway');
    const mainRouting = document.getElementById('main-routing');
    const navServersLink = document.getElementById('nav-servers');
    const navGatewayLink = document.getElementById('nav-gateway');
    const navRoutingLink = document.getElementById('nav-routing');
    const serverListBody = document.getElementById('server-list');
    const addServerBtn = document.getElementById('add-server-btn');
    const cancelBtn = document.getElementById('cancel-btn');
    const clearLogBtn = document.getElementById('clear-log-btn');
    const serverTypeSelect = document.getElementById('type');
    const networkSelect = document.getElementById('network');
    const vlessSecuritySelect = document.getElementById('security');
    const hostInput = document.getElementById('host');
    const sniInput = document.getElementById('sni');
    const cancelRuleBtn = document.getElementById('cancel-rule-btn');
    
    // --- State Management ---
    let statusInterval = null;

    // --- Core Functions ---
    function startStatusPolling() {
        if (statusInterval) clearInterval(statusInterval);
        fetchStatus(); // Fetch immediately
        statusInterval = setInterval(fetchStatus, 3000); // Then poll every 3 seconds
    }

    function showPage(pageName) {
         // Hide all main sections
        mainServers.style.display = 'none';
        mainGateway.style.display = 'none';
        mainRouting.style.display = 'none';

        // Deactivate all nav links
        navServersLink.classList.remove('active');
        navGatewayLink.classList.remove('active');
        navRoutingLink.classList.remove('active');

        // Show the selected page and activate the corresponding link
        switch (pageName) {
            case 'gateway':
                mainGateway.style.display = 'block';
                navGatewayLink.classList.add('active');
                loadSettings(); // Load settings for this page
                break;
            case 'routing':
                mainRouting.style.display = 'block';
                navRoutingLink.classList.add('active');
                loadSettings(); // Also load settings for this page
                break;
            case 'servers':
            default:
                mainServers.style.display = 'block';
                navServersLink.classList.add('active');
                break;
        }
    }

    // --- Event Listeners ---
    navServersLink.addEventListener('click', (e) => { e.preventDefault(); showPage('servers'); });
    navGatewayLink.addEventListener('click', (e) => { e.preventDefault(); showPage('gateway'); });
    navRoutingLink.addEventListener('click', (e) => { e.preventDefault(); showPage('routing'); });

    addServerBtn.addEventListener('click', () => showDialog());
    cancelBtn.addEventListener('click', (e) => {
        e.preventDefault();
        closeDialog();
    });

    clearLogBtn.addEventListener('click', () => {
        clearLogMessages();
        renderLogPanel();
    });

    // Form visibility listeners
    serverTypeSelect.addEventListener('change', updateFormVisibility);
    networkSelect.addEventListener('change', updateFormVisibility);
    vlessSecuritySelect.addEventListener('change', updateFormVisibility);

    // Auto-fill SNI from Host
    hostInput.addEventListener('blur', () => {
        if (hostInput.value && !sniInput.value) {
            sniInput.value = hostInput.value;
        }
    });

    // Form submission
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const serverData = getFormServerData();
        const id = serverData.id;
        
        const method = id ? 'PUT' : 'POST';
        const url = id ? `/api/servers?id=${id}` : '/api/servers';

        try {
            updateStatusMessage(`Saving server: ${serverData.remarks}...`);
            const response = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(serverData)
            });
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`Failed to save server: ${errorText}`);
            }
            closeDialog();
            await fetchServers();
            await fetchStatus(); // Refresh status immediately after change
        } catch (error) {
            alert('Error: ' + error.message);
            updateStatusMessage(`Failed to save server.`, 'failed');
        }
    });

    // Event delegation for server list actions
    serverListBody.addEventListener('click', async (e) => {
        const target = e.target;
        const id = target.dataset.id;
        if (!id) return;

        if (target.classList.contains('delete-btn')) {
            if (confirm('Are you sure you want to delete this server?')) {
                try {
                    updateStatusMessage(`Deleting server...`);
                    const response = await fetch(`/api/servers?id=${id}`, { method: 'DELETE' });
                    if (!response.ok) throw new Error('Failed to delete');
                    await fetchServers();
                    await fetchStatus();
                } catch (error) {
                    alert('Error deleting server: ' + error.message);
                }
            }
        } else if (target.classList.contains('edit-btn')) {
            const serverToEdit = serversCache.find(s => s.id === id);
            if (serverToEdit) showDialog(serverToEdit);
         } else if (target.classList.contains('activate-btn') || target.classList.contains('deactivate-btn')) {
            const setActive = target.dataset.active === 'true';
            const actionText = setActive ? 'Activating' : 'Deactivating';
            try {
                const response = await fetch(`/api/servers/set_active_state?id=${id}&active=${setActive}`, { method: 'POST' });
                if (!response.ok) {
                    throw new Error(`Failed to ${actionText.toLowerCase()}`);
                }

                // 1. Update local cache
                const serverInCache = serversCache.find(s => s.id === id);
                if (serverInCache) {
                    serverInCache.active = setActive;
                }

                // 2. Optionally trigger a status fetch to update health/port info sooner
                fetchStatus();
            } catch (error) {
                alert(`Error: ${error.message}`);
                fetchStatus();
            }
        }
    });

     cancelRuleBtn.addEventListener('click', (e) => {
        e.preventDefault();
        closeRuleDialog();
    });

    ruleForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const { data, index } = getRuleFormData();

        // 1. Update the local cache and re-render the table immediately for responsiveness
        saveRuleToCache(data, index);

        // 2. Assemble the complete routing settings payload
        const routingSettingsPayload = getRoutingSettingsData();

        // 3. Save the entire routing configuration to the backend
        try {
            updateStatusMessage('Saving rule to server...');
            await saveSettings('routing', routingSettingsPayload);
            updateStatusMessage('Rule saved successfully and applied.');
        } catch (error) {
            alert('Error saving rule to server: ' + error.message);
            updateStatusMessage('Failed to save rule.', 'failed');
            // On failure, we might want to reload settings from the server to ensure consistency
            loadSettings();
            return; // Don't close the dialog on error
        }

        // 4. Close the dialog on success
        closeRuleDialog();
    });

    // --- Initial Load ---
    initializeSettingsPage(); // 初始化设置页面的事件监听器
    showPage('servers'); // 默认显示服务器列表页面

    fetchServers();
    startStatusPolling();
});