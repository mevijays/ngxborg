// Package mcpserver provides an MCP (Model Context Protocol) server
// for ngxborg, enabling agentic integrations to manage backup servers.
//
// This server exposes tools for:
//   - User management (create, delete, list, disable/enable, key management)
//   - Repository management (create, delete, purge, disable/enable, list)
//   - System diagnostics (doctor check)
//
// All operations call internal packages directly — no CLI subprocess —
// and are served over HTTP at /mcp on the same port as the web UI.
package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"ngxborg/internal/borgrepo"
	"ngxborg/internal/build"
	"ngxborg/internal/posix"
	"ngxborg/internal/provision"
	"ngxborg/internal/sshaccess"
	"ngxborg/internal/system"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server is the MCP server for ngxborg.
type Server struct {
	srv *server.MCPServer
}

// New creates a new MCP server instance.
func New() *Server {
	srv := server.NewMCPServer(
		"ngxborg",
		"1.0.0",
	)

	// Register tools
	registerUserTools(srv)
	registerRepoTools(srv)
	registerSystemTools(srv)

	return &Server{srv: srv}
}

// StreamableHTTPHandler returns an http.Handler that serves the MCP protocol
// at the given path (typically "/mcp"). The handler is anonymous — no PAM
// authentication is required.
func (s *Server) StreamableHTTPHandler(path string) http.Handler {
	httpServer := server.NewStreamableHTTPServer(s.srv)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path && r.URL.Path != path+"/" {
			http.NotFound(w, r)
			return
		}
		httpServer.ServeHTTP(w, r)
	})
}

// ---- User Management Tools ----

