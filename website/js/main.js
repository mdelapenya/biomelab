// Mobile hamburger menu
var navHamburger = document.getElementById('nav-hamburger');
var navLinks = document.getElementById('nav-links');

function closeNav() {
  navLinks.classList.remove('open');
  navHamburger.setAttribute('aria-expanded', 'false');
}

navHamburger.addEventListener('click', function (e) {
  e.stopPropagation();
  var open = navLinks.classList.toggle('open');
  this.setAttribute('aria-expanded', open);
});

document.addEventListener('click', function (e) {
  if (!navLinks.classList.contains('open')) return;
  if (navLinks.contains(e.target) || navHamburger.contains(e.target)) return;
  closeNav();
});

navLinks.querySelectorAll('a').forEach(function (link) {
  link.addEventListener('click', closeNav);
});

document.getElementById('install-copy').addEventListener('click', function () {
  var text = 'brew install --cask mdelapenya/tap/biomelab';
  navigator.clipboard.writeText(text).then(function () {
    var box = document.getElementById('install-copy');
    box.classList.add('copied');
    var icon = box.querySelector('.copy-icon');
    var orig = icon.innerHTML;
    icon.innerHTML = '<path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/>';
    setTimeout(function () { box.classList.remove('copied'); icon.innerHTML = orig; }, 2000);
  });
});

// Preload alternate theme images so swaps are instant (no flash).
var preloaded = {};
document.querySelectorAll('.themed-img').forEach(function (img) {
  ['srcDark', 'srcLight'].forEach(function (key) {
    var url = img.dataset[key];
    if (url && !preloaded[url]) {
      preloaded[url] = true;
      var link = document.createElement('link');
      link.rel = 'prefetch';
      link.as = 'image';
      link.href = url;
      document.head.appendChild(link);
    }
  });
});

function applyThemeImages(theme) {
  document.querySelectorAll('.themed-img').forEach(function (img) {
    var src = theme === 'light' ? img.dataset.srcLight : img.dataset.srcDark;
    if (src) img.setAttribute('src', src);
  });
}
applyThemeImages(document.documentElement.getAttribute('data-theme'));

document.getElementById('theme-toggle').addEventListener('click', function () {
  var html = document.documentElement;
  var next = html.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
  html.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  applyThemeImages(next);
});

fetch('https://api.github.com/repos/mdelapenya/biomelab')
  .then(function (r) { return r.json(); })
  .then(function (data) {
    if (data.stargazers_count !== undefined) {
      document.getElementById('star-count').textContent = data.stargazers_count.toLocaleString();
    }
  })
  .catch(function () { });

// Copy buttons on install tab panels
document.querySelectorAll('.copy-btn').forEach(function (btn) {
  btn.addEventListener('click', function () {
    navigator.clipboard.writeText(btn.dataset.copy).then(function () {
      btn.classList.add('copied');
      var svg = btn.innerHTML;
      btn.innerHTML = '<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/></svg>';
      setTimeout(function () { btn.classList.remove('copied'); btn.innerHTML = svg; }, 2000);
    });
  });
});

document.querySelectorAll('.tab-bar .tab').forEach(function (tab) {
  tab.addEventListener('click', function () {
    document.querySelectorAll('.tab-bar .tab').forEach(function (t) { t.classList.remove('active'); t.setAttribute('aria-selected', 'false'); });
    document.querySelectorAll('.tab-panel').forEach(function (p) { p.classList.remove('active'); });
    tab.classList.add('active');
    tab.setAttribute('aria-selected', 'true');
    document.getElementById('tab-' + tab.dataset.tab).classList.add('active');
  });
});

// Sandbox diagram: hover a branch in a sandbox → highlight matching worktree in host
document.querySelectorAll('.sbx-wt[data-wt]').forEach(function (wt) {
  wt.addEventListener('mouseenter', function () {
    var name = wt.dataset.wt;
    var match = document.querySelector('.sbx-octo-wt[data-wt="' + name + '"]');
    if (match) {
      wt.classList.add('sbx-wt-active');
      match.classList.add('sbx-octo-active');
    }
  });
  wt.addEventListener('mouseleave', function () {
    var name = wt.dataset.wt;
    var match = document.querySelector('.sbx-octo-wt[data-wt="' + name + '"]');
    if (match) {
      wt.classList.remove('sbx-wt-active');
      match.classList.remove('sbx-octo-active');
    }
  });
});

// Kanban diagram: hover a card → subtle glow effect
document.querySelectorAll('.kb-card[data-kb]').forEach(function (card) {
  card.addEventListener('mouseenter', function () {
    card.classList.add('kb-card-hover');
  });
  card.addEventListener('mouseleave', function () {
    card.classList.remove('kb-card-hover');
  });
});

document.querySelectorAll('a[href^="#"]').forEach(function (anchor) {
  anchor.addEventListener('click', function (e) {
    e.preventDefault();
    var target = document.querySelector(this.getAttribute('href'));
    if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
  });
});

// Shared modal show/hide helpers — the open/close boilerplate
// (toggle [hidden] / .open class, body-class for scroll-lock, the
// 200ms fade-out before hidden) was identical across all five
// modals on the page. The optional hooks let callers slot in the
// only thing that actually differs per modal: setting titles /
// populating bodies on show, cancelling typewriter timers or
// clearing pending callbacks on hide.
function showModal(id, onShow) {
  var modal = document.getElementById(id);
  if (!modal) return;
  if (onShow) onShow(modal);
  modal.removeAttribute('hidden');
  document.body.classList.add('rgt-modal-open');
  requestAnimationFrame(function () { modal.classList.add('open'); });
}
function hideModal(id, onHide) {
  var modal = document.getElementById(id);
  if (!modal) return;
  if (onHide) onHide();
  modal.classList.remove('open');
  document.body.classList.remove('rgt-modal-open');
  setTimeout(function () { modal.setAttribute('hidden', ''); }, 200);
}

