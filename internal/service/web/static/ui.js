import { serversCache, logMessages } from './state.js';

// --- UI Element References ---
const serverListBody = document.getElementById('server-list');
const dialog = document.getElementById('server-dialog');
export const form = document.getElementById('server-form');
const dialogTitle = document.getElementById('dialog-title');
const serverIdInput = document.getElementById('server-id');
const serverTypeSelect = document.getElementById('type');
const networkSelect = document.getElementById('network');
const vlessSecuritySelect = document.getElementById('security');
const hostInput = document.getElementById('host');
const sniInput = document.getElementById('sni');
const logContent = document.getElementById('log-content');

// --- NEW UI Element References for Rule Dialog ---
const ruleDialog = document.getElementById('rule-dialog');
export const ruleForm = document.getElementById('rule-form');
const ruleDialogTitle = document.getElementById('rule-dialog-title');
const ruleIndexInput = document.getElementById('rule-index');
const ruleTargetSelect = document.getElementById('rule-target');

/**
 * Renders the server list table based on the current serversCache.
 */
export function renderServers() {
    serverListBody.innerHTML = '';
    serversCache.forEach(server => {
        const row = document.createElement('tr');
        row.className = server.active ? 'active-row' : '';
        row.dataset.id = server.id;

        let statusIndicator = '';
        if (server.active) {
            let statusClass = 'status-unknown';
            if (server.health === 1) statusClass = 'status-up';
            if (server.health === 2) statusClass = 'status-down';
            statusIndicator = `<span class="status-indicator ${statusClass}"></span>`;
        }

        let details = `Listen: ${escapeHTML(String(server.localPort || 'N/A'))}`;
        if (server.active && server.connections >= 0) {
             details += ` | Conns: ${server.connections}`;
        }
        if (server.active && server.latency >= 0) {
            details += ` | Latency: ${server.latency}ms`;
        }
        if (server.type === 'vless') {
            details += ` | SNI: ${escapeHTML(server.sni || 'auto')}`;
        } else if (server.type === 'worker') {
            details += ` | Edge IP: ${escapeHTML(server.edgeIP || 'N/A')}`;
        }

        const activateButtonText = server.active ? 'Deactivate' : 'Activate';
        const activateButtonClass = server.active ? 'deactivate-btn' : 'activate-btn';

        row.innerHTML = `
            <td>${statusIndicator}</td>
            <td>${escapeHTML(server.remarks)}</td>
            <td>${escapeHTML(server.address)}:${escapeHTML(String(server.port))}</td>
            <td>${escapeHTML(server.type)}</td>
            <td>${details}</td>
            <td class="actions">
                <button class="${activateButtonClass}" data-id="${server.id}" data-active="${!server.active}">${activateButtonText}</button>
                <button class="edit-btn" data-id="${server.id}">Edit</button>
                <button class="delete-btn" data-id="${server.id}">Delete</button>
            </td>
        `;
        serverListBody.appendChild(row);
    });
}

/**
 * Renders the log panel with the latest messages.
 */
export function renderLogPanel() {
    if (logContent) {
        logContent.textContent = logMessages.join('\n');
        // Auto-scroll to the bottom
        logContent.scrollTop = logContent.scrollHeight;
    }
}

/**
 * Updates the message in the log panel directly.
 * Used for showing transient states like errors or actions.
 * @param {string} message - The message to display.
 */
export function updateStatusMessage(message) {
    const timestamp = new Date().toLocaleTimeString();
    logMessages.push(`[${timestamp}] [UI] ${message}`);
    if (logMessages.length > 50) {
        logMessages.shift();
    }
    renderLogPanel();
}


const setFieldVisibility = (wrapperId, visible) => {
    const wrapper = document.getElementById(wrapperId);
    if (wrapper) {
        wrapper.style.display = visible ? 'grid' : 'none';
    }
};

function updateVlessSecurityFields(securityType) {
    const isTls = securityType === 'tls';
    const isReality = securityType === 'reality';

    setFieldVisibility('sni-wrapper', isTls || isReality);
    setFieldVisibility('fingerprint-wrapper', isTls || isReality);
    setFieldVisibility('publicKey-wrapper', isReality);
    setFieldVisibility('shortId-wrapper', isReality);
}

