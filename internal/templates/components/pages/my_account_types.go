package pages

import "fmt"

// MyAccountProps mirrors the data map the old my_account.html read. The
// handler builds these in admin/members.go and renders via
// TemplateRenderer.RenderWithTempl.
type MyAccountProps struct {
	UserID      string
	IsUnlinked  bool
	CSRFToken   string
	Connections []MyAccountConnectionRow
}

// MyAccountConnectionRow is the per-row shape for the "My Connections"
// list. The handler flattens *string fields into Go strings so the templ
// stays declarative.
type MyAccountConnectionRow struct {
	ID              string
	Provider        string
	Status          string
	InstitutionName string // empty falls back to "Connection" in the template
}

// myAccountConnectionURL returns the GET endpoint for a connection detail
// page. Centralising it keeps the templ side free of string concatenation
// inside `href={…}` slots — same pattern as accessAPIKeyRevokeURL.
func myAccountConnectionURL(id string) string {
	return "/connections/" + id
}

// myAccountInstitutionLabel returns the InstitutionName when present,
// or "Connection" as a fallback — mirrors the original `{{if .InstitutionName}}…{{else}}Connection{{end}}`.
func myAccountInstitutionLabel(c MyAccountConnectionRow) string {
	if c.InstitutionName == "" {
		return "Connection"
	}
	return c.InstitutionName
}

// myAccountAvatarBootstrap renders the Alpine factory + inline <script> for
// the avatar uploader. Lifting it into Go keeps the templ side clean and
// lets us interpolate the user ID via jsStringLit instead of templating
// inside a <script> body. Mirrors the bootstrap pattern from #801 (tag_form)
// and #654 (category_form).
func myAccountAvatarBootstrap(p MyAccountProps) string {
	return fmt.Sprintf(`<script>
function myAvatar() {
  var uid = %s;
  return {
    avatarSrc: '/avatars/' + uid + '?v=' + Date.now(),
    hasCustom: false,
    error: '',
    uploading: false,

    init() {
      var self = this;
      fetch('/avatars/' + uid, { method: 'HEAD' })
        .then(function(r) {
          var ct = r.headers.get('content-type') || '';
          self.hasCustom = ct.indexOf('svg') === -1;
        })
        .catch(function() {});
    },

    onFileSelect(e) {
      var file = e.target.files[0];
      if (!file) return;
      this.error = '';

      if (!file.type.match(/^image\/(png|jpeg|gif)$/)) {
        this.error = 'Please select a PNG, JPG, or GIF image.';
        e.target.value = '';
        return;
      }
      if (file.size > 5 * 1024 * 1024) {
        this.error = 'Image must be under 5 MB.';
        e.target.value = '';
        return;
      }

      var self = this;
      self.uploading = true;
      var fd = new FormData();
      fd.append('avatar', file);

      fetch('/my-account/avatar', { method: 'POST', body: fd })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          return res.json();
        })
        .then(function() {
          self.avatarSrc = '/avatars/' + uid + '?v=' + Date.now();
          self.hasCustom = true;
          self.uploading = false;
        })
        .catch(function(err) {
          self.error = (err.error || 'Upload failed');
          self.uploading = false;
        });
      e.target.value = '';
    },

    removeAvatar() {
      var self = this;
      self.error = '';
      fetch('/my-account/avatar', { method: 'DELETE' })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          self.avatarSrc = '/avatars/' + uid + '?v=' + Date.now();
          self.hasCustom = false;
        })
        .catch(function(err) {
          self.error = (err.error || 'Remove failed');
        });
    },

    regenerate() {
      var self = this;
      self.error = '';
      fetch('/my-account/avatar/regenerate', { method: 'POST' })
        .then(function(res) {
          if (!res.ok) return res.json().then(function(d) { throw d; });
          return res.json();
        })
        .then(function() {
          self.avatarSrc = '/avatars/' + uid + '?v=' + Date.now();
          self.hasCustom = false;
        })
        .catch(function(err) {
          self.error = (err.error || 'Regenerate failed');
        });
    }
  };
}
</script>`, jsStringLit(p.UserID))
}