// Regent activity modal — click any kanban card (works in both
// kanban and grid views since the DOM nodes are the same) or press
// 'l' to open a WhatsApp-style log mirroring the biomelab GUI.
(function () {
  var modal = document.getElementById('rgt-modal');
  if (!modal) return;
  var body = modal.querySelector('.rgt-modal-body');
  var title = modal.querySelector('#rgt-modal-title');
  var exportBtn = modal.querySelector('.rgt-modal-export');
  var lastBranch = 'feat/auth-flow';

  function open(branch) {
    lastBranch = branch || lastBranch;
    showModal('rgt-modal', function () {
      title.textContent = 'Regent activity — ' + lastBranch;
      body.innerHTML = renderSteps(SAMPLE);
      body.scrollTop = 0;
    });
  }

  function close() { hideModal('rgt-modal'); }

  modal.querySelectorAll('[data-rgt-close]').forEach(function (el) {
    el.addEventListener('click', close);
  });

  // Only Esc closes the modal — opening is click-only on the web.
  // The 'l' shortcut exists in the desktop app (where every card
  // has a clear "selected" state); on the web a stray keypress on
  // the landing page would pop the modal unexpectedly.
  document.addEventListener('keydown', function (e) {
    if (modal.hasAttribute('hidden')) return;
    if (e.key === 'Escape') close();
  });

  // Card click handlers are attached by the keyboard simulator IIFE
  // (its wireCard) so existing and dynamically-created cards share
  // one wiring path. We just expose open() here for it to call.
  window.kbDemo = window.kbDemo || {};
  window.kbDemo.openLog = open;

  body.addEventListener('click', function (e) {
    var btn = e.target.closest('.rgt-tools-toggle');
    if (!btn) return;
    var target = document.getElementById(btn.dataset.target);
    if (!target) return;
    var caret = btn.querySelector('span');
    if (target.hasAttribute('hidden')) {
      target.removeAttribute('hidden');
      if (caret) caret.textContent = '▼';
    } else {
      target.setAttribute('hidden', '');
      if (caret) caret.textContent = '▶';
    }
  });

  exportBtn.addEventListener('click', function () {
    var payload = { session_id: 'demo-' + lastBranch, steps: SAMPLE };
    var blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = 'regent-log-' + lastBranch.replace(/\//g, '-') + '.json';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
  });

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[c];
    });
  }
  function nl2br(s) { return esc(s).replace(/\n/g, '<br>'); }

  function renderSteps(steps) {
    return steps.map(function (s, i) {
      return '<div class="rgt-step">'
        + '<div class="rgt-meta">● ' + esc(s.hash) + ' · ' + esc(s.timestamp) + ' · ' + esc(s.origin) + '</div>'
        + (s.human ? '<div class="rgt-row rgt-row-right"><div class="rgt-bubble rgt-bubble-human">' + nl2br(s.human) + '</div></div>' : '')
        + (s.agent || (s.tools && s.tools.length) ? renderAgent(s, i) : '')
        + '</div>';
    }).join('');
  }

  function renderAgent(s, i) {
    var hasTools = s.tools && s.tools.length;
    var id = 'rgt-tools-' + i;
    return '<div class="rgt-row rgt-row-left">'
      + '<div class="rgt-bubble rgt-bubble-agent">'
      + (s.agent ? '<div class="rgt-msg">' + nl2br(s.agent) + '</div>' : '')
      + (hasTools ? '<button class="rgt-tools-toggle" data-target="' + id + '"><span>▶</span> ' + s.tools.length + ' tool' + (s.tools.length === 1 ? '' : 's') + '</button>' : '')
      + '</div></div>'
      + (hasTools ? '<div class="rgt-bubble rgt-bubble-tools" id="' + id + '" hidden>' + renderTools(s.tools) + '</div>' : '');
  }

  function renderTools(tools) {
    return tools.map(function (t) {
      var args = Object.keys(t.args || {}).sort();
      return '<div class="rgt-tool">'
        + '<div class="rgt-tool-name">└─ ' + esc(t.name) + '</div>'
        + (args.length ? '<div class="rgt-tool-bubble">'
            + args.map(function (k) {
                return '<div class="rgt-tool-arg">   ' + esc(k) + ': ' + esc(String(t.args[k])) + '</div>';
              }).join('')
            + '</div>' : '')
        + '</div>';
    }).join('');
  }

  var SAMPLE = [
    {
      hash: 'a1b2c3d4',
      timestamp: '2026-05-19 14:02:11',
      origin: 'claude_code',
      human: 'Add an integration test for the user repository using testcontainers-go — Postgres 16, run the migration suite against it, then a happy-path Create/Get.',
      agent: 'Wired testcontainers-go: added a Postgres fixture under tests/fixtures/postgres.go that boots postgres:16-alpine, applies migrations, and returns a *pgxpool.Pool. Switched user_repo_integration_test.go to use it. 14 cases green with -race.',
      tools: [
        { name: 'Read', args: { file_path: '/workspace/go.mod' } },
        { name: 'Bash', args: {
            command: 'go get github.com/testcontainers/testcontainers-go/modules/postgres@latest',
            description: 'Add the Postgres testcontainers module'
          } },
        { name: 'Write', args: {
            file_path: '/workspace/tests/fixtures/postgres.go',
            content: 'package fixtures\n\nimport (\n    "context"\n    "testing"\n\n    "github.com/jackc/pgx/v5/pgxpool"\n    "github.com/testcontainers/testcontainers-go/modules/postgres"\n)\n\nfunc Postgres(t *testing.T) *pgxpool.Pool { /* ... */ }'
          } },
        { name: 'Edit', args: {
            file_path: '/workspace/internal/user/user_repo_integration_test.go',
            old_string: 'pool := mockPool(t)',
            new_string: 'pool := fixtures.Postgres(t)'
          } },
        { name: 'Bash', args: {
            command: 'go test -race -run TestUserRepo ./internal/user/...',
            description: 'Run user repo integration tests with the race detector'
          } }
      ]
    },
    {
      hash: 'e5f6g7h8',
      timestamp: '2026-05-19 14:07:43',
      origin: 'claude_code',
      human: 'Verify this Docker Sandbox can\'t reach the public internet beyond the allowlist — try curling api.example.com and github.com from inside, make sure only github resolves.',
      agent: 'Confirmed: api.example.com fails at DNS resolution (the sbx network policy filters it), github.com:443 returns 200. The sandbox spec already pins egress to {github.com, proxy.golang.org}; no change needed.',
      tools: [
        { name: 'Read', args: { file_path: '/workspace/sandbox.spec.yaml' } },
        { name: 'Bash', args: {
            command: 'sbx exec my-sandbox -- curl -sS -o /dev/null -w "%{http_code}\\n" https://api.example.com',
            description: 'Try to reach a non-allowlisted host from inside the sandbox'
          } },
        { name: 'Bash', args: {
            command: 'sbx exec my-sandbox -- curl -sS -o /dev/null -w "%{http_code}\\n" https://github.com',
            description: 'Confirm github.com resolves through the allowlist'
          } }
      ]
    },
    {
      hash: '9c0d1f2a',
      timestamp: '2026-05-19 14:14:08',
      origin: 'claude_code',
      human: 'Same testcontainers pattern for Redis — the cache layer mock has hidden a JSON-encoding bug twice already this quarter.',
      agent: 'Mirrored the Postgres fixture for Redis 7.2 and switched cache_test.go over. Tests went from 50ms (mock) to ≈1.4s (real Redis) but immediately caught the encoding bug — the mock was lenient on []byte vs string keys. Added a regression test for it.',
      tools: [
        { name: 'Read', args: { file_path: '/workspace/tests/fixtures/postgres.go' } },
        { name: 'Write', args: {
            file_path: '/workspace/tests/fixtures/redis.go',
            content: 'package fixtures\n\nimport (\n    "context"\n    "testing"\n\n    "github.com/redis/go-redis/v9"\n    "github.com/testcontainers/testcontainers-go/modules/redis"\n)\n\nfunc Redis(t *testing.T) *redis.Client { /* ... */ }'
          } },
        { name: 'Edit', args: {
            file_path: '/workspace/internal/cache/cache_test.go',
            old_string: 'client := mockRedis(t)',
            new_string: 'client := fixtures.Redis(t)'
          } },
        { name: 'Bash', args: {
            command: 'go test -race -run TestCache ./internal/cache/...',
            description: 'Run cache tests against the real Redis container'
          } }
      ]
    }
  ];
})();

