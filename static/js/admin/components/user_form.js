// User-form Alpine factories for /users/new (create) and /users/{id}/edit (edit).
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Two factories ship from this module:
//
//   - `userForm` — create flow. Local state only (no server-rendered seed).
//     Drives the avatar preview shuffle, optional login-account toggle, and
//     the two-step POST (`/-/users` + optional `/-/members`).
//   - `avatarEditor` — edit flow. Reads the user's UUID from `data-user-id`
//     on the x-data root and the initial `hasCustomAvatar` boolean from
//     `data-has-custom-avatar`, so the factory body keeps the no-arg shape
//     the convention requires.
//
// The <script src> loads synchronously at the top of the templ component
// (so the alpine:init listener registers before Alpine fires the event).

document.addEventListener('alpine:init', function () {
  // Create flow — drives /users/new. Maintains local-only state for the
  // name/email/login toggle, the avatar-preview seed, and the two-step
  // submit (user creation + optional login-account creation).
  Alpine.data('userForm', function () {
    var initialSeed = 'new-member-' + Date.now();
    return {
      createLogin: false,
      role: 'viewer',
      submitting: false,
      error: '',
      setupURL: '',
      avatarSeed: initialSeed,
      avatarPreviewSrc: '/avatars/preview/' + encodeURIComponent(initialSeed),

      init: function () {
        // No server-rendered initial state to parse — the factory is
        // purely client-side.
      },

      updateAvatarPreview: function (name) {
        var seed = name.trim() || this.avatarSeed;
        this.avatarPreviewSrc = '/avatars/preview/' + encodeURIComponent(seed) + '?v=' + Date.now();
      },

      shuffleAvatar: function () {
        this.avatarSeed = 'seed-' + Math.random().toString(36).slice(2);
        var nameVal = document.getElementById('name').value.trim();
        // If name is typed, append seed to make it unique; otherwise use seed alone.
        var seed = nameVal ? nameVal + '-' + this.avatarSeed : this.avatarSeed;
        this.avatarPreviewSrc = '/avatars/preview/' + encodeURIComponent(seed) + '?v=' + Date.now();
      },

      submit: function () {
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
  });

  // Edit flow — drives /users/{id}/edit. Reads the user's UUID and the
  // initial hasCustomAvatar boolean from data-* attributes on the x-data
  // root, so the factory body keeps the no-arg shape the convention
  // requires. Handles avatar upload/remove/regenerate plus the name/email
  // PUT.
  Alpine.data('avatarEditor', function () {
    return {
      userId: '',
      avatarSrc: '',
      hasCustomAvatar: false,
      avatarError: '',
      avatarUploading: false,
      formError: '',
      submitting: false,

      init: function () {
        this.userId = this.$el.dataset.userId || '';
        this.hasCustomAvatar = this.$el.dataset.hasCustomAvatar === 'true';
        this.avatarSrc = '/avatars/' + this.userId + '?v=' + Date.now();
      },

      onFileSelect: function (e) {
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
          .then(function (res) {
            if (!res.ok) return res.json().then(function (d) { throw d; });
            return res.json();
          })
          .then(function () {
            self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
            self.hasCustomAvatar = true;
            self.avatarUploading = false;
          })
          .catch(function (err) {
            self.avatarError = (err.error || 'Upload failed');
            self.avatarUploading = false;
          });
        e.target.value = '';
      },

      removeAvatar: function () {
        var self = this;
        self.avatarError = '';
        fetch('/-/users/' + self.userId + '/avatar', { method: 'DELETE' })
          .then(function (res) {
            if (!res.ok) return res.json().then(function (d) { throw d; });
            self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
            self.hasCustomAvatar = false;
          })
          .catch(function (err) {
            self.avatarError = (err.error || 'Remove failed');
          });
      },

      regenerate: function () {
        var self = this;
        self.avatarError = '';
        fetch('/-/users/' + self.userId + '/avatar/regenerate', { method: 'POST' })
          .then(function (res) {
            if (!res.ok) return res.json().then(function (d) { throw d; });
            return res.json();
          })
          .then(function () {
            self.avatarSrc = '/avatars/' + self.userId + '?v=' + Date.now();
            self.hasCustomAvatar = false;
          })
          .catch(function (err) {
            self.avatarError = (err.error || 'Regenerate failed');
          });
      },

      submitForm: function () {
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
        .then(function (res) {
          if (res.ok) {
            window.location.href = '/users';
          } else {
            return res.json().then(function (data) {
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
        .catch(function () {
          self.submitting = false;
          self.formError = 'Network error. Please try again.';
        });
      }
    };
  });
});
