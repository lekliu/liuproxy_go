document.addEventListener('DOMContentLoaded', () => {
    // --- UI Elements ---
    const serverListBody = document.getElementById('server-list');
    const addServerBtn = document.getElementById('add-server-btn');
    const dialog = document.getElementById('server-dialog');
    const form = document.getElementById('server-form');
    const cancelBtn = document.getElementById('cancel-btn');
    const dialogTitle = document.getElementById('dialog-title');
    const serverIdInput = document.getElementById('server-id');
    const statusBar = document.getElementById('status-bar');
    const statusText = document.getElementById('status-text');

    // --- State Management ---
    let statusInterval = null;

    // --- API Functions ---
    async function fetchServers() {
        try {
            const response = await fetch('/api/servers');
            if (!response.ok) throw new Error('Failed to fetch servers');
            const servers = await response.json();
            renderServers(servers || []);
        } catch (error) {
            updateStatus('Error fetching server list', 'failed');
        }
    }

    async function fetchStatus() {
        try {
            const response = await fetch('/api/status');
            if (!response.ok) {
                // stopStatusPolling();
                updateStatus('Failed to get status', 'failed');
                return;
            }
            const data = await response.json();
            const status = data.status || "Unknown";
            updateStatus(status, status.startsWith('Failed') ? 'failed' : (status.startsWith('Connecting') ? 'connecting' : (status.startsWith('Connected') ? 'connected' : 'idle')));

        } catch (error) {
            updateStatus('Error polling status', 'failed');
        }
    }

    // --- UI Update Functions ---

    function renderServers(servers) {
        serverListBody.innerHTML = '';
        servers.forEach(server => {
            const row = document.createElement('tr');
            row.className = server.active ? 'active' : '';
            row.innerHTML = `
                <td>${server.active ? '<span class="status-indicator"></span>' : ''}</td>
                <td>${escapeHTML(server.remarks)}</td>
                <td>${escapeHTML(server.address)}:${escapeHTML(server.port)}</td>
                <td>${escapeHTML(server.type)}</td>
                <td class="actions">
                    <button class="activate-btn" data-id="${server.id}" ${server.active ? 'disabled' : ''}>Activate</button>
                    <button class="edit-btn" data-id="${server.id}">Edit</button>
                    <button class="delete-btn" data-id="${server.id}">Delete</button>
                </td>
            `;
            serverListBody.appendChild(row);
        });
    }

    function updateStatus(message, type) {
        statusText.textContent = message;
        statusBar.className = 'status-bar'; // Reset classes
        statusBar.classList.add(`status-${type}`);
    }

    function startStatusPolling() {
        stopStatusPolling(); // Ensure no multiple intervals are running
        fetchStatus(); // Fetch immediately
        statusInterval = setInterval(fetchStatus, 3000); // Poll every 2 seconds
    }

    function stopStatusPolling() {
        if (statusInterval) {
            clearInterval(statusInterval);
            statusInterval = null;
        }
    }

    function showDialog(server = null) {
        form.reset();
        if (server) {
            dialogTitle.textContent = 'Edit Server';
            serverIdInput.value = server.id;
            document.getElementById('remarks').value = server.remarks;
            document.getElementById('type').value = server.type;
            document.getElementById('address').value = server.address;
            document.getElementById('port').value = server.port;
            document.getElementById('scheme').value = server.scheme;
            document.getElementById('path').value = server.path;
            document.getElementById('edgeIP').value = server.edgeIP;
        } else {
            dialogTitle.textContent = 'Add Server';
            serverIdInput.value = '';
        }
        dialog.showModal();
    }
    
    // --- Event Listeners ---

    addServerBtn.addEventListener('click', () => showDialog());
    cancelBtn.addEventListener('click', () => dialog.close());

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const id = serverIdInput.value;
        const formData = new FormData(form);
        const serverData = Object.fromEntries(formData.entries());
        serverData.port = parseInt(serverData.port, 10);
        serverData.active = false; // Active state is handled by activate endpoint

        const method = id ? 'PUT' : 'POST';
        const url = id ? `/api/servers?id=${id}` : '/api/servers';

        try {
            const response = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(serverData)
            });
            if (!response.ok) throw new Error(`Failed to save server. Status: ${response.status}`);
            dialog.close();
            await fetchServers();
        } catch (error) {
            alert('Error: ' + error.message);
        }
    });

    serverListBody.addEventListener('click', async (e) => {
        const target = e.target;
        const id = target.dataset.id;
        if (!id) return;

        if (target.classList.contains('delete-btn')) {
            if (confirm('Are you sure you want to delete this server?')) {
                try {
                    const response = await fetch(`/api/servers?id=${id}`, { method: 'DELETE' });
                    if (!response.ok) throw new Error('Failed to delete');
                    await fetchServers();
                    startStatusPolling(); // Re-check status after delete
                } catch (error) {
                    alert('Error deleting server: ' + error.message);
                }
            }
        } else if (target.classList.contains('edit-btn')) {
            const response = await fetch('/api/servers');
            const servers = await response.json();
            const serverToEdit = servers.find(s => s.id === id);
            if (serverToEdit) showDialog(serverToEdit);
        } else if (target.classList.contains('activate-btn')) {
            try {
                updateStatus('Activating...', 'connecting');
                const response = await fetch(`/api/servers/activate?id=${id}`, { method: 'POST' });
                if (!response.ok) throw new Error('Failed to activate');
                await fetchServers();
                startStatusPolling(); // Start polling to get real-time connection status
            } catch (error) {
                alert('Error activating server: ' + error.message);
                fetchStatus(); // Fetch final status even on error
            }
        }
    });
    
    function escapeHTML(str) {
        if (!str) return '';
        return str.toString()
            .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;').replace(/'/g, '&#039;');
    }

    // Initial load and status check
    fetchServers();
    startStatusPolling();
});