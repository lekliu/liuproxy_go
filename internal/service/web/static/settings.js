// ***********  liuproxy_go\internal\web\static\settings.js ***********
// This module handles all logic for the Settings page.
import { fetchAllSettings, saveSettings, fetchAvailableClientIPs } from './api.js';
import { updateStatusMessage, showRuleDialog, populateRuleTargetOptions } from './ui.js';
import { serversCache } from './state.js';

// --- UI Element References ---
const gatewaySettingsForm = document.getElementById('gateway-settings-form');
const routingSettingsForm = document.getElementById('routing-settings-form');
const stickyRulesTextarea = document.getElementById('sticky_rules');
const ruleListBody = document.getElementById('rule-list-body');


// --- State ---
let routingRulesCache = []; // Data Model: The single source of truth for all rules.
let filterText = '';          // View State: Current text for value filtering.
let filterType = 'all';       // View State: Current selected type for filtering.
let sortDirection = 'asc';    // View State: Current sort direction ('asc' or 'desc').

/**
 * Loads all settings from the backend and populates the form.
 */
export async function loadSettings() {
    try {
        const settings = await fetchAllSettings();
        if (settings) {
            if (settings.gateway) {
                populateGatewaySettings(settings.gateway);
            }
            if (settings.routing) {
                routingRulesCache = JSON.parse(JSON.stringify(settings.routing.rules || []));
                renderRulesTable(); // Initial render
            }
        }
    } catch (error) {
        console.error('Failed to load settings:', error);
        updateStatusMessage('Error: Could not load system settings.', 'failed');
    }
}

/**
 * Populates the Gateway Settings card with data.
 * @param {object} gatewaySettings - The gateway settings object from the API.
 */
function populateGatewaySettings(gatewaySettings) {
    const form = gatewaySettingsForm;
    // Set radio button for mode
    const mode = gatewaySettings.sticky_session_mode || 'disabled';
    const radio = form.querySelector(`input[name="sticky_session_mode"][value="${mode}"]`);
    if (radio) radio.checked = true;

    // Set TTL
    form.elements.sticky_session_ttl.value = gatewaySettings.sticky_session_ttl || 300;

    // Set rules in textarea
    stickyRulesTextarea.value = (gatewaySettings.sticky_rules || []).join('\n');

    // Set load balancer strategy
    form.elements.load_balancer_strategy.value = gatewaySettings.load_balancer_strategy || 'least_connections';
}

/**
 * Collects data from the Gateway Settings card and formats it for the API.
 * @returns {object} The gateway settings object to be sent.
 */
function getGatewaySettingsData() {
    const formData = new FormData(gatewaySettingsForm);
    const rules = stickyRulesTextarea.value.split('\n').map(rule => rule.trim()).filter(rule => rule);
    return {
        sticky_session_mode: formData.get('sticky_session_mode'),
        sticky_session_ttl: parseInt(formData.get('sticky_session_ttl'), 10),
        sticky_rules: rules,
        load_balancer_strategy: formData.get('load_balancer_strategy'),
    };
}

/**
 * Renders the rules table by filtering and sorting the master `routingRulesCache`.
 */