export function updateFormVisibility() {
    const type = serverTypeSelect.value;
    const networkType = networkSelect.value;
    document.getElementById('vless-fields').style.display = type === 'vless' ? '' : 'none';
    document.getElementById('worker-fields').style.display = type === 'worker' ? '' : 'none';

    const showWs = (type === 'vless' && networkType === 'ws') || type === 'worker' || type === 'goremote';
    document.getElementById('common-ws-fields').style.display = showWs ? '' : 'none';

    const showGrpc = type === 'vless' && networkType === 'grpc';
    document.getElementById('vless-grpc-fields').style.display = showGrpc ? '' : 'none';

    if (type === 'vless') {
        updateVlessSecurityFields(vlessSecuritySelect.value);
    }
}

export function showDialog(server = null) {
    form.reset();
    if (server) {
        dialogTitle.textContent = 'Edit Server';
        serverIdInput.value = server.id;
        Object.keys(server).forEach(key => {
            const input = form.elements[key];
            if (input) {
                input.value = server[key];
            }
        });
    } else {
        dialogTitle.textContent = 'Add Server';
        serverIdInput.value = '';
        serverTypeSelect.value = 'goremote';
    }
    updateFormVisibility();
    dialog.showModal();
}

export function closeDialog() {
    dialog.close();
}

/**
 * Populates the target dropdown in the rule editor dialog.
 * @param {Array} servers - The current list of server profiles from serversCache.
 */
export function populateRuleTargetOptions(servers) {
    ruleTargetSelect.innerHTML = `
        <option value="DIRECT">DIRECT</option>
        <option value="REJECT">REJECT</option>
    `;
    servers.forEach(server => {
        const option = document.createElement('option');
        option.value = server.remarks;
        option.textContent = server.remarks;
        ruleTargetSelect.appendChild(option);
    });
}

/**
 * Shows the rule editor dialog, optionally pre-filled with rule data.
 * @param {object|null} rule - The rule to edit, or null to create a new one.
 * @param {number|null} index - The index of the rule being edited.
 */
export function showRuleDialog(rule = null, index = null) {
    ruleForm.reset();
    const fetchBtn = document.getElementById('fetch-clients-btn');
    if (rule && rule.type === 'source_ip' || !rule && ruleForm.elements.type.value === 'source_ip') {
        fetchBtn.style.display = 'block';
    } else {
        fetchBtn.style.display = 'none';
    }
    if (rule) {
        ruleDialogTitle.textContent = 'Edit Rule';
        ruleIndexInput.value = index;
        // Populate form fields from the rule object
        ruleForm.elements.type.value = rule.type;
        ruleForm.elements.target.value = rule.target;
        ruleForm.elements.priority.value = rule.priority;
        // Convert array to newline-separated string for textarea
        if (Array.isArray(rule.value)) {
            ruleForm.elements.value.value = rule.value.join('\n');
        }
    } else {
        ruleDialogTitle.textContent = 'Add Rule';
        ruleIndexInput.value = ''; // Indicate a new rule
    }
    ruleDialog.showModal();
}

/**
 * Closes the rule editor dialog.
 */
export function closeRuleDialog() {
    ruleDialog.close();
}

/**
 * Gets the rule data from the form.
 * @returns {{data: object, index: number|null}}
 */
export function getRuleFormData() {
    const formData = new FormData(ruleForm);
    const valueText = formData.get('value') || '';
    const values = valueText.split('\n').map(v => v.trim()).filter(v => v.length > 0);

    const ruleData = {
        priority: parseInt(formData.get('priority'), 10) || 99,
        type: formData.get('type'),
        value: values, // Always an array
        target: formData.get('target'),
    };
    const indexStr = formData.get('rule-index');
    return {
        data: ruleData,
        index: indexStr ? parseInt(indexStr, 10) : null
    };
}

export function getFormServerData() {
    const formData = new FormData(form);
    const serverData = Object.fromEntries(formData.entries());

    if (serverData.type === 'vless') {
        if (serverData.network === 'grpc') {
            delete serverData.path;
            delete serverData.host;
            delete serverData.scheme;
        } else {
            delete serverData.grpcServiceName;
            delete serverData.grpcAuthority;
            delete serverData.grpcMode;
        }
    }

    serverData.port = parseInt(serverData.port, 10) || 0;
    serverData.localPort = parseInt(serverData.localPort, 10) || 0;
    delete serverData.active;

    return serverData;
}

function escapeHTML(str) {
    if (str === null || str === undefined) return '';
    return str.toString()
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#039;');
}