// Kanban / grid toggle — press 'g' to switch views
(function () {
  var diagram = document.querySelector('.kanban-diagram-html');
  var title   = document.querySelector('#kb-title');
  var hint    = document.querySelector('.kb-footer-hint .kb-footer-mode');
  var STORE_KEY = 'kb-view';

  function getHintHTML(mode) {
    if (mode === 'grid') {
      return 'Press <kbd class="kb-g-glow">g</kbd> to return to kanban board';
    } else {
      return 'Try <kbd class="kb-g-glow">g</kbd> for grid view';
    }
  }

  function setView(mode) {
    // The diagram title stays as the page's inviting CTA in both
    // views — only the layout class and the footer hint flip on the
    // 'g' toggle.
    if (mode === 'grid') {
      diagram.classList.add('kb-view-grid');
      hint.innerHTML = getHintHTML('grid');
    } else {
      diagram.classList.remove('kb-view-grid');
      hint.innerHTML = getHintHTML('kanban');
    }
    localStorage.setItem(STORE_KEY, mode);
  }

  // Restore saved preference
  var saved = localStorage.getItem(STORE_KEY);
  if (saved === 'grid') setView('grid');

  document.addEventListener('keydown', function (e) {
    // Ignore when focus is in an input / textarea
    var tag = document.activeElement && document.activeElement.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    if (e.key === 'g' || e.key === 'G') {
      setView(diagram.classList.contains('kb-view-grid') ? 'kanban' : 'grid');
    }
  });
})();

