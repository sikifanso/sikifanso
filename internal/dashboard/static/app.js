(function() {
  'use strict';

  var POLL_INTERVAL = 10000;

  function updateStatus() {
    fetch('/api/status')
      .then(function(resp) { return resp.json(); })
      .then(function(data) {
        // Update node count.
        var nodesEl = document.getElementById('nodes-value');
        if (nodesEl) {
          nodesEl.textContent = data.nodesReady + '/' + data.nodeCount;
          nodesEl.className = 'value' + (data.nodesReady === data.nodeCount ? ' ok' : ' warn');
        }

        // Update enabled count.
        var enabledCount = 0;
        for (var i = 0; i < data.catalogApps.length; i++) {
          if (data.catalogApps[i].enabled) enabledCount++;
        }
        var enabledEl = document.getElementById('enabled-value');
        if (enabledEl) {
          enabledEl.textContent = enabledCount + '/' + data.catalogApps.length;
        }

        // Update app rows.
        for (var j = 0; j < data.catalogApps.length; j++) {
          var app = data.catalogApps[j];
          var row = document.getElementById('app-' + app.name);
          if (!row) continue;

          var enabledBadge = row.querySelector('.enabled-badge');
          if (enabledBadge) {
            enabledBadge.textContent = app.enabled ? 'on' : 'off';
            enabledBadge.className = 'badge enabled-badge ' + (app.enabled ? 'on' : 'off');
          }

          var healthBadge = row.querySelector('.health-badge');
          if (healthBadge && app.enabled) {
            healthBadge.textContent = app.health;
            healthBadge.className = 'badge health-badge ' + badgeClass(app.health);
          } else if (healthBadge) {
            healthBadge.textContent = '-';
            healthBadge.className = 'badge health-badge unknown';
          }

          var syncBadge = row.querySelector('.sync-badge');
          if (syncBadge && app.enabled) {
            syncBadge.textContent = app.syncStatus;
            syncBadge.className = 'badge sync-badge ' + badgeClass(app.syncStatus);
          } else if (syncBadge) {
            syncBadge.textContent = '-';
            syncBadge.className = 'badge sync-badge unknown';
          }
        }

        // Update timestamp.
        var tsEl = document.getElementById('last-refresh');
        if (tsEl) {
          tsEl.textContent = 'Last refresh: ' + new Date().toLocaleTimeString();
        }
      })
      .catch(function(err) {
        console.error('status poll failed:', err);
      });
  }

  function badgeClass(status) {
    switch (status) {
      case 'Healthy': case 'Synced': return 'healthy';
      case 'Degraded': case 'Missing': return 'degraded';
      case 'OutOfSync': return 'outofsync';
      default: return 'unknown';
    }
  }

  window.toggleApp = function(name, btn) {
    btn.disabled = true;
    btn.textContent = '...';
    fetch('/api/catalog/' + encodeURIComponent(name) + '/toggle', { method: 'POST' })
      .then(function(resp) {
        if (!resp.ok) throw new Error('toggle failed');
        return resp.json();
      })
      .then(function() {
        updateStatus();
        btn.disabled = false;
        btn.textContent = 'Toggle';
      })
      .catch(function(err) {
        console.error('toggle failed:', err);
        btn.disabled = false;
        btn.textContent = 'Toggle';
      });
  };

  setInterval(updateStatus, POLL_INTERVAL);
})();