function renderRulesTable() {
    ruleListBody.innerHTML = '';

    // 1. Filter the data
    let rulesToRender = routingRulesCache.filter(rule => {
        const typeMatch = filterType === 'all' || rule.type === filterType;
        const textMatch = !filterText || (Array.isArray(rule.value) && rule.value.join(' ').toLowerCase().includes(filterText.toLowerCase()));
        return typeMatch && textMatch;
    });

    // 2. Sort the filtered data
    rulesToRender.sort((a, b) => {
        const priorityA = a.priority || 99;
        const priorityB = b.priority || 99;
        return sortDirection === 'asc' ? priorityA - priorityB : priorityB - priorityA;
    });

    // 3. Render the view model
    rulesToRender.forEach(rule => {
        // IMPORTANT: Find the index from the ORIGINAL cache to ensure edits/deletes work correctly
        const originalIndex = routingRulesCache.findIndex(r => r === rule);

        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${rule.priority}</td>
            <td>${rule.type}</td>
            <td>${Array.isArray(rule.value) ? rule.value.join(', ') : rule.value}</td>
            <td>${rule.target}</td>
            <td class="actions">
                <button type="button" class="edit-rule-btn" data-original-index="${originalIndex}">Edit</button>
                <button type="button" class="delete-rule-btn" data-original-index="${originalIndex}">Delete</button>
            </td>
        `;
        ruleListBody.appendChild(row);
    });

    // Update sort indicator
    const indicator = document.getElementById('priority-sort-indicator');
    indicator.textContent = sortDirection === 'asc' ? '▲' : '▼';
}

/**
 * Collects the complete, unfiltered routing data for saving.
 * @returns {object} The routing settings object to be sent.
 */
export function getRoutingSettingsData() {
    return {
        rules: routingRulesCache, // Always save the complete data model
    };
}

/**
 * Saves or updates a rule in the local cache and re-renders the table.
 * @param {object} ruleData - The rule object from the form.
 * @param {number|null} index - The original index of the rule to update, or null for a new rule.
 */
export function saveRuleToCache(ruleData, originalIndex) {
    if (originalIndex !== null && originalIndex >= 0) {
        // Directly update the rule at its original, correct index
        routingRulesCache[originalIndex] = ruleData;
    } else {
        // This is a new rule, add it to the cache
        routingRulesCache.push(ruleData);
    }
    renderRulesTable(); // Re-render with new data
}


/**
 * Initializes all event listeners for the Settings page.
 */
export function initializeSettingsPage() {
    // Note: All interactive elements are now children of either gatewaySettingsForm or routingSettingsForm
    const gatewayPage = document.getElementById('main-gateway');
    const routingPage = document.getElementById('main-routing');

    // --- Gateway Page Listeners ---
    gatewayPage.addEventListener('click', async (e) => {
        if (e.target.classList.contains('save-btn') && e.target.dataset.module === 'gateway') {
            const settingsData = getGatewaySettingsData();
            e.target.textContent = 'Saving...';
            e.target.disabled = true;
            try {
                await saveSettings('gateway', settingsData);
                updateStatusMessage(`Successfully saved Gateway settings.`);
            } catch (error) {
                alert(`Error saving Gateway settings: ${error.message}`);
            } finally {
                e.target.textContent = 'Save Gateway Settings';
                e.target.disabled = false;
            }
        }
    });

    // --- Routing Page Listeners ---
    routingPage.addEventListener('click', async (e) => {
        const target = e.target;
        if (target.id === 'add-rule-btn') {
            populateRuleTargetOptions(serversCache);
            showRuleDialog(null, null);
        } else if (target.classList.contains('save-btn') && target.dataset.module === 'routing') {
            const settingsData = getRoutingSettingsData();
            target.textContent = 'Saving...';
            target.disabled = true;
            try {
                await saveSettings('routing', settingsData);
                updateStatusMessage(`Successfully saved Routing settings.`);
            } catch (error) {
                alert(`Error saving Routing settings: ${error.message}`);
            } finally {
                target.textContent = 'Save All Routing Changes';
                target.disabled = false;
            }
        }
    });

    const valueFilterInput = document.getElementById('rule-value-filter');
    const typeFilterSelect = document.getElementById('rule-type-filter');
    const priorityHeader = document.getElementById('priority-header');
    const fetchClientsBtn = document.getElementById('fetch-clients-btn');
    const ruleTypeSelect = document.getElementById('rule-type');
    const ruleValueTextarea = document.getElementById('rule-value');

    valueFilterInput.addEventListener('input', () => { filterText = valueFilterInput.value; renderRulesTable(); });
    typeFilterSelect.addEventListener('change', () => { filterType = typeFilterSelect.value; renderRulesTable(); });
    priorityHeader.addEventListener('click', () => { sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'; renderRulesTable(); });

    // Show/hide fetch button when rule type changes in the dialog
    ruleTypeSelect.addEventListener('change', () => {
        fetchClientsBtn.style.display = ruleTypeSelect.value === 'source_ip' ? 'block' : 'none';
    });

    // Fetch clients button listener
    fetchClientsBtn.addEventListener('click', async () => {
        try {
            fetchClientsBtn.textContent = 'Fetching...';
            fetchClientsBtn.disabled = true;
            const ips = await fetchAvailableClientIPs();
            if (ips.length > 0) {
                ruleValueTextarea.value = ips.join('\n');
                updateStatusMessage(`Fetched ${ips.length} available client IP(s).`);
            } else {
                updateStatusMessage('No new online client IPs found.');
            }
        } catch (error) {
            alert('Error fetching client IPs: ' + error.message);
        } finally {
            fetchClientsBtn.textContent = 'Fetch IPs';
            fetchClientsBtn.disabled = false;
        }
    });

    ruleListBody.addEventListener('click', async (e) => {
        const target = e.target;
        const originalIndex = parseInt(target.dataset.originalIndex, 10);

        if (target.classList.contains('edit-rule-btn')) {
            populateRuleTargetOptions(serversCache);
            showRuleDialog(routingRulesCache[originalIndex], originalIndex);
        } else if (target.classList.contains('delete-rule-btn')) {
            if (confirm('Are you sure you want to delete this rule?')) {
                routingRulesCache.splice(originalIndex, 1);
                renderRulesTable();

                const routingSettingsPayload = getRoutingSettingsData();
                try {
                    updateStatusMessage('Deleting rule from server...');
                    await saveSettings('routing', routingSettingsPayload);
                    updateStatusMessage('Rule deleted successfully.');
                } catch (error) {
                    alert('Error deleting rule: ' + error.message);
                    updateStatusMessage('Failed to delete rule.', 'failed');
                    loadSettings(); // Revert on failure
                }
            }
        }
    });
}