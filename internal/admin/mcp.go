package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// MCPSettingsGetHandler serves GET /admin/mcp.
func MCPSettingsGetHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := svc.GetMCPConfig(r.Context())
		if err != nil {
			http.Error(w, "Failed to load MCP config", http.StatusInternalServerError)
			return
		}

		// Build tool list for display.
		type toolInfo struct {
			Name           string
			Description    string
			Classification string
			Enabled        bool
			WriteLocked    bool // true when global mode is read-only and tool is write
		}
		disabledSet := make(map[string]bool)
		for _, t := range cfg.DisabledTools {
			disabledSet[t] = true
		}

		var tools []toolInfo
		for _, td := range mcpServer.AllToolDefs() {
			enabled := !disabledSet[td.Tool.Name]
			writeLocked := string(td.Classification) == "write" && cfg.Mode == "read_only"
			tools = append(tools, toolInfo{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Classification: string(td.Classification),
				Enabled:        enabled,
				WriteLocked:    writeLocked,
			})
		}

		// Count API keys by scope.
		keys, _ := svc.ListAPIKeys(r.Context())
		fullAccessCount := 0
		readOnlyCount := 0
		for _, k := range keys {
			if k.RevokedAt != nil {
				continue
			}
			if k.Scope == "read_only" {
				readOnlyCount++
			} else {
				fullAccessCount++
			}
		}

		// Serialize templates as JSON for Alpine.js.
		templatesJSON, _ := json.Marshal(breadboxmcp.InstructionTemplates)

		data := BaseTemplateData(r, sm, "mcp", "MCP Settings")
		data["MCPConfig"] = cfg
		data["Tools"] = tools
		data["Templates"] = breadboxmcp.InstructionTemplates
		data["TemplatesJSON"] = string(templatesJSON)
		data["FullAccessCount"] = fullAccessCount
		data["ReadOnlyCount"] = readOnlyCount
		data["BuiltInInstructions"] = breadboxmcp.BuiltInInstructions

		tr.Render(w, r, "mcp_settings.html", data)
	}
}

// MCPSaveModeHandler handles POST /admin/mcp/mode.
func MCPSaveModeHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := r.FormValue("mode")
		if err := svc.SaveMCPMode(r.Context(), mode); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid mode: must be read_only or read_write")
			http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "MCP mode updated.")
		http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
	}
}

// MCPSaveToolsHandler handles POST /admin/mcp/tools.
func MCPSaveToolsHandler(svc *service.Service, mcpServer *breadboxmcp.MCPServer, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			SetFlash(r.Context(), sm, "error", "Invalid form data")
			http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
			return
		}

		// Enabled tools come as form values. Build disabled list from what's NOT checked.
		enabledSet := make(map[string]bool)
		for _, name := range r.Form["enabled_tools"] {
			enabledSet[name] = true
		}

		var disabled []string
		for _, td := range mcpServer.AllToolDefs() {
			if !enabledSet[td.Tool.Name] {
				disabled = append(disabled, td.Tool.Name)
			}
		}

		if err := svc.SaveMCPDisabledTools(r.Context(), disabled); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to save tool settings")
			http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Tool access updated.")
		http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
	}
}

// MCPSaveInstructionsHandler handles POST /admin/mcp/instructions.
func MCPSaveInstructionsHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instructions := strings.TrimSpace(r.FormValue("instructions"))
		templateSlug := r.FormValue("template")

		if err := svc.SaveMCPInstructions(r.Context(), instructions, templateSlug); err != nil {
			SetFlash(r.Context(), sm, "error", err.Error())
			http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Instructions saved.")
		http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
	}
}
