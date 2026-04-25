package pages

import (
	"fmt"

	"breadbox/internal/templates/components"
)

// CreateLoginProps mirrors the data map create_login.html consumed. The
// underlying page has two modes: IsManage=true renders the manage view for
// an existing login account; IsManage=false renders the create form.
//
// All fields are flattened from db row primitives (pgtype.Text -> string +
// bool, pgtype.UUID -> formatted string) so the templ component never sees
// pg types directly.
type CreateLoginProps struct {
	IsManage bool

	// User identity (always required).
	UserID    string
	UserName  string
	UserEmail string // empty when User.Email is null

	// Manage-mode fields.
	LoginAccountID    string // formatted UUID
	LoginUsername     string
	LoginRole         string // "viewer" or "editor"
	LoginPasswordSet  bool
	SetupURL          string // empty when no token / password already set

	// Breadcrumb trail.
	Breadcrumbs []components.Breadcrumb
}

// manageLoginRoleAlpine returns the inline x-data expression for the role
// edit card. Mirrors the original create_login.html block; the role PUT
// endpoint is /-/members/{id}/role.
func manageLoginRoleAlpine(p CreateLoginProps) string {
	return fmt.Sprintf(`{
    initialRole: %s,
    role: %s,
    saving: false,
    saved: false,
    error: '',
    get dirty() { return this.role !== this.initialRole; },
    save() {
      this.error = '';
      this.saving = true;
      var self = this;
      fetch('/-/members/' + %s + '/role', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({role: this.role})
      }).then(function(r) {
        if (!r.ok) return r.json().then(function(d) { throw d; });
        self.initialRole = self.role;
        self.saving = false;
        self.saved = true;
        setTimeout(function() { self.saved = false; }, 2000);
      }).catch(function(e) {
        self.saving = false;
        self.error = (e && e.error && e.error.message) || 'Failed to save changes';
      });
    }
  }`,
		jsStringLit(p.LoginRole),
		jsStringLit(p.LoginRole),
		jsStringLit(p.LoginAccountID),
	)
}

// manageLoginSetupAlpine returns the inline x-data expression for the
// setup-link card. Used only when the password isn't yet set.
func manageLoginSetupAlpine(p CreateLoginProps) string {
	return fmt.Sprintf(`{
    setupURL: %s,
    regenerating: false,
    copied: false,
    regenerate() {
      var self = this;
      self.regenerating = true;
      fetch('/-/members/' + %s + '/setup-token', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'}
      }).then(function(r) { return r.json(); }).then(function(data) {
        self.regenerating = false;
        if (data.setup_token) {
          self.setupURL = window.location.origin + '/setup-account/' + data.setup_token;
          self.copied = false;
        }
      }).catch(function() { self.regenerating = false; });
    }
  }`,
		jsStringLit(p.SetupURL),
		jsStringLit(p.LoginAccountID),
	)
}

// manageLoginDeleteAlpine returns the inline x-data expression for the
// danger-zone delete card.
func manageLoginDeleteAlpine(p CreateLoginProps) string {
	return fmt.Sprintf(`{
    confirming: false,
    deleting: false,
    error: '',
    confirmDelete() {
      this.error = '';
      this.deleting = true;
      var self = this;
      fetch('/-/members/' + %s, { method: 'DELETE' })
        .then(function(r) {
          if (!r.ok) return r.json().then(function(d) { throw d; }, function() { throw { error: { message: 'Failed to delete login account' } }; });
          window.location.href = '/users';
        })
        .catch(function(e) {
          self.deleting = false;
          self.confirming = false;
          self.error = (e && e.error && e.error.message) || 'Failed to delete login account';
        });
    }
  }`,
		jsStringLit(p.LoginAccountID),
	)
}

// newLoginAlpine returns the inline x-data expression for the create-form
// view. The submit handler updates the user's email if changed, then
// POSTs /-/members. On success it swaps in a "Login created" panel that
// renders the setup link returned by the server.
func newLoginAlpine(p CreateLoginProps) string {
	return fmt.Sprintf(`{
    submitting: false,
    error: '',
    setupURL: '',
    done: false,
    submit() {
      this.error = '';
      var email = document.getElementById('login_email').value.trim();
      var role = document.getElementById('login_role').value;
      if (!email) { this.error = 'Email is required'; return; }
      if (!email.includes('@') || !email.includes('.')) { this.error = 'Please enter a valid email address'; return; }
      this.submitting = true;
      var self = this;
      var userID = %s;
      var initialEmail = %s;
      var emailChanged = email !== initialEmail;
      var chain = Promise.resolve();
      if (emailChanged) {
        chain = fetch('/-/users/' + userID, {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({email: email})
        }).then(function(r) { if (!r.ok) return r.json().then(function(d) { throw d; }); });
      }
      chain.then(function() {
        return fetch('/-/members', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({user_id: userID, username: email, role: role})
        });
      }).then(function(r) {
        if (!r.ok) return r.json().then(function(d) { throw d; });
        return r.json();
      }).then(function(data) {
        self.submitting = false;
        if (data.setup_token) {
          self.setupURL = window.location.origin + '/setup-account/' + data.setup_token;
          self.done = true;
          if (window.bbProgress) window.bbProgress.finish();
          var main = document.querySelector('main');
          if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
          self.$nextTick(function() { if (typeof lucide !== 'undefined') lucide.createIcons(); });
        } else {
          window.location.href = '/users';
        }
      }).catch(function(e) {
        self.submitting = false;
        if (window.bbProgress) window.bbProgress.finish();
        var main = document.querySelector('main');
        if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
        self.error = (e && e.error && e.error.message) || 'Failed to create login account';
      });
    }
  }`,
		jsStringLit(p.UserID),
		jsStringLit(p.UserEmail),
	)
}
