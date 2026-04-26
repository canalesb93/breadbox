// Connection-new wizard Alpine component for /connections/new.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// The Teller environment (sandbox/development/production) flows in via a
// data-teller-env attribute on the x-data root and is read once in init().
//
// The factory is intentionally thin: most of the work is direct DOM wiring
// (form submit, button clicks) and bank-provider SDK orchestration (Plaid
// Link / TellerConnect), neither of which makes sense as Alpine reactive
// state. Wrapping the previously-IIFE bootstrap inside init() gets us out of
// the Go-string sidecar without re-architecting the wizard.
document.addEventListener('alpine:init', function () {
  Alpine.data('connectionNew', function () {
    return {
      init: function () {
        var tellerEnv = this.$el.dataset.tellerEnv || '';

        var memberForm = document.getElementById('member-form');
        var stepSelect = document.getElementById('step-select');
        var stepLink = document.getElementById('step-link');
        var memberNameEl = document.getElementById('member-name');
        var linkStatus = document.getElementById('link-status');
        var retryBtn = document.getElementById('retry-btn');
        var backBtn = document.getElementById('back-btn');
        var selectedUserId = '';
        var selectedProvider = '';

        memberForm.addEventListener('submit', function (e) {
          e.preventDefault();
          var select = document.getElementById('user_id');
          selectedUserId = select.value;
          var providerRadio = document.querySelector('input[name="provider"]:checked');
          selectedProvider = providerRadio ? providerRadio.value : 'plaid';
          if (!selectedUserId) return;
          if (selectedProvider === 'csv') {
            window.location.href = '/connections/import-csv';
            return;
          }
          memberNameEl.textContent = select.options[select.selectedIndex].text;
          stepSelect.classList.add('hidden');
          stepLink.classList.remove('hidden');
          setTimeout(function () { if (typeof lucide !== 'undefined') lucide.createIcons(); }, 50);
          loadProviderSDK(selectedProvider, startLink);
        });

        function loadProviderSDK(provider, callback) {
          if (provider === 'plaid' && typeof Plaid === 'undefined') {
            var script = document.createElement('script');
            script.src = 'https://cdn.plaid.com/link/v2/stable/link-initialize.js';
            script.onload = callback;
            document.head.appendChild(script);
          } else if (provider === 'teller' && typeof TellerConnect === 'undefined') {
            var script = document.createElement('script');
            script.src = 'https://cdn.teller.io/connect/connect.js';
            script.onload = callback;
            document.head.appendChild(script);
          } else {
            callback();
          }
        }

        backBtn.addEventListener('click', function (e) {
          e.preventDefault();
          stepLink.classList.add('hidden');
          stepSelect.classList.remove('hidden');
          linkStatus.textContent = 'Opening bank connection dialog...';
          linkStatus.classList.remove('text-error');
          linkStatus.classList.add('text-base-content/50');
          retryBtn.classList.add('hidden');
        });

        function showError(msg) {
          linkStatus.textContent = msg;
          linkStatus.classList.add('text-error');
          linkStatus.classList.remove('text-base-content/50');
          retryBtn.classList.remove('hidden');
        }

        function showMessage(msg) {
          linkStatus.textContent = msg;
          linkStatus.classList.remove('text-error');
          linkStatus.classList.add('text-base-content/50');
        }

        function startLink() {
          linkStatus.textContent = 'Opening bank connection dialog...';
          linkStatus.classList.remove('text-error');
          linkStatus.classList.add('text-base-content/50');
          retryBtn.classList.add('hidden');

          fetch('/-/link-token', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({user_id: selectedUserId, provider: selectedProvider})
          })
          .then(function (res) { return res.json(); })
          .then(function (data) {
            if (data.link_token) {
              if (selectedProvider === 'teller') {
                initTellerConnect(data.link_token);
              } else {
                initPlaidLink(data.link_token);
              }
            } else {
              showError(data.error || 'Failed to initialize bank connection.');
            }
          })
          .catch(function () {
            showError('Network error starting bank connection. Please try again.');
          });
        }

        function initPlaidLink(token) {
          var handler = Plaid.create({
            token: token,
            onSuccess: function (publicToken, metadata) {
              showMessage('Saving connection...');
              fetch('/-/exchange-token', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                  public_token: publicToken,
                  user_id: selectedUserId,
                  institution_id: metadata.institution.institution_id,
                  institution_name: metadata.institution.name,
                  accounts: metadata.accounts,
                  provider: 'plaid'
                })
              })
              .then(function (res) { return res.json(); })
              .then(function (data) {
                if (data.connection_id) {
                  window.location.href = '/connections/' + data.connection_id;
                } else {
                  showError(data.error || 'An unknown error occurred.');
                }
              })
              .catch(function () {
                showError('Network error. Please try again.');
              });
            },
            onExit: function (err) {
              if (err) {
                showError('Plaid Link exited with an error: ' + (err.display_message || err.error_message || 'Unknown error'));
              } else {
                showMessage('Bank connection cancelled.');
              }
            }
          });
          handler.open();
        }

        function initTellerConnect(appId) {
          var tellerConnect = TellerConnect.setup({
            applicationId: appId,
            environment: tellerEnv,
            products: ['transactions', 'balance'],
            onSuccess: function (enrollment) {
              showMessage('Saving connection...');
              var publicToken = JSON.stringify({
                access_token: enrollment.accessToken,
                enrollment_id: enrollment.enrollment.id,
                institution_name: enrollment.enrollment.institution.name
              });
              fetch('/-/exchange-token', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                  public_token: publicToken,
                  user_id: selectedUserId,
                  institution_id: enrollment.enrollment.institution.name,
                  institution_name: enrollment.enrollment.institution.name,
                  provider: 'teller'
                })
              })
              .then(function (res) { return res.json(); })
              .then(function (data) {
                if (data.connection_id) {
                  window.location.href = '/connections/' + data.connection_id;
                } else {
                  showError(data.error || 'An unknown error occurred.');
                }
              })
              .catch(function () { showError('Network error. Please try again.'); });
            },
            onExit: function () {
              showMessage('Bank connection cancelled.');
            },
            onFailure: function (failure) {
              showError('Teller Connect error: ' + (failure.message || 'Unknown error'));
            }
          });
          tellerConnect.open();
        }

        retryBtn.addEventListener('click', startLink);
      }
    };
  });
});
