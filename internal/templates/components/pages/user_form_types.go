package pages

import (
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/templates/components"
)

// UserFormProps mirrors the data map the old user_form.html read: the
// edit/create mode flag, the user record (in edit mode), the user's UUID
// (for avatar endpoints), and the breadcrumb trail.
type UserFormProps struct {
	IsEdit      bool
	User        *db.User
	UserID      string
	Breadcrumbs []components.Breadcrumb
}

// userFormName returns the user's name or empty string.
func userFormName(u *db.User) string {
	if u == nil {
		return ""
	}
	return u.Name
}

// userFormEmail returns the user's email (empty when absent or invalid).
func userFormEmail(u *db.User) string {
	if u == nil || !u.Email.Valid {
		return ""
	}
	return u.Email.String
}

// userFormHasAvatar reports whether the user has a custom avatar uploaded.
func userFormHasAvatar(u *db.User) bool {
	return u != nil && len(u.AvatarData) > 0
}

// userFormCreateBootstrap renders the Alpine factory for the create flow.
// Lifted into a Go helper so the JS body stays plain text and templ doesn't
// have to dance around <script> escaping. Mirrors the pattern used by
// tag_form_types.go and category_form_types.go.
func userFormCreateBootstrap() string {
	return `<script>
function userForm() {
  var initialSeed = 'new-member-' + Date.now();
  return {
    createLogin: false,
    role: 'viewer',
    submitting: false,
    error: '',
    setupURL: '',
    avatarSeed: initialSeed,
    avatarPreviewSrc: '/avatars/preview/' + encodeURIComponent(initialSeed),

    updateAvatarPreview(name) {
      var seed = name.trim() || this.avatarSeed;
      this.avatarPreviewSrc = '/avatars/preview/' + encodeURIComponent(seed) + '?v=' + Date.now();
    },

    shuffleAvatar() {
      this.avatarSeed = 'seed-' + Math.random().toString(36).slice(2);
      var nameVal = document.getElementById('name').value.trim();
      // If name is typed, append seed to make it unique; otherwise use seed alone.
      var seed = nameVal ? nameVal + '-' + this.avatarSeed : this.avatarSeed;
      this.avatarPreviewSrc = '/avatars/preview/' + encodeURIComponent(seed) + '?v=' + Date.now();
    },

    submit() {
      this.error = '';
      var name = document.getElementById('name').value.trim();
      var email = document.getElementById('email').value.trim();

      if (!name) {
        this.error = 'Name is required.';
        return;
      }

      if (this.createLogin && !email) {
        this.error = 'Email is required when creating a login account.';
        return;
      }

      if (this.createLogin && email && (!email.includes('@') || !email.includes('.'))) {
        this.error = 'Please enter a valid email address.';
        return;
      }

      var body = { name: name };
      if (email) {
        body.email = email;
      }

      var self = this;
      self.submitting = true;

      // Step 1: Create the user.
      fetch('/-/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })
      .then(function (res) {
        if (!res.ok) return res.json().then(function (d) { throw d; });
        return res.json();
      })
      .then(function (userData) {
        if (!self.createLogin) {
          // No login account needed -- redirect.
          window.location.href = '/users?created=1';
          return;
        }

        // Step 2: Create login account.
        return fetch('/-/members', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            user_id: userData.id,
            username: email,
            role: self.role
          })
        })
        .then(function (res) {
          if (!res.ok) return res.json().then(function (d) { throw d; });
          return res.json();
        })
        .then(function (loginData) {
          self.submitting = false;
          if (loginData.setup_token) {
            self.setupURL = window.location.origin + '/setup-account/' + loginData.setup_token;
            // Re-initialize Lucide icons for the success state.
            self.$nextTick(function () { if (window.lucide) lucide.createIcons(); });
          } else {
            window.location.href = '/users?created=1';
          }
        });
      })
      .catch(function (e) {
        self.submitting = false;
        if (window.bbProgress) window.bbProgress.finish();
        var main = document.querySelector('main');
        if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
        self.error = (e.error && typeof e.error === 'object' && e.error.message) || (e.error && typeof e.error === 'string' && e.error) || 'An error occurred.';
      });
    }
  };
}
</script>`
}

// userFormEditBootstrap renders the Alpine factory for the edit flow.
// The two interpolation sites are the user's UUID (passed to the fetch
// endpoints) and the initial hasCustomAvatar boolean.
func userFormEditBootstrap(p UserFormProps) string {
	return fmt.Sprintf(`<script>
function avatarEditor(userId) {
  return {
    userId: userId,
    avatarSrc: '/avatars/' + userId + '?v=' + Date.now(),
    hasCustomAvatar: %t,
    avatarError: '',
    avatarUploading: false,
    formError: '',
    submitting: false,

    onFileSelect(e) {
      var file = e.target.files[0];
      if (!file) return;
      this.avatarError = '';

      if (!file.type.match(/^image\/(png|jpeg|gif)$/)) {
        this.avatarError = 'Please select a PNG, JPG, or GIF image.';
        e.target.value = '';
        return;
      }
      if (file.size > 5 * 1024 * 1024) {
        this.avatarError = 'Image must be under 5 MB.';
        e.target.value = '';
        return;
      }

      var self = this;
      self.avatarUploading = true;
      var fd = new FormData();
      fd.append('avatar', file);

      fetch('/-/users/' + self.userId + '/avatar', { method: 'POST', body: fd })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          return res.json();
        })
        .then(function() {
          self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
          self.hasCustomAvatar = true;
          self.avatarUploading = false;
        })
        .catch(function(err) {
          self.avatarError = (err.error || 'Upload failed');
          self.avatarUploading = false;
        });
      e.target.value = '';
    },

    removeAvatar() {
      var self = this;
      self.avatarError = '';
      fetch('/-/users/' + self.userId + '/avatar', { method: 'DELETE' })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
          self.hasCustomAvatar = false;
        })
        .catch(function(err) {
          self.avatarError = (err.error || 'Remove failed');
        });
    },

    regenerate() {
      var self = this;
      self.avatarError = '';
      fetch('/-/users/' + self.userId + '/avatar/regenerate', { method: 'POST' })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          return res.json();
        })
        .then(function() {
          self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
          self.hasCustomAvatar = false;
        })
        .catch(function(err) {
          self.avatarError = (err.error || 'Regenerate failed');
        });
    },

    submitForm() {
      this.formError = '';
      var name = document.getElementById('name').value.trim();
      var email = document.getElementById('email').value.trim();

      if (!name) {
        this.formError = 'Name is required.';
        return;
      }

      var self = this;
      self.submitting = true;

      fetch('/-/users/' + self.userId, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name, email: email || '' })
      })
      .then(function(res) {
        if (res.ok) {
          window.location.href = '/users';
        } else {
          return res.json().then(function(data) {
            self.submitting = false;
            var msg = 'An error occurred.';
            if (data.error && typeof data.error === 'object' && data.error.message) {
              msg = data.error.message;
            } else if (data.error && typeof data.error === 'string') {
              msg = data.error;
            }
            self.formError = msg;
          });
        }
      })
      .catch(function() {
        self.submitting = false;
        self.formError = 'Network error. Please try again.';
      });
    }
  };
}
</script>`,
		userFormHasAvatar(p.User),
	)
}