func registerUserTools(srv *server.MCPServer) {
	runner := system.Runner{Timeout: 5 * time.Minute}

	// Create user
	srv.AddTool(mcp.NewTool("ngxborg_user_create",
		mcp.WithDescription("Create a new ngxborg tenant or admin user"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username for the new account")),
		mcp.WithBoolean("admin",
			mcp.Description("Whether to create as admin (ngxborg-admin group)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))
		admin := req.GetBool("admin", false)

		if username == "" {
			return mcp.NewToolResultError("username is required"), nil
		}

		if err := posix.CreateUser(ctx, runner, username, admin); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create user: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("User %q created successfully", username)), nil
	})

	// Delete user
	srv.AddTool(mcp.NewTool("ngxborg_user_delete",
		mcp.WithDescription("Delete a ngxborg user and their account"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))

		if username == "" {
			return mcp.NewToolResultError("username is required"), nil
		}

		if err := runner.Run(ctx, "userdel", "--remove", username); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete user: %v", err)), nil
		}

		// Remove from ngxborg groups if still present
		if posix.InGroup(username, posix.TenantGroup) {
			runner.Run(ctx, "gpasswd", "-d", username, posix.TenantGroup)
		}
		if posix.InGroup(username, posix.AdminGroup) {
			runner.Run(ctx, "gpasswd", "-d", username, posix.AdminGroup)
		}

		return mcp.NewToolResultText(fmt.Sprintf("User %q deleted", username)), nil
	})

	// List users
	srv.AddTool(mcp.NewTool("ngxborg_user_list",
		mcp.WithDescription("List all ngxborg users"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenants, err := posix.ListTenants()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list tenants: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("ngxborg tenants:\n")
		for _, t := range tenants {
			role := "tenant"
			if posix.IsAdmin(t) {
				role = "admin"
			}
			sb.WriteString(fmt.Sprintf("  %s (%s)\n", t, role))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// Disable user
	srv.AddTool(mcp.NewTool("ngxborg_user_disable",
		mcp.WithDescription("Disable a ngxborg user (locks all SSH keys and web UI access)"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to disable")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))

		if username == "" {
			return mcp.NewToolResultError("username is required"), nil
		}

		if err := runner.Run(ctx, "passwd", "-l", username); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to disable user: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("User %q disabled", username)), nil
	})

	// Enable user
	srv.AddTool(mcp.NewTool("ngxborg_user_enable",
		mcp.WithDescription("Enable a disabled ngxborg user"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to enable")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))

		if username == "" {
			return mcp.NewToolResultError("username is required"), nil
		}

		if err := runner.Run(ctx, "passwd", "-u", username); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to enable user: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("User %q enabled", username)), nil
	})

	// Add SSH key
	srv.AddTool(mcp.NewTool("ngxborg_user_key_add",
		mcp.WithDescription("Add an SSH key for a repository"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to add key for")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
		mcp.WithString("pubkey", mcp.Required(),
			mcp.Description("SSH public key or @file path")),
		mcp.WithBoolean("append-only",
			mcp.Description("Enable append-only mode for the repository")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))
		pubkey := strings.TrimSpace(req.GetString("pubkey", ""))
		appendOnly := req.GetBool("append-only", false)

		if username == "" || repo == "" || pubkey == "" {
			return mcp.NewToolResultError("username, repo, and pubkey are required"), nil
		}

		// Read pubkey content
		pubKeyLine := pubkey
		if strings.HasPrefix(pubkey, "@") {
			data, err := os.ReadFile(pubkey[1:])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read pubkey file: %v", err)), nil
			}
			pubKeyLine = strings.TrimSpace(string(data))
		}

		repoPath := borgrepo.Path(username, repo)

		if err := sshaccess.AddKey(username, repoPath, pubKeyLine, appendOnly); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to add key: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Key added for %q restricted to %q", username, repoPath)), nil
	})

	// List keys
	srv.AddTool(mcp.NewTool("ngxborg_user_key_list",
		mcp.WithDescription("List SSH keys for a user"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to list keys for")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))

		if username == "" {
			return mcp.NewToolResultError("username is required"), nil
		}

		keys, err := sshaccess.ListKeys(username)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list keys: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("SSH keys for %q:\n", username))
		if len(keys) == 0 {
			sb.WriteString("  (none)\n")
		}
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("  repo=%s appendOnly=%v type=%s material=%s comment=%q\n",
				k.RepoPath, k.AppendOnly, k.KeyType, k.KeyMaterial, k.Comment))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// Remove key
	srv.AddTool(mcp.NewTool("ngxborg_user_key_remove",
		mcp.WithDescription("Remove an SSH key from a user"),
		mcp.WithString("username", mcp.Required(),
			mcp.Description("Username to remove key from")),
		mcp.WithString("key-material", mcp.Required(),
			mcp.Description("Key material to remove")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username := strings.TrimSpace(req.GetString("username", ""))
		keyMaterial := strings.TrimSpace(req.GetString("key-material", ""))

		if username == "" || keyMaterial == "" {
			return mcp.NewToolResultError("username and key-material are required"), nil
		}

		if err := sshaccess.RemoveKey(username, keyMaterial); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to remove key: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Key %q removed from %q", keyMaterial, username)), nil
	})
}

// ---- Repository Management Tools ----

func registerRepoTools(srv *server.MCPServer) {
	// Create repository
	srv.AddTool(mcp.NewTool("ngxborg_repo_create",
		mcp.WithDescription("Create a new backup repository"),
		mcp.WithString("tenant", mcp.Required(),
			mcp.Description("Tenant username")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))

		if tenant == "" || repo == "" {
			return mcp.NewToolResultError("tenant and repo are required"), nil
		}

		_, err := borgrepo.CreateRepo(tenant, repo)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create repo: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Repository %q created for tenant %q", repo, tenant)), nil
	})

	// List repositories
	srv.AddTool(mcp.NewTool("ngxborg_repo_list",
		mcp.WithDescription("List all repositories"),
		mcp.WithString("tenant",
			mcp.Description("Filter by tenant (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))

		var repos []borgrepo.Repo
		var err error

		if tenant != "" {
			repos, err = borgrepo.ListRepos(tenant)
		} else {
			repos, err = borgrepo.ListAllRepos()
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list repos: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("Repositories:\n")
		if len(repos) == 0 {
			sb.WriteString("  (none)\n")
		}
		for _, r := range repos {
			sb.WriteString(fmt.Sprintf("  tenant=%s name=%s path=%s initialized=%v disabled=%v sizeMB=%d created=%s\n",
				r.Tenant, r.Name, r.Path, r.Initialized, r.Disabled, r.SizeMB, r.CreatedAt.Format("2006-01-02")))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// Delete repository (soft delete)
	srv.AddTool(mcp.NewTool("ngxborg_repo_delete",
		mcp.WithDescription("Soft delete a repository (can be recovered)"),
		mcp.WithString("tenant", mcp.Required(),
			mcp.Description("Tenant username")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))

		if tenant == "" || repo == "" {
			return mcp.NewToolResultError("tenant and repo are required"), nil
		}

		if err := borgrepo.Delete(tenant, repo); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete repo: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Repository %q soft-deleted for tenant %q", repo, tenant)), nil
	})

	// Purge repository (permanent delete)
	srv.AddTool(mcp.NewTool("ngxborg_repo_purge",
		mcp.WithDescription("Permanently delete a repository (irreversible)"),
		mcp.WithString("tenant", mcp.Required(),
			mcp.Description("Tenant username")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
		mcp.WithBoolean("yes", mcp.Required(),
			mcp.Description("Confirm permanent deletion")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))
		confirm := req.GetBool("yes", false)

		if tenant == "" || repo == "" {
			return mcp.NewToolResultError("tenant and repo are required"), nil
		}

		if !confirm {
			return mcp.NewToolResultError("Confirmation required: use --yes flag"), nil
		}

		if err := borgrepo.Purge(tenant, repo); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to purge repo: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Repository %q permanently purged for tenant %q", repo, tenant)), nil
	})

	// Disable repository
	srv.AddTool(mcp.NewTool("ngxborg_repo_disable",
		mcp.WithDescription("Disable a repository (blocks all SSH keys restricted to it)"),
		mcp.WithString("tenant", mcp.Required(),
			mcp.Description("Tenant username")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))

		if tenant == "" || repo == "" {
			return mcp.NewToolResultError("tenant and repo are required"), nil
		}

		if err := borgrepo.Disable(tenant, repo); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to disable repo: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Repository %q disabled for tenant %q", repo, tenant)), nil
	})

	// Enable repository
	srv.AddTool(mcp.NewTool("ngxborg_repo_enable",
		mcp.WithDescription("Enable a disabled repository"),
		mcp.WithString("tenant", mcp.Required(),
			mcp.Description("Tenant username")),
		mcp.WithString("repo", mcp.Required(),
			mcp.Description("Repository name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant := strings.TrimSpace(req.GetString("tenant", ""))
		repo := strings.TrimSpace(req.GetString("repo", ""))

		if tenant == "" || repo == "" {
			return mcp.NewToolResultError("tenant and repo are required"), nil
		}

		if err := borgrepo.Enable(tenant, repo); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to enable repo: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Repository %q enabled for tenant %q", repo, tenant)), nil
	})
}

// ---- System Tools ----

func registerSystemTools(srv *server.MCPServer) {
	// Doctor check
	srv.AddTool(mcp.NewTool("ngxborg_doctor",
		mcp.WithDescription("Run system diagnostics and health checks"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		provCtx, err := provision.New(ctx, false)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create provision context: %v", err)), nil
		}
		results := provCtx.Diagnose()

		var sb strings.Builder
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", r.Status, r.Name, r.Detail))
			if r.Fix != "" {
				sb.WriteString(fmt.Sprintf("  Fix: %s\n", r.Fix))
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// Version
	srv.AddTool(mcp.NewTool("ngxborg_version",
		mcp.WithDescription("Show ngxborg version information"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("ngxborg %s\n", build.Version))
		sb.WriteString(fmt.Sprintf("Maintainer:  %s\n", build.Maintainer))
		sb.WriteString(fmt.Sprintf("Repository:  %s\n", build.RepoURL))
		return mcp.NewToolResultText(sb.String()), nil
	})

	// Setup
	srv.AddTool(mcp.NewTool("ngxborg_setup",
		mcp.WithDescription("Initialize ngxborg on a fresh system"),
		mcp.WithString("admin-port",
			mcp.Description("Admin SSH port, default 22")),
		mcp.WithString("borg-port",
			mcp.Description("Borg SSH port, default 2222")),
		mcp.WithBoolean("dry-run",
			mcp.Description("Show what would be done without making changes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		adminPort := req.GetString("admin-port", "22")
		borgPort := req.GetString("borg-port", "2222")
		dryRun := req.GetBool("dry-run", false)

		// Parse ports
		var adminPortInt, borgPortInt int
		fmt.Sscanf(adminPort, "%d", &adminPortInt)
		fmt.Sscanf(borgPort, "%d", &borgPortInt)

		opts := provision.SetupOptions{
			AdminPort: adminPortInt,
			BorgPort:  borgPortInt,
		}

		provCtx, err := provision.New(ctx, dryRun)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create provision context: %v", err)), nil
		}

		if err := provCtx.Setup(opts); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Setup failed: %v", err)), nil
		}

		return mcp.NewToolResultText("ngxborg setup completed successfully"), nil
	})
}