// ────────────────────────────────────────────────────────────────────
// Keyboard simulator — make the keyboard grid an interactive demo
// backed by a shared STATE.cards source of truth. Both the kanban
// board (columns) and the grid view (flat) render from the same
// model, so any mutation (create, send-PR, delete) shows up
// consistently in either view when you toggle with `g`.
// ────────────────────────────────────────────────────────────────────
(function () {
  var board = document.querySelector('.kanban-diagram-html');
  if (!board) return;

  // ── Source of truth ────────────────────────────────────────────
  //
  // Each entry models one biomelab worktree card. Stage drives which
  // kanban column it lives in; prState / ciState drive the badges.
  // Bootstrapped from the handcrafted DOM that ships in index.html,
  // then re-rendered from this model after every mutation.
  var STATE = {
    cards: [],
    selectedCardId: null,
    nextSampleN: 1,
    mainSync: 'up' // 'up' | 'behind' — drives the main card's sync pill
  };

  var STAGES = ['closed', 'created', 'sent', 'in-review', 'merged'];
  var STAGE_LABEL = {
    closed: 'closed', created: 'created',
    sent: 'PR sent', 'in-review': 'in review', merged: 'merged'
  };
  var STAGE_DOT = {
    closed: 'red', created: 'gray', sent: 'blue',
    'in-review': 'yellow', merged: 'green'
  };

  function cssEsc(s) { return String(s).replace(/[\\"]/g, '\\$&'); }

  // ── Parse initial DOM into STATE.cards ─────────────────────────

  function stageOfCol(col) {
    var cl = col.className;
    if (/kb-col-closed/.test(cl))    return 'closed';
    if (/kb-col-created/.test(cl))   return 'created';
    if (/kb-col-sent/.test(cl))      return 'sent';
    if (/kb-col-in-review/.test(cl)) return 'in-review';
    if (/kb-col-merged/.test(cl))    return 'merged';
    return 'created';
  }

  function parseCard(el, stage) {
    var agent = null;
    var ag = el.querySelector('.kb-agent');
    if (ag && !ag.classList.contains('kb-agent-dim')) {
      agent = ag.textContent.replace(/[●○]\s*/g, '').trim() || null;
    }
    var prNumber = null, prState = null, ciState = null;
    var prNum = el.querySelector('.kb-pr-num');
    if (prNum) prNumber = parseInt(prNum.textContent.replace('#', ''), 10) || null;
    var stateSpan = el.querySelector('[class*="kb-pr-state-"]');
    if (stateSpan) {
      var ms = stateSpan.className.match(/kb-pr-state-(\w+)/);
      if (ms) prState = ms[1];
    }
    var ci = el.querySelector('[class*="kb-ci-"]:not(.kb-ci-empty)');
    if (ci) {
      var mc = ci.className.match(/kb-ci-(\w+)/);
      if (mc) ciState = mc[1];
    }
    return {
      id: el.dataset.kb,
      branch: el.dataset.kb,
      agent: agent,
      prNumber: prNumber,
      prState: prState,
      ciState: ciState,
      stage: stage
    };
  }

  function parseInitialState() {
    var cards = [];
    document.querySelectorAll('.kb-board .kb-col .kb-card[data-kb]').forEach(function (el) {
      var col = el.closest('.kb-col');
      if (!col) return;
      cards.push(parseCard(el, stageOfCol(col)));
    });
    return cards;
  }

  // ── Render board + grid from STATE.cards ───────────────────────

  function cardHTML(c, mode) {
    var dot = (c.prState === 'draft') ? 'dim' : (STAGE_DOT[c.stage] || 'gray');
    var draftClass = (c.prState === 'draft') ? ' kb-card-draft' : '';
    var closedClass = (c.stage === 'closed') ? ' kb-card-closed' : '';
    var branchDim = (c.prState === 'draft') ? ' kb-branch-dim' : '';

    var top = '<div class="kb-card-top">'
      + '<span class="kb-dot kb-dot-' + dot + '"></span>';
    if (c.editing) {
      // Inline-edit branch name: replaces the static span with a text
      // input until the user commits with Enter (or blur). Esc cancels
      // and removes the half-created card.
      top += '<input class="kb-branch-edit" value="' + esc(c.branch) + '" spellcheck="false">';
    } else {
      top += '<span class="kb-branch' + branchDim + '">' + esc(c.branch) + '</span>';
    }
    if (mode === 'grid') {
      top += '<span class="kb-stage-pill kb-stage-' + c.stage + '">'
        + esc(STAGE_LABEL[c.stage] || c.stage) + '</span>';
    }
    top += '</div>';

    var meta = '<div class="kb-card-meta">'
      + (c.agent
          ? '<span class="kb-agent kb-agent-green">● ' + esc(c.agent) + '</span>'
          : '<span class="kb-agent kb-agent-dim">○ no agent</span>')
      + '</div>';

    var status = '<div class="kb-card-status">';
    if (c.prNumber) {
      var ps = c.prState || 'open';
      status += '<span class="kb-pr-label">PR <span class="kb-pr-num">#' + c.prNumber + '</span> '
        + '<span class="kb-pr-state-' + ps + '">' + esc(ps) + '</span></span>';
      if (c.ciState) {
        var icon = c.ciState === 'pass' ? '✓' : c.ciState === 'fail' ? '✗' : '●';
        var title = c.ciState === 'pass' ? 'CI passed' : c.ciState === 'fail' ? 'CI failed' : 'CI pending';
        status += '<span class="kb-ci kb-ci-' + c.ciState + '" title="' + title + '">' + icon + '</span>';
      }
    } else {
      status += '<span class="kb-no-pr">no PR</span>';
    }
    status += '</div>';

    return '<div class="kb-card' + closedClass + draftClass + '" data-kb="' + esc(c.id) + '">'
      + top + meta + status + '</div>';
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[c];
    });
  }

  function renderBoard() {
    STAGES.forEach(function (stage) {
      var col = document.querySelector('.kb-col-' + stage);
      if (!col) return;
      col.querySelectorAll('.kb-card').forEach(function (c) { c.remove(); });
      var inStage = STATE.cards.filter(function (c) { return c.stage === stage; });
      inStage.forEach(function (c) {
        col.insertAdjacentHTML('beforeend', cardHTML(c, 'board'));
      });
      var badge = col.querySelector('.kb-col-badge');
      if (badge) badge.textContent = String(inStage.length);
    });
  }

  function renderGrid() {
    var grid = document.querySelector('.kb-grid');
    if (!grid) return;
    grid.querySelectorAll('.kb-card').forEach(function (c) { c.remove(); });
    STATE.cards.forEach(function (c) {
      grid.insertAdjacentHTML('beforeend', cardHTML(c, 'grid'));
    });
  }

  function renderAll() {
    renderBoard();
    renderGrid();
    document.querySelectorAll('.kb-card[data-kb]').forEach(wireCard);
    if (STATE.selectedCardId) applySelection(STATE.selectedCardId);
  }

  // ── Selection ──────────────────────────────────────────────────

  function applySelection(id) {
    document.querySelectorAll('.kb-selected').forEach(function (el) {
      el.classList.remove('kb-selected');
    });
    if (!id) return;
    document.querySelectorAll('[data-kb="' + cssEsc(id) + '"]').forEach(function (el) {
      el.classList.add('kb-selected');
    });
  }

  function selectCard(id) {
    STATE.selectedCardId = id;
    applySelection(id);
  }

  function getCard(id) {
    for (var i = 0; i < STATE.cards.length; i++) {
      if (STATE.cards[i].id === id) return STATE.cards[i];
    }
    return null;
  }

  function pulseCard(id) {
    document.querySelectorAll('[data-kb="' + cssEsc(id) + '"]').forEach(function (el) {
      el.classList.add('kb-pulse');
      setTimeout(function () { el.classList.remove('kb-pulse'); }, 620);
    });
  }

  // ── Main worktree sync cycle ───────────────────────────────────
  //
  // Every 5 s the main card drifts to "behind" so the user has a
  // genuine reason to press 'p'. Pulling sets it back to "up-to-date"
  // and restarts the timer so the next drift happens 5 s from the
  // pull (not mid-toast). The number-behind is a fixed demo value;
  // in the real app it'd come from `git rev-list --count
  // origin/main..HEAD`.
  var MAIN_DRIFT_MS = 5000;
  var mainSyncTimer = null;

  function setMainSync(state) {
    STATE.mainSync = state;
    var syncEl = document.querySelector('.kb-main-card .kb-sync');
    if (!syncEl) return;
    syncEl.classList.remove('kb-sync-up', 'kb-sync-behind');
    if (state === 'behind') {
      // Random 1-5: keeps the demo feeling alive each cycle instead of
      // always showing the same number.
      var n = Math.floor(Math.random() * 5) + 1;
      syncEl.classList.add('kb-sync-behind');
      syncEl.textContent = '↓ ' + n + ' behind';
    } else {
      syncEl.classList.add('kb-sync-up');
      syncEl.textContent = '↕ up-to-date';
    }
    pulseCard('main');
  }

  function startMainSyncCycle() {
    if (mainSyncTimer) clearInterval(mainSyncTimer);
    mainSyncTimer = setInterval(function () {
      if (STATE.mainSync === 'up') setMainSync('behind');
    }, MAIN_DRIFT_MS);
  }

  function pullMain() {
    if (STATE.mainSync === 'up') {
      showToast('Main already up-to-date');
      return;
    }
    showToast('Pulling from remote…');
    // Flip the sync pill to "up-to-date" only when the toast finishes,
    // so the UI shows a real "pulling" window — the toast is the
    // progress indicator, and the green state lands as it fades out.
    // The follow-up startMainSyncCycle restart pushes the next drift
    // a fresh interval away from THIS moment.
    setTimeout(function () {
      setMainSync('up');
      startMainSyncCycle();
    }, TOAST_VISIBLE_MS);
  }

  // ── Toast — bottom-right transient feedback ────────────────────

  var TOAST_VISIBLE_MS = 1700;
  var toast = document.getElementById('kb-toast');
  var toastTimer = null;
  function showToast(msg) {
    if (!toast) return;
    toast.textContent = msg;
    toast.removeAttribute('hidden');
    requestAnimationFrame(function () { toast.classList.add('open'); });
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      toast.classList.remove('open');
      setTimeout(function () { toast.setAttribute('hidden', ''); }, 220);
    }, TOAST_VISIBLE_MS);
  }

  // ── Project sandbox setup demo ────────────────────────────────
  // This is deliberately a small, self-contained simulation: it explains
  // the project-level sbx setup without pretending to call Docker or sbx.
  var sandboxDemo = null;
  var sandboxDemoLastFocus = null;
  var sandboxDemoConfig = null;
  var SANDBOX_AGENTS = {
    claude: { label: 'Claude Code', kits: ['docker.io/sbx/code-server-kit:latest', 'docker.io/sbx/playwright-kit:latest'] },
    codex: { label: 'Codex', kits: ['docker.io/sbx/playwright-kit:latest'] },
    copilot: { label: 'GitHub Copilot', kits: ['docker.io/sbx/playwright-kit:latest'] }
  };

  function sandboxFocusables() {
    return sandboxDemo ? sandboxDemo.querySelectorAll('button, select, input:not([disabled])') : [];
  }
  function closeSandboxDemo() {
    if (!sandboxDemo) return;
    sandboxDemo.remove();
    sandboxDemo = null;
    sandboxDemoConfig = null;
    document.body.classList.remove('rgt-modal-open');
    if (sandboxDemoLastFocus && document.contains(sandboxDemoLastFocus)) sandboxDemoLastFocus.focus();
  }
  function sandboxState() {
    var agentSelect = sandboxDemo.querySelector('#sbx-demo-agent');
    var kitsChoice = sandboxDemo.querySelector('input[name="sbx-demo-kits"]:checked');
    if (agentSelect) sandboxDemoConfig.agent = agentSelect.value;
    if (kitsChoice) sandboxDemoConfig.addKits = kitsChoice.value === 'yes';
    var kitInputs = sandboxDemo.querySelectorAll('input[name="sbx-demo-kit"]');
    if (kitInputs.length) {
      sandboxDemoConfig.kits = Array.prototype.map.call(
        sandboxDemo.querySelectorAll('input[name="sbx-demo-kit"]:checked'), function (input) { return input.value; }
      );
    }
    return sandboxDemoConfig;
  }
  function renderSandboxDemo(step) {
    var state = sandboxState();
    var agent = SANDBOX_AGENTS[state.agent];
    var kitArgs = state.kits.map(function (kit) { return '--kit ' + kit; }).join(' ');
    var body = sandboxDemo.querySelector('.sbx-demo-body');
    if (step === 'success') {
      body.innerHTML = '<div class="sbx-demo-success" role="status">'
        + '<span aria-hidden="true">✓</span><div><h4>Sandbox created (simulated)</h4>'
        + '<p>No backend was called. In biomelab, this project environment would now be available to all ' + agent.label + ' worktrees.</p></div></div>';
      sandboxDemo.querySelector('.sbx-demo-foot').innerHTML = '<button class="confirm-btn confirm-yes" data-sbx-demo-done>Done</button>';
      sandboxDemo.querySelector('[data-sbx-demo-done]').focus();
      return;
    }
    if (step === 'confirm') {
      body.innerHTML = '<p class="sbx-demo-copy">Review the project environment before creating it.</p>'
        + '<dl class="sbx-demo-summary"><div><dt>Project</dt><dd><code>biomelab</code> <span>demo board</span></dd></div>'
        + '<div><dt>Agent</dt><dd>' + agent.label + '</dd></div><div><dt>Kits</dt><dd>'
        + (state.addKits && state.kits.length ? state.kits.map(function (kit) { return '<code>' + kit + '</code>'; }).join('<br>') : 'No kits added') + '</dd></div></dl>'
        + '<p class="sbx-demo-command"><span>Would run</span><code>sbx create --name biomelab-' + state.agent + (kitArgs ? ' ' + kitArgs : '') + ' ' + state.agent + ' /workspace/biomelab</code></p>'
        + '<p class="sbx-demo-note">Image references are illustrative OCI images; nothing will be pulled in this demo.</p>';
      sandboxDemo.querySelector('.sbx-demo-foot').innerHTML = '<button class="confirm-btn confirm-no" data-sbx-demo-back>Back</button><button class="confirm-btn confirm-yes" data-sbx-demo-create>Create sandbox</button>';
      sandboxDemo.querySelector('[data-sbx-demo-create]').focus();
      return;
    }
    body.innerHTML = '<p class="sbx-demo-copy"><strong>Demo setup</strong> · Project <code>biomelab</code> on this board. One environment is shared by that agent\'s worktrees.</p>'
      + '<label class="sbx-demo-field" for="sbx-demo-agent">Agent<select id="sbx-demo-agent"><option value="claude">Claude Code</option><option value="codex">Codex</option><option value="copilot">GitHub Copilot</option></select></label>'
      + '<fieldset class="sbx-demo-field"><legend>Do you want to add kits?</legend><label><input type="radio" name="sbx-demo-kits" value="no"> No</label><label><input type="radio" name="sbx-demo-kits" value="yes"> Yes</label></fieldset>'
      + '<div class="sbx-demo-kits" hidden></div>';
    sandboxDemo.querySelector('#sbx-demo-agent').value = state.agent;
    sandboxDemo.querySelector('input[name="sbx-demo-kits"][value="' + (state.addKits ? 'yes' : 'no') + '"]').checked = true;
    sandboxDemo.querySelector('.sbx-demo-foot').innerHTML = '<button class="confirm-btn confirm-no" data-sbx-demo-cancel>Cancel</button><button class="confirm-btn confirm-yes" data-sbx-demo-continue>Continue</button>';
    function renderKits(current) {
      var kits = sandboxDemo.querySelector('.sbx-demo-kits');
      kits.hidden = !current.addKits;
      kits.innerHTML = current.addKits ? '<strong>Compatible illustrative kits</strong>'
        + SANDBOX_AGENTS[current.agent].kits.map(function (kit) {
          return '<label><input type="checkbox" name="sbx-demo-kit" value="' + kit + '"' + (current.kits.indexOf(kit) !== -1 ? ' checked' : '') + '> <code>' + kit + '</code></label>';
        }).join('') : '';
      kits.querySelectorAll('input[name="sbx-demo-kit"]').forEach(function (input) {
        input.addEventListener('change', function () { sandboxState(); });
      });
    }
    function updateKits() { renderKits(sandboxState()); }
    renderKits(state);
    sandboxDemo.querySelector('#sbx-demo-agent').addEventListener('change', updateKits);
    sandboxDemo.querySelectorAll('input[name="sbx-demo-kits"]').forEach(function (input) { input.addEventListener('change', updateKits); });
    sandboxDemo.querySelector('#sbx-demo-agent').focus();
  }
  function openSandboxDemo() {
    if (sandboxDemo) return;
    sandboxDemoLastFocus = document.activeElement;
    sandboxDemoConfig = { agent: 'claude', addKits: false, kits: [] };
    sandboxDemo = document.createElement('div');
    sandboxDemo.className = 'rgt-modal sbx-demo-modal';
    sandboxDemo.setAttribute('role', 'dialog');
    sandboxDemo.setAttribute('aria-modal', 'true');
    sandboxDemo.setAttribute('aria-labelledby', 'sbx-demo-title');
    sandboxDemo.innerHTML = '<div class="rgt-modal-overlay" data-sbx-demo-cancel></div><div class="rgt-modal-window">'
      + '<header class="rgt-modal-head"><h3 id="sbx-demo-title">New project sandbox <span>DEMO</span></h3><button class="rgt-modal-close" data-sbx-demo-cancel aria-label="Close">×</button></header>'
      + '<div class="rgt-modal-body sbx-demo-body"></div><footer class="rgt-modal-foot sbx-demo-foot"></footer></div>';
    document.body.appendChild(sandboxDemo);
    document.body.classList.add('rgt-modal-open');
    requestAnimationFrame(function () { if (sandboxDemo) sandboxDemo.classList.add('open'); });
    renderSandboxDemo('setup');
  }

  // Capture phase is important: the board's global key handlers were
  // registered earlier. While this dialog is up, no key (even from a SELECT)
  // can leak through to create/delete/toggle board actions. Modifier shortcuts
  // retain their browser default because we only stop propagation for them.
  document.addEventListener('keydown', function (e) {
    if (!sandboxDemo) return;
    if (e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); closeSandboxDemo(); return; }
    if (e.key === 'Tab') {
      var items = sandboxFocusables();
      var first = items[0], last = items[items.length - 1];
      if (items.length && ((e.shiftKey && document.activeElement === first) || (!e.shiftKey && document.activeElement === last))) {
        e.preventDefault(); (e.shiftKey ? last : first).focus();
      }
    }
    e.stopImmediatePropagation();
  }, true);
  document.addEventListener('click', function (e) {
    if (!sandboxDemo) return;
    if (e.target.closest('[data-sbx-demo-cancel], [data-sbx-demo-done]')) { closeSandboxDemo(); return; }
    if (e.target.closest('[data-sbx-demo-continue]')) { renderSandboxDemo('confirm'); return; }
    if (e.target.closest('[data-sbx-demo-back]')) { renderSandboxDemo('setup'); return; }
    if (e.target.closest('[data-sbx-demo-create]')) { renderSandboxDemo('success'); }
  });

  // ── Actions ────────────────────────────────────────────────────

  function createCard() {
    var n = STATE.nextSampleN++;
    var branch = 'feat/demo-' + n;
    STATE.cards.push({
      id: branch, branch: branch, agent: 'claude',
      prNumber: null, prState: null, ciState: null,
      stage: 'created',
      editing: true
    });
    STATE.selectedCardId = branch;
    renderAll();
    pulseCard(branch);
    focusBranchEdit(branch);
    showToast('Name your worktree · Enter to confirm');
  }

  // focusBranchEdit puts the cursor in the inline branch input of the
  // just-created card so the user can type a real name immediately.
  // Enter commits + leaves editing mode; Esc cancels and removes the
  // card; blur also commits (clicking elsewhere). selectionStart/End
  // place the caret at the end so users can append-or-overwrite at will.
  function focusBranchEdit(currentId) {
    var input = document.querySelector('.kb-card[data-kb="' + cssEsc(currentId) + '"] .kb-branch-edit');
    if (!input) return;
    input.focus();
    input.setSelectionRange(0, input.value.length); // select-all for easy overwrite

    var committed = false;
    function commit() {
      if (committed) return;
      committed = true;
      var newName = input.value.trim();
      var card = getCard(currentId);
      if (!card) return;
      if (!newName) {
        // Empty name → treat as cancel and drop the card.
        STATE.cards = STATE.cards.filter(function (c) { return c.id !== currentId; });
        STATE.selectedCardId = null;
        renderAll();
        return;
      }
      // Update id + branch. selectedCardId follows the rename so
      // subsequent d/m/l/P still target this card.
      card.id = newName;
      card.branch = newName;
      card.editing = false;
      STATE.selectedCardId = newName;
      renderAll();
      pulseCard(newName);
    }
    function cancel() {
      if (committed) return;
      committed = true;
      STATE.cards = STATE.cards.filter(function (c) { return c.id !== currentId; });
      STATE.selectedCardId = null;
      renderAll();
    }
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); commit(); }
      else if (e.key === 'Escape') { e.preventDefault(); cancel(); }
    });
    input.addEventListener('blur', commit);
  }

  function deleteCard() {
    if (!STATE.selectedCardId) { showToast('Select a card first (click one)'); return; }
    if (STATE.selectedCardId === 'main') {
      showToast('Cannot delete the main worktree');
      return;
    }
    var id = STATE.selectedCardId;
    openConfirmModal(
      'Delete worktree',
      "Delete worktree '" + id + "'?\n\nThis removes the directory, branch, and metadata.",
      'Delete',
      function () {
        STATE.cards = STATE.cards.filter(function (c) { return c.id !== id; });
        STATE.selectedCardId = null;
        renderAll();
        showToast('Deleted worktree: ' + id);
      }
    );
  }

  // sendPR: only valid on cards in 'created'. Assigns a fresh PR
  // number, sets prState/ciState to open/pending, and moves the card
  // to the 'sent' column. Both views re-render so the transition is
  // visible whether the user is on kanban or grid.
  function sendPR() {
    if (!STATE.selectedCardId) { showToast('Select a card first (click one)'); return; }
    if (STATE.selectedCardId === 'main') {
      showToast('Main is the parent worktree — no PR flow');
      return;
    }
    var card = getCard(STATE.selectedCardId);
    if (!card) return;
    if (card.stage !== 'created') {
      showToast('Send PR only works on cards in Created');
      return;
    }
    card.prNumber = nextPRNumber();
    card.prState = 'open';
    card.ciState = 'pending';
    card.stage = 'sent';
    renderAll();
    pulseCard(card.id);
    showToast('Pushed branch · opened PR #' + card.prNumber);
  }

  function nextPRNumber() {
    var max = 0;
    STATE.cards.forEach(function (c) { if (c.prNumber && c.prNumber > max) max = c.prNumber; });
    return max + 1;
  }

  function refreshAll() {
    document.querySelectorAll('.kb-card, .kb-main-card').forEach(function (c) {
      c.classList.add('kb-pulse');
      setTimeout(function () { c.classList.remove('kb-pulse'); }, 620);
    });
    showToast('Refreshing cards…');
  }

  function openLogModal() {
    if (!STATE.selectedCardId) { showToast('Select a card first (click one)'); return; }
    if (window.kbDemo && window.kbDemo.openLog) {
      window.kbDemo.openLog(STATE.selectedCardId);
    }
  }

  function openNoteModal() {
    if (!STATE.selectedCardId) { showToast('Select a card first (click one)'); return; }
    showModal('note-modal', function () {
      document.getElementById('note-modal-title').textContent = 'Note — ' + STATE.selectedCardId;
      var entry = document.getElementById('note-entry');
      var preview = document.getElementById('note-preview');
      var sample =
        '# ' + STATE.selectedCardId + '\n\n'
        + '**TODO** — write the task description here.\n\n'
        + '- Use *Markdown* — bold, italic, `inline code`\n'
        + '- Live preview on the right →\n'
        + '- Saved alongside the worktree as `.biomelab/note.md`\n\n'
        + 'When you `Shift+P` to send the PR, biomelab uses this note as the body.';
      entry.value = sample;
      preview.innerHTML = renderMd(sample);
      entry.oninput = function () { preview.innerHTML = renderMd(entry.value); };
    });
  }

  function closeNoteModal() { hideModal('note-modal'); }

  // ── '?' keyboard shortcuts modal ──────────────────────────────
  //
  // The .keyboard-grid lives inside this modal (single source of
  // truth — the standalone landing-page section was removed in favor
  // of the modal-only flow). Toggle behavior on '?': open if closed,
  // close if open. Esc also closes.
  function openHelpModal()  { showModal('kb-help-modal'); }
  function closeHelpModal() { hideModal('kb-help-modal'); }

  // ── Confirm dialog ─────────────────────────────────────────────
  //
  // Generic Yes/No prompt mirroring biomelab's showConfirmDelete.
  // openConfirmModal stores the success callback in pendingConfirm;
  // confirmYes invokes it, anything else (Cancel / Esc / overlay
  // click / ×) just closes without firing.
  var pendingConfirm = null;
  function openConfirmModal(title, message, yesLabel, onYes) {
    pendingConfirm = onYes || null;
    showModal('confirm-modal', function () {
      document.getElementById('confirm-modal-title').textContent = title;
      document.getElementById('confirm-modal-body').textContent = message;
      var yesBtn = document.querySelector('.confirm-yes');
      if (yesBtn && yesLabel) yesBtn.textContent = yesLabel;
    });
  }
  function closeConfirmModal() {
    hideModal('confirm-modal', function () { pendingConfirm = null; });
  }
  function confirmYes() {
    var fn = pendingConfirm;
    pendingConfirm = null;
    closeConfirmModal();
    if (fn) fn();
  }

  // ── Fake terminal (Matrix opening) ─────────────────────────────
  //
  // Opens on Enter (or ⏎ cap click) when a card is selected. Types
  // out the iconic Matrix intro lines character-by-character. Purely
  // playful — the real biomelab opens a host Terminal window.

  var TERM_LINES = [
    'Wake up, Neo...',
    'The Matrix has you...',
    'Follow the white rabbit.',
    '',
    'Knock, knock, Neo.'
  ];
  var termTimer = null;

  function openTerminalModal() {
    showModal('term-modal', function () {
      document.getElementById('term-modal-title').textContent =
        STATE.selectedCardId ? 'Terminal — ' + STATE.selectedCardId : 'Terminal';
      var out = document.getElementById('term-output');
      if (termTimer) clearTimeout(termTimer);
      if (out) out.innerHTML = '<span class="term-cursor"></span>';
    });
    // Brief pause before typing starts — mimics the moment of stillness
    // right before the lines start landing in Neo's screen.
    termTimer = setTimeout(typeMatrixIntro, 600);
  }

  function closeTerminalModal() {
    hideModal('term-modal', function () {
      if (termTimer) { clearTimeout(termTimer); termTimer = null; }
    });
  }

  // ── Editor splash (cyan sibling of the Matrix terminal) ────────
  //
  // Opens on 'e'. Aspirational rather than functional — a brief boot
  // sequence and a coding quote, typed out like the Matrix lines but
  // tinted cyan to read as a different surface. The selected card's
  // branch threads into the path so the splash feels card-specific.

  var editorTimer = null;

  function openEditorModal() {
    var branch = STATE.selectedCardId || 'main';
    showModal('editor-modal', function () {
      document.getElementById('editor-modal-title').textContent = 'Editor — ' + branch;
      var out = document.getElementById('editor-output');
      if (editorTimer) clearTimeout(editorTimer);
      if (out) out.innerHTML = '<span class="editor-cursor"></span>';
    });
    editorTimer = setTimeout(function () { typeEditorSplash(branch); }, 450);
  }

  function closeEditorModal() {
    hideModal('editor-modal', function () {
      if (editorTimer) { clearTimeout(editorTimer); editorTimer = null; }
    });
  }

  function typeEditorSplash(branch) {
    var out = document.getElementById('editor-output');
    if (!out) return;
    // Mixed segments: plain typed lines + a pre-rendered quote block
    // that fades in after the boot lines finish. Keeping the segments
    // in one array means the typewriter handles ordering / timing.
    var bootLines = [
      '$ code .',
      '',
      '  Indexing workspace…',
      '  142 files loaded.',
      '  Spawning language servers…',
      '  Ready.',
      '',
      '  Today\'s canvas: ' + branch
    ];
    var quote = '\n\n  "Talk is cheap.\n   Show me the code."\n\n        — Linus Torvalds, LKML 2000';

    var li = 0, ci = 0, typed = '';
    function step() {
      if (li >= bootLines.length) {
        // Boot finished — drop the quote in one beat for emphasis.
        typed += quote;
        out.innerHTML = esc(typed) + '<span class="editor-cursor"></span>';
        return;
      }
      var line = bootLines[li];
      if (ci < line.length) {
        typed += line.charAt(ci);
        ci++;
        out.innerHTML = esc(typed) + '<span class="editor-cursor"></span>';
        editorTimer = setTimeout(step, 28 + Math.random() * 30);
      } else {
        typed += '\n';
        li++;
        ci = 0;
        out.innerHTML = esc(typed) + '<span class="editor-cursor"></span>';
        editorTimer = setTimeout(step, 180);
      }
    }
    step();
  }

  function typeMatrixIntro() {
    var out = document.getElementById('term-output');
    if (!out) return;
    var li = 0, ci = 0, typed = '';
    function step() {
      if (li >= TERM_LINES.length) {
        // All lines typed — leave the cursor blinking at the end.
        out.innerHTML = esc(typed) + '<span class="term-cursor"></span>';
        return;
      }
      var line = TERM_LINES[li];
      if (ci < line.length) {
        typed += line.charAt(ci);
        ci++;
        out.innerHTML = esc(typed) + '<span class="term-cursor"></span>';
        // Slight jitter so the typing feels human, not metronomic.
        termTimer = setTimeout(step, 55 + Math.random() * 50);
      } else {
        typed += '\n';
        li++;
        ci = 0;
        out.innerHTML = esc(typed) + '<span class="term-cursor"></span>';
        termTimer = setTimeout(step, 1100);
      }
    }
    step();
  }

  function renderMd(s) {
    var esc = String(s).replace(/[&<>"']/g, function (c) {
      return { '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[c];
    });
    return esc
      .replace(/^### (.+)$/gm, '<h4>$1</h4>')
      .replace(/^## (.+)$/gm, '<h3>$1</h3>')
      .replace(/^# (.+)$/gm, '<h2>$1</h2>')
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/`(.+?)`/g, '<code>$1</code>')
      .replace(/^- (.+)$/gm, '<li>$1</li>')
      .replace(/(<li>.*<\/li>(?:\n|$))+/g, function (m) { return '<ul>' + m.replace(/\n/g, '') + '</ul>'; })
      .replace(/\n/g, '<br>');
  }

  // ── Wire existing + future cards ───────────────────────────────

  function wireCard(card) {
    // Click SELECTS only (matches biomelab's GUI behavior: select-
    // then-act). To view the log the user presses 'l' (keyboard or
    // the l key-cap in the Keyboard-First grid). Capture phase keeps
    // the selection from being clobbered by any sibling listener.
    card.addEventListener('click', function () {
      selectCard(card.dataset.kb);
    }, true);
  }
  document.querySelectorAll('.kb-card[data-kb]').forEach(wireCard);

  // ── Keyboard handler ───────────────────────────────────────────

  document.addEventListener('keydown', function (e) {
    var tag = document.activeElement && document.activeElement.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    var rgtModal     = document.getElementById('rgt-modal');
    var noteModal    = document.getElementById('note-modal');
    var helpModal    = document.getElementById('kb-help-modal');
    var termModal    = document.getElementById('term-modal');
    var editorModal  = document.getElementById('editor-modal');
    var confirmModal = document.getElementById('confirm-modal');
    if (rgtModal && !rgtModal.hasAttribute('hidden')) return;
    if (noteModal && !noteModal.hasAttribute('hidden')) {
      if (e.key === 'Escape') closeNoteModal();
      return;
    }
    if (helpModal && !helpModal.hasAttribute('hidden')) {
      // Toggle off: Esc or another '?' closes the cheatsheet.
      if (e.key === 'Escape' || e.key === '?') {
        closeHelpModal();
        e.preventDefault();
      }
      return;
    }
    if (termModal && !termModal.hasAttribute('hidden')) {
      if (e.key === 'Escape') closeTerminalModal();
      return;
    }
    if (editorModal && !editorModal.hasAttribute('hidden')) {
      if (e.key === 'Escape') closeEditorModal();
      return;
    }
    if (confirmModal && !confirmModal.hasAttribute('hidden')) {
      // Esc = Cancel · Enter = Confirm. Other keys are swallowed so
      // the user can't accidentally trigger a card action while the
      // dialog is in front.
      if (e.key === 'Escape') { closeConfirmModal(); e.preventDefault(); }
      else if (e.key === 'Enter') { confirmYes(); e.preventDefault(); }
      return;
    }
    if (e.key === '?') { openHelpModal(); e.preventDefault(); return; }

    switch (e.key) {
      case 'c': case 'C': createCard();      e.preventDefault(); break;
      case 'd': case 'D': deleteCard();      e.preventDefault(); break;
      case 'r': case 'R': refreshAll();      e.preventDefault(); break;
      case 'm': case 'M': openNoteModal();   e.preventDefault(); break;
      case 'l': case 'L': openLogModal();    e.preventDefault(); break;
      // Toast-only actions (educational reactions for keys without
      // their own animation on the demo board)
      case 'e':
        if (STATE.selectedCardId) { openEditorModal(); e.preventDefault(); }
        else showToast('Select a card first (click one)');
        break;
      case 'f': showToast('Fetching PR into a new worktree…');        e.preventDefault(); break;
      case 'n': openSandboxDemo();                                    e.preventDefault(); break;
      case 'p':
        if (STATE.selectedCardId === 'main') pullMain();
        else showToast('Pulling from remote…');
        e.preventDefault();
        break;
      case 'P': sendPR();                                              e.preventDefault(); break;
      case 's': showToast('Starting stopped sandbox…');               e.preventDefault(); break;
      case 'S': showToast('Stopping running sandbox…');               e.preventDefault(); break;
      case 'Enter':
        if (STATE.selectedCardId) { openTerminalModal(); e.preventDefault(); }
        break;
    }
  });

  // ── Click handlers for the key-caps in the "Keyboard-First" grid ─

  function fireByCap(text) {
    switch (text) {
      case 'c': createCard(); return;
      case 'd': deleteCard(); return;
      case 'r': refreshAll(); return;
      case 'm': openNoteModal(); return;
      case 'l': openLogModal(); return;
      case 'e':
        if (STATE.selectedCardId) openEditorModal();
        else showToast('Select a card first (click one)');
        return;
      case 'f': showToast('Fetching PR into a new worktree…'); return;
      case 'n': openSandboxDemo(); return;
      case 'p':
        if (STATE.selectedCardId === 'main') pullMain();
        else showToast('Pulling from remote…');
        return;
      case 'P': sendPR(); return;
      case 's': showToast('Starting stopped sandbox…'); return;
      case 'S': showToast('Stopping running sandbox…'); return;
      case '⏎':
        if (STATE.selectedCardId) openTerminalModal();
        else showToast('Select a card first (click one)');
        return;
      case 'g':
        // Delegate to the existing kanban toggle by dispatching a
        // keyboard event the toggle IIFE already listens for.
        document.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', bubbles: true }));
        return;
    }
  }
  // Delegate so both the page-load key-items AND the clones inside the
  // '?' help modal trigger the same actions.
  document.addEventListener('click', function (e) {
    var item = e.target.closest('.key-item');
    if (!item) return;
    var cap = item.querySelector('.key-cap');
    if (!cap) return;
    fireByCap(cap.textContent.trim());
  });

  // ── Modal close handlers (delegated) ───────────────────────────
  //
  // Delegate clicks on data-{kind}-close targets to document so the
  // bindings survive any markup tweaks and work for nested overlays
  // / buttons without per-element wiring. One listener replaces three
  // per-element registrations.
  document.addEventListener('click', function (e) {
    if (e.target.closest('[data-note-close]'))    { closeNoteModal();    return; }
    if (e.target.closest('[data-kbhelp-close]'))  { closeHelpModal();    return; }
    if (e.target.closest('[data-term-close]'))    { closeTerminalModal(); return; }
    if (e.target.closest('[data-editor-close]'))  { closeEditorModal();  return; }
    if (e.target.closest('[data-confirm-close]')) { closeConfirmModal(); return; }
    if (e.target.closest('.confirm-yes'))         { confirmYes();        return; }
  });
  // Esc while typing in the textarea: the global keydown handler bows
  // out on TEXTAREA focus, so add a dedicated Escape on the entry so
  // the user doesn't have to mouse to the × button to close.
  var noteEntry = document.getElementById('note-entry');
  if (noteEntry) {
    noteEntry.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeNoteModal();
      }
    });
  }

  // ── Bootstrap STATE from the initial handcrafted DOM ───────────
  //
  // The page ships a static kanban + grid layout for first paint /
  // SEO. We parse it once into STATE.cards on init so subsequent
  // mutations (create, send-PR, delete) flow through one source of
  // truth and re-render both views. The grid view is wired here too —
  // its initial DOM is replaced by render so the two views are always
  // in sync, including for hand-authored cards that may have drifted
  // between the two markup blocks in index.html.
  STATE.cards = parseInitialState();
  renderAll();

  // Wire the static main worktree card once. It's NOT in STATE.cards
  // (it doesn't live in a column or the grid), so renderAll never
  // touches it — a single binding here is enough.
  var mainCardEl = document.querySelector('.kb-main-card[data-kb]');
  if (mainCardEl) wireCard(mainCardEl);

  // Kick off the periodic sync drift so the main card flips to
  // "behind" every 10 s and the user has a real reason to try 'p'.
  startMainSyncCycle();
})();
