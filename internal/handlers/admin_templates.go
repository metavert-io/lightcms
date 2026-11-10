package handlers

var adminTemplates = map[string]string{

	// The styled refusal a signed-in user gets for a page or form their role
	// may not use (see refuseAdmin).
	"forbidden": adminLayoutStart + `
        <div class="page-header">
            <h1>Not permitted</h1>
        </div>
        <div class="error-message" id="forbidden-message" role="alert">{{.Message}}</div>
        <div class="form-actions" style="justify-content: flex-start;">
            <a href="/cm" class="btn btn-outline" id="forbidden-back" data-back>Go back</a>
            <a href="/cm" class="btn btn-primary">Dashboard</a>
        </div>
        <script>
        // Return to the page the refused form was on when there is one
        document.getElementById('forbidden-back').addEventListener('click', function(e) {
            if (document.referrer && document.referrer.indexOf(location.origin + '/cm') === 0) {
                e.preventDefault();
                location.href = document.referrer;
            }
        });
        </script>
    ` + adminLayoutEnd,

	"indexnow_tool": adminLayoutStart + `
        <div class="page-header">
            <h1>📡 IndexNow</h1>
            <p style="color: var(--text-muted);">Tells Bing, Yandex, Seznam, Naver and other <a href="https://www.indexnow.org" target="_blank" style="color: var(--primary);">IndexNow</a> search engines the moment a page is published, updated, moved or removed — instead of waiting for them to recrawl.</p>
        </div>

        {{if .Error}}<div class="card" style="border-color: #ef4444; margin-bottom: 1rem;"><p style="color:#f87171; margin:0;">⚠️ {{.Error}}</p></div>{{end}}
        {{if .Saved}}<div class="card" style="border-color: #22c55e; margin-bottom: 1rem;"><p style="color:#4ade80; margin:0;">✅ Saved.</p></div>{{end}}
        {{if .Rotated}}<div class="card" style="border-color: #22c55e; margin-bottom: 1rem;"><p style="color:#4ade80; margin:0;">✅ New key generated. The old key file stops working immediately.</p></div>{{end}}
        {{if .Notice}}<div class="card" style="border-color: #22c55e; margin-bottom: 1rem;"><p style="color:#4ade80; margin:0;">✅ {{.Notice}}</p></div>{{end}}

        <div class="card" style="margin-bottom:1rem;">
            <h3 style="margin-top:0; font-size:1rem;">Status</h3>
            {{if .Status.Active}}
            <p style="margin:0 0 0.5rem;"><span style="color:#4ade80; font-weight:600;">● Active</span> — changes to published pages are submitted automatically.</p>
            {{else if .Status.Ineligible}}
            <p style="margin:0 0 0.5rem;"><span style="color:#e0a030; font-weight:600;">● Inactive on this server</span> — {{.Status.Ineligible}}. IndexNow only runs in production with a public <code>BASE_URL</code>.</p>
            {{else}}
            <p style="margin:0 0 0.5rem;"><span style="color:var(--text-muted); font-weight:600;">● Turned off</span> for this site.</p>
            {{end}}
            {{if .Status.VerifyError}}<p style="color:#f87171; margin:0 0 0.5rem;">Key file check failed: {{.Status.VerifyError}}</p>{{else if .Status.Verified}}<p style="color:var(--text-muted); margin:0 0 0.5rem;">Key file verified at your public URL.</p>{{end}}
            <p style="margin:0 0 0.25rem;">Key file: {{if .Status.KeyURL}}<a href="{{.Status.KeyURL}}" target="_blank" style="color: var(--primary);"><code>{{.Status.KeyURL}}</code></a>{{end}}</p>
            <p style="margin:0 0 0.25rem;">URLs submitted: <strong>{{.Status.TotalSubmitted}}</strong>{{if .Status.PendingCount}} · {{.Status.PendingCount}} queued{{end}}</p>
            <p style="margin:0 0 0.25rem;">Initial full-site submission: {{if .Status.InitialSubmitAt}}{{.Status.InitialSubmitAt.Format "Jan 2, 2006 15:04"}} UTC{{else}}<span style="color:var(--text-muted);">not yet (runs automatically shortly after IndexNow becomes active)</span>{{end}}</p>
            {{if .Status.LastSubmitAt}}<p style="margin:0;">Last submission: {{.Status.LastSubmitAt.Format "Jan 2, 2006 15:04"}} UTC{{if .Status.LastStatus}} · HTTP {{.Status.LastStatus}}{{end}}{{if .Status.LastError}} · <span style="color:#f87171;">{{.Status.LastError}}</span>{{end}}</p>{{end}}
        </div>

        <div class="card" style="margin-bottom:1rem;">
            <form method="POST" action="/cm/tools/indexnow">
                {{.CSRFField}}
                <input type="hidden" name="action" value="save">
                <div style="display:flex; align-items:center; gap:10px; margin-bottom:1rem;">
                    <input type="checkbox" id="enabled" name="enabled" {{if .Status.Enabled}}checked{{end}} style="width:18px; height:18px;">
                    <label for="enabled" style="font-weight:600;">Submit changed pages to IndexNow</label>
                </div>
                <button type="submit" class="btn btn-primary">Save</button>
            </form>
            <div style="display:flex; gap:0.5rem; flex-wrap:wrap; margin-top:1rem; padding-top:1rem; border-top:1px solid var(--border);">
                <form method="POST" action="/cm/tools/indexnow">
                    {{.CSRFField}}
                    <input type="hidden" name="action" value="submit_all">
                    <button type="submit" class="btn" {{if not .Status.Active}}disabled{{end}}>Submit all published pages now</button>
                </form>
                <form method="POST" action="/cm/tools/indexnow">
                    {{.CSRFField}}
                    <input type="hidden" name="action" value="rotate">
                    <button type="submit" class="btn">Generate new key</button>
                </form>
            </div>
            <p style="color:var(--text-muted); font-size:0.85rem; margin:0.75rem 0 0;">Full-site submission is limited to once an hour. Edits are batched for a few seconds, and each URL is resubmitted at most once every 10 minutes. Template, theme and snippet changes that re-render the whole site are not submitted.</p>
        </div>

        {{if .Status.History}}
        <div class="card">
            <h3 style="margin-top:0; font-size:1rem;">Recent submissions</h3>
            <table style="width:100%; border-collapse:collapse; font-size:0.9rem;">
                <thead><tr style="text-align:left; color:var(--text-muted);"><th style="padding:6px 8px;">When (UTC)</th><th style="padding:6px 8px;">Trigger</th><th style="padding:6px 8px;">URLs</th><th style="padding:6px 8px;">Result</th></tr></thead>
                <tbody>
                {{range .Status.History}}
                <tr style="border-top:1px solid var(--border);">
                    <td style="padding:6px 8px; white-space:nowrap;">{{.At.Format "Jan 2 15:04:05"}}</td>
                    <td style="padding:6px 8px;">{{.Trigger}}</td>
                    <td style="padding:6px 8px;" title="{{range .Sample}}{{.}}&#10;{{end}}">{{.URLCount}}</td>
                    <td style="padding:6px 8px;">{{if .Error}}<span style="color:#f87171;">{{.Error}}</span>{{else}}<span style="color:#4ade80;">HTTP {{.Status}}</span>{{end}}</td>
                </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
` + adminLayoutEnd,

	"agent_tool": adminLayoutStart + `
        <div class="page-header">
            <h1>🤵 CMS Agent</h1>
            <p style="color: var(--text-muted);">Your site's built-in agent. It runs the daily analyses (site health, traffic, agent activity) and reports to you by email. More capabilities coming.</p>
        </div>

        {{if .Error}}<div class="card" style="border-color: #ef4444; margin-bottom: 1rem;"><p style="color:#f87171;">⚠️ {{.Error}}</p></div>{{end}}
        {{if .Sent}}<div class="card" style="border-color: #22c55e; margin-bottom: 1rem;"><p style="color:#4ade80; margin:0;">✅ Test digest sent to <strong>{{.Config.Email}}</strong> — check your inbox. (Message logged {{if .Config.LastDigestAt}}{{.Config.LastDigestAt.Format "15:04:05"}} UTC{{end}})</p></div>{{end}}
        {{if .Saved}}<div class="card" style="border-color: #22c55e; margin-bottom: 1rem;"><p style="color:#4ade80; margin:0;">✅ Configuration saved.</p></div>{{end}}
        {{if .SendFailed}}<div class="card" style="border-color: #ef4444; margin-bottom: 1rem;"><p style="color:#f87171; margin:0;">❌ Test digest failed{{if .Config.LastError}}: {{.Config.LastError}}{{end}}. Check your Resend domain verification and the recipient address.</p></div>{{end}}

        {{if not .EmailConfigured}}
        <div class="card" style="border-color: #e0a030; margin-bottom: 1rem;">
            <h3 style="margin-top:0;">📮 Email delivery not configured</h3>
            <p>The CMS Agent sends email through <a href="https://resend.com" target="_blank" style="color: var(--primary);">Resend</a>. To enable it:</p>
            <ol style="line-height:1.9;">
                <li>Create a Resend account and API key, and verify your sending domain.</li>
                <li>Set <code>RESEND_API_KEY</code> and <code>EMAIL_FROM</code> (e.g. <code>LightCMS Agent &lt;agent@yourdomain.com&gt;</code>) as environment variables — or <code>resend_api_key</code> / <code>email_from</code> in config.dev.json.</li>
                <li>Restart the server.</li>
            </ol>
        </div>
        {{end}}

        <div class="card">
            <form method="POST" action="/cm/tools/agent/config">
                {{.CSRFField}}
                <div style="display:flex; align-items:center; gap:10px; margin-bottom:1.25rem;">
                    <input type="checkbox" id="enabled" name="enabled" {{if .Config.Enabled}}checked{{end}} style="width:18px; height:18px;">
                    <label for="enabled" style="font-weight:600;">Send me digests</label>
                </div>

                <div style="display:grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap:1rem; margin-bottom:1.25rem;">
                    <div>
                        <label style="display:block; font-size:0.85rem; color:var(--text-muted); margin-bottom:4px;">Recipient email</label>
                        <input type="email" name="email" value="{{.Config.Email}}" placeholder="you@example.com"
                            style="width:100%; padding:8px 10px; border:1px solid var(--border); border-radius:8px; background:var(--bg); color:var(--text); font:inherit;">
                    </div>
                    <div>
                        <label style="display:block; font-size:0.85rem; color:var(--text-muted); margin-bottom:4px;">Frequency</label>
                        <select name="frequency" style="width:100%; padding:8px 10px; border:1px solid var(--border); border-radius:8px; background:var(--bg); color:var(--text); font:inherit;">
                            <option value="daily" {{if eq .Config.Frequency "daily"}}selected{{end}}>Daily</option>
                            <option value="weekdays" {{if eq .Config.Frequency "weekdays"}}selected{{end}}>Weekdays only</option>
                            <option value="weekly" {{if eq .Config.Frequency "weekly"}}selected{{end}}>Weekly (Mondays)</option>
                        </select>
                    </div>
                    <div>
                        <label style="display:block; font-size:0.85rem; color:var(--text-muted); margin-bottom:4px;">Send hour (UTC)</label>
                        <input type="number" name="send_hour" min="0" max="23" value="{{.Config.SendHour}}"
                            style="width:100%; padding:8px 10px; border:1px solid var(--border); border-radius:8px; background:var(--bg); color:var(--text); font:inherit;">
                    </div>
                </div>

                <h3 style="font-size:1rem; margin-bottom:0.5rem;">What the agent reports on</h3>
                <div style="display:grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap:0.5rem 1.5rem; margin-bottom:1.5rem;">
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_site_health" {{if .Config.IncludeSiteHealth}}checked{{end}}> <span><strong>Site health</strong> — stale pages, missing meta descriptions, lingering drafts</span></label>
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_traffic" {{if .Config.IncludeTraffic}}checked{{end}}> <span><strong>Traffic</strong> — visitors, uptime, top pages and referrers</span></label>
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_pending" {{if .Config.IncludePending}}checked{{end}}> <span><strong>Awaiting review</strong> — forks to merge, approvals, scheduled posts</span></label>
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_agent_work" {{if .Config.IncludeAgentWork}}checked{{end}}> <span><strong>Agent activity</strong> — what AI agents changed, from provenance</span></label>
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_broken_links" {{if .Config.IncludeBrokenLinks}}checked{{end}}> <span><strong>Broken links</strong> — runs a link-check job (slower)</span></label>
                    <label style="display:flex; gap:8px; align-items:flex-start;"><input type="checkbox" name="include_ai_commentary" {{if .Config.IncludeAICommentary}}checked{{end}} {{if not .AIAvailable}}disabled{{end}}> <span><strong>AI commentary</strong> — Claude writes an executive summary{{if not .AIAvailable}} (requires ANTHROPIC_API_KEY){{end}}</span></label>
                </div>

                <button type="submit" class="btn btn-primary">Save Configuration</button>
            </form>
        </div>

        <div class="card" style="margin-top:1rem;">
            <h3 style="margin-top:0; font-size:1rem;">Status</h3>
            {{if .Config.LastDigestAt}}<p>Last digest: {{.Config.LastDigestAt.Format "Jan 2, 2006 15:04"}} UTC</p>{{else}}<p style="color:var(--text-muted);">No digest sent yet.</p>{{end}}
            {{if .Config.LastError}}<p style="color:#f87171;">Last error: {{.Config.LastError}}</p>{{end}}
            <form method="POST" action="/cm/tools/agent/test" style="margin-top:0.5rem;">
                {{.CSRFField}}
                <button type="submit" class="btn" {{if not .EmailConfigured}}disabled{{end}}>Send test digest now</button>
            </form>
        </div>
` + adminLayoutEnd,

	"login": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - LightCMS</title>
    <link rel="icon" type="image/x-icon" href="/static/images/favicon.ico">
    <link rel="icon" type="image/png" sizes="16x16" href="/static/images/favicon-16x16.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/static/images/favicon-32x32.png">
    <link rel="icon" type="image/png" sizes="48x48" href="/static/images/favicon-48x48.png">
    <link rel="apple-touch-icon" sizes="180x180" href="/static/images/apple-touch-icon.png">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Inter', system-ui, sans-serif;
            background: linear-gradient(135deg, #0f172a 0%, #1e1b4b 50%, #0f172a 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1rem;
        }
        .login-card {
            background: rgba(30, 27, 75, 0.5);
            backdrop-filter: blur(20px);
            border: 1px solid rgba(99, 102, 241, 0.2);
            border-radius: 24px;
            padding: 3rem;
            width: 100%;
            max-width: 420px;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        .logo {
            text-align: center;
            margin-bottom: 0.5rem;
        }
        .logo img {
            height: 48px;
            width: auto;
        }
        .subtitle {
            color: #94a3b8;
            text-align: center;
            margin-bottom: 2rem;
            font-size: 0.9rem;
        }
        .error {
            background: rgba(239, 68, 68, 0.1);
            border: 1px solid rgba(239, 68, 68, 0.3);
            color: #f87171;
            padding: 0.75rem 1rem;
            border-radius: 8px;
            margin-bottom: 1.5rem;
            font-size: 0.9rem;
        }
        label {
            display: block;
            color: #e2e8f0;
            margin-bottom: 0.5rem;
            font-weight: 500;
        }
        input[type="email"], input[type="password"] {
            width: 100%;
            padding: 0.875rem 1rem;
            background: rgba(15, 23, 42, 0.5);
            border: 1px solid rgba(99, 102, 241, 0.3);
            border-radius: 12px;
            color: #f1f5f9;
            font-size: 1rem;
            transition: all 0.2s;
            margin-bottom: 1rem;
        }
        input[type="email"]:focus, input[type="password"]:focus {
            outline: none;
            border-color: #6366f1;
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
        }
        button {
            width: 100%;
            padding: 0.875rem;
            background: linear-gradient(135deg, #6366f1, #8b5cf6);
            border: none;
            border-radius: 12px;
            color: white;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            margin-top: 1.5rem;
            transition: all 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 20px -10px rgba(99, 102, 241, 0.5);
        }
    </style>
</head>
<body>
    <div class="login-card">
        <h1 class="logo"><img src="/static/images/lightcms-logo.png" alt="LightCMS"></h1>
        <p class="subtitle">Content Management System</p>
        {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
        <form method="POST" action="/cm/login" autocomplete="off">
            {{.CSRFField}}
            <label for="email">Email</label>
            <input type="email" id="email" name="email" placeholder="Enter email" value="{{.Email}}" required autofocus autocomplete="username" {{if .RateLimited}}disabled{{end}}>
            <label for="password">Password</label>
            <input type="password" id="password" name="password" placeholder="Enter password" required autocomplete="current-password" {{if .RateLimited}}disabled{{end}}>
            <button type="submit" {{if .RateLimited}}disabled style="opacity: 0.5; cursor: not-allowed;"{{end}}>Sign In</button>
        </form>
    </div>
</body>
</html>`,

	"dashboard": adminLayoutStart + `
        <div class="dashboard">
            <h1>Dashboard</h1>
            <div class="stats-grid">
                <div class="stat-card">
                    <div class="stat-icon">📄</div>
                    <div class="stat-info">
                        <span class="stat-value">{{.ContentCount}}</span>
                        <span class="stat-label">Content Items</span>
                    </div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon">📋</div>
                    <div class="stat-info">
                        <span class="stat-value">{{.TemplateCount}}</span>
                        <span class="stat-label">Templates</span>
                    </div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon">👤</div>
                    <div class="stat-info">
                        <span class="stat-value">{{.DAU}}</span>
                        <span class="stat-label">Daily Active Users</span>
                    </div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon">👥</div>
                    <div class="stat-info">
                        <span class="stat-value">{{.MAU}}</span>
                        <span class="stat-label">Monthly Active Users</span>
                    </div>
                </div>
                <div class="stat-card">
                    <div class="stat-icon">✏️</div>
                    <div class="stat-info">
                        <span class="stat-value">{{.ContentCreatedToday}}</span>
                        <span class="stat-label">Pages Created Today</span>
                    </div>
                </div>
            </div>

            <div class="quick-actions">
                <h2>Quick Actions</h2>
                <div class="action-buttons">
                    {{if index $.Can "content.create"}}<a href="/cm/content/new" class="btn btn-primary">New Content</a>{{end}}
                    {{if index $.Can "template.create"}}<a href="/cm/templates/new" class="btn btn-secondary">New Template</a>{{end}}
                    <a href="/" target="_blank" class="btn btn-outline">View Site</a>
                </div>
            </div>

            {{if .RecentContent}}
            <div class="recent-content">
                <h2>Recent Content</h2>
                <div class="content-list">
                    {{range .RecentContent}}
                    <div class="content-item">
                        <span class="content-info">
                            <span class="content-title">{{.Title}}</span>
                            <span class="content-slug">/{{if .Slug}}{{.Slug}}{{else}}(homepage){{end}}</span>
                        </span>
                        <span class="content-template">{{.TemplateName}}</span>
                        <span class="content-status {{if .Published}}published{{else}}draft{{end}}">
                            {{if .Published}}Published{{else}}Draft{{end}}
                        </span>
                        <span class="content-actions">
                            <a href="/{{.Slug}}" target="_blank" class="btn btn-sm btn-outline">View</a>
                            <a href="/cm/content/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                        </span>
                    </div>
                    {{end}}
                </div>
            </div>
            {{end}}

            {{if .PendingApprovals}}
            <div class="recent-content" style="margin-top:2rem;">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.75rem;">
                    <h2 style="margin:0;">Requiring Approval</h2>
                    <a href="/cm/approvals" class="btn btn-sm btn-outline">View All</a>
                </div>
                <div class="table-container">
                    <table>
                        <thead><tr><th>Content</th><th>Submitted By</th><th>Submitted</th></tr></thead>
                        <tbody>
                            {{range .PendingApprovals}}
                            <tr>
                                <td><a href="/cm/content/{{.ContentID.Hex}}">{{if .ContentTitle}}{{.ContentTitle}}{{else}}{{.ContentID.Hex}}{{end}}</a></td>
                                <td style="font-size:0.85rem;color:var(--muted);">{{.SubmittedByEmail}}</td>
                                <td style="font-size:0.85rem;color:var(--muted);">{{.CreatedAt.Format "Jan 2, 3:04 PM"}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
            </div>
            {{end}}

            {{if .RecentComments}}
            <div class="recent-content" style="margin-top:2rem;">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.75rem;">
                    <h2 style="margin:0;">Recent Comments</h2>
                </div>
                <div style="display:flex;flex-direction:column;gap:0.75rem;">
                    {{range .RecentComments}}
                    <div style="display:flex;gap:0.75rem;align-items:flex-start;padding:0.75rem;background:var(--bg-card);border-radius:var(--radius);border:1px solid var(--border);">
                        <div style="flex:1;min-width:0;">
                            <div style="display:flex;align-items:center;gap:0.5rem;margin-bottom:0.25rem;flex-wrap:wrap;">
                                <span style="font-weight:600;font-size:0.85rem;">{{.UserDisplayName}}</span>
                                <span style="color:var(--muted);font-size:0.8rem;">{{.CreatedAt.Format "Jan 2, 3:04 PM"}}</span>
                                <a href="/cm/content/{{.ContentID.Hex}}" style="margin-left:auto;font-size:0.8rem;color:var(--accent);">View page</a>
                            </div>
                            <p style="margin:0;color:var(--text-muted);font-size:0.9rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{.Text}}</p>
                        </div>
                    </div>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
    ` + adminLayoutEnd,

	"templates_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Templates</h1>
            {{if index $.Can "template.create"}}<a href="/cm/templates/new" class="btn btn-primary">New Template</a>{{end}}
        </div>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Category</th>
                        <th>Fields</th>
                        <th>System</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Templates}}
                    <tr>
                        <td><strong>{{.Name}}</strong><br><small>{{.Description}}</small></td>
                        <td>{{.Category}}</td>
                        <td>{{len .Fields}}</td>
                        <td>{{if .IsSystem}}Yes{{else}}No{{end}}</td>
                        <td class="actions">
                            <a href="/cm/templates/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                            {{if not .IsSystem}}
                            <form method="POST" action="/cm/templates/{{.ID.Hex}}/delete" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this template?')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                            {{end}}
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"template_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New Template{{else}}Edit Template{{end}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-group">
                <label for="name">Template Name</label>
                <input type="text" id="name" name="name" value="{{if .Template}}{{.Template.Name}}{{end}}" required>
            </div>
            <div class="form-group">
                <label for="description">Description</label>
                <textarea id="description" name="description" rows="2">{{if .Template}}{{.Template.Description}}{{end}}</textarea>
            </div>
            <div class="form-group">
                <label for="category">Category</label>
                <input type="text" id="category" name="category" value="{{if .Template}}{{.Template.Category}}{{end}}" placeholder="e.g., blog, press, pages">
            </div>

            <div class="form-section">
                <h3>Fields</h3>
                <div id="fields-container">
                    {{if .Template}}{{range .Template.Fields}}
                    <div class="field-row">
                        <input type="text" name="field_name[]" value="{{.Name}}" placeholder="Field name">
                        <input type="text" name="field_label[]" value="{{.Label}}" placeholder="Label">
                        <select name="field_type[]">
                            <option value="text" {{if eq .Type "text"}}selected{{end}}>Text</option>
                            <option value="textarea" {{if eq .Type "textarea"}}selected{{end}}>Textarea</option>
                            <option value="richtext" {{if eq .Type "richtext"}}selected{{end}}>Rich Text</option>
                            <option value="markdown" {{if eq .Type "markdown"}}selected{{end}}>Markdown</option>
                            <option value="rawhtml" {{if eq .Type "rawhtml"}}selected{{end}}>Raw HTML</option>
                            <option value="date" {{if eq .Type "date"}}selected{{end}}>Date</option>
                            <option value="image" {{if eq .Type "image"}}selected{{end}}>Image</option>
                            <option value="select" {{if eq .Type "select"}}selected{{end}}>Select</option>
                        </select>
                        <input type="text" name="field_placeholder[]" value="{{.Placeholder}}" placeholder="Placeholder">
                        <input type="text" name="field_options[]" value="{{.Options}}" placeholder="Options (comma-sep)">
                        <label class="checkbox-label"><input type="checkbox" name="field_required[]" {{if .Required}}checked{{end}}> Required</label>
                        <button type="button" class="btn btn-sm btn-danger" onclick="this.parentElement.remove()">×</button>
                    </div>
                    {{end}}{{end}}
                </div>
                <button type="button" class="btn btn-secondary" onclick="addField()">Add Field</button>
            </div>

            <div class="form-group">
                <label for="html_layout">HTML Layout</label>
                <p class="help-text">Use {{.FieldName}} placeholders. Available: {{.title}}, {{.slug}}, {{.published_at}}, plus your custom fields.</p>
                <textarea id="html_layout" name="html_layout" rows="15" class="code-editor">{{if .Template}}{{.Template.HTMLLayout}}{{end}}</textarea>
            </div>

            <div class="form-actions">
                <a href="/cm/templates" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">{{if .IsNew}}Create Template{{else}}Update Template{{end}}</button>
            </div>
        </form>

        <script>
        function addField() {
            const container = document.getElementById('fields-container');
            const row = document.createElement('div');
            row.className = 'field-row';
            row.innerHTML = ` + "`" + `
                <input type="text" name="field_name[]" placeholder="Field name">
                <input type="text" name="field_label[]" placeholder="Label">
                <select name="field_type[]">
                    <option value="text">Text</option>
                    <option value="textarea">Textarea</option>
                    <option value="richtext">Rich Text</option>
                    <option value="markdown">Markdown</option>
                    <option value="rawhtml">Raw HTML</option>
                    <option value="date">Date</option>
                    <option value="image">Image</option>
                    <option value="select">Select</option>
                </select>
                <input type="text" name="field_placeholder[]" placeholder="Placeholder">
                <input type="text" name="field_options[]" placeholder="Options (comma-sep)">
                <label class="checkbox-label"><input type="checkbox" name="field_required[]"> Required</label>
                <button type="button" class="btn btn-sm btn-danger" onclick="this.parentElement.remove()">×</button>
            ` + "`" + `;
            container.appendChild(row);
        }
        </script>
    ` + adminLayoutEnd,

	"content_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Content</h1>
            {{if index $.Can "content.create"}}<a href="/cm/content/new" class="btn btn-primary">New Content</a>{{end}}
        </div>

        <div class="filter-bar" style="display: flex; gap: 1rem; margin-bottom: 1.5rem; align-items: center; flex-wrap: wrap;">
            <div class="filter-group" style="display: flex; gap: 0.5rem; align-items: center;">
                <label style="font-size: 0.9rem; color: var(--muted);">Folder:</label>
                <select id="folder-filter" onchange="applyFilters()" style="padding: 0.5rem; background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text);">
                    <option value="all" {{if eq .FolderFilter "all"}}selected{{else if eq .FolderFilter ""}}selected{{end}}>All Folders</option>
                    <option value="root" {{if eq .FolderFilter "root"}}selected{{end}}>/ (Root)</option>
                    {{range .Folders}}
                    <option value="{{.Path}}" {{if eq $.FolderFilter .Path}}selected{{end}}>{{.Path}}</option>
                    {{end}}
                </select>
            </div>
            <div class="filter-group" style="display: flex; gap: 0.5rem; align-items: center;">
                <button type="button" onclick="openSearchModal()" class="btn btn-sm btn-outline" title="Search Content" style="display: flex; align-items: center; gap: 0.4rem;">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <circle cx="11" cy="11" r="8"></circle>
                        <path d="M21 21l-4.35-4.35"></path>
                    </svg>
                    Search
                </button>
            </div>
            <div id="search-indicator" class="filter-group" style="display: none; gap: 0.5rem; align-items: center;">
                <span id="search-query-display" style="font-size: 0.9rem; color: var(--primary);"></span>
                <button type="button" onclick="clearSearch()" class="btn btn-sm btn-outline">Show All</button>
            </div>
        </div>

        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Title</th>
                        <th>Template</th>
                        <th>Path</th>
                        <th>Status</th>
                        <th>Updated</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Content}}
                    <tr{{if .Deleted}} style="opacity: 0.7;"{{end}}>
                        <td><strong>{{.Title}}</strong></td>
                        <td>{{.TemplateName}}</td>
                        <td><code>{{if .Deleted}}(deleted){{else if .FullPath}}{{.FullPath}}{{else}}/{{.Slug}}{{end}}</code></td>
                        <td>
                            {{if .Deleted}}
                            <span class="status-badge" style="background: var(--danger);">Deleted</span>
                            {{else}}
                            <span class="status-badge {{if .Published}}published{{else}}draft{{end}}">{{if .Published}}Published{{else}}Draft{{end}}</span>
                            {{if .Hold}}<span class="status-badge" style="background: var(--warning);" title="On hold: cannot be published until the hold is cleared">Held</span>{{end}}
                            {{end}}
                        </td>
                        <td>{{.UpdatedAt.Format "Jan 2, 2006"}}</td>
                        <td class="actions">
                            {{if .Deleted}}
                            <a href="/cm/content/{{.ID.Hex}}/versions/latest/view" target="_blank" class="btn btn-sm btn-outline">View</a>
                            <a href="/cm/content/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                            <form method="POST" action="/cm/content/{{.ID.Hex}}/undelete" style="display:inline">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-primary">Restore</button>
                            </form>
                            {{else}}
                            <a href="{{if .FullPath}}{{.FullPath}}{{else}}/{{.Slug}}{{end}}" target="_blank" class="btn btn-sm btn-outline">View</a>
                            <a href="/cm/content/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                            <form method="POST" action="/cm/content/{{.ID.Hex}}/regenerate" style="display:inline">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-secondary" title="Regenerate static file">↻</button>
                            </form>
                            <form method="POST" action="/cm/content/{{.ID.Hex}}/delete" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this content?')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                            {{end}}
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>

        {{if gt .TotalPages 1}}
        <div style="display: flex; align-items: center; justify-content: space-between; margin-top: 1rem; padding: 0.75rem 0;">
            <span style="color: var(--text-muted); font-size: 0.9rem;">{{.Total}} pages &middot; page {{.CurrentPage}} of {{.TotalPages}}</span>
            <div style="display: flex; gap: 0.5rem;">
                {{if gt .CurrentPage 1}}<a href="?page={{subtract .CurrentPage 1}}{{if .FolderFilter}}&folder={{.FolderFilter}}{{end}}{{if .ShowDeleted}}&deleted=true{{end}}" class="btn btn-outline">← Previous</a>{{end}}
                {{if lt .CurrentPage .TotalPages}}<a href="?page={{add .CurrentPage 1}}{{if .FolderFilter}}&folder={{.FolderFilter}}{{end}}{{if .ShowDeleted}}&deleted=true{{end}}" class="btn btn-outline">Next →</a>{{end}}
            </div>
        </div>
        {{end}}

        <!-- Search Modal -->
        <div id="search-modal" class="modal-overlay" style="display: none;">
            <div class="modal-content" style="max-width: 500px;">
                <div class="modal-header">
                    <h2 id="search-modal-title">Search Content</h2>
                    <button type="button" onclick="closeSearchModal()" class="modal-close">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group" style="margin-bottom: 1rem;">
                        <label style="display: block; margin-bottom: 0.5rem; font-weight: 500;">Search Query</label>
                        <input type="text" id="search-query" placeholder="Leave empty to show all..."
                               style="width: 100%; padding: 0.75rem; background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text); font-size: 1rem;"
                               onkeydown="if(event.key==='Enter') { if(document.getElementById('enable-replace').checked) { previewReplace(); } else { performSearch(); } }">
                    </div>
                    <div class="form-group" style="margin-bottom: 1rem;">
                        <label style="display: block; margin-bottom: 0.5rem; font-weight: 500;">Search In</label>
                        <div style="display: flex; gap: 1rem;">
                            <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                                <input type="radio" name="search-type" value="name" checked style="accent-color: var(--primary);" onchange="updateSearchMode()">
                                <span>Title only</span>
                            </label>
                            <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                                <input type="radio" name="search-type" value="slug" style="accent-color: var(--primary);" onchange="updateSearchMode()">
                                <span>Slug</span>
                            </label>
                            <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                                <input type="radio" name="search-type" value="fulltext" style="accent-color: var(--primary);" onchange="updateSearchMode()">
                                <span>Full text</span>
                            </label>
                        </div>
                    </div>
                    <div class="form-group" style="margin-bottom: 1rem;">
                        <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                            <input type="checkbox" id="include-deleted" style="accent-color: var(--danger); width: 16px; height: 16px;">
                            <span style="color: var(--danger);">Include deleted content</span>
                        </label>
                    </div>
                    <div class="form-group" style="margin-bottom: 1rem; padding-top: 0.5rem; border-top: 1px solid var(--border);">
                        <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                            <input type="checkbox" id="enable-replace" style="accent-color: var(--warning); width: 16px; height: 16px;" onchange="toggleReplaceMode()">
                            <span style="color: var(--warning);">Search and Replace (full text only)</span>
                        </label>
                    </div>
                    <div id="replace-field" class="form-group" style="margin-bottom: 1.5rem; display: none;">
                        <label style="display: block; margin-bottom: 0.5rem; font-weight: 500;">Replace With</label>
                        <input type="text" id="replace-query" placeholder="Replacement text..."
                               style="width: 100%; padding: 0.75rem; background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text); font-size: 1rem;">
                    </div>
                    <button type="button" id="search-btn" onclick="performSearch()" class="btn btn-primary" style="width: 100%;">Search</button>
                    <button type="button" id="preview-replace-btn" onclick="previewReplace()" class="btn btn-warning" style="width: 100%; display: none;">Preview Replacements</button>
                </div>
            </div>
        </div>

        <!-- Replace Preview Modal -->
        <div id="replace-preview-modal" class="modal-overlay" style="display: none;">
            <div class="modal-content" style="max-width: 900px;">
                <div class="modal-header" style="background: rgba(234, 179, 8, 0.1); border-bottom-color: var(--warning);">
                    <h2 style="color: var(--warning);">Search and Replace Preview</h2>
                    <button type="button" onclick="closeReplacePreview()" class="modal-close">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="replace-warning" style="background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: var(--radius); padding: 1rem; margin-bottom: 1.5rem;">
                        <strong style="color: var(--danger);">Warning:</strong> Site-wide replace can be destructive. Please carefully review all changes below before proceeding. Each affected page will have a version saved before changes are made.
                    </div>
                    <div id="replace-summary" style="margin-bottom: 1rem; padding: 0.75rem; background: var(--card-bg); border-radius: var(--radius);">
                        <!-- Summary will be inserted here -->
                    </div>
                    <div id="replace-preview-list" style="max-height: 400px; overflow-y: auto; margin-bottom: 1.5rem;">
                        <!-- Preview items will be inserted here -->
                    </div>
                    <div style="display: flex; gap: 1rem;">
                        <button type="button" onclick="closeReplacePreview()" class="btn btn-outline" style="flex: 1;">Cancel</button>
                        <button type="button" id="execute-replace-btn" onclick="executeReplace()" class="btn btn-danger" style="flex: 1;">Accept Replacements</button>
                    </div>
                </div>
            </div>
        </div>

        <style>
            code {
                font-family: 'JetBrains Mono', monospace;
                background: rgba(99, 102, 241, 0.1);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.85rem;
            }
            .modal-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 1000;
                backdrop-filter: blur(4px);
            }
            .modal-content {
                background: var(--background);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                width: 90%;
                max-height: 90vh;
                overflow-y: auto;
                box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
            }
            .modal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem 1.5rem;
                border-bottom: 1px solid var(--border);
            }
            .modal-header h2 {
                margin: 0;
                font-size: 1.25rem;
            }
            .modal-close {
                background: none;
                border: none;
                color: var(--muted);
                font-size: 1.5rem;
                cursor: pointer;
                padding: 0;
                line-height: 1;
            }
            .modal-close:hover {
                color: var(--text);
            }
            .modal-body {
                padding: 1.5rem;
            }
            .search-results-row {
                opacity: 0;
                animation: fadeIn 0.3s ease forwards;
            }
            @keyframes fadeIn {
                to { opacity: 1; }
            }
            .btn-warning {
                background: linear-gradient(135deg, #eab308, #ca8a04);
                color: #000;
            }
            .btn-warning:hover {
                box-shadow: 0 10px 20px -10px rgba(234, 179, 8, 0.5);
            }
            .replace-preview-item {
                background: var(--card-bg);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1rem;
                margin-bottom: 0.75rem;
            }
            .replace-preview-item h4 {
                margin: 0 0 0.5rem 0;
                font-size: 1rem;
            }
            .replace-preview-item .path {
                font-size: 0.85rem;
                color: var(--muted);
                margin-bottom: 0.75rem;
            }
            .replace-excerpt {
                font-family: 'JetBrains Mono', monospace;
                font-size: 0.85rem;
                background: rgba(15, 23, 42, 0.5);
                padding: 0.75rem;
                border-radius: 4px;
                overflow-x: auto;
                white-space: pre-wrap;
                word-break: break-word;
            }
            .replace-old {
                background: rgba(239, 68, 68, 0.2);
                color: #fca5a5;
                text-decoration: line-through;
            }
            .replace-new {
                background: rgba(34, 197, 94, 0.2);
                color: #86efac;
            }
        </style>
        <script>
            var csrfToken = '{{.CSRFToken}}';
            var originalTableBody = null;
            var isSearchActive = false;

            function applyFilters() {
                var folder = document.getElementById('folder-filter').value;
                var params = new URLSearchParams(window.location.search);
                if (folder && folder !== 'all') {
                    params.set('folder', folder);
                } else {
                    params.delete('folder');
                }
                window.location.search = params.toString();
            }

            function openSearchModal() {
                document.getElementById('search-modal').style.display = 'flex';
                document.getElementById('search-query').focus();
            }

            function closeSearchModal() {
                document.getElementById('search-modal').style.display = 'none';
            }

            function performSearch() {
                var query = document.getElementById('search-query').value.trim();
                var searchType = document.querySelector('input[name="search-type"]:checked').value;
                var includeDeleted = document.getElementById('include-deleted').checked;

                // Store original table body if not already stored
                if (!originalTableBody) {
                    originalTableBody = document.querySelector('tbody').innerHTML;
                }

                // Show loading state
                var tbody = document.querySelector('tbody');
                tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--muted);">Searching...</td></tr>';

                var url = '/api/content/search?type=' + searchType;
                if (query) {
                    url += '&q=' + encodeURIComponent(query);
                }
                if (includeDeleted) {
                    url += '&deleted=true';
                }

                fetch(url)
                    .then(function(response) { return response.json(); })
                    .then(function(results) {
                        closeSearchModal();
                        displaySearchResults(results, query, includeDeleted);
                    })
                    .catch(function(err) {
                        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--danger);">Search failed: ' + escapeHtml(err.message) + '</td></tr>';
                    });
            }

            function displaySearchResults(results, query, includeDeleted) {
                var tbody = document.querySelector('tbody');
                isSearchActive = true;

                // Show search indicator with appropriate message
                document.getElementById('search-indicator').style.display = 'flex';
                var indicatorText = '';
                if (query) {
                    indicatorText = 'Search: <strong>' + escapeHtml(query) + '</strong>';
                } else {
                    indicatorText = '<strong>All content</strong>';
                }
                if (includeDeleted) {
                    indicatorText += ' <span style="color: var(--danger);">(including deleted)</span>';
                }
                document.getElementById('search-query-display').innerHTML = indicatorText;

                if (results.length === 0) {
                    var msg = query ? 'No results found for "' + escapeHtml(query) + '"' : 'No content found';
                    tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--muted);">' + msg + '</td></tr>';
                    return;
                }

                var html = '';
                results.forEach(function(item, index) {
                    var rowStyle = item.deleted ? ' style="opacity: 0.7; animation-delay: ' + (index * 0.05) + 's;"' : ' style="animation-delay: ' + (index * 0.05) + 's;"';
                    html += '<tr class="search-results-row"' + rowStyle + '>';
                    html += '<td><strong>' + escapeHtml(item.title) + '</strong></td>';
                    html += '<td>' + escapeHtml(item.template_name) + '</td>';
                    html += '<td><code>' + (item.deleted ? '(deleted)' : escapeHtml(item.full_path)) + '</code></td>';

                    if (item.deleted) {
                        html += '<td><span class="status-badge" style="background: var(--danger);">Deleted</span></td>';
                    } else {
                        var statusClass = item.published ? 'published' : 'draft';
                        var statusText = item.published ? 'Published' : 'Draft';
                        var heldBadge = item.hold ? ' <span class="status-badge" style="background: var(--warning);" title="On hold: cannot be published until the hold is cleared">Held</span>' : '';
                        html += '<td><span class="status-badge ' + statusClass + '">' + statusText + '</span>' + heldBadge + '</td>';
                    }

                    html += '<td>' + escapeHtml(item.updated_at) + '</td>';
                    html += '<td class="actions">';
                    if (item.deleted) {
                        html += '<a href="/cm/content/' + item.id + '/versions/latest/view" target="_blank" class="btn btn-sm btn-outline">View</a>';
                        html += '<a href="/cm/content/' + item.id + '" class="btn btn-sm">Edit</a>';
                        html += '<form method="POST" action="/cm/content/' + item.id + '/undelete" style="display:inline">';
                        html += '<input type="hidden" name="gorilla.csrf.Token" value="' + csrfToken + '">';
                        html += '<button type="submit" class="btn btn-sm btn-primary">Restore</button>';
                        html += '</form>';
                    } else {
                        html += '<a href="' + item.full_path + '" target="_blank" class="btn btn-sm btn-outline">View</a>';
                        html += '<a href="/cm/content/' + item.id + '" class="btn btn-sm">Edit</a>';
                        html += '<form method="POST" action="/cm/content/' + item.id + '/delete" style="display:inline" onsubmit="return confirmDelete(this, \'Are you sure you want to delete this content?\')">';
                        html += '<input type="hidden" name="gorilla.csrf.Token" value="' + csrfToken + '">';
                        html += '<button type="submit" class="btn btn-sm btn-danger">Delete</button>';
                        html += '</form>';
                    }
                    html += '</td>';
                    html += '</tr>';
                });
                tbody.innerHTML = html;
            }

            function clearSearch() {
                if (originalTableBody) {
                    document.querySelector('tbody').innerHTML = originalTableBody;
                }
                document.getElementById('search-indicator').style.display = 'none';
                document.getElementById('search-query').value = '';
                document.getElementById('include-deleted').checked = false;
                isSearchActive = false;
            }

            function escapeHtml(text) {
                var div = document.createElement('div');
                div.textContent = text;
                return div.innerHTML;
            }

            // Search and Replace functions
            var replacePreviewData = null;

            function toggleReplaceMode() {
                var enabled = document.getElementById('enable-replace').checked;
                var replaceField = document.getElementById('replace-field');
                var searchBtn = document.getElementById('search-btn');
                var previewBtn = document.getElementById('preview-replace-btn');
                var titleEl = document.getElementById('search-modal-title');

                if (enabled) {
                    replaceField.style.display = 'block';
                    searchBtn.style.display = 'none';
                    previewBtn.style.display = 'block';
                    titleEl.textContent = 'Search and Replace';
                    // Force full text mode
                    document.querySelector('input[name="search-type"][value="fulltext"]').checked = true;
                    document.getElementById('include-deleted').checked = false;
                    document.getElementById('include-deleted').disabled = true;
                } else {
                    replaceField.style.display = 'none';
                    searchBtn.style.display = 'block';
                    previewBtn.style.display = 'none';
                    titleEl.textContent = 'Search Content';
                    document.getElementById('include-deleted').disabled = false;
                }
            }

            function updateSearchMode() {
                var searchType = document.querySelector('input[name="search-type"]:checked').value;
                if (searchType !== 'fulltext' && document.getElementById('enable-replace').checked) {
                    document.getElementById('enable-replace').checked = false;
                    toggleReplaceMode();
                }
            }

            function previewReplace() {
                var searchQuery = document.getElementById('search-query').value.trim();
                var replaceQuery = document.getElementById('replace-query').value;

                if (!searchQuery) {
                    showAlert('Please enter a search query for replace.', 'Search and Replace');
                    return;
                }

                // Show loading in preview modal
                document.getElementById('replace-preview-modal').style.display = 'flex';
                document.getElementById('replace-preview-list').innerHTML = '<div style="text-align: center; padding: 2rem; color: var(--muted);">Loading preview...</div>';
                document.getElementById('replace-summary').innerHTML = '';
                document.getElementById('execute-replace-btn').disabled = true;

                fetch('/cm/replace/preview?search=' + encodeURIComponent(searchQuery) + '&replace=' + encodeURIComponent(replaceQuery))
                    .then(function(response) { return response.json(); })
                    .then(function(data) {
                        if (data.error) {
                            // e.g. 403: search and replace is limited to administrators
                            replacePreviewData = null;
                            document.getElementById('replace-preview-list').innerHTML = '<div style="text-align: center; padding: 2rem; color: var(--danger);">' + escapeHtml(data.error) + '</div>';
                            return;
                        }
                        replacePreviewData = data;
                        displayReplacePreview(data, searchQuery, replaceQuery);
                    })
                    .catch(function(err) {
                        document.getElementById('replace-preview-list').innerHTML = '<div style="text-align: center; padding: 2rem; color: var(--danger);">Failed to load preview: ' + escapeHtml(err.message) + '</div>';
                    });
            }

            function displayReplacePreview(data, searchQuery, replaceQuery) {
                var listEl = document.getElementById('replace-preview-list');
                var summaryEl = document.getElementById('replace-summary');
                var executeBtn = document.getElementById('execute-replace-btn');

                if (!data.matches || data.matches.length === 0) {
                    summaryEl.innerHTML = '<span style="color: var(--muted);">No matches found for "' + escapeHtml(searchQuery) + '"</span>';
                    listEl.innerHTML = '';
                    executeBtn.disabled = true;
                    return;
                }

                var totalMatches = 0;
                data.matches.forEach(function(m) { totalMatches += m.match_count; });

                summaryEl.innerHTML = 'Found <strong>' + totalMatches + '</strong> occurrence(s) across <strong>' + data.matches.length + '</strong> page(s). Replacing "<code>' + escapeHtml(searchQuery) + '</code>" with "<code>' + escapeHtml(replaceQuery) + '</code>"';

                var html = '';
                data.matches.forEach(function(item) {
                    html += '<div class="replace-preview-item">';
                    html += '<h4>' + escapeHtml(item.title) + '</h4>';
                    html += '<div class="path"><code>' + escapeHtml(item.full_path) + '</code> &middot; ' + item.match_count + ' occurrence(s)</div>';

                    item.excerpts.forEach(function(excerpt) {
                        html += '<div class="replace-excerpt">' + excerpt + '</div>';
                    });

                    html += '</div>';
                });

                listEl.innerHTML = html;
                executeBtn.disabled = false;
            }

            function closeReplacePreview() {
                document.getElementById('replace-preview-modal').style.display = 'none';
                replacePreviewData = null;
            }

            function executeReplace() {
                if (!replacePreviewData || !replacePreviewData.matches || replacePreviewData.matches.length === 0) {
                    return;
                }

                var searchQuery = document.getElementById('search-query').value.trim();
                var replaceQuery = document.getElementById('replace-query').value;
                var count = replacePreviewData.matches.length;

                showConfirm('Are you sure you want to replace text in ' + count + ' page(s)?<br><br>This action will save a version of each page before making changes.', 'Confirm Replace').then(function(confirmed) {
                    if (!confirmed) return;

                    var executeBtn = document.getElementById('execute-replace-btn');
                    executeBtn.disabled = true;
                    executeBtn.textContent = 'Replacing...';

                    fetch('/cm/replace/execute', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
                        body: JSON.stringify({
                            search: searchQuery,
                            replace: replaceQuery
                        })
                    })
                    .then(function(response) {
                        // A 403 that is not JSON (a rejected CSRF token) still gets a readable message
                        return response.json().catch(function(err) {
                            if (response.status === 403) return { error: 'The request was refused (403). Reload the page and try again.' };
                            throw err;
                        });
                    })
                    .then(function(data) {
                        if (data.error) {
                            showAlert(dialogText('Error: ' + data.error), 'Replace Failed');
                            executeBtn.disabled = false;
                            executeBtn.textContent = 'Accept Replacements';
                        } else {
                            closeReplacePreview();
                            closeSearchModal();
                            showAlert(dialogText('Successfully updated ' + data.updated_count + ' page(s).'), 'Replace Complete', function() {
                                // Reload the page to see changes
                                window.location.reload();
                            });
                        }
                    })
                    .catch(function(err) {
                        showAlert(dialogText('Failed to execute replace: ' + err.message), 'Replace Failed');
                        executeBtn.disabled = false;
                        executeBtn.textContent = 'Accept Replacements';
                    });
                });
            }

            // Close modal on escape key
            document.addEventListener('keydown', function(e) {
                if (e.key === 'Escape') {
                    closeSearchModal();
                    closeReplacePreview();
                }
            });

            // Close modal on overlay click
            document.getElementById('search-modal').addEventListener('click', function(e) {
                if (e.target === this) {
                    closeSearchModal();
                }
            });
            document.getElementById('replace-preview-modal').addEventListener('click', function(e) {
                if (e.target === this) {
                    closeReplacePreview();
                }
            });
        </script>
    ` + adminLayoutEnd,

	"content_select_template": adminLayoutStart + `
        <div class="page-header">
            <h1>Create New Content</h1>
        </div>
        <p class="page-subtitle">Select a template to get started:</p>
        <div class="template-grid">
            {{range .Templates}}
            <a href="/cm/content/new/{{.ID.Hex}}" class="template-card">
                <h3>{{.Name}}</h3>
                <p>{{.Description}}</p>
                <span class="template-category">{{.Category}}</span>
            </a>
            {{end}}
        </div>
    ` + adminLayoutEnd,

	"version_diff": adminLayoutStart + `
        <div class="page-header">
            <h1>Version Comparison</h1>
            <a href="/cm/content/{{.Current.ID.Hex}}" class="btn btn-outline">← Back to Editor</a>
        </div>
        <p style="color: var(--muted); margin-bottom: 2rem;">
            Comparing <strong>Version {{.Version.Version}}</strong> (saved {{.Version.CreatedAt.Format "Jan 2, 2006 3:04 PM"}})
            with <strong>Current Version</strong>
        </p>

        <div class="diff-container">
            <!-- Title -->
            <div class="diff-section" data-field="title">
                <h3>Title <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.Title}}">{{.Version.Title}}</div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.Title}}">{{.Current.Title}}</div>
                    </div>
                </div>
            </div>

            <!-- Slug -->
            <div class="diff-section" data-field="slug">
                <h3>Slug <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.Slug}}"><code>{{.Version.Slug}}</code></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.Slug}}"><code>{{.Current.Slug}}</code></div>
                    </div>
                </div>
            </div>

            <!-- Full Path -->
            <div class="diff-section" data-field="full_path">
                <h3>Full Path <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.FullPath}}"><code>{{.Version.FullPath}}</code></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.FullPath}}"><code>{{.Current.FullPath}}</code></div>
                    </div>
                </div>
            </div>

            <!-- Content Data Fields -->
            {{range $key, $value := .Version.Data}}
            <div class="diff-section diff-field-section" data-field="{{$key}}">
                <h3>{{$key}} <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{$.Version.Version}}</div>
                        <div class="diff-content diff-html" data-old="{{$value}}"></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content diff-html" data-new="{{index $.Current.Data $key}}"></div>
                    </div>
                </div>
            </div>
            {{end}}

            <!-- Settings -->
            <div class="diff-section" data-field="settings">
                <h3>Settings <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="published:{{.Version.Published}};header:{{.Version.UseHeader}};footer:{{.Version.UseFooter}};theme:{{.Version.UseTheme}}">
                            <p>Published: {{if .Version.Published}}Yes{{else}}No{{end}}</p>
                            <p>Use Header: {{if .Version.UseHeader}}Yes{{else}}No{{end}}</p>
                            <p>Use Footer: {{if .Version.UseFooter}}Yes{{else}}No{{end}}</p>
                            <p>Use Theme: {{if .Version.UseTheme}}Yes{{else}}No{{end}}</p>
                        </div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="published:{{.Current.Published}};header:{{.Current.UseHeader}};footer:{{.Current.UseFooter}};theme:{{.Current.UseTheme}}">
                            <p>Published: {{if .Current.Published}}Yes{{else}}No{{end}}</p>
                            <p>Use Header: {{if .Current.UseHeader}}Yes{{else}}No{{end}}</p>
                            <p>Use Footer: {{if .Current.UseFooter}}Yes{{else}}No{{end}}</p>
                            <p>Use Theme: {{if .Current.UseTheme}}Yes{{else}}No{{end}}</p>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="form-actions" style="margin-top: 2rem;">
            <a href="/cm/content/{{.Current.ID.Hex}}" class="btn btn-outline">Back to Editor</a>
            <a href="/cm/content/{{.Current.ID.Hex}}/versions/{{.Version.Version}}/view" target="_blank" class="btn btn-secondary">Preview Version {{.Version.Version}}</a>
            <form method="POST" action="/cm/content/{{.Current.ID.Hex}}/versions/{{.Version.Version}}/revert" style="display:inline" onsubmit="return confirmRevert(this, {{.Version.Version}})">
            {{.CSRFField}}
                <button type="submit" class="btn btn-primary">Revert to Version {{.Version.Version}}</button>
            </form>
        </div>

        <style>
            .diff-container {
                display: flex;
                flex-direction: column;
                gap: 1.5rem;
            }
            .diff-section {
                background: var(--card-bg);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1rem;
            }
            .diff-section.has-changes {
                border-color: #f59e0b;
            }
            .diff-section.no-changes {
                opacity: 0.6;
            }
            .diff-section h3 {
                margin: 0 0 1rem 0;
                font-size: 1rem;
                color: var(--accent);
                border-bottom: 1px solid var(--border);
                padding-bottom: 0.5rem;
                display: flex;
                align-items: center;
                gap: 0.75rem;
            }
            .diff-badge {
                font-size: 0.7rem;
                padding: 0.15rem 0.5rem;
                border-radius: 4px;
                text-transform: uppercase;
                font-weight: 600;
            }
            .diff-badge.changed {
                background: rgba(245, 158, 11, 0.2);
                color: #f59e0b;
            }
            .diff-badge.unchanged {
                background: rgba(107, 114, 128, 0.2);
                color: #6b7280;
            }
            .diff-row {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 1rem;
            }
            .diff-col {
                min-width: 0;
            }
            .diff-label {
                font-size: 0.75rem;
                text-transform: uppercase;
                color: var(--muted);
                margin-bottom: 0.5rem;
                font-weight: 600;
            }
            .diff-old .diff-label {
                color: #f59e0b;
            }
            .diff-new .diff-label {
                color: #10b981;
            }
            .diff-content {
                background: rgba(15, 23, 42, 0.5);
                border: 1px solid var(--border);
                border-radius: 4px;
                padding: 0.75rem;
                font-size: 0.9rem;
                overflow-x: auto;
                max-height: 400px;
                overflow-y: auto;
            }
            .diff-old .diff-content {
                border-left: 3px solid #f59e0b;
            }
            .diff-new .diff-content {
                border-left: 3px solid #10b981;
            }
            .diff-html {
                white-space: pre-wrap;
                word-break: break-word;
                font-family: 'JetBrains Mono', monospace;
                font-size: 0.8rem;
            }
            code {
                font-family: 'JetBrains Mono', monospace;
                background: rgba(99, 102, 241, 0.1);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
            }
            /* Inline diff highlighting */
            .diff-added {
                background: rgba(16, 185, 129, 0.3);
                color: #10b981;
                padding: 0 2px;
                border-radius: 2px;
            }
            .diff-removed {
                background: rgba(239, 68, 68, 0.3);
                color: #ef4444;
                padding: 0 2px;
                border-radius: 2px;
                text-decoration: line-through;
            }
            .diff-current {
                outline: 2px solid #6366f1;
                outline-offset: 2px;
                background: rgba(99, 102, 241, 0.2) !important;
            }
            /* Navigation controls */
            .diff-nav {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.5rem;
                padding: 0.5rem;
                background: var(--bg-tertiary);
                border-radius: 4px;
                font-size: 0.8rem;
            }
            .diff-nav-btn {
                padding: 0.25rem 0.5rem;
                background: var(--primary);
                color: white;
                border: none;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.75rem;
                font-weight: 500;
            }
            .diff-nav-btn:hover {
                opacity: 0.9;
            }
            .diff-nav-btn:disabled {
                opacity: 0.5;
                cursor: not-allowed;
            }
            .diff-nav-count {
                color: var(--muted);
                margin-left: auto;
            }
            @media (max-width: 768px) {
                .diff-row {
                    grid-template-columns: 1fr;
                }
            }
        </style>

        <script>
        document.addEventListener('DOMContentLoaded', function() {
            // Process each diff section
            document.querySelectorAll('.diff-section').forEach(function(section) {
                var oldEl = section.querySelector('[data-old]');
                var newEl = section.querySelector('[data-new]');
                var badge = section.querySelector('.diff-badge');

                if (!oldEl || !newEl || !badge) return;

                var oldVal = oldEl.getAttribute('data-old') || '';
                var newVal = newEl.getAttribute('data-new') || '';

                if (oldVal === newVal) {
                    badge.textContent = 'Unchanged';
                    badge.className = 'diff-badge unchanged';
                    section.classList.add('no-changes');
                } else {
                    badge.textContent = 'Changed';
                    badge.className = 'diff-badge changed';
                    section.classList.add('has-changes');

                    // For content fields, show inline diff with navigation
                    if (section.classList.contains('diff-field-section')) {
                        showInlineDiff(section, oldEl, newEl, oldVal, newVal);
                    }
                }
            });

            // Line-level diff with navigation for long content
            function showInlineDiff(section, oldEl, newEl, oldText, newText) {
                // Escape HTML for display
                function escapeHtml(str) {
                    return str.replace(/&/g, '&amp;')
                              .replace(/</g, '&lt;')
                              .replace(/>/g, '&gt;')
                              .replace(/"/g, '&quot;');
                }

                // Find the differences using a simple line-based approach
                var oldLines = oldText.split('\n');
                var newLines = newText.split('\n');

                // Build a map of lines for quick lookup
                var oldLineSet = new Set(oldLines);
                var newLineSet = new Set(newLines);

                // Track change indices for pairing old/new
                var changeIdx = 0;

                // Highlight changed/removed lines in old version
                var oldHighlighted = oldLines.map(function(line, idx) {
                    var escaped = escapeHtml(line);
                    if (!newLineSet.has(line)) {
                        return '<span class="diff-removed" data-diff-idx="' + (changeIdx++) + '" data-line="' + idx + '">' + escaped + '</span>';
                    }
                    return '<span data-line="' + idx + '">' + escaped + '</span>';
                }).join('\n');

                // Reset for new side - track which change we're on
                var newChangeIdx = 0;

                // Highlight changed/added lines in new version
                var newHighlighted = newLines.map(function(line, idx) {
                    var escaped = escapeHtml(line);
                    if (!oldLineSet.has(line)) {
                        return '<span class="diff-added" data-diff-idx="' + (newChangeIdx++) + '" data-line="' + idx + '">' + escaped + '</span>';
                    }
                    return '<span data-line="' + idx + '">' + escaped + '</span>';
                }).join('\n');

                oldEl.innerHTML = oldHighlighted;
                newEl.innerHTML = newHighlighted;

                // Get all change elements from both sides
                var oldChanges = oldEl.querySelectorAll('.diff-removed');
                var newChanges = newEl.querySelectorAll('.diff-added');
                var totalChanges = Math.max(oldChanges.length, newChanges.length);

                // Only add navigation if there are changes
                if (totalChanges > 0) {
                    var currentIndex = 0;

                    // Create navigation controls
                    var nav = document.createElement('div');
                    nav.className = 'diff-nav';
                    nav.innerHTML =
                        '<button type="button" class="diff-nav-btn" data-action="prev">Prev</button>' +
                        '<button type="button" class="diff-nav-btn" data-action="next">Next</button>' +
                        '<span class="diff-nav-count"><span class="diff-nav-current">1</span> of ' + totalChanges + ' changes</span>';

                    // Insert navigation before the diff-row
                    var diffRow = section.querySelector('.diff-row');
                    diffRow.parentNode.insertBefore(nav, diffRow);

                    var prevBtn = nav.querySelector('[data-action="prev"]');
                    var nextBtn = nav.querySelector('[data-action="next"]');
                    var currentSpan = nav.querySelector('.diff-nav-current');

                    // Sync scroll between panels (for manual scrolling)
                    var syncing = false;
                    function syncScroll(source, target) {
                        if (syncing) return;
                        syncing = true;
                        var scrollRatio = source.scrollTop / (source.scrollHeight - source.clientHeight || 1);
                        target.scrollTop = scrollRatio * (target.scrollHeight - target.clientHeight);
                        setTimeout(function() { syncing = false; }, 50);
                    }

                    oldEl.addEventListener('scroll', function() { syncScroll(oldEl, newEl); });
                    newEl.addEventListener('scroll', function() { syncScroll(newEl, oldEl); });

                    function scrollToChange(index) {
                        // Remove current highlight from all changes on both sides
                        oldChanges.forEach(function(el) { el.classList.remove('diff-current'); });
                        newChanges.forEach(function(el) { el.classList.remove('diff-current'); });

                        // Get corresponding elements on both sides
                        var oldChange = oldChanges[index];
                        var newChange = newChanges[index];

                        // Highlight both sides if they exist
                        if (oldChange) oldChange.classList.add('diff-current');
                        if (newChange) newChange.classList.add('diff-current');

                        // Disable sync during programmatic scroll
                        syncing = true;

                        // Scroll both panels to center on the change
                        if (newChange) {
                            var containerTop = newEl.getBoundingClientRect().top;
                            var elementTop = newChange.getBoundingClientRect().top;
                            var relativePos = elementTop - containerTop;
                            var targetScroll = newEl.scrollTop + relativePos - (newEl.clientHeight / 2);
                            newEl.scroll({ top: Math.max(0, targetScroll), behavior: 'instant' });
                        }
                        if (oldChange) {
                            var containerTop = oldEl.getBoundingClientRect().top;
                            var elementTop = oldChange.getBoundingClientRect().top;
                            var relativePos = elementTop - containerTop;
                            var targetScroll = oldEl.scrollTop + relativePos - (oldEl.clientHeight / 2);
                            oldEl.scroll({ top: Math.max(0, targetScroll), behavior: 'instant' });
                        }

                        // Re-enable sync after a delay
                        setTimeout(function() { syncing = false; }, 100);

                        // Update counter
                        currentSpan.textContent = (index + 1);
                    }

                    prevBtn.addEventListener('click', function(e) {
                        e.preventDefault();
                        currentIndex = (currentIndex - 1 + totalChanges) % totalChanges;
                        scrollToChange(currentIndex);
                    });

                    nextBtn.addEventListener('click', function(e) {
                        e.preventDefault();
                        currentIndex = (currentIndex + 1) % totalChanges;
                        scrollToChange(currentIndex);
                    });

                    // Auto-scroll to first change after a short delay
                    setTimeout(function() {
                        scrollToChange(0);
                    }, 200);
                }
            }
        });
        </script>
    ` + adminLayoutEnd,

	"content_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New {{.Template.Name}}{{else}}Edit Content{{end}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        <form method="POST" action="{{if .IsNew}}/cm/content/create{{else}}/cm/content/{{.Content.ID.Hex}}{{end}}" enctype="multipart/form-data" class="form-card">
            {{.CSRFField}}
            <input type="hidden" name="template_id" value="{{.Template.ID.Hex}}">
            <input type="hidden" name="create_redirect" id="create_redirect" value="">
            <input type="hidden" name="slug_rename_enabled" id="slug_rename_enabled" value="">
            <input type="hidden" name="version_comment" id="version_comment" value="">

            {{if not .IsNew}}
            <div class="form-group" style="background: var(--bg-tertiary); padding: 1rem; border-radius: var(--radius); margin-bottom: 1.5rem;">
                <div style="display: flex; align-items: center; justify-content: space-between;">
                    <div>
                        <label style="margin-bottom: 0.25rem; display: block;">Template</label>
                        <span style="font-size: 1.1rem; font-weight: 500;">{{.Template.Name}}</span>
                    </div>
                    <button type="button" class="btn btn-sm btn-outline" onclick="showChangeTemplateModal()">Change Template</button>
                </div>
            </div>
            {{end}}

            <div class="form-group">
                <label for="title">Title</label>
                <input type="text" id="title" name="title" value="{{if .Content}}{{.Content.Title}}{{end}}" required>
            </div>
            <div class="form-group">
                <label for="slug">Slug (URL path)</label>
                <div class="slug-input-wrapper" style="display: flex; gap: 0.5rem; align-items: center;">
                    {{if .IsNew}}
                    <input type="text" id="slug" name="slug" value="" placeholder="auto-generated from title" style="flex: 1;">
                    {{else}}
                    <input type="text" id="slug" name="slug" value="{{.Content.Slug}}" readonly style="flex: 1; background: var(--bg-tertiary);">
                    <button type="button" id="rename-slug-btn" class="btn btn-sm btn-outline" onclick="enableSlugRename()">Rename</button>
                    <button type="button" id="cancel-rename-btn" class="btn btn-sm btn-outline" onclick="cancelSlugRename()" style="display: none;">Cancel</button>
                    {{end}}
                </div>
                <p class="help-text slug-error" id="slug-error" style="color: var(--error); display: none;"></p>
                {{if not .IsNew}}<p class="help-text">Leave empty for root page (/). Click Rename to change the slug.</p>{{end}}
            </div>
            <div class="form-group">
                <label for="folder_id">Folder</label>
                <select id="folder_id" name="folder_id">
                    <option value="root">/ (root)</option>
                    {{range .Folders}}
                    <option value="{{.ID.Hex}}" {{if $.Content}}{{if $.Content.FolderID}}{{if eq .ID.Hex $.Content.FolderID.Hex}}selected{{end}}{{end}}{{end}}>{{.Path}}</option>
                    {{end}}
                </select>
                <p class="help-text">The full URL will be: <code id="preview-path">/</code></p>
            </div>

            {{range .Template.Fields}}
            <div class="form-group">
                <label for="field_{{.Name}}">{{.Label}}{{if .Required}} *{{end}}</label>
                {{if eq .Type "text"}}
                <input type="text" id="field_{{.Name}}" name="field_{{.Name}}"
                    value="{{if $.Content}}{{index $.Content.Data .Name}}{{end}}"
                    placeholder="{{.Placeholder}}" {{if .Required}}required{{end}}>
                {{else if eq .Type "textarea"}}
                <textarea id="field_{{.Name}}" name="field_{{.Name}}" rows="4"
                    placeholder="{{.Placeholder}}" {{if .Required}}required{{end}}>{{if $.Content}}{{index $.Content.Data .Name}}{{end}}</textarea>
                {{else if eq .Type "richtext"}}
                <div class="richtext-toggle" style="margin-bottom: 0.5rem;">
                    <button type="button" class="btn btn-sm btn-outline toggle-html-btn" onclick="toggleFieldHtmlMode(this, 'field_{{.Name}}')" title="Toggle between rich editor and raw HTML">
                        &lt;/&gt; Edit HTML
                    </button>
                </div>
                <textarea id="field_{{.Name}}" name="field_{{.Name}}" class="richtext"
                    {{if .Required}}required{{end}}>{{if $.Content}}{{index $.Content.Data .Name}}{{end}}</textarea>
                {{else if eq .Type "rawhtml"}}
                <textarea id="field_{{.Name}}" name="field_{{.Name}}" rows="20" class="code-editor"
                    placeholder="{{.Placeholder}}" {{if .Required}}required{{end}}>{{if $.Content}}{{index $.Content.Data .Name}}{{end}}</textarea>
                {{else if eq .Type "date"}}
                <input type="date" id="field_{{.Name}}" name="field_{{.Name}}"
                    value="{{if $.Content}}{{index $.Content.Data .Name}}{{end}}" {{if .Required}}required{{end}}>
                {{else if eq .Type "image"}}
                {{if $.Content}}{{if index $.Content.Data .Name}}
                <div class="current-image">
                    <img src="{{index $.Content.Data .Name}}" alt="Current image" style="max-width: 200px; margin-bottom: 0.5rem;">
                </div>
                {{end}}{{end}}
                <input type="file" id="field_{{.Name}}" name="field_{{.Name}}" accept="image/*">
                {{else if eq .Type "select"}}
                <select id="field_{{.Name}}" name="field_{{.Name}}" {{if .Required}}required{{end}}>
                    <option value="">Select...</option>
                    {{$currentVal := ""}}{{if $.Content}}{{$currentVal = index $.Content.Data .Name}}{{end}}
                    {{range $opt := (split .Options ",")}}
                    <option value="{{$opt}}" {{if eq $opt $currentVal}}selected{{end}}>{{$opt}}</option>
                    {{end}}
                </select>
                {{end}}
            </div>
            {{end}}

            <div class="form-section">
                <h3>SEO Settings</h3>
                <div class="form-group">
                    <label for="content_tags">Tags</label>
                    <input type="text" id="content_tags" name="content_tags" value="{{if .Content}}{{join .Content.Tags ", "}}{{end}}" placeholder="e.g., AI & Machine Intelligence, Generative AI">
                    <p class="help-text">Comma-separated tags used to include this page in <code>lc:query</code> index pages</p>
                </div>
                <div class="form-group">
                    <label for="meta_description">Meta Description</label>
                    <textarea id="meta_description" name="meta_description" rows="2" placeholder="Brief description for search engines (150-160 characters)">{{if .Content}}{{.Content.MetaDescription}}{{end}}</textarea>
                    <p class="help-text">Recommended: 150-160 characters for best SEO results</p>
                </div>
                <div class="form-group">
                    <label for="og_image">Social Share Image (OG Image)</label>
                    {{if .Content}}{{if .Content.OGImage}}
                    <div class="current-image">
                        <img src="{{.Content.OGImage}}" alt="Current OG image" style="max-width: 200px; margin-bottom: 0.5rem;">
                    </div>
                    {{end}}{{end}}
                    <input type="file" id="og_image" name="og_image" accept="image/*">
                    <p class="help-text">Recommended: 1200x630 pixels for best display on social media</p>
                </div>
                <div class="form-group">
                    <label for="author_name">Author</label>
                    <input type="text" id="author_name" name="author_name" value="{{if .Content}}{{.Content.AuthorName}}{{end}}" placeholder="Leave blank to use the site default (Tools → SEO &amp; AI)">
                    <input type="url" id="author_url" name="author_url" value="{{if .Content}}{{.Content.AuthorURL}}{{end}}" placeholder="Author profile URL (optional)" style="margin-top: 0.5rem;">
                    <p class="help-text">Shown to search engines and AI systems in structured data and feeds</p>
                </div>
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="noindex" {{if .Content}}{{if .Content.NoIndex}}checked{{end}}{{end}}>
                        Hide from search engines &amp; AI
                    </label>
                    <p class="help-text">Adds noindex and leaves the page out of the sitemap, llms.txt, feeds and IndexNow. The page stays visible to visitors.</p>
                </div>
            </div>

            <div class="form-section">
                <h3>Page Settings</h3>
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="published" {{if .Content}}{{if .Content.ForkID}}disabled{{else if .Content.Published}}checked{{end}}{{end}}>
                        Published
                    </label>
                    {{if .Content}}{{if .Content.ForkID}}<p class="help-text">This is a copy inside a fork. It goes live when the fork is merged, not by publishing it here.</p>{{end}}{{end}}
                </div>
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="hold" {{if .Content}}{{if .Content.Hold}}checked{{end}}{{end}}>
                        Hold
                    </label>
                    <p class="help-text">A held draft cannot be published — here, in bulk, on a schedule, by the API or by merging a fork — until this is unchecked. Holding a page that is already published does not unpublish it.</p>
                </div>
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="hidden" name="use_header" value="off">
                        <input type="checkbox" name="use_header" value="on" {{if .IsNew}}checked{{else}}{{if .Content.UseHeader}}checked{{end}}{{end}}>
                        Include site header
                    </label>
                </div>
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="hidden" name="use_footer" value="off">
                        <input type="checkbox" name="use_footer" value="on" {{if .IsNew}}checked{{else}}{{if .Content.UseFooter}}checked{{end}}{{end}}>
                        Include site footer
                    </label>
                </div>
                {{if eq .Template.Slug "blank-page"}}
                <div class="form-group checkbox-group">
                    <label class="checkbox-label">
                        <input type="hidden" name="use_theme" value="off">
                        <input type="checkbox" name="use_theme" value="on" id="use_theme" {{if .IsNew}}checked{{else}}{{if .Content.UseTheme}}checked{{end}}{{end}}>
                        Use site theme (CSS, layout wrapper)
                    </label>
                </div>
                {{end}}
            </div>

            <div class="form-actions" style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem;">
                <div style="display: flex; gap: 0.5rem; align-items: center;">
                    <a href="/cm/content" class="btn btn-outline">Cancel</a>
                    {{if .ForkPageID}}
                    <a href="/cm/forks?fork_page={{.ForkPageID}}" class="btn btn-outline" title="Copy this page into a fork workspace for staged editing">🌿 Fork to workspace</a>
                    {{end}}
                    <button type="submit" class="btn btn-primary">{{if .IsNew}}Create{{else}}Update{{end}}</button>
                </div>
                {{if not .IsNew}}
                <button type="submit" form="delete-page-form" class="btn btn-danger">{{if .Content.ForkID}}Remove from Fork{{else}}Delete Page{{end}}</button>
                {{end}}
            </div>
        </form>
        {{if not .IsNew}}
        <!-- Separate form (a form cannot nest inside the edit form); the Delete Page button points here with form= -->
        <form id="delete-page-form" method="POST" action="/cm/content/{{.Content.ID.Hex}}/delete" onsubmit="return confirmDelete(this, {{if .Content.ForkID}}'Remove this copy from the fork? The live page is not affected.'{{else}}'Delete this page? It is unpublished and moved to deleted content, where it can be restored (Search, Include deleted content).'{{end}})">
            {{$.CSRFField}}
        </form>
        {{end}}

        <!-- Redirect confirmation modal -->
        <div id="redirect-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
            <div style="background: #1e293b; border-radius: var(--radius); max-width: 500px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(99, 102, 241, 0.3);">
                <div style="padding: 1.5rem; border-bottom: 1px solid rgba(99, 102, 241, 0.2); background: #1a2332;">
                    <h3 style="margin: 0; color: var(--text);">Create Redirect?</h3>
                </div>
                <div style="padding: 1.5rem; background: #1e293b;">
                    <p style="margin: 0 0 0.5rem 0;">You are changing the URL from:</p>
                    <p style="margin: 0 0 1rem 0;"><code id="redirect-old-path" style="background: #0f172a; padding: 0.25rem 0.5rem; border-radius: 4px; color: var(--accent);"></code></p>
                    <p style="margin: 0 0 0.5rem 0;">to:</p>
                    <p style="margin: 0 0 1rem 0;"><code id="redirect-new-path" style="background: #0f172a; padding: 0.25rem 0.5rem; border-radius: 4px; color: var(--accent);"></code></p>
                    <p style="margin: 0; color: var(--muted); font-size: 0.9rem;">Creating a redirect will preserve any existing links or bookmarks to the old URL.</p>
                </div>
                <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(99, 102, 241, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                    <button type="button" class="btn btn-outline" id="redirect-no-btn">Rename without Redirect</button>
                    <button type="button" class="btn btn-primary" id="redirect-yes-btn">Yes, Redirect</button>
                </div>
            </div>
        </div>

        {{if not .IsNew}}
        <!-- Change template modal -->
        <div id="change-template-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
            <div style="background: #1e293b; border-radius: var(--radius); max-width: 500px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(245, 158, 11, 0.3);">
                <div style="padding: 1.5rem; border-bottom: 1px solid rgba(245, 158, 11, 0.2); background: #1a2332;">
                    <h3 style="margin: 0; color: var(--warning);">Change Template</h3>
                </div>
                <div style="padding: 1.5rem; background: #1e293b;">
                    <p style="margin: 0 0 1rem 0; color: var(--warning);">Changing templates may not preserve all field data.</p>
                    <p style="margin: 0 0 1.5rem 0; color: var(--muted);">Fields with matching names will be carried over. Fields that don't exist in the new template will be lost. You'll see a preview of the changes before confirming.</p>
                    <div class="form-group" style="margin-bottom: 0;">
                        <label for="new-template-select" style="margin-bottom: 0.5rem; display: block;">Select New Template</label>
                        <select id="new-template-select" style="width: 100%; padding: 0.75rem; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text);">
                            {{range .AllTemplates}}
                            {{if ne .ID.Hex $.Template.ID.Hex}}
                            <option value="{{.ID.Hex}}">{{.Name}}</option>
                            {{end}}
                            {{end}}
                        </select>
                    </div>
                </div>
                <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(245, 158, 11, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                    <button type="button" class="btn btn-outline" onclick="closeChangeTemplateModal()">Cancel</button>
                    <button type="button" class="btn" style="background: var(--warning); color: white;" onclick="proceedToTemplatePreview()">Proceed</button>
                </div>
            </div>
        </div>

        <!-- Version comment modal -->
        <div id="version-comment-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
            <div style="background: #1e293b; border-radius: var(--radius); max-width: 500px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(99, 102, 241, 0.3);">
                <div style="padding: 1.5rem; border-bottom: 1px solid rgba(99, 102, 241, 0.2); background: #1a2332;">
                    <h3 style="margin: 0; color: var(--text);">Save Version <span id="version-number-display" style="color: var(--accent);"></span></h3>
                </div>
                <div style="padding: 1.5rem; background: #1e293b;">
                    <p style="margin: 0 0 1rem 0; color: var(--muted);">Add an optional comment to describe this version's changes.</p>
                    <div class="form-group" style="margin-bottom: 0;">
                        <label for="version-comment-input" style="margin-bottom: 0.5rem; display: block;">Version Comment (optional)</label>
                        <textarea id="version-comment-input" rows="3" style="width: 100%; padding: 0.75rem; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text); resize: vertical;" placeholder="e.g., Updated hero image, Fixed typo in introduction..."></textarea>
                    </div>
                </div>
                <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(99, 102, 241, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                    <button type="button" class="btn btn-outline" id="version-comment-cancel">Cancel</button>
                    <button type="button" class="btn btn-primary" id="version-comment-save">Save</button>
                </div>
            </div>
        </div>
        {{end}}

        {{if not .IsNew}}
        {{if .Content.Deleted}}
        <div class="form-section" style="margin-top: 2rem; background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: var(--radius); padding: 1.5rem;">
            <h3 style="color: var(--danger);">⚠️ This content is deleted</h3>
            <p style="margin-bottom: 1rem;">This page was deleted on {{if .Content.DeletedAt}}{{.Content.DeletedAt.Format "Jan 2, 2006 3:04 PM"}}{{else}}unknown date{{end}}.</p>
            <form method="POST" action="/cm/content/{{.Content.ID.Hex}}/undelete" style="display: inline;">
            {{.CSRFField}}
                <button type="submit" class="btn btn-primary">Restore This Page</button>
            </form>
        </div>
        {{end}}

        <!-- Bottom tabbed panel: Discussion / Version History / Forks -->
        <div class="form-section" style="margin-top: 2rem;" id="bottom-tabs-panel">
            <div style="display: flex; gap: 0; border-bottom: 1px solid var(--border); margin-bottom: 1.5rem;">
                <button type="button" class="bottom-tab active" data-tab="discussion" onclick="switchBottomTab('discussion')">💬 Discussion{{if .Comments}} <span style="background: var(--accent); color: white; border-radius: 9999px; font-size: 0.75rem; padding: 0 0.4rem; margin-left: 0.25rem;">{{len .Comments}}</span>{{end}}</button>
                <button type="button" class="bottom-tab" data-tab="history" onclick="switchBottomTab('history')">🕒 Version History{{if .Versions}} <span style="background: rgba(148,163,184,0.2); color: var(--muted); border-radius: 9999px; font-size: 0.75rem; padding: 0 0.4rem; margin-left: 0.25rem;">{{len .Versions}}</span>{{end}}</button>
                <button type="button" class="bottom-tab" data-tab="forks" onclick="switchBottomTab('forks')">🌿 Forks</button>
                {{if not .IsNew}}<button type="button" class="bottom-tab" data-tab="analytics" onclick="switchBottomTab('analytics')">📊 Analytics</button>{{end}}
            </div>

            <!-- Discussion tab -->
            <div id="tab-discussion" class="bottom-tab-content">
                {{if .Comments}}
                <div id="comment-thread" style="display: flex; flex-direction: column; gap: 1rem; margin-bottom: 1.5rem;">
                    {{range .Comments}}
                    <div class="comment-item" data-comment-id="{{.ID.Hex}}" style="display: flex; gap: 0.75rem; align-items: flex-start;">
                        <div style="width: 32px; height: 32px; border-radius: 50%; background: var(--bg-tertiary); display: flex; align-items: center; justify-content: center; font-size: 0.8rem; color: var(--muted); flex-shrink: 0;">{{slice .UserDisplayName 0 1}}</div>
                        <div style="flex: 1; min-width: 0;">
                            <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.25rem; flex-wrap: wrap;">
                                <span style="font-weight: 600; font-size: 0.9rem;">{{.UserDisplayName}}</span>
                                <span style="color: var(--muted); font-size: 0.8rem;">{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</span>
                                {{if eq $.CurrentUserRole "admin"}}
                                <button type="button" class="btn btn-sm" style="margin-left: auto; padding: 0.1rem 0.5rem; font-size: 0.75rem; background: transparent; color: var(--danger); border: 1px solid rgba(239,68,68,0.3);" onclick="deleteComment('{{$.Content.ID.Hex}}', '{{.ID.Hex}}', this)">Delete</button>
                                {{end}}
                            </div>
                            <p style="margin: 0; color: var(--text); font-size: 0.95rem; white-space: pre-wrap; word-break: break-word;">{{.Text}}</p>
                        </div>
                    </div>
                    {{end}}
                </div>
                {{else}}
                <div id="comment-thread" style="display: flex; flex-direction: column; gap: 1rem; margin-bottom: 1.5rem;"></div>
                <p id="no-comments-msg" style="color: var(--muted); font-size: 0.9rem; margin-bottom: 1.5rem;">No discussion yet. Be the first to comment.</p>
                {{end}}

                <!-- Post a comment -->
                <div style="border-top: 1px solid var(--border); padding-top: 1rem;">
                    <div style="position: relative;">
                        <textarea id="comment-input" rows="3" placeholder="Write a comment... Use @name to mention someone" style="width: 100%; padding: 0.75rem; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text); resize: vertical; font-size: 0.95rem; box-sizing: border-box;"></textarea>
                        <div id="mention-dropdown" style="display: none; position: absolute; bottom: 100%; left: 0; background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--radius); max-height: 160px; overflow-y: auto; z-index: 100; min-width: 200px; box-shadow: 0 4px 12px rgba(0,0,0,0.3);"></div>
                    </div>
                    <div style="display: flex; justify-content: flex-end; margin-top: 0.5rem;">
                        <button type="button" class="btn btn-primary" onclick="postComment('{{.Content.ID.Hex}}')">Post Comment</button>
                    </div>
                </div>
            </div>

            <!-- Version History tab -->
            <div id="tab-history" class="bottom-tab-content" style="display: none;">
                {{if .Versions}}
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Version</th>
                                <th>Title</th>
                                <th>Comment</th>
                                <th>By</th>
                                <th>Saved</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range .Versions}}
                            <tr>
                                <td>v{{.Version}}</td>
                                <td>{{.Title}}</td>
                                <td style="color: var(--muted); font-size: 0.9rem; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="{{.Comment}}">{{if .Comment}}{{.Comment}}{{else}}-{{end}}</td>
                                <td style="color: var(--muted); font-size: 0.9rem;">{{if .ModifiedByEmail}}{{.ModifiedByEmail}}{{else}}-{{end}}</td>
                                <td>{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td>
                                <td class="actions">
                                    <a href="/cm/content/{{.ContentID.Hex}}/versions/{{.Version}}/diff" class="btn btn-sm btn-outline">Diff</a>
                                    <a href="/cm/content/{{.ContentID.Hex}}/versions/{{.Version}}/view" target="_blank" class="btn btn-sm btn-outline">Preview</a>
                                    <form method="POST" action="/cm/content/{{.ContentID.Hex}}/versions/{{.Version}}/revert" style="display:inline" onsubmit="return confirmRevert(this, {{.Version}})">
                                    {{$.CSRFField}}
                                        <button type="submit" class="btn btn-sm btn-secondary">Revert</button>
                                    </form>
                                </td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
                {{else}}
                <p style="color: var(--muted); font-size: 0.9rem;">No version history yet.</p>
                {{end}}

                {{if .SameSlugPages}}
                <h4 style="margin: 1.5rem 0 0.75rem; color: var(--muted); font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em;">Historical Pages at This URL</h4>
                <p style="color: var(--muted); margin-bottom: 0.75rem; font-size: 0.9rem;">Other pages that used the same slug "{{.Content.Slug}}".</p>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr><th>Title</th><th>Status</th><th>Last Updated</th><th>Actions</th></tr>
                        </thead>
                        <tbody>
                            {{range .SameSlugPages}}
                            <tr{{if .Deleted}} style="opacity: 0.7;"{{end}}>
                                <td>{{.Title}}</td>
                                <td>
                                    {{if .Deleted}}<span class="status-badge" style="background: var(--danger);">Deleted</span>
                                    {{else if .Published}}<span class="status-badge published">Published</span>
                                    {{else}}<span class="status-badge draft">Draft</span>{{end}}
                                </td>
                                <td>{{.UpdatedAt.Format "Jan 2, 2006 3:04 PM"}}</td>
                                <td class="actions"><a href="/cm/content/{{.ID.Hex}}" class="btn btn-sm">Edit</a></td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
                {{end}}
            </div>

            <!-- Forks tab -->
            <div id="tab-forks" class="bottom-tab-content" style="display: none;">
                {{if .ForkPageID}}
                <p style="color: var(--muted); margin-bottom: 1rem; font-size: 0.9rem;">Fork this page into an isolated workspace to draft changes before they go live.</p>
                <a href="/cm/forks?fork_page={{.ForkPageID}}" class="btn btn-outline">🌿 Add to Fork Workspace</a>
                {{else}}
                <p style="color: var(--muted); font-size: 0.9rem;">This page is already inside a fork workspace.</p>
                {{end}}
            </div>

            {{if not .IsNew}}
            <div id="tab-analytics" class="bottom-tab-content" style="display: none;">
                <div style="display: flex; gap: 2rem; margin-bottom: 1.5rem; flex-wrap: wrap;">
                    <div>
                        <div style="font-size: 0.75rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em;">Views (30d)</div>
                        <div style="font-size: 1.5rem; font-weight: 700; margin-top: 0.25rem;">{{.PageViews30d}}</div>
                    </div>
                    <div>
                        <div style="font-size: 0.75rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em;">Views (7d)</div>
                        <div style="font-size: 1.5rem; font-weight: 700; margin-top: 0.25rem;">{{.PageViews7d}}</div>
                    </div>
                    <div>
                        <a href="/cm/analytics/page?path={{.Content.FullPath}}&range=30d" style="color: var(--primary); font-size: 0.875rem; text-decoration: none;">View full analytics &rarr;</a>
                    </div>
                </div>
                <div style="margin-bottom: 0.75rem; font-size: 0.8rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; font-weight: 600;">Top Referrers (30d)</div>
                <div id="page-analytics-refs"></div>
                <div id="page-analytics-refs-empty" style="display:none; color: var(--muted); font-size: 0.875rem;">No external referrer data for this page.</div>
                <script>
                (function() {
                    var refs = JSON.parse('{{.PageReferrersJSON}}');
                    var container = document.getElementById('page-analytics-refs');
                    var emptyEl = document.getElementById('page-analytics-refs-empty');
                    if (!refs || refs.length === 0) { emptyEl.style.display = 'block'; return; }
                    var maxH = refs[0].hits;
                    refs.forEach(function(r) {
                        var row = document.createElement('div');
                        row.style.cssText = 'display:flex;align-items:center;gap:0.75rem;margin-bottom:0.375rem;font-size:0.8125rem;';

                        var label = document.createElement('div');
                        label.style.cssText = 'width:180px;min-width:100px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;';
                        var a = document.createElement('a');
                        a.href = 'https://' + r.domain;
                        a.target = '_blank';
                        a.textContent = r.domain;
                        a.style.cssText = 'color:var(--primary);text-decoration:none;';
                        label.appendChild(a);

                        var barWrap = document.createElement('div');
                        barWrap.style.cssText = 'flex:1;height:18px;background:var(--bg-tertiary,#1e293b);border-radius:4px;overflow:hidden;';
                        var bar = document.createElement('div');
                        bar.style.cssText = 'height:100%;background:#4ade80;border-radius:4px;min-width:2px;width:' + (r.hits/maxH*100) + '%;';
                        barWrap.appendChild(bar);

                        var count = document.createElement('div');
                        count.style.cssText = 'width:50px;text-align:right;color:var(--muted);font-family:monospace;font-size:0.75rem;';
                        count.textContent = r.hits + ' hits';

                        row.appendChild(label);
                        row.appendChild(barWrap);
                        row.appendChild(count);
                        container.appendChild(row);
                    });
                })();
                </script>
            </div>
            {{end}}
        </div>

        <style>
            .bottom-tab {
                padding: 0.6rem 1.25rem;
                background: transparent;
                border: none;
                border-bottom: 2px solid transparent;
                color: var(--muted);
                cursor: pointer;
                font-size: 0.9rem;
                font-weight: 500;
                transition: color 0.15s, border-color 0.15s;
                margin-bottom: -1px;
            }
            .bottom-tab:hover { color: var(--text); }
            .bottom-tab.active { color: var(--primary); border-bottom-color: var(--primary); }
            #mention-dropdown button {
                display: block; width: 100%; text-align: left;
                padding: 0.5rem 0.75rem; background: none; border: none;
                color: var(--text); cursor: pointer; font-size: 0.9rem;
            }
            #mention-dropdown button:hover { background: var(--bg-tertiary); }
        </style>
        <script>
        function switchBottomTab(name) {
            document.querySelectorAll('.bottom-tab').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.bottom-tab-content').forEach(c => c.style.display = 'none');
            document.querySelector('[data-tab="'+name+'"]').classList.add('active');
            document.getElementById('tab-'+name).style.display = 'block';
        }

        // @mention autocomplete
        let mentionUsers = null;
        async function loadMentionUsers() {
            if (mentionUsers !== null) return mentionUsers;
            try {
                const r = await fetch('/api/v1/users');
                // The endpoint answers {"users": [...]}
                if (r.ok) { const data = await r.json(); mentionUsers = Array.isArray(data) ? data : (data.users || []); }
            } catch(e) {}
            return mentionUsers || [];
        }

        const commentInput = document.getElementById('comment-input');
        const mentionDropdown = document.getElementById('mention-dropdown');
        let mentionStart = -1;

        if (commentInput) {
            commentInput.addEventListener('input', async function() {
                const val = this.value;
                const pos = this.selectionStart;
                const before = val.slice(0, pos);
                const atIdx = before.lastIndexOf('@');
                if (atIdx >= 0 && (atIdx === 0 || /\s/.test(before[atIdx-1]))) {
                    const query = before.slice(atIdx + 1).toLowerCase();
                    if (!query.includes(' ')) {
                        mentionStart = atIdx;
                        const users = await loadMentionUsers();
                        const matches = users.filter(u => (u.email||'').toLowerCase().includes(query) || (u.display_name||'').toLowerCase().includes(query)).slice(0, 6);
                        if (matches.length > 0) {
                            // One button per match, built with the DOM: the id and
                            // name travel as data attributes and text, never as markup
                            // or as code in an inline handler.
                            mentionDropdown.textContent = '';
                            matches.forEach(u => {
                                const b = document.createElement('button');
                                b.type = 'button';
                                b.dataset.mentionId = u.id || u.ID || '';
                                b.dataset.mentionName = u.display_name || u.email || '';
                                b.textContent = u.display_name || u.email || '';
                                mentionDropdown.appendChild(b);
                            });
                            mentionDropdown.style.display = 'block';
                            return;
                        }
                    }
                }
                mentionStart = -1;
                mentionDropdown.style.display = 'none';
            });
            commentInput.addEventListener('keydown', function(e) {
                if (e.key === 'Escape') { mentionDropdown.style.display = 'none'; }
            });
            mentionDropdown.addEventListener('click', function(e) {
                const b = e.target.closest('button[data-mention-name]');
                if (b) insertMention(b.dataset.mentionId, b.dataset.mentionName);
            });
            document.addEventListener('click', function(e) {
                if (!mentionDropdown.contains(e.target) && e.target !== commentInput) {
                    mentionDropdown.style.display = 'none';
                }
            });
        }

        let pendingMentions = [];
        function insertMention(id, name) {
            if (mentionStart < 0) return;
            const val = commentInput.value;
            const pos = commentInput.selectionStart;
            commentInput.value = val.slice(0, mentionStart) + '@' + name + ' ' + val.slice(pos);
            mentionDropdown.style.display = 'none';
            if (id) pendingMentions.push(id);
            commentInput.focus();
        }

        async function postComment(contentId) {
            const text = commentInput.value.trim();
            if (!text) return;
            const btn = document.querySelector('#tab-discussion .btn-primary');
            btn.disabled = true;
            btn.textContent = 'Posting...';
            try {
                const resp = await fetch('/api/v1/content/' + contentId + '/comments', {
                    method: 'POST',
                    headers: csrfHeaders({'Content-Type': 'application/json'}),
                    body: JSON.stringify({text: text, mentions: pendingMentions})
                });
                if (!resp.ok) {
                    const err = await resp.json().catch(() => ({error: 'Request failed'}));
                    showAlert(dialogText(err.error || 'Failed to post comment'), 'Comment Failed');
                    return;
                }
                const comment = await resp.json();
                commentInput.value = '';
                pendingMentions = [];
                document.getElementById('no-comments-msg') && document.getElementById('no-comments-msg').remove();
                const thread = document.getElementById('comment-thread');
                const div = document.createElement('div');
                div.className = 'comment-item';
                div.dataset.commentId = comment.id;
                div.style.cssText = 'display:flex;gap:0.75rem;align-items:flex-start;';
                var adminBtn = currentUserRole === 'admin' ? '<button type="button" class="btn btn-sm" style="margin-left:auto;padding:0.1rem 0.5rem;font-size:0.75rem;background:transparent;color:var(--danger);border:1px solid rgba(239,68,68,0.3);" data-delete-comment>Delete</button>' : '';
                var displayName = escHtml(comment.user_display_name || comment.user_email || '?');
                div.innerHTML = '<div style="width:32px;height:32px;border-radius:50%;background:var(--bg-tertiary);display:flex;align-items:center;justify-content:center;font-size:0.8rem;color:var(--muted);flex-shrink:0;">' + displayName[0] + '</div>' +
                '<div style="flex:1;min-width:0;"><div style="display:flex;align-items:center;gap:0.5rem;margin-bottom:0.25rem;flex-wrap:wrap;">' +
                '<span style="font-weight:600;font-size:0.9rem;">' + displayName + '</span>' +
                '<span style="color:var(--muted);font-size:0.8rem;">Just now</span>' +
                adminBtn +
                '</div><p style="margin:0;color:var(--text);font-size:0.95rem;white-space:pre-wrap;word-break:break-word;">' + escHtml(comment.text) + '</p></div>';
                // The delete button gets a listener, not an inline handler built from the ids
                var delBtn = div.querySelector('[data-delete-comment]');
                if (delBtn) delBtn.addEventListener('click', function() { deleteComment(contentId, comment.id, delBtn); });
                thread.appendChild(div);
                // update badge
                const tabBtn = document.querySelector('[data-tab="discussion"]');
                const countEl = tabBtn.querySelector('span');
                const count = document.querySelectorAll('.comment-item').length;
                if (countEl) countEl.textContent = count;
                else tabBtn.innerHTML += ' <span style="background:var(--accent);color:white;border-radius:9999px;font-size:0.75rem;padding:0 0.4rem;margin-left:0.25rem;">' + count + '</span>';
            } catch(e) {
                showAlert(dialogText('Failed to post comment: ' + e.message), 'Comment Failed');
            } finally {
                btn.disabled = false;
                btn.textContent = 'Post Comment';
            }
        }

        async function deleteComment(contentId, commentId, btn) {
            if (!(await showConfirm('Delete this comment?', 'Delete Comment'))) return;
            btn.disabled = true;
            try {
                const resp = await fetch('/api/v1/content/' + contentId + '/comments/' + commentId, {method: 'DELETE', headers: csrfHeaders()});
                if (!resp.ok) { showAlert('Failed to delete comment', 'Delete Failed'); btn.disabled = false; return; }
                btn.closest('.comment-item').remove();
                const count = document.querySelectorAll('.comment-item').length;
                const tabBtn = document.querySelector('[data-tab="discussion"]');
                const countEl = tabBtn.querySelector('span');
                if (count === 0) {
                    if (countEl) countEl.remove();
                    const thread = document.getElementById('comment-thread');
                    const msg = document.createElement('p');
                    msg.id = 'no-comments-msg';
                    msg.style.cssText = 'color:var(--muted);font-size:0.9rem;margin-bottom:1.5rem;';
                    msg.textContent = 'No discussion yet. Be the first to comment.';
                    thread.parentNode.insertBefore(msg, thread.nextSibling);
                } else if (countEl) { countEl.textContent = count; }
            } catch(e) { showAlert(dialogText('Error: ' + e.message), 'Delete Failed'); btn.disabled = false; }
        }

        function escHtml(s) {
            return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        }

        // html/template quotes and escapes the value in script context; do not pre-quote it
        const currentUserRole = {{.CurrentUserRole}};
        </script>
        {{end}}

        <link href="/static/admin/quill/quill.snow.css?v=2.0.3" rel="stylesheet">
        <script src="/static/admin/quill/quill.js?v=2.0.3"></script>
        <style>
            .quill-wrapper { margin-bottom: 1rem; }
            .quill-wrapper .ql-toolbar { background: rgba(15, 23, 42, 0.5); border-color: var(--border); border-radius: var(--radius) var(--radius) 0 0; }
            .quill-wrapper .ql-container { background: rgba(15, 23, 42, 0.5); border-color: var(--border); border-radius: 0 0 var(--radius) var(--radius); min-height: 300px; }
            .quill-wrapper .ql-editor { color: var(--text); min-height: 280px; font-size: 1rem; }
            .quill-wrapper .ql-editor.ql-blank::before { color: var(--text-muted); }
            .ql-toolbar .ql-stroke { stroke: var(--text); }
            .ql-toolbar .ql-fill { fill: var(--text); }
            .ql-toolbar .ql-picker { color: var(--text); }
            .ql-toolbar .ql-picker-options { background: var(--bg-card); border-color: var(--border); }
            .ql-toolbar button:hover, .ql-toolbar button.ql-active { color: var(--primary); }
            .ql-toolbar button:hover .ql-stroke, .ql-toolbar button.ql-active .ql-stroke { stroke: var(--primary); }

            /* Search and Replace styles */
            .search-replace-panel {
                position: fixed;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                background: var(--bg-card);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1.5rem;
                width: 100%;
                max-width: 500px;
                box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
                z-index: 10001;
            }
            .search-replace-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                z-index: 10000;
            }
            .search-replace-panel h3 {
                margin-bottom: 1rem;
                font-size: 1.25rem;
                color: var(--text);
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }
            .search-replace-row {
                display: flex;
                gap: 0.5rem;
                margin-bottom: 0.75rem;
                align-items: center;
            }
            .search-replace-row label {
                width: 70px;
                color: var(--text);
                font-weight: 500;
                font-size: 0.9rem;
            }
            .search-replace-row input {
                flex: 1;
                padding: 0.6rem 0.75rem;
                background: rgba(15, 23, 42, 0.5);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                color: var(--text);
                font-size: 0.95rem;
            }
            .search-replace-row input:focus {
                outline: none;
                border-color: var(--primary);
                box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
            }
            .search-replace-options {
                display: flex;
                gap: 1rem;
                margin-bottom: 1rem;
                padding: 0.5rem 0;
            }
            .search-replace-options label {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                color: var(--text-muted);
                font-size: 0.85rem;
                cursor: pointer;
            }
            .search-replace-options input[type="checkbox"] {
                width: 16px;
                height: 16px;
                cursor: pointer;
            }
            .search-replace-status {
                padding: 0.5rem 0.75rem;
                background: rgba(15, 23, 42, 0.3);
                border-radius: var(--radius);
                margin-bottom: 1rem;
                font-size: 0.85rem;
                color: var(--text-muted);
                min-height: 36px;
                display: flex;
                align-items: center;
            }
            .search-replace-status.has-matches {
                color: var(--accent);
            }
            .search-replace-status.no-matches {
                color: var(--danger);
            }
            .search-replace-actions {
                display: flex;
                gap: 0.5rem;
                flex-wrap: wrap;
                padding-top: 1rem;
                border-top: 1px solid var(--border);
            }
            .search-replace-actions .btn {
                padding: 0.5rem 1rem;
                font-size: 0.9rem;
            }
            .search-replace-actions .btn-group {
                display: flex;
                gap: 0.5rem;
            }
            .search-replace-actions .spacer {
                flex: 1;
            }
            .search-highlight {
                background-color: rgba(250, 204, 21, 0.4) !important;
                color: inherit !important;
            }
            .search-highlight-current {
                background-color: rgba(250, 204, 21, 0.8) !important;
                color: #000 !important;
                outline: 2px solid var(--primary);
            }
            .ql-search-replace {
                width: auto !important;
                padding: 0 8px !important;
                font-size: 12px !important;
            }
            .ql-search-replace::after {
                content: 'Find';
            }
            /* Custom link modal styles */
            .link-modal-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10000;
            }
            .link-modal {
                background: var(--bg-card);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1.5rem;
                width: 100%;
                max-width: 500px;
                box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
            }
            .link-modal h3 {
                margin-bottom: 1.5rem;
                font-size: 1.25rem;
                color: var(--text);
            }
            .link-type-tabs {
                display: flex;
                gap: 0.5rem;
                margin-bottom: 1.5rem;
            }
            .link-type-tab {
                flex: 1;
                padding: 0.75rem;
                border: 1px solid var(--border);
                border-radius: var(--radius);
                background: transparent;
                color: var(--text-muted);
                cursor: pointer;
                font-size: 0.9rem;
                transition: all 0.2s;
            }
            .link-type-tab:hover {
                border-color: var(--primary);
                color: var(--text);
            }
            .link-type-tab.active {
                background: linear-gradient(135deg, var(--primary), var(--secondary));
                border-color: transparent;
                color: white;
            }
            .link-input-group {
                margin-bottom: 1rem;
                position: relative;
            }
            .link-input-group label {
                display: block;
                margin-bottom: 0.5rem;
                color: var(--text);
                font-weight: 500;
            }
            .link-input-group input {
                width: 100%;
                padding: 0.75rem 1rem;
                background: rgba(15, 23, 42, 0.5);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                color: var(--text);
                font-size: 1rem;
            }
            .link-input-group input:focus {
                outline: none;
                border-color: var(--primary);
                box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
            }
            .autocomplete-dropdown {
                position: absolute;
                top: 100%;
                left: 0;
                right: 0;
                background: var(--bg-card);
                border: 1px solid var(--border);
                border-top: none;
                border-radius: 0 0 var(--radius) var(--radius);
                max-height: 200px;
                overflow-y: auto;
                z-index: 10001;
                display: none;
            }
            .autocomplete-dropdown.show {
                display: block;
            }
            .autocomplete-item {
                padding: 0.75rem 1rem;
                cursor: pointer;
                border-bottom: 1px solid var(--border);
            }
            .autocomplete-item:last-child {
                border-bottom: none;
            }
            .autocomplete-item:hover,
            .autocomplete-item.selected {
                background: var(--bg-hover);
            }
            .autocomplete-item .slug {
                font-family: 'JetBrains Mono', monospace;
                font-size: 0.85rem;
                color: var(--accent);
            }
            .autocomplete-item .title {
                font-size: 0.8rem;
                color: var(--text-muted);
                margin-top: 0.25rem;
            }
            .link-modal-actions {
                display: flex;
                gap: 1rem;
                justify-content: flex-end;
                margin-top: 1.5rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border);
            }
            .link-hint {
                font-size: 0.8rem;
                color: var(--text-muted);
                margin-top: 0.5rem;
            }
        </style>
        <script>
        // Global slugs cache for autocomplete
        var siteSlugs = [];

        // Load all slugs for autocomplete
        function loadSlugs() {
            fetch('/api/slugs')
                .then(function(res) { return res.json(); })
                .then(function(data) { siteSlugs = data; })
                .catch(function(err) { console.log('Failed to load slugs:', err); });
        }

        // Custom link handler
        function createLinkModal(quill, existingLink) {
            var selection = quill.getSelection();
            var selectedText = selection ? quill.getText(selection.index, selection.length) : '';

            var overlay = document.createElement('div');
            overlay.className = 'link-modal-overlay';

            var modal = document.createElement('div');
            modal.className = 'link-modal';
            modal.innerHTML = ` + "`" + `
                <h3>${existingLink ? 'Edit Link' : 'Insert Link'}</h3>
                <div class="link-type-tabs">
                    <button type="button" class="link-type-tab active" data-type="internal">Internal Page</button>
                    <button type="button" class="link-type-tab" data-type="external">External URL</button>
                </div>
                <div id="internal-link-section">
                    <div class="link-input-group">
                        <label>Select Page</label>
                        <input type="text" id="internal-link-input" placeholder="Start typing to search pages..." autocomplete="off">
                        <div class="autocomplete-dropdown" id="autocomplete-dropdown"></div>
                        <p class="link-hint">Type to search, use Tab to accept suggestion</p>
                    </div>
                </div>
                <div id="external-link-section" style="display: none;">
                    <div class="link-input-group">
                        <label>URL</label>
                        <input type="text" id="external-link-input" placeholder="https://example.com">
                    </div>
                </div>
                <div class="link-modal-actions">
                    <button type="button" class="btn btn-outline" id="link-cancel">Cancel</button>
                    ${existingLink ? '<button type="button" class="btn btn-danger" id="link-remove">Remove Link</button>' : ''}
                    <button type="button" class="btn btn-primary" id="link-save">Save</button>
                </div>
            ` + "`" + `;

            overlay.appendChild(modal);
            document.body.appendChild(overlay);

            var internalInput = modal.querySelector('#internal-link-input');
            var externalInput = modal.querySelector('#external-link-input');
            var dropdown = modal.querySelector('#autocomplete-dropdown');
            var currentLinkType = 'internal';
            var selectedIndex = -1;
            var filteredSlugs = [];

            // Pre-fill with existing link
            if (existingLink) {
                if (existingLink.startsWith('/')) {
                    internalInput.value = existingLink;
                    currentLinkType = 'internal';
                } else {
                    externalInput.value = existingLink;
                    currentLinkType = 'external';
                    modal.querySelector('[data-type="internal"]').classList.remove('active');
                    modal.querySelector('[data-type="external"]').classList.add('active');
                    modal.querySelector('#internal-link-section').style.display = 'none';
                    modal.querySelector('#external-link-section').style.display = 'block';
                }
            }

            // Tab switching
            modal.querySelectorAll('.link-type-tab').forEach(function(tab) {
                tab.addEventListener('click', function() {
                    modal.querySelectorAll('.link-type-tab').forEach(function(t) { t.classList.remove('active'); });
                    tab.classList.add('active');
                    currentLinkType = tab.dataset.type;
                    if (currentLinkType === 'internal') {
                        modal.querySelector('#internal-link-section').style.display = 'block';
                        modal.querySelector('#external-link-section').style.display = 'none';
                        internalInput.focus();
                    } else {
                        modal.querySelector('#internal-link-section').style.display = 'none';
                        modal.querySelector('#external-link-section').style.display = 'block';
                        externalInput.focus();
                    }
                });
            });

            // Autocomplete functionality
            function updateAutocomplete() {
                var query = internalInput.value.toLowerCase();
                filteredSlugs = siteSlugs.filter(function(item) {
                    return item.slug.toLowerCase().includes(query) || item.title.toLowerCase().includes(query);
                }).slice(0, 10);

                if (filteredSlugs.length > 0 && query.length > 0) {
                    dropdown.innerHTML = filteredSlugs.map(function(item, i) {
                        return '<div class="autocomplete-item' + (i === selectedIndex ? ' selected' : '') + '" data-slug="' + item.slug + '">' +
                            '<div class="slug">' + item.slug + '</div>' +
                            '<div class="title">' + item.title + '</div>' +
                        '</div>';
                    }).join('');
                    dropdown.classList.add('show');
                } else {
                    dropdown.classList.remove('show');
                }
            }

            internalInput.addEventListener('input', function() {
                selectedIndex = -1;
                updateAutocomplete();
            });

            internalInput.addEventListener('keydown', function(e) {
                if (e.key === 'Tab' && filteredSlugs.length > 0) {
                    e.preventDefault();
                    var idx = selectedIndex >= 0 ? selectedIndex : 0;
                    internalInput.value = filteredSlugs[idx].slug;
                    dropdown.classList.remove('show');
                } else if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    selectedIndex = Math.min(selectedIndex + 1, filteredSlugs.length - 1);
                    updateAutocomplete();
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    selectedIndex = Math.max(selectedIndex - 1, -1);
                    updateAutocomplete();
                } else if (e.key === 'Enter') {
                    e.preventDefault();
                    if (selectedIndex >= 0 && filteredSlugs[selectedIndex]) {
                        internalInput.value = filteredSlugs[selectedIndex].slug;
                        dropdown.classList.remove('show');
                    } else {
                        modal.querySelector('#link-save').click();
                    }
                } else if (e.key === 'Escape') {
                    overlay.remove();
                }
            });

            dropdown.addEventListener('click', function(e) {
                var item = e.target.closest('.autocomplete-item');
                if (item) {
                    internalInput.value = item.dataset.slug;
                    dropdown.classList.remove('show');
                }
            });

            // Close dropdown when clicking outside
            document.addEventListener('click', function closeDropdown(e) {
                if (!dropdown.contains(e.target) && e.target !== internalInput) {
                    dropdown.classList.remove('show');
                }
            });

            // Save link
            modal.querySelector('#link-save').addEventListener('click', function() {
                var link = currentLinkType === 'internal' ? internalInput.value : externalInput.value;
                if (link) {
                    // Ensure internal links start with /
                    if (currentLinkType === 'internal' && !link.startsWith('/')) {
                        link = '/' + link;
                    }
                    if (selection && selection.length > 0) {
                        quill.formatText(selection.index, selection.length, 'link', link);
                    } else {
                        // Insert link at cursor position
                        var text = selectedText || link;
                        quill.insertText(selection ? selection.index : 0, text, 'link', link);
                    }
                }
                overlay.remove();
            });

            // Cancel
            modal.querySelector('#link-cancel').addEventListener('click', function() {
                overlay.remove();
            });

            // Remove link
            var removeBtn = modal.querySelector('#link-remove');
            if (removeBtn) {
                removeBtn.addEventListener('click', function() {
                    if (selection && selection.length > 0) {
                        quill.formatText(selection.index, selection.length, 'link', false);
                    }
                    overlay.remove();
                });
            }

            // Close on overlay click
            overlay.addEventListener('click', function(e) {
                if (e.target === overlay) {
                    overlay.remove();
                }
            });

            // Focus appropriate input
            setTimeout(function() {
                if (currentLinkType === 'internal') {
                    internalInput.focus();
                } else {
                    externalInput.focus();
                }
            }, 100);
        }

        // Search and Replace functionality
        var searchReplaceState = {
            quill: null,
            matches: [],
            currentIndex: -1,
            searchTerm: '',
            caseSensitive: false,
            overlay: null
        };

        function openSearchReplace(quill) {
            searchReplaceState.quill = quill;
            searchReplaceState.matches = [];
            searchReplaceState.currentIndex = -1;

            var overlay = document.createElement('div');
            overlay.className = 'search-replace-overlay';
            searchReplaceState.overlay = overlay;

            var panel = document.createElement('div');
            panel.className = 'search-replace-panel';
            panel.innerHTML = ` + "`" + `
                <h3>
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" width="20" height="20">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                    </svg>
                    Find and Replace
                </h3>
                <div class="search-replace-row">
                    <label>Find:</label>
                    <input type="text" id="sr-search" placeholder="Search text..." autofocus>
                </div>
                <div class="search-replace-row">
                    <label>Replace:</label>
                    <input type="text" id="sr-replace" placeholder="Replacement text...">
                </div>
                <div class="search-replace-options">
                    <label>
                        <input type="checkbox" id="sr-case-sensitive">
                        Case sensitive
                    </label>
                </div>
                <div class="search-replace-status" id="sr-status">
                    Enter search text to find matches
                </div>
                <div class="search-replace-actions">
                    <div class="btn-group">
                        <button type="button" class="btn btn-outline" id="sr-prev">Previous</button>
                        <button type="button" class="btn btn-outline" id="sr-next">Next</button>
                    </div>
                    <div class="btn-group">
                        <button type="button" class="btn btn-outline" id="sr-replace-one">Replace</button>
                        <button type="button" class="btn btn-primary" id="sr-replace-all">Replace All</button>
                    </div>
                    <div class="spacer"></div>
                    <button type="button" class="btn btn-outline" id="sr-close">Close</button>
                </div>
            ` + "`" + `;

            overlay.appendChild(panel);
            document.body.appendChild(overlay);

            var searchInput = document.getElementById('sr-search');
            var caseSensitive = document.getElementById('sr-case-sensitive');

            searchInput.addEventListener('input', function() { performSearch(); });
            caseSensitive.addEventListener('change', function() {
                searchReplaceState.caseSensitive = this.checked;
                performSearch();
            });
            document.getElementById('sr-prev').addEventListener('click', function() { navigateMatch(-1); });
            document.getElementById('sr-next').addEventListener('click', function() { navigateMatch(1); });
            document.getElementById('sr-replace-one').addEventListener('click', function() { replaceCurrentMatch(); });
            document.getElementById('sr-replace-all').addEventListener('click', function() { replaceAllWithReview(); });
            document.getElementById('sr-close').addEventListener('click', function() { closeSearchReplace(); });

            overlay.addEventListener('keydown', function(e) {
                if (e.key === 'Escape') closeSearchReplace();
                else if (e.key === 'Enter' && e.target.id === 'sr-search') { e.preventDefault(); navigateMatch(1); }
                else if (e.key === 'Enter' && e.target.id === 'sr-replace') { e.preventDefault(); replaceCurrentMatch(); }
            });
            overlay.addEventListener('click', function(e) { if (e.target === overlay) closeSearchReplace(); });
            searchInput.focus();
        }

        function performSearch() {
            var searchInput = document.getElementById('sr-search');
            var status = document.getElementById('sr-status');
            var searchTerm = searchInput.value;
            searchReplaceState.searchTerm = searchTerm;
            clearHighlights();
            if (!searchTerm) {
                searchReplaceState.matches = [];
                searchReplaceState.currentIndex = -1;
                status.textContent = 'Enter search text to find matches';
                status.className = 'search-replace-status';
                return;
            }
            var quill = searchReplaceState.quill;
            var text = quill.getText();
            var matches = [];
            var searchText = searchReplaceState.caseSensitive ? searchTerm : searchTerm.toLowerCase();
            var contentText = searchReplaceState.caseSensitive ? text : text.toLowerCase();
            var pos = 0;
            while ((pos = contentText.indexOf(searchText, pos)) !== -1) {
                matches.push({ index: pos, length: searchTerm.length });
                pos += 1;
            }
            searchReplaceState.matches = matches;
            if (matches.length > 0) {
                searchReplaceState.currentIndex = 0;
                highlightMatches();
                status.textContent = 'Found ' + matches.length + ' match' + (matches.length > 1 ? 'es' : '') + ' - showing 1 of ' + matches.length;
                status.className = 'search-replace-status has-matches';
            } else {
                searchReplaceState.currentIndex = -1;
                status.textContent = 'No matches found';
                status.className = 'search-replace-status no-matches';
            }
        }

        function highlightMatches() {
            var quill = searchReplaceState.quill;
            var matches = searchReplaceState.matches;
            var currentIndex = searchReplaceState.currentIndex;
            matches.forEach(function(match, idx) {
                quill.formatText(match.index, match.length, 'background', idx === currentIndex ? '#facc15' : 'rgba(250, 204, 21, 0.4)');
            });
            if (currentIndex >= 0 && matches[currentIndex]) {
                var bounds = quill.getBounds(matches[currentIndex].index, matches[currentIndex].length);
                quill.root.scrollTop = bounds.top - quill.root.clientHeight / 2;
            }
        }

        function clearHighlights() {
            var quill = searchReplaceState.quill;
            if (quill) quill.formatText(0, quill.getLength(), 'background', false);
        }

        function navigateMatch(direction) {
            var matches = searchReplaceState.matches;
            if (matches.length === 0) return;
            var newIndex = searchReplaceState.currentIndex + direction;
            if (newIndex < 0) newIndex = matches.length - 1;
            if (newIndex >= matches.length) newIndex = 0;
            searchReplaceState.currentIndex = newIndex;
            clearHighlights();
            highlightMatches();
            var status = document.getElementById('sr-status');
            status.textContent = 'Found ' + matches.length + ' match' + (matches.length > 1 ? 'es' : '') + ' - showing ' + (newIndex + 1) + ' of ' + matches.length;
        }

        function replaceCurrentMatch() {
            var matches = searchReplaceState.matches;
            var currentIndex = searchReplaceState.currentIndex;
            if (currentIndex < 0 || !matches[currentIndex]) return;
            var quill = searchReplaceState.quill;
            var replaceText = document.getElementById('sr-replace').value;
            var match = matches[currentIndex];
            quill.formatText(match.index, match.length, 'background', false);
            quill.deleteText(match.index, match.length);
            quill.insertText(match.index, replaceText);
            performSearch();
        }

        function replaceAllWithReview() {
            var matches = searchReplaceState.matches;
            if (matches.length === 0) return;
            var quill = searchReplaceState.quill;
            var replaceText = document.getElementById('sr-replace').value;
            var searchTerm = searchReplaceState.searchTerm;
            var status = document.getElementById('sr-status');
            var reviewIndex = 0, replacedCount = 0, skippedCount = 0;

            function escapeHtml(text) {
                var div = document.createElement('div');
                div.textContent = text;
                return div.innerHTML;
            }

            function restoreActions() {
                var actionsDiv = document.querySelector('.search-replace-actions');
                actionsDiv.innerHTML = '<div class="btn-group"><button type="button" class="btn btn-outline" id="sr-prev">Previous</button><button type="button" class="btn btn-outline" id="sr-next">Next</button></div><div class="btn-group"><button type="button" class="btn btn-outline" id="sr-replace-one">Replace</button><button type="button" class="btn btn-primary" id="sr-replace-all">Replace All</button></div><div class="spacer"></div><button type="button" class="btn btn-outline" id="sr-close">Close</button>';
                document.getElementById('sr-prev').addEventListener('click', function() { navigateMatch(-1); });
                document.getElementById('sr-next').addEventListener('click', function() { navigateMatch(1); });
                document.getElementById('sr-replace-one').addEventListener('click', function() { replaceCurrentMatch(); });
                document.getElementById('sr-replace-all').addEventListener('click', function() { replaceAllWithReview(); });
                document.getElementById('sr-close').addEventListener('click', function() { closeSearchReplace(); });
            }

            function reviewNext() {
                var text = quill.getText();
                var searchText = searchReplaceState.caseSensitive ? searchTerm : searchTerm.toLowerCase();
                var contentText = searchReplaceState.caseSensitive ? text : text.toLowerCase();
                var pos = contentText.indexOf(searchText);
                if (pos === -1) {
                    status.textContent = 'Replaced ' + replacedCount + ' matches. Done.';
                    status.className = 'search-replace-status has-matches';
                    restoreActions();
                    return;
                }
                clearHighlights();
                quill.formatText(pos, searchTerm.length, 'background', '#facc15');
                var bounds = quill.getBounds(pos, searchTerm.length);
                quill.root.scrollTop = bounds.top - quill.root.clientHeight / 2;
                status.innerHTML = 'Match ' + (reviewIndex + 1) + ': Replace "<strong>' + escapeHtml(searchTerm) + '</strong>" with "<strong>' + escapeHtml(replaceText) + '</strong>"?';

                var actionsDiv = document.querySelector('.search-replace-actions');
                actionsDiv.innerHTML = '<button type="button" class="btn btn-primary" id="sr-confirm-yes">Yes</button><button type="button" class="btn btn-outline" id="sr-confirm-skip">Skip</button><button type="button" class="btn btn-outline" id="sr-confirm-all">All Remaining</button><div class="spacer"></div><button type="button" class="btn btn-outline" id="sr-confirm-cancel">Cancel</button>';

                document.getElementById('sr-confirm-yes').addEventListener('click', function() {
                    quill.formatText(pos, searchTerm.length, 'background', false);
                    quill.deleteText(pos, searchTerm.length);
                    quill.insertText(pos, replaceText);
                    replacedCount++;
                    reviewIndex++;
                    reviewNext();
                });
                document.getElementById('sr-confirm-skip').addEventListener('click', function() {
                    quill.formatText(pos, searchTerm.length, 'background', false);
                    skippedCount++;
                    reviewIndex++;
                    // Skip this match by inserting a marker then continuing
                    reviewNext();
                });
                document.getElementById('sr-confirm-all').addEventListener('click', function() {
                    while (true) {
                        var txt = quill.getText();
                        var sTxt = searchReplaceState.caseSensitive ? searchTerm : searchTerm.toLowerCase();
                        var cTxt = searchReplaceState.caseSensitive ? txt : txt.toLowerCase();
                        var p = cTxt.indexOf(sTxt);
                        if (p === -1) break;
                        quill.deleteText(p, searchTerm.length);
                        quill.insertText(p, replaceText);
                        replacedCount++;
                    }
                    status.textContent = 'Replaced ' + replacedCount + ' matches total';
                    status.className = 'search-replace-status has-matches';
                    restoreActions();
                    performSearch();
                });
                document.getElementById('sr-confirm-cancel').addEventListener('click', function() {
                    clearHighlights();
                    status.textContent = 'Cancelled. Replaced ' + replacedCount + ' matches.';
                    restoreActions();
                    performSearch();
                });
            }
            reviewNext();
        }

        function closeSearchReplace() {
            clearHighlights();
            if (searchReplaceState.overlay) {
                searchReplaceState.overlay.remove();
                searchReplaceState.overlay = null;
            }
            searchReplaceState.quill = null;
            searchReplaceState.matches = [];
            searchReplaceState.currentIndex = -1;
        }

        // Initialize Quill with custom link handler and search/replace
        function initQuillEditor(textarea) {
            var wrapper = document.createElement('div');
            wrapper.className = 'quill-wrapper';
            var editorDiv = document.createElement('div');
            wrapper.appendChild(editorDiv);
            textarea.parentNode.insertBefore(wrapper, textarea);
            textarea.style.display = 'none';

            var quill = new Quill(editorDiv, {
                theme: 'snow',
                modules: {
                    toolbar: {
                        container: [
                            [{ 'header': [1, 2, 3, false] }],
                            ['bold', 'italic', 'underline', 'strike'],
                            [{ 'color': [] }, { 'background': [] }],
                            [{ 'list': 'ordered'}, { 'list': 'bullet' }],
                            [{ 'indent': '-1'}, { 'indent': '+1' }],
                            ['link', 'image', 'video'],
                            ['blockquote', 'code-block'],
                            ['clean'],
                            ['search-replace']
                        ],
                        handlers: {
                            'link': function(value) {
                                var selection = quill.getSelection();
                                var existingLink = null;
                                if (selection && selection.length > 0) {
                                    var format = quill.getFormat(selection);
                                    existingLink = format.link || null;
                                }
                                createLinkModal(quill, existingLink);
                            },
                            'search-replace': function() {
                                openSearchReplace(quill);
                            }
                        }
                    }
                }
            });

            // Store original HTML before Quill modifies it
            textarea.dataset.originalHtml = textarea.value;

            quill.root.innerHTML = textarea.value;

            quill.on('text-change', function() {
                textarea.value = quill.root.innerHTML;
                // Mark that content has been edited via Quill
                textarea.dataset.editedViaQuill = 'true';
            });

            textarea.form.addEventListener('submit', function() {
                textarea.value = quill.root.innerHTML;
            });

            // Store quill instance on wrapper for later access
            wrapper.quillInstance = quill;

            return quill;
        }

        document.addEventListener('DOMContentLoaded', function() {
            // Load slugs for autocomplete
            loadSlugs();

            document.querySelectorAll('.richtext').forEach(function(textarea) {
                initQuillEditor(textarea);
            });

            // Initialize raw mode if checkbox is checked
            if (document.getElementById('raw_mode') && document.getElementById('raw_mode').checked) {
                toggleEditorMode();
            }
        });

        function toggleEditorMode() {
            var rawMode = document.getElementById('raw_mode');
            if (!rawMode) return;

            var textareas = document.querySelectorAll('.richtext');
            textareas.forEach(function(textarea) {
                var wrapper = textarea.previousElementSibling;
                if (rawMode.checked) {
                    // Switch to raw mode - hide Quill, show textarea
                    if (wrapper && wrapper.classList.contains('quill-wrapper')) {
                        // Sync Quill content to textarea before hiding
                        if (wrapper.quillInstance) {
                            textarea.value = wrapper.quillInstance.root.innerHTML;
                        }
                        wrapper.style.display = 'none';
                    }
                    textarea.style.display = 'block';
                    textarea.classList.add('code-editor');
                    textarea.rows = 20;
                } else {
                    // Switch to rich mode - show Quill, hide textarea
                    if (wrapper && wrapper.classList.contains('quill-wrapper')) {
                        wrapper.style.display = 'block';
                        // Sync textarea content to Quill
                        if (wrapper.quillInstance) {
                            wrapper.quillInstance.root.innerHTML = textarea.value;
                        }
                    }
                    textarea.style.display = 'none';
                    textarea.classList.remove('code-editor');
                }
            });
        }

        function toggleFieldHtmlMode(button, fieldId) {
            var textarea = document.getElementById(fieldId);
            if (!textarea) return;

            // Find the quill-wrapper - it's inserted before the textarea by initQuillEditor
            var wrapper = textarea.previousElementSibling;
            while (wrapper && !wrapper.classList.contains('quill-wrapper')) {
                wrapper = wrapper.previousElementSibling;
            }

            var isRawMode = textarea.style.display !== 'none';

            if (!isRawMode) {
                // Switch to raw HTML mode - hide Quill, show textarea
                if (wrapper && wrapper.classList.contains('quill-wrapper')) {
                    wrapper.style.display = 'none';
                }
                // Use original HTML if user hasn't edited via Quill, otherwise use current Quill content
                if (textarea.dataset.originalHtml && textarea.dataset.editedViaQuill !== 'true') {
                    textarea.value = textarea.dataset.originalHtml;
                } else if (wrapper && wrapper.quillInstance) {
                    textarea.value = wrapper.quillInstance.root.innerHTML;
                }
                textarea.style.display = 'block';
                textarea.classList.add('code-editor');
                textarea.rows = 30;
                button.innerHTML = '&#x270D; Rich Editor';
                button.classList.add('active');
            } else {
                // Switch to rich editor mode - show Quill, hide textarea
                if (wrapper && wrapper.classList.contains('quill-wrapper')) {
                    wrapper.style.display = 'block';
                    // Sync textarea content to Quill
                    if (wrapper.quillInstance) {
                        wrapper.quillInstance.root.innerHTML = textarea.value;
                    }
                }
                // Clear the edited flag since we're syncing back to Quill
                textarea.dataset.editedViaQuill = 'true';
                textarea.style.display = 'none';
                textarea.classList.remove('code-editor');
                button.innerHTML = '&lt;/&gt; Edit HTML';
                button.classList.remove('active');
            }
        }

        // Track original slug for comparison
        var originalSlug = document.getElementById('slug') ? document.getElementById('slug').value : '';
        var slugEl = document.getElementById('slug');
        var isNewContent = slugEl && !slugEl.readOnly;
        var slugRenameEnabled = false;

        // Update URL path preview when folder or slug changes
        function updatePathPreview() {
            var folderSelect = document.getElementById('folder_id');
            var slugInput = document.getElementById('slug');
            var titleInput = document.getElementById('title');
            var preview = document.getElementById('preview-path');
            if (!preview) return;

            var folder = folderSelect ? folderSelect.options[folderSelect.selectedIndex] : null;
            var folderPath = (folder && folder.value !== 'root') ? folder.text.split(' ')[0] : '';
            var slug = slugInput ? slugInput.value : '';

            // Only auto-generate slug from title for NEW content when slug is empty
            if (isNewContent && !slug && titleInput && titleInput.value) {
                slug = titleInput.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
            }

            var fullPath = folderPath ? folderPath + '/' + slug : '/' + slug;
            if (!slug) fullPath = folderPath || '/';

            preview.textContent = fullPath;
        }

        // Enable slug renaming for existing content
        function enableSlugRename() {
            var slugInput = document.getElementById('slug');
            var renameBtn = document.getElementById('rename-slug-btn');
            var cancelBtn = document.getElementById('cancel-rename-btn');
            var renameEnabledField = document.getElementById('slug_rename_enabled');

            if (slugInput && renameBtn && cancelBtn) {
                slugInput.readOnly = false;
                slugInput.style.background = '';
                renameBtn.style.display = 'none';
                cancelBtn.style.display = '';
                slugRenameEnabled = true;
                if (renameEnabledField) renameEnabledField.value = 'yes';
                slugInput.focus();
            }
        }

        // Cancel slug rename and restore original value
        function cancelSlugRename() {
            var slugInput = document.getElementById('slug');
            var renameBtn = document.getElementById('rename-slug-btn');
            var cancelBtn = document.getElementById('cancel-rename-btn');
            var errorEl = document.getElementById('slug-error');
            var renameEnabledField = document.getElementById('slug_rename_enabled');

            if (slugInput && renameBtn && cancelBtn) {
                slugInput.value = originalSlug;
                slugInput.readOnly = true;
                slugInput.style.background = 'var(--bg-tertiary)';
                renameBtn.style.display = '';
                cancelBtn.style.display = 'none';
                slugRenameEnabled = false;
                if (renameEnabledField) renameEnabledField.value = '';
                if (errorEl) errorEl.style.display = 'none';
                updatePathPreview();
            }
        }

        // Check for duplicate slug before form submission
        async function validateSlug() {
            var slugInput = document.getElementById('slug');
            var errorEl = document.getElementById('slug-error');
            var folderSelect = document.getElementById('folder_id');

            if (!slugInput || !errorEl) return true;

            var slug = slugInput.value.trim();
            var folderPath = '';
            if (folderSelect && folderSelect.value !== 'root') {
                var selectedOption = folderSelect.options[folderSelect.selectedIndex];
                folderPath = selectedOption.text.split(' ')[0];
            }

            var fullPath = folderPath ? folderPath + '/' + slug : '/' + slug;
            if (!slug) fullPath = folderPath || '/';

            // Skip validation if slug hasn't changed
            var currentFullPath = '{{if .Content}}{{.Content.FullPath}}{{end}}';
            if (!currentFullPath) currentFullPath = originalSlug ? '/' + originalSlug : '/';
            if (fullPath === currentFullPath) return true;

            try {
                var response = await fetch('/api/content/check-slug?path=' + encodeURIComponent(fullPath) + '{{if .Content}}&exclude={{.Content.ID.Hex}}{{end}}');
                var data = await response.json();

                if (data.exists) {
                    errorEl.textContent = 'A page already exists at "' + fullPath + '". Please choose a different slug.';
                    errorEl.style.display = 'block';
                    slugInput.focus();
                    return false;
                }
                errorEl.style.display = 'none';
                return true;
            } catch (e) {
                console.error('Error checking slug:', e);
                return true; // Allow submission, server will validate
            }
        }

        // Set up event listeners for path preview
        document.addEventListener('DOMContentLoaded', function() {
            var folderSelect = document.getElementById('folder_id');
            var slugInput = document.getElementById('slug');
            var titleInput = document.getElementById('title');
            var form = document.querySelector('form.form-card');

            if (folderSelect) folderSelect.addEventListener('change', updatePathPreview);
            if (slugInput) slugInput.addEventListener('input', updatePathPreview);

            // Only auto-update slug from title for new content
            if (isNewContent && titleInput) {
                titleInput.addEventListener('input', updatePathPreview);
            }

            // Show redirect confirmation modal and return a promise
            function showRedirectModal(oldPath, newPath) {
                return new Promise(function(resolve) {
                    var modal = document.getElementById('redirect-modal');
                    var oldPathEl = document.getElementById('redirect-old-path');
                    var newPathEl = document.getElementById('redirect-new-path');
                    var yesBtn = document.getElementById('redirect-yes-btn');
                    var noBtn = document.getElementById('redirect-no-btn');

                    oldPathEl.textContent = oldPath;
                    newPathEl.textContent = newPath;
                    modal.style.display = 'flex';

                    function cleanup() {
                        modal.style.display = 'none';
                        yesBtn.removeEventListener('click', onYes);
                        noBtn.removeEventListener('click', onNo);
                    }

                    function onYes() {
                        cleanup();
                        resolve(true);
                    }

                    function onNo() {
                        cleanup();
                        resolve(false);
                    }

                    yesBtn.addEventListener('click', onYes);
                    noBtn.addEventListener('click', onNo);
                });
            }

            // Show version comment modal and return a promise
            function showVersionCommentModal(nextVersionNumber) {
                return new Promise(function(resolve) {
                    var modal = document.getElementById('version-comment-modal');
                    var versionDisplay = document.getElementById('version-number-display');
                    var commentInput = document.getElementById('version-comment-input');
                    var saveBtn = document.getElementById('version-comment-save');
                    var cancelBtn = document.getElementById('version-comment-cancel');

                    if (!modal) {
                        // Modal doesn't exist (probably new content)
                        resolve({ proceed: true, comment: '' });
                        return;
                    }

                    versionDisplay.textContent = 'v' + nextVersionNumber;
                    commentInput.value = '';
                    modal.style.display = 'flex';
                    commentInput.focus();

                    function cleanup() {
                        modal.style.display = 'none';
                        saveBtn.removeEventListener('click', onSave);
                        cancelBtn.removeEventListener('click', onCancel);
                        commentInput.removeEventListener('keydown', onKeydown);
                    }

                    function onSave() {
                        cleanup();
                        resolve({ proceed: true, comment: commentInput.value.trim() });
                    }

                    function onCancel() {
                        cleanup();
                        resolve({ proceed: false, comment: '' });
                    }

                    function onKeydown(e) {
                        if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                            onSave();
                        } else if (e.key === 'Escape') {
                            onCancel();
                        }
                    }

                    saveBtn.addEventListener('click', onSave);
                    cancelBtn.addEventListener('click', onCancel);
                    commentInput.addEventListener('keydown', onKeydown);
                });
            }

            // Validate slug on form submit and prompt for redirect if slug changed
            if (form) {
                form.addEventListener('submit', async function(e) {
                    e.preventDefault();
                    if (await validateSlug()) {
                        // Check if slug has changed (for existing content only)
                        if (!isNewContent && slugRenameEnabled) {
                            var slugInput = document.getElementById('slug');
                            var folderSelect = document.getElementById('folder_id');
                            var newSlug = slugInput ? slugInput.value.trim() : '';

                            var folderPath = '';
                            if (folderSelect && folderSelect.value !== 'root') {
                                var selectedOption = folderSelect.options[folderSelect.selectedIndex];
                                folderPath = selectedOption.text.split(' ')[0];
                            }

                            var newFullPath = folderPath ? folderPath + '/' + newSlug : '/' + newSlug;
                            if (!newSlug) newFullPath = folderPath || '/';

                            var oldFullPath = '{{if .Content}}{{.Content.FullPath}}{{end}}';
                            if (!oldFullPath) oldFullPath = originalSlug ? '/' + originalSlug : '/';

                            // If path actually changed, show redirect modal
                            if (newFullPath !== oldFullPath) {
                                var createRedirect = await showRedirectModal(oldFullPath, newFullPath);
                                document.getElementById('create_redirect').value = createRedirect ? 'yes' : 'no';
                            }
                        }

                        // Show version comment modal for updates (not new content)
                        if (!isNewContent) {
                            var currentVersionCount = {{if .Versions}}{{len .Versions}}{{else}}0{{end}};
                            var nextVersion = currentVersionCount + 1;
                            var result = await showVersionCommentModal(nextVersion);
                            if (!result.proceed) {
                                return; // User cancelled
                            }
                            document.getElementById('version_comment').value = result.comment;
                        }

                        form.submit();
                    }
                });
            }

            // Initial preview
            updatePathPreview();
        });

        // Change template modal functions
        function showChangeTemplateModal() {
            var modal = document.getElementById('change-template-modal');
            if (modal) {
                modal.style.display = 'flex';
            }
        }

        function closeChangeTemplateModal() {
            var modal = document.getElementById('change-template-modal');
            if (modal) {
                modal.style.display = 'none';
            }
        }

        function proceedToTemplatePreview() {
            var select = document.getElementById('new-template-select');
            if (select && select.value) {
                var contentId = '{{if .Content}}{{.Content.ID.Hex}}{{end}}';
                window.location.href = '/cm/content/' + contentId + '/change-template/' + select.value;
            }
        }
        </script>
    ` + adminLayoutEnd,

	"change_template_preview": adminLayoutStart + `
        <div class="page-header">
            <h1>Change Template Preview</h1>
        </div>
        <div class="form-card">
            <div style="margin-bottom: 1.5rem; padding: 1rem; background: var(--bg-tertiary); border-radius: var(--radius);">
                <p style="margin: 0;">
                    <strong>Content:</strong> {{.Content.Title}}<br>
                    <strong>From:</strong> {{.OldTemplate.Name}} → <strong>To:</strong> {{.NewTemplate.Name}}
                </p>
            </div>

            <h3 style="margin-bottom: 1rem; color: var(--text);">Field Mapping</h3>

            {{if .MappedFields}}
            <div style="margin-bottom: 1.5rem;">
                <h4 style="color: var(--success); margin-bottom: 0.75rem;">Fields that will be preserved:</h4>
                <table style="width: 100%;">
                    <thead>
                        <tr>
                            <th style="text-align: left; padding: 0.5rem; border-bottom: 1px solid var(--border);">Field Name</th>
                            <th style="text-align: left; padding: 0.5rem; border-bottom: 1px solid var(--border);">Current Value</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .MappedFields}}
                        <tr>
                            <td style="padding: 0.5rem; border-bottom: 1px solid var(--border);">
                                <code>{{.Name}}</code>
                                {{if ne .OldType .NewType}}
                                <span style="color: var(--warning); font-size: 0.85rem;">(type: {{.OldType}} → {{.NewType}})</span>
                                {{end}}
                            </td>
                            <td style="padding: 0.5rem; border-bottom: 1px solid var(--border); max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                                {{if .Value}}{{.Value}}{{else}}<span style="color: var(--muted);">(empty)</span>{{end}}
                            </td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{end}}

            {{if .LostFields}}
            <div style="margin-bottom: 1.5rem; padding: 1rem; background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: var(--radius);">
                <h4 style="color: var(--danger); margin-bottom: 0.75rem;">Fields that will be LOST:</h4>
                <p style="color: var(--muted); margin-bottom: 0.75rem;">These fields exist in the current template but not in the new template. Their data will be permanently lost.</p>
                <table style="width: 100%;">
                    <thead>
                        <tr>
                            <th style="text-align: left; padding: 0.5rem; border-bottom: 1px solid rgba(239, 68, 68, 0.3);">Field Name</th>
                            <th style="text-align: left; padding: 0.5rem; border-bottom: 1px solid rgba(239, 68, 68, 0.3);">Current Value (will be lost)</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .LostFields}}
                        <tr>
                            <td style="padding: 0.5rem; border-bottom: 1px solid rgba(239, 68, 68, 0.2);"><code>{{.Name}}</code></td>
                            <td style="padding: 0.5rem; border-bottom: 1px solid rgba(239, 68, 68, 0.2); max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--danger);">
                                {{if .Value}}{{.Value}}{{else}}<span style="color: var(--muted);">(empty)</span>{{end}}
                            </td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{end}}

            {{if .NewFields}}
            <div style="margin-bottom: 1.5rem;">
                <h4 style="color: var(--primary); margin-bottom: 0.75rem;">New fields (will start empty):</h4>
                <ul style="margin: 0; padding-left: 1.5rem;">
                    {{range .NewFields}}
                    <li><code>{{.Name}}</code> ({{.Type}})</li>
                    {{end}}
                </ul>
            </div>
            {{end}}

            <div style="padding: 1rem; background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: var(--radius); margin-bottom: 1.5rem;">
                <p style="margin: 0; color: var(--warning);">
                    <strong>Note:</strong> This action will save a new version of the content with the template change. You can revert to a previous version if needed.
                </p>
            </div>

            <div class="form-actions">
                <a href="/cm/content/{{.Content.ID.Hex}}" class="btn btn-outline">Cancel</a>
                <form method="POST" action="/cm/content/{{.Content.ID.Hex}}/change-template/{{.NewTemplate.ID.Hex}}/confirm" style="display: inline;">
            {{.CSRFField}}
                    <button type="submit" class="btn" style="background: var(--warning); color: white;">Confirm Template Change</button>
                </form>
            </div>
        </div>
    ` + adminLayoutEnd,

	"collections_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Collections</h1>
            {{if index $.Can "settings.edit"}}<a href="/cm/collections/new" class="btn btn-primary">New Collection</a>{{end}}
        </div>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Category Filter</th>
                        <th>Sort</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Collections}}
                    <tr>
                        <td><strong>{{.Name}}</strong><br><small>/{{.Slug}}</small></td>
                        <td>{{.Category}}</td>
                        <td>{{.SortField}} ({{.SortOrder}})</td>
                        <td class="actions">
                            <a href="/{{.Slug}}" target="_blank" class="btn btn-sm btn-outline">View</a>
                            <a href="/cm/collections/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                            <form method="POST" action="/cm/collections/{{.ID.Hex}}/delete" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this collection?')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"collection_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New Collection{{else}}Edit Collection{{end}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-group">
                <label for="name">Collection Name</label>
                <input type="text" id="name" name="name" value="{{if .Collection}}{{.Collection.Name}}{{end}}" required>
            </div>
            <div class="form-group">
                <label for="description">Description</label>
                <textarea id="description" name="description" rows="2">{{if .Collection}}{{.Collection.Description}}{{end}}</textarea>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label for="category">Category Filter</label>
                    <input type="text" id="category" name="category" value="{{if .Collection}}{{.Collection.Category}}{{end}}" placeholder="e.g., blog">
                </div>
                <div class="form-group">
                    <label for="items_per_page">Items Per Page</label>
                    <input type="number" id="items_per_page" name="items_per_page" value="{{if .Collection}}{{.Collection.ItemsPerPage}}{{else}}10{{end}}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label for="sort_field">Sort Field</label>
                    <input type="text" id="sort_field" name="sort_field" value="{{if .Collection}}{{.Collection.SortField}}{{else}}published_at{{end}}">
                </div>
                <div class="form-group">
                    <label for="sort_order">Sort Order</label>
                    <select id="sort_order" name="sort_order">
                        <option value="desc" {{if .Collection}}{{if eq .Collection.SortOrder "desc"}}selected{{end}}{{end}}>Descending (newest first)</option>
                        <option value="asc" {{if .Collection}}{{if eq .Collection.SortOrder "asc"}}selected{{end}}{{end}}>Ascending (oldest first)</option>
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label for="item_template">Item Template</label>
                <p class="help-text">HTML template for each item. Use {{.field_name}} placeholders.</p>
                <textarea id="item_template" name="item_template" rows="10" class="code-editor">{{if .Collection}}{{.Collection.ItemTemplate}}{{end}}</textarea>
            </div>
            <div class="form-group">
                <label for="page_template">Page Template</label>
                <p class="help-text">Overall page template. Use {{.collection_name}}, {{.collection_description}}, {{.items}}, {{.pagination}}</p>
                <textarea id="page_template" name="page_template" rows="10" class="code-editor">{{if .Collection}}{{.Collection.PageTemplate}}{{end}}</textarea>
            </div>

            <div class="form-actions">
                <a href="/cm/collections" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">{{if .IsNew}}Create Collection{{else}}Update Collection{{end}}</button>
            </div>
        </form>
    ` + adminLayoutEnd,

	"theme": adminLayoutStart + `
        <div class="page-header">
            <h1>Theme Settings</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}
        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-section">
                <h3>Site Identity</h3>
                <div class="form-row">
                    <div class="form-group">
                        <label for="site_name">Site Name</label>
                        <input type="text" id="site_name" name="site_name" value="{{.Settings.SiteName}}">
                    </div>
                    <div class="form-group">
                        <label for="site_tagline">Tagline</label>
                        <input type="text" id="site_tagline" name="site_tagline" value="{{.Settings.SiteTagline}}">
                    </div>
                </div>
                <div class="form-group" style="margin-top: 1rem;">
                    <label for="logo_url">Logo URL (optional)</label>
                    <input type="text" id="logo_url" name="logo_url" value="{{.Settings.LogoURL}}" placeholder="/assets/images/logo.png">
                    <small style="color: var(--text-muted);">Upload images in <a href="/cm/assets">Asset Library</a>, then paste the URL here</small>
                </div>
            </div>

            <div class="form-section">
                <h3>Colors</h3>
                <div class="color-grid">
                    <div class="form-group">
                        <label for="primary_color">Primary</label>
                        <input type="color" id="primary_color" name="primary_color" value="{{.Settings.PrimaryColor}}">
                    </div>
                    <div class="form-group">
                        <label for="secondary_color">Secondary</label>
                        <input type="color" id="secondary_color" name="secondary_color" value="{{.Settings.SecondaryColor}}">
                    </div>
                    <div class="form-group">
                        <label for="accent_color">Accent</label>
                        <input type="color" id="accent_color" name="accent_color" value="{{.Settings.AccentColor}}">
                    </div>
                    <div class="form-group">
                        <label for="background_color">Background</label>
                        <input type="color" id="background_color" name="background_color" value="{{.Settings.BackgroundColor}}">
                    </div>
                    <div class="form-group">
                        <label for="text_color">Text</label>
                        <input type="color" id="text_color" name="text_color" value="{{.Settings.TextColor}}">
                    </div>
                </div>
            </div>

            <div class="form-section">
                <h3>Typography</h3>
                <div class="form-row">
                    <div class="form-group">
                        <label for="font_family">Body Font</label>
                        <input type="text" id="font_family" name="font_family" value="{{.Settings.FontFamily}}">
                    </div>
                    <div class="form-group">
                        <label for="heading_font">Heading Font</label>
                        <input type="text" id="heading_font" name="heading_font" value="{{.Settings.HeadingFont}}">
                    </div>
                    <div class="form-group">
                        <label for="border_radius">Border Radius</label>
                        <input type="text" id="border_radius" name="border_radius" value="{{.Settings.BorderRadius}}" placeholder="e.g., 12px">
                    </div>
                </div>
            </div>

            <div class="form-section">
                <h3>Custom CSS</h3>
                <p class="help-text">Injected into the &lt;style&gt; tag in the page &lt;head&gt;, after the theme CSS variables.</p>
                <div class="form-group">
                    <textarea id="custom_css" name="custom_css" rows="10" class="code-editor">{{.Settings.CustomCSS}}</textarea>
                </div>
            </div>

            <div class="form-section">
                <h3>Head HTML</h3>
                <p class="help-text">Injected immediately after the opening &lt;head&gt; tag on every page (for analytics, scripts, meta tags, etc.).</p>
                <div class="form-group">
                    <textarea id="head_html" name="head_html" rows="8" class="code-editor" placeholder="<!-- Google Analytics, meta tags, external scripts, etc. -->">{{.Settings.HeadHTML}}</textarea>
                </div>
            </div>

            <div class="form-section">
                <h3>Site Header</h3>
                <p class="help-text">Injected at the start of the &lt;body&gt; on pages with header enabled (navigation, logo, etc.).</p>
                <div class="form-group">
                    <textarea id="header_html" name="header_html" rows="12" class="code-editor">{{.Settings.HeaderHTML}}</textarea>
                </div>
            </div>

            <div class="form-section">
                <h3>Site Footer</h3>
                <p class="help-text">Injected at the end of the &lt;body&gt; on pages with footer enabled (copyright, links, etc.).</p>
                <div class="form-group">
                    <textarea id="footer_html" name="footer_html" rows="15" class="code-editor">{{.Settings.FooterHTML}}</textarea>
                </div>
            </div>

            <div class="form-actions">
                {{if index $.Can "settings.edit"}}<button type="submit" class="btn btn-primary">Save Theme</button>{{else}}<span style="color: var(--text-muted); font-size: 0.9rem;">Read-only: your role cannot change these settings.</span>{{end}}
            </div>
        </form>

        {{if .Versions}}
        <div class="form-card" style="margin-top: 2rem;">
            <div class="form-section">
                <h3>Version History</h3>
                <p style="color: var(--text-muted); margin-bottom: 1rem;">Previous versions of theme settings are saved automatically when you update.</p>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Version</th>
                                <th>Site Name</th>
                                <th>Comment</th>
                                <th>Saved</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range .Versions}}
                            <tr>
                                <td>v{{.Version}}</td>
                                <td>{{.SiteName}}</td>
                                <td style="color: var(--text-muted); font-size: 0.9rem; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="{{.Comment}}">{{if .Comment}}{{.Comment}}{{else}}-{{end}}</td>
                                <td>{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td>
                                <td class="actions">
                                    <a href="/cm/theme/versions/{{.Version}}" class="btn btn-sm btn-outline">View Diff</a>
                                    <form method="POST" action="/cm/theme/versions/{{.Version}}/revert" style="display:inline" data-confirm="Revert to version {{.Version}}? This will create a new version with the old settings." data-confirm-title="Revert Theme">
                                        {{$.CSRFField}}
                                        <button type="submit" class="btn btn-sm btn-secondary">Revert</button>
                                    </form>
                                </td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
        {{end}}

        <link href="/static/admin/quill/quill.snow.css?v=2.0.3" rel="stylesheet">
        <script src="/static/admin/quill/quill.js?v=2.0.3"></script>
        <style>
            .quill-wrapper { margin-bottom: 1rem; }
            .quill-wrapper .ql-toolbar { background: rgba(15, 23, 42, 0.5); border-color: var(--border); border-radius: var(--radius) var(--radius) 0 0; }
            .quill-wrapper .ql-container { background: rgba(15, 23, 42, 0.5); border-color: var(--border); border-radius: 0 0 var(--radius) var(--radius); min-height: 150px; }
            .quill-wrapper .ql-editor { color: var(--text); min-height: 130px; font-size: 1rem; }
            .quill-wrapper .ql-editor.ql-blank::before { color: var(--text-muted); }
            .ql-toolbar .ql-stroke { stroke: var(--text); }
            .ql-toolbar .ql-fill { fill: var(--text); }
            .ql-toolbar .ql-picker { color: var(--text); }
            .ql-toolbar .ql-picker-options { background: var(--bg-card); border-color: var(--border); }
            .ql-toolbar button:hover, .ql-toolbar button.ql-active { color: var(--primary); }
            .ql-toolbar button:hover .ql-stroke, .ql-toolbar button.ql-active .ql-stroke { stroke: var(--primary); }

            /* Custom link modal styles */
            .link-modal-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10000;
            }
            .link-modal {
                background: var(--bg-card);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1.5rem;
                width: 100%;
                max-width: 500px;
                box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
            }
            .link-modal h3 {
                margin-bottom: 1.5rem;
                font-size: 1.25rem;
                color: var(--text);
            }
            .link-type-tabs {
                display: flex;
                gap: 0.5rem;
                margin-bottom: 1.5rem;
            }
            .link-type-tab {
                flex: 1;
                padding: 0.75rem;
                border: 1px solid var(--border);
                border-radius: var(--radius);
                background: transparent;
                color: var(--text-muted);
                cursor: pointer;
                font-size: 0.9rem;
                transition: all 0.2s;
            }
            .link-type-tab:hover {
                border-color: var(--primary);
                color: var(--text);
            }
            .link-type-tab.active {
                background: linear-gradient(135deg, var(--primary), var(--secondary));
                border-color: transparent;
                color: white;
            }
            .link-input-group {
                margin-bottom: 1rem;
                position: relative;
            }
            .link-input-group label {
                display: block;
                margin-bottom: 0.5rem;
                color: var(--text);
                font-weight: 500;
            }
            .link-input-group input {
                width: 100%;
                padding: 0.75rem 1rem;
                background: rgba(15, 23, 42, 0.5);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                color: var(--text);
                font-size: 1rem;
            }
            .link-input-group input:focus {
                outline: none;
                border-color: var(--primary);
                box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
            }
            .autocomplete-dropdown {
                position: absolute;
                top: 100%;
                left: 0;
                right: 0;
                background: var(--bg-card);
                border: 1px solid var(--border);
                border-top: none;
                border-radius: 0 0 var(--radius) var(--radius);
                max-height: 200px;
                overflow-y: auto;
                z-index: 10001;
                display: none;
            }
            .autocomplete-dropdown.show {
                display: block;
            }
            .autocomplete-item {
                padding: 0.75rem 1rem;
                cursor: pointer;
                border-bottom: 1px solid var(--border);
            }
            .autocomplete-item:last-child {
                border-bottom: none;
            }
            .autocomplete-item:hover,
            .autocomplete-item.selected {
                background: var(--bg-hover);
            }
            .autocomplete-item .slug {
                font-family: 'JetBrains Mono', monospace;
                font-size: 0.85rem;
                color: var(--accent);
            }
            .autocomplete-item .title {
                font-size: 0.8rem;
                color: var(--text-muted);
                margin-top: 0.25rem;
            }
            .link-modal-actions {
                display: flex;
                gap: 1rem;
                justify-content: flex-end;
                margin-top: 1.5rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border);
            }
            .link-hint {
                font-size: 0.8rem;
                color: var(--text-muted);
                margin-top: 0.5rem;
            }
        </style>
        <script>
        // Global slugs cache for autocomplete
        var siteSlugs = [];

        // Load all slugs for autocomplete
        function loadSlugs() {
            fetch('/api/slugs')
                .then(function(res) { return res.json(); })
                .then(function(data) { siteSlugs = data; })
                .catch(function(err) { console.log('Failed to load slugs:', err); });
        }

        // Custom link handler
        function createLinkModal(quill, existingLink) {
            var selection = quill.getSelection();
            var selectedText = selection ? quill.getText(selection.index, selection.length) : '';

            var overlay = document.createElement('div');
            overlay.className = 'link-modal-overlay';

            var modal = document.createElement('div');
            modal.className = 'link-modal';
            modal.innerHTML = ` + "`" + `
                <h3>${existingLink ? 'Edit Link' : 'Insert Link'}</h3>
                <div class="link-type-tabs">
                    <button type="button" class="link-type-tab active" data-type="internal">Internal Page</button>
                    <button type="button" class="link-type-tab" data-type="external">External URL</button>
                </div>
                <div id="internal-link-section">
                    <div class="link-input-group">
                        <label>Select Page</label>
                        <input type="text" id="internal-link-input" placeholder="Start typing to search pages..." autocomplete="off">
                        <div class="autocomplete-dropdown" id="autocomplete-dropdown"></div>
                        <p class="link-hint">Type to search, use Tab to accept suggestion</p>
                    </div>
                </div>
                <div id="external-link-section" style="display: none;">
                    <div class="link-input-group">
                        <label>URL</label>
                        <input type="text" id="external-link-input" placeholder="https://example.com">
                    </div>
                </div>
                <div class="link-modal-actions">
                    <button type="button" class="btn btn-outline" id="link-cancel">Cancel</button>
                    ${existingLink ? '<button type="button" class="btn btn-danger" id="link-remove">Remove Link</button>' : ''}
                    <button type="button" class="btn btn-primary" id="link-save">Save</button>
                </div>
            ` + "`" + `;

            overlay.appendChild(modal);
            document.body.appendChild(overlay);

            var internalInput = modal.querySelector('#internal-link-input');
            var externalInput = modal.querySelector('#external-link-input');
            var dropdown = modal.querySelector('#autocomplete-dropdown');
            var currentLinkType = 'internal';
            var selectedIndex = -1;
            var filteredSlugs = [];

            // Pre-fill with existing link
            if (existingLink) {
                if (existingLink.startsWith('/')) {
                    internalInput.value = existingLink;
                    currentLinkType = 'internal';
                } else {
                    externalInput.value = existingLink;
                    currentLinkType = 'external';
                    modal.querySelector('[data-type="internal"]').classList.remove('active');
                    modal.querySelector('[data-type="external"]').classList.add('active');
                    modal.querySelector('#internal-link-section').style.display = 'none';
                    modal.querySelector('#external-link-section').style.display = 'block';
                }
            }

            // Tab switching
            modal.querySelectorAll('.link-type-tab').forEach(function(tab) {
                tab.addEventListener('click', function() {
                    modal.querySelectorAll('.link-type-tab').forEach(function(t) { t.classList.remove('active'); });
                    tab.classList.add('active');
                    currentLinkType = tab.dataset.type;
                    if (currentLinkType === 'internal') {
                        modal.querySelector('#internal-link-section').style.display = 'block';
                        modal.querySelector('#external-link-section').style.display = 'none';
                        internalInput.focus();
                    } else {
                        modal.querySelector('#internal-link-section').style.display = 'none';
                        modal.querySelector('#external-link-section').style.display = 'block';
                        externalInput.focus();
                    }
                });
            });

            // Autocomplete functionality
            function updateAutocomplete() {
                var query = internalInput.value.toLowerCase();
                filteredSlugs = siteSlugs.filter(function(item) {
                    return item.slug.toLowerCase().includes(query) || item.title.toLowerCase().includes(query);
                }).slice(0, 10);

                if (filteredSlugs.length > 0 && query.length > 0) {
                    dropdown.innerHTML = filteredSlugs.map(function(item, i) {
                        return '<div class="autocomplete-item' + (i === selectedIndex ? ' selected' : '') + '" data-slug="' + item.slug + '">' +
                            '<div class="slug">' + item.slug + '</div>' +
                            '<div class="title">' + item.title + '</div>' +
                        '</div>';
                    }).join('');
                    dropdown.classList.add('show');
                } else {
                    dropdown.classList.remove('show');
                }
            }

            internalInput.addEventListener('input', function() {
                selectedIndex = -1;
                updateAutocomplete();
            });

            internalInput.addEventListener('keydown', function(e) {
                if (e.key === 'Tab' && filteredSlugs.length > 0) {
                    e.preventDefault();
                    var idx = selectedIndex >= 0 ? selectedIndex : 0;
                    internalInput.value = filteredSlugs[idx].slug;
                    dropdown.classList.remove('show');
                } else if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    selectedIndex = Math.min(selectedIndex + 1, filteredSlugs.length - 1);
                    updateAutocomplete();
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    selectedIndex = Math.max(selectedIndex - 1, -1);
                    updateAutocomplete();
                } else if (e.key === 'Enter') {
                    e.preventDefault();
                    if (selectedIndex >= 0 && filteredSlugs[selectedIndex]) {
                        internalInput.value = filteredSlugs[selectedIndex].slug;
                        dropdown.classList.remove('show');
                    } else {
                        modal.querySelector('#link-save').click();
                    }
                } else if (e.key === 'Escape') {
                    overlay.remove();
                }
            });

            dropdown.addEventListener('click', function(e) {
                var item = e.target.closest('.autocomplete-item');
                if (item) {
                    internalInput.value = item.dataset.slug;
                    dropdown.classList.remove('show');
                }
            });

            // Close dropdown when clicking outside
            document.addEventListener('click', function closeDropdown(e) {
                if (!dropdown.contains(e.target) && e.target !== internalInput) {
                    dropdown.classList.remove('show');
                }
            });

            // Save link
            modal.querySelector('#link-save').addEventListener('click', function() {
                var link = currentLinkType === 'internal' ? internalInput.value : externalInput.value;
                if (link) {
                    // Ensure internal links start with /
                    if (currentLinkType === 'internal' && !link.startsWith('/')) {
                        link = '/' + link;
                    }
                    if (selection && selection.length > 0) {
                        quill.formatText(selection.index, selection.length, 'link', link);
                    } else {
                        // Insert link at cursor position
                        var text = selectedText || link;
                        quill.insertText(selection ? selection.index : 0, text, 'link', link);
                    }
                }
                overlay.remove();
            });

            // Cancel
            modal.querySelector('#link-cancel').addEventListener('click', function() {
                overlay.remove();
            });

            // Remove link
            var removeBtn = modal.querySelector('#link-remove');
            if (removeBtn) {
                removeBtn.addEventListener('click', function() {
                    if (selection && selection.length > 0) {
                        quill.formatText(selection.index, selection.length, 'link', false);
                    }
                    overlay.remove();
                });
            }

            // Close on overlay click
            overlay.addEventListener('click', function(e) {
                if (e.target === overlay) {
                    overlay.remove();
                }
            });

            // Focus appropriate input
            setTimeout(function() {
                if (currentLinkType === 'internal') {
                    internalInput.focus();
                } else {
                    externalInput.focus();
                }
            }, 100);
        }

        // Initialize Quill with custom link handler
        function initQuillEditor(textarea) {
            var wrapper = document.createElement('div');
            wrapper.className = 'quill-wrapper';
            var editorDiv = document.createElement('div');
            wrapper.appendChild(editorDiv);
            textarea.parentNode.insertBefore(wrapper, textarea);
            textarea.style.display = 'none';

            var quill = new Quill(editorDiv, {
                theme: 'snow',
                modules: {
                    toolbar: {
                        container: [
                            [{ 'header': [1, 2, 3, false] }],
                            ['bold', 'italic', 'underline', 'strike'],
                            [{ 'color': [] }, { 'background': [] }],
                            [{ 'list': 'ordered'}, { 'list': 'bullet' }],
                            [{ 'indent': '-1'}, { 'indent': '+1' }],
                            ['link', 'image', 'video'],
                            ['blockquote', 'code-block'],
                            ['clean']
                        ],
                        handlers: {
                            'link': function(value) {
                                var selection = quill.getSelection();
                                var existingLink = null;
                                if (selection && selection.length > 0) {
                                    var format = quill.getFormat(selection);
                                    existingLink = format.link || null;
                                }
                                createLinkModal(quill, existingLink);
                            }
                        }
                    }
                }
            });

            quill.root.innerHTML = textarea.value;

            quill.on('text-change', function() {
                textarea.value = quill.root.innerHTML;
            });

            textarea.form.addEventListener('submit', function() {
                textarea.value = quill.root.innerHTML;
            });

            // Store quill instance on wrapper for later access
            wrapper.quillInstance = quill;

            return quill;
        }

        document.addEventListener('DOMContentLoaded', function() {
            // Load slugs for autocomplete
            loadSlugs();

            document.querySelectorAll('.richtext').forEach(function(textarea) {
                initQuillEditor(textarea);
            });
        });
        </script>
    ` + adminLayoutEnd,

	"theme_versions": adminLayoutStart + `
        <div class="page-header">
            <h1>Theme Version History</h1>
            <a href="/cm/theme" class="btn btn-outline">← Back to Theme</a>
        </div>
        <p class="page-subtitle">View and restore previous theme configurations. Each change to the theme settings creates a new version.</p>
        {{if not .Versions}}
        <div class="empty-state">
            <p>No theme versions yet. Make changes to the theme to create version history.</p>
        </div>
        {{else}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Version</th>
                        <th>Site Name</th>
                        <th>Comment</th>
                        <th>Created</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range $i, $v := .Versions}}
                    <tr{{if eq $i 0}} class="current-version"{{end}}>
                        <td>
                            <strong>v{{$v.Version}}</strong>
                            {{if eq $i 0}}<span class="badge badge-success">Current</span>{{end}}
                        </td>
                        <td>{{$v.SiteName}}</td>
                        <td>{{if $v.Comment}}{{$v.Comment}}{{else}}<span class="text-muted">—</span>{{end}}</td>
                        <td>{{$v.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td>
                        <td class="actions">
                            <a href="/cm/theme/versions/{{$v.Version}}" class="btn btn-sm">View Diff</a>
                            {{if ne $i 0}}
                            <form method="POST" action="/cm/theme/versions/{{$v.Version}}/revert" style="display:inline" onsubmit="return confirmRevert(this, {{$v.Version}})">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-primary">Revert</button>
                            </form>
                            {{end}}
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
        <style>
            .empty-state {
                background: var(--bg-card);
                border: 1px dashed var(--border);
                border-radius: var(--radius);
                padding: 3rem;
                text-align: center;
                color: var(--text-muted);
            }
            .current-version {
                background: rgba(99, 102, 241, 0.1);
            }
            .badge {
                display: inline-block;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.7rem;
                font-weight: 600;
                text-transform: uppercase;
                margin-left: 0.5rem;
            }
            .badge-success {
                background: rgba(16, 185, 129, 0.2);
                color: #10b981;
            }
            .text-muted {
                color: var(--text-muted);
            }
        </style>
    ` + adminLayoutEnd,

	"theme_version_diff": adminLayoutStart + `
        <div class="page-header">
            <h1>Theme Version Comparison</h1>
            <a href="/cm/theme/versions" class="btn btn-outline">← Back to Versions</a>
        </div>
        <p style="color: var(--muted); margin-bottom: 2rem;">
            Comparing <strong>Version {{.Version.Version}}</strong> (saved {{.Version.CreatedAt.Format "Jan 2, 2006 3:04 PM"}})
            with <strong>Current Theme</strong>
        </p>

        <div class="diff-container">
            <!-- Site Identity -->
            <div class="diff-section" data-field="site_name">
                <h3>Site Name <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.SiteName}}">{{.Version.SiteName}}</div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.SiteName}}">{{.Current.SiteName}}</div>
                    </div>
                </div>
            </div>

            <div class="diff-section" data-field="site_tagline">
                <h3>Tagline <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.SiteTagline}}">{{.Version.SiteTagline}}</div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.SiteTagline}}">{{.Current.SiteTagline}}</div>
                    </div>
                </div>
            </div>

            <!-- Colors -->
            <div class="diff-section" data-field="colors">
                <h3>Colors <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.PrimaryColor}}|{{.Version.SecondaryColor}}|{{.Version.AccentColor}}|{{.Version.BackgroundColor}}|{{.Version.TextColor}}">
                            <div class="color-row"><span class="color-swatch" style="background: {{.Version.PrimaryColor}}"></span> Primary: {{.Version.PrimaryColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Version.SecondaryColor}}"></span> Secondary: {{.Version.SecondaryColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Version.AccentColor}}"></span> Accent: {{.Version.AccentColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Version.BackgroundColor}}"></span> Background: {{.Version.BackgroundColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Version.TextColor}}"></span> Text: {{.Version.TextColor}}</div>
                        </div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.PrimaryColor}}|{{.Current.SecondaryColor}}|{{.Current.AccentColor}}|{{.Current.BackgroundColor}}|{{.Current.TextColor}}">
                            <div class="color-row"><span class="color-swatch" style="background: {{.Current.PrimaryColor}}"></span> Primary: {{.Current.PrimaryColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Current.SecondaryColor}}"></span> Secondary: {{.Current.SecondaryColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Current.AccentColor}}"></span> Accent: {{.Current.AccentColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Current.BackgroundColor}}"></span> Background: {{.Current.BackgroundColor}}</div>
                            <div class="color-row"><span class="color-swatch" style="background: {{.Current.TextColor}}"></span> Text: {{.Current.TextColor}}</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Typography -->
            <div class="diff-section" data-field="typography">
                <h3>Typography <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content" data-old="{{.Version.FontFamily}}|{{.Version.HeadingFont}}|{{.Version.BorderRadius}}">
                            <p>Body Font: {{.Version.FontFamily}}</p>
                            <p>Heading Font: {{.Version.HeadingFont}}</p>
                            <p>Border Radius: {{.Version.BorderRadius}}</p>
                        </div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content" data-new="{{.Current.FontFamily}}|{{.Current.HeadingFont}}|{{.Current.BorderRadius}}">
                            <p>Body Font: {{.Current.FontFamily}}</p>
                            <p>Heading Font: {{.Current.HeadingFont}}</p>
                            <p>Border Radius: {{.Current.BorderRadius}}</p>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Custom CSS -->
            <div class="diff-section diff-field-section" data-field="custom_css">
                <h3>Custom CSS <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content diff-code" data-old="{{.Version.CustomCSS}}"><pre>{{.Version.CustomCSS}}</pre></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content diff-code" data-new="{{.Current.CustomCSS}}"><pre>{{.Current.CustomCSS}}</pre></div>
                    </div>
                </div>
            </div>

            <!-- Head HTML -->
            <div class="diff-section diff-field-section" data-field="head_html">
                <h3>Head HTML <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content diff-code" data-old="{{.Version.HeadHTML}}"><pre>{{.Version.HeadHTML}}</pre></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content diff-code" data-new="{{.Current.HeadHTML}}"><pre>{{.Current.HeadHTML}}</pre></div>
                    </div>
                </div>
            </div>

            <!-- Header HTML -->
            <div class="diff-section diff-field-section" data-field="header_html">
                <h3>Header HTML <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content diff-code" data-old="{{.Version.HeaderHTML}}"><pre>{{.Version.HeaderHTML}}</pre></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content diff-code" data-new="{{.Current.HeaderHTML}}"><pre>{{.Current.HeaderHTML}}</pre></div>
                    </div>
                </div>
            </div>

            <!-- Footer HTML -->
            <div class="diff-section diff-field-section" data-field="footer_html">
                <h3>Footer HTML <span class="diff-badge"></span></h3>
                <div class="diff-row">
                    <div class="diff-col diff-old">
                        <div class="diff-label">Version {{.Version.Version}}</div>
                        <div class="diff-content diff-code" data-old="{{.Version.FooterHTML}}"><pre>{{.Version.FooterHTML}}</pre></div>
                    </div>
                    <div class="diff-col diff-new">
                        <div class="diff-label">Current</div>
                        <div class="diff-content diff-code" data-new="{{.Current.FooterHTML}}"><pre>{{.Current.FooterHTML}}</pre></div>
                    </div>
                </div>
            </div>
        </div>

        <div class="form-actions" style="margin-top: 2rem;">
            <a href="/cm/theme/versions" class="btn btn-outline">Back to Versions</a>
            <form method="POST" action="/cm/theme/versions/{{.Version.Version}}/revert" style="display:inline" onsubmit="return confirmRevert(this, {{.Version.Version}})">
            {{.CSRFField}}
                <button type="submit" class="btn btn-primary">Revert to Version {{.Version.Version}}</button>
            </form>
        </div>

        <style>
            .diff-container {
                display: flex;
                flex-direction: column;
                gap: 1.5rem;
            }
            .diff-section {
                background: var(--card-bg);
                border: 1px solid var(--border);
                border-radius: var(--radius);
                padding: 1rem;
            }
            .diff-section.has-changes {
                border-color: #f59e0b;
            }
            .diff-section.no-changes {
                opacity: 0.6;
            }
            .diff-section h3 {
                margin: 0 0 1rem 0;
                font-size: 1rem;
                color: var(--accent);
                border-bottom: 1px solid var(--border);
                padding-bottom: 0.5rem;
                display: flex;
                align-items: center;
                gap: 0.75rem;
            }
            .diff-badge {
                font-size: 0.7rem;
                padding: 0.15rem 0.5rem;
                border-radius: 4px;
                text-transform: uppercase;
                font-weight: 600;
            }
            .diff-badge.changed {
                background: rgba(245, 158, 11, 0.2);
                color: #f59e0b;
            }
            .diff-badge.unchanged {
                background: rgba(107, 114, 128, 0.2);
                color: #6b7280;
            }
            .diff-row {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 1rem;
            }
            .diff-col {
                min-width: 0;
            }
            .diff-label {
                font-size: 0.75rem;
                text-transform: uppercase;
                color: var(--muted);
                margin-bottom: 0.5rem;
                font-weight: 600;
            }
            .diff-old .diff-label {
                color: #f59e0b;
            }
            .diff-new .diff-label {
                color: #10b981;
            }
            .diff-content {
                background: rgba(15, 23, 42, 0.5);
                border: 1px solid var(--border);
                border-radius: 4px;
                padding: 0.75rem;
                font-size: 0.9rem;
                overflow-x: auto;
                max-height: 400px;
                overflow-y: auto;
            }
            .diff-old .diff-content {
                border-left: 3px solid #f59e0b;
            }
            .diff-new .diff-content {
                border-left: 3px solid #10b981;
            }
            .diff-code pre {
                white-space: pre-wrap;
                word-break: break-word;
                font-family: 'JetBrains Mono', monospace;
                font-size: 0.8rem;
                margin: 0;
            }
            .color-row {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.25rem;
            }
            .color-swatch {
                display: inline-block;
                width: 20px;
                height: 20px;
                border-radius: 4px;
                border: 1px solid rgba(255,255,255,0.2);
            }
            @media (max-width: 768px) {
                .diff-row {
                    grid-template-columns: 1fr;
                }
            }
        </style>

        <script>
        document.addEventListener('DOMContentLoaded', function() {
            // Process each diff section
            document.querySelectorAll('.diff-section').forEach(function(section) {
                var oldEl = section.querySelector('[data-old]');
                var newEl = section.querySelector('[data-new]');
                var badge = section.querySelector('.diff-badge');

                if (!oldEl || !newEl || !badge) return;

                var oldVal = oldEl.getAttribute('data-old') || '';
                var newVal = newEl.getAttribute('data-new') || '';

                if (oldVal === newVal) {
                    badge.textContent = 'Unchanged';
                    badge.className = 'diff-badge unchanged';
                    section.classList.add('no-changes');
                } else {
                    badge.textContent = 'Changed';
                    badge.className = 'diff-badge changed';
                    section.classList.add('has-changes');
                }
            });
        });
        </script>
    ` + adminLayoutEnd,

	"folders_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Folders</h1>
            {{if index $.Can "settings.edit"}}<a href="/cm/folders/new" class="btn btn-primary">New Folder</a>{{end}}
        </div>
        <p class="page-subtitle">Organize your content into folders to create clean URL structures like /blog/2024/post-name</p>
        {{if not .Folders}}
        <div class="empty-state">
            <p>No folders yet. Create your first folder to start organizing content.</p>
        </div>
        {{else}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Folder</th>
                        <th>Path</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .FolderTree}}
                    <tr>
                        <td style="padding-left: {{if .Depth}}{{multiply .Depth 24}}px{{else}}0{{end}}">
                            <span style="color: var(--accent);">📁</span>
                            <strong>{{.Folder.Name}}</strong>
                        </td>
                        <td><code>{{.Folder.Path}}</code></td>
                        <td class="actions">
                            <a href="/cm/folders/{{.Folder.ID.Hex}}" class="btn btn-sm">Edit</a>
                            <form method="POST" action="/cm/folders/{{.Folder.ID.Hex}}/delete" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this folder?\n\nMake sure it has no content or subfolders.')">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
        <style>
            .empty-state {
                background: var(--bg-card);
                border: 1px dashed var(--border);
                border-radius: var(--radius);
                padding: 3rem;
                text-align: center;
                color: var(--text-muted);
            }
            code {
                font-family: 'JetBrains Mono', monospace;
                background: rgba(99, 102, 241, 0.1);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.85rem;
            }
        </style>
    ` + adminLayoutEnd,

	"folder_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New Folder{{else}}Edit Folder{{end}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-group">
                <label for="name">Folder Name</label>
                <input type="text" id="name" name="name" value="{{if .Folder}}{{.Folder.Name}}{{end}}" required placeholder="e.g., Blog Posts">
            </div>
            <div class="form-group">
                <label for="slug">URL Segment (slug)</label>
                <input type="text" id="slug" name="slug" value="{{if .Folder}}{{.Folder.Slug}}{{end}}" placeholder="auto-generated from name">
                <p class="help-text">This will be part of the URL path. Leave empty to auto-generate.</p>
            </div>
            <div class="form-group">
                <label for="parent_id">Parent Folder</label>
                <select id="parent_id" name="parent_id">
                    <option value="root">/ (root)</option>
                    {{range .Folders}}
                    {{if $.Folder}}
                    {{if ne .ID.Hex $.Folder.ID.Hex}}
                    <option value="{{.ID.Hex}}" {{if $.Folder.ParentID}}{{if eq .ID.Hex $.Folder.ParentID.Hex}}selected{{end}}{{end}}>{{.Path}}</option>
                    {{end}}
                    {{else}}
                    <option value="{{.ID.Hex}}">{{.Path}}</option>
                    {{end}}
                    {{end}}
                </select>
            </div>

            {{if .Folder}}
            <div class="form-section">
                <h3>Current Path</h3>
                <p><code>{{.Folder.Path}}</code></p>
            </div>
            {{end}}

            <div class="form-actions">
                <a href="/cm/folders" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">{{if .IsNew}}Create Folder{{else}}Update Folder{{end}}</button>
            </div>
        </form>
        <style>
            code {
                font-family: 'JetBrains Mono', monospace;
                background: rgba(99, 102, 241, 0.1);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.9rem;
            }
        </style>
    ` + adminLayoutEnd,

	"security": adminLayoutStart + `
        <div class="page-header">
            <h1>Security Settings</h1>
        </div>
        {{if .IsDefaultPassword}}
        <div class="security-alert" style="margin-bottom: 2rem;">
            <div class="alert-icon">⚠️</div>
            <div class="alert-content">
                <strong>Warning:</strong> You are using the default password. Please change it immediately to secure your site.
            </div>
        </div>
        {{end}}
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}
        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-section">
                <h3>Change Password</h3>
                <p class="help-text">Password must be at least 8 characters and contain uppercase, lowercase, and numbers.</p>
                <div class="form-group">
                    <label for="current_password">Current Password</label>
                    <input type="password" id="current_password" name="current_password" required autocomplete="current-password">
                </div>
                <div class="form-group">
                    <label for="new_password">New Password</label>
                    <input type="password" id="new_password" name="new_password" required autocomplete="new-password">
                </div>
                <div class="form-group">
                    <label for="confirm_password">Confirm New Password</label>
                    <input type="password" id="confirm_password" name="confirm_password" required autocomplete="new-password">
                </div>
            </div>

            <div class="form-section">
                <h3>Password Requirements</h3>
                <ul class="password-requirements">
                    <li>At least 8 characters long</li>
                    <li>At least one uppercase letter (A-Z)</li>
                    <li>At least one lowercase letter (a-z)</li>
                    <li>At least one number (0-9)</li>
                </ul>
            </div>

            <div class="form-actions">
                <button type="submit" class="btn btn-primary">Change Password</button>
            </div>
        </form>
    ` + adminLayoutEnd,

	"config": adminLayoutStart + `
        <div class="page-header">
            <h1>Configuration</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}

        <div class="form-card" style="margin-bottom: 1.5rem;">
            <div class="form-section">
                <h3>Version Information</h3>
                <div class="version-info">
                    <div class="version-row">
                        <span class="version-label">Software Version:</span>
                        <span class="version-value">{{.SoftwareVersion}} ({{.EnvLabel}})</span>
                    </div>
                    <div class="version-row">
                        <span class="version-label">Database Version:</span>
                        <span class="version-value">{{if .DatabaseVersion}}{{.DatabaseVersion}}{{else}}<em>Not set</em>{{end}}</span>
                    </div>
                </div>
            </div>
        </div>
        <style>
            .version-info { display: flex; flex-direction: column; gap: 0.5rem; }
            .version-row { display: flex; align-items: center; gap: 1rem; }
            .version-label { font-weight: 500; color: var(--text-muted); min-width: 150px; }
            .version-value { font-family: monospace; background: var(--bg-dark); padding: 0.25rem 0.75rem; border-radius: 4px; }
        </style>

        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-section">
                <h3>Page Title Templates</h3>
                <p class="help-text">Customize how page titles appear in browser tabs and search results.</p>
                <p class="help-text">Available variables: <code>{{"{{"}}title{{"}}"}}</code> (page title), <code>{{"{{"}}site_name{{"}}"}}</code> (from Theme settings: "{{.SiteName}}")</p>

                <div class="form-group">
                    <label for="title_template">Title Template (when page has a title)</label>
                    <input type="text" id="title_template" name="title_template" value="{{.Config.TitleTemplate}}" placeholder="{{"{{"}}title{{"}}"}} - {{"{{"}}site_name{{"}}"}}">
                    <p class="help-text">Example result: "About Us - {{.SiteName}}"</p>
                </div>

                <div class="form-group">
                    <label for="title_template_no_title">Title Template (when page has no title)</label>
                    <input type="text" id="title_template_no_title" name="title_template_no_title" value="{{.Config.TitleTemplateNoTitle}}" placeholder="{{"{{"}}site_name{{"}}"}}">
                    <p class="help-text">Example result: "{{.SiteName}}"</p>
                </div>
            </div>

            <div class="form-section">
                <h3>File Uploads</h3>
                <p class="help-text">Maximum allowed size for uploaded assets. Requests larger than this limit are rejected before reading the body.</p>
                <div class="form-group">
                    <label for="max_upload_bytes">Maximum Upload Size (bytes)</label>
                    <input type="number" id="max_upload_bytes" name="max_upload_bytes" value="{{.Config.MaxUploadBytes}}" min="1024" step="1024">
                    <p class="help-text">Current: {{formatBytes .Config.MaxUploadBytes}}. Default is 1 MiB (1048576 bytes).</p>
                </div>
            </div>

            <div class="form-section">
                <h3>Cloudflare CDN</h3>
                <p class="help-text">Configure Cloudflare to automatically purge cached pages when content is published or unpublished. Leave blank to disable.</p>
                {{if .CFStatus}}
                {{if eq .CFStatus "ok"}}
                <div style="padding: 0.75rem 1rem; margin-bottom: 1rem; border-radius: 6px; background: rgba(52,199,89,0.12); border: 1px solid var(--success); color: var(--success); font-size: 0.9rem;">
                    ✓ Cloudflare connected — cache purging is active for this zone
                </div>
                {{else}}
                <div style="padding: 0.75rem 1rem; margin-bottom: 1rem; border-radius: 6px; background: rgba(255,59,48,0.08); border: 1px solid var(--danger); color: var(--danger); font-size: 0.9rem;">
                    ✗ Cloudflare connection failed: {{.CFError}}
                </div>
                {{end}}
                {{end}}
                <div class="form-group">
                    <label for="cf_cache_enabled">
                        <input type="checkbox" id="cf_cache_enabled" name="cf_cache_enabled" value="true" {{if .Config.CFCacheEnabled}}checked{{end}}>
                        Enable Cloudflare cache purging
                    </label>
                </div>
                <div class="form-group">
                    <label for="cloudflare_zone_id">Zone ID</label>
                    <input type="text" id="cloudflare_zone_id" name="cloudflare_zone_id" value="{{.Config.CloudflareZoneID}}" placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx">
                    <p class="help-text">Found in your Cloudflare dashboard under the domain overview.</p>
                </div>
                <div class="form-group">
                    <label for="cloudflare_api_token">API Token</label>
                    <input type="password" id="cloudflare_api_token" name="cloudflare_api_token" value="{{.Config.CloudflareAPIToken}}" placeholder="API token with Cache Purge permission" autocomplete="new-password">
                    <p class="help-text">Create a token with <strong>Cache Purge</strong> permission for your zone in the Cloudflare dashboard.</p>
                </div>
            </div>

            <div class="form-actions">
                {{if index $.Can "settings.edit"}}<button type="submit" class="btn btn-primary">Save Configuration</button>{{else}}<span style="color: var(--text-muted); font-size: 0.9rem;">Read-only: your role cannot change these settings.</span>{{end}}
            </div>
        </form>
    ` + adminLayoutEnd,

	"redirects_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Redirects</h1>
            {{if index $.Can "settings.edit"}}<a href="/cm/redirects/new" class="btn btn-primary">New Redirect</a>{{end}}
        </div>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>From</th>
                        <th>To</th>
                        <th>Status</th>
                        <th>Description</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Redirects}}
                    <tr>
                        <td><code>{{.FromPath}}</code></td>
                        <td><code>{{.ToPath}}</code></td>
                        <td><span class="status-badge">{{.StatusCode}}</span></td>
                        <td>{{.Description}}</td>
                        <td class="actions">
                            <a href="/cm/redirects/{{.ID.Hex}}" class="btn btn-sm">Edit</a>
                            <form method="POST" action="/cm/redirects/{{.ID.Hex}}/delete" style="display:inline" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this redirect?\n\nNote: Browsers cache 301 redirects. After deletion, users may need to clear their browser cache.')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{else}}
                    <tr>
                        <td colspan="5" class="text-muted" style="text-align:center;padding:2rem;">No redirects configured yet.</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"redirect_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New Redirect{{else}}Edit Redirect{{end}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        <form method="POST" action="{{if .IsNew}}/cm/redirects/new{{else}}/cm/redirects/{{.Redirect.ID.Hex}}{{end}}" class="form-card">
            {{.CSRFField}}
            <div class="form-group">
                <label for="from_path">From Path</label>
                <input type="text" id="from_path" name="from_path" value="{{if .Redirect}}{{.Redirect.FromPath}}{{end}}" placeholder="/old-page" required>
                <p class="help-text">The path to redirect from (e.g., /machine-intelligence)</p>
            </div>
            <div class="form-group">
                <label for="to_path">To Path</label>
                <input type="text" id="to_path" name="to_path" value="{{if .Redirect}}{{.Redirect.ToPath}}{{end}}" placeholder="/new-page" required>
                <p class="help-text">The destination path (e.g., /artificial-intelligence) or full URL</p>
            </div>
            <div class="form-group">
                <label for="status_code">Redirect Type</label>
                <select id="status_code" name="status_code">
                    <option value="301" {{if .Redirect}}{{if eq .Redirect.StatusCode 301}}selected{{end}}{{end}}>301 (Permanent)</option>
                    <option value="302" {{if .Redirect}}{{if eq .Redirect.StatusCode 302}}selected{{end}}{{end}}>302 (Temporary)</option>
                </select>
                <p class="help-text">Use 301 for permanent redirects (SEO-friendly), 302 for temporary</p>
            </div>
            <div class="form-group">
                <label for="description">Description (optional)</label>
                <input type="text" id="description" name="description" value="{{if .Redirect}}{{.Redirect.Description}}{{end}}" placeholder="Why this redirect exists">
            </div>
            <div class="form-actions">
                <a href="/cm/redirects" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">{{if .IsNew}}Create{{else}}Update{{end}}</button>
            </div>
        </form>
    ` + adminLayoutEnd,

	"contact_messages_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Messages {{if .UnreadCount}}<span class="badge">{{.UnreadCount}} unread</span>{{end}}</h1>
            {{if .UnreadCount}}
            <form method="POST" action="/cm/messages/mark-all-read" style="display:inline">
                <input type="hidden" name="gorilla.csrf.Token" value="{{.CSRFToken}}">
                <button type="submit" class="btn btn-secondary">Mark All as Read</button>
            </form>
            {{end}}
        </div>
        <style>
            .badge { background: var(--primary); color: white; padding: 0.25rem 0.5rem; border-radius: 12px; font-size: 0.8rem; margin-left: 0.5rem; }
            .unread { background: rgba(99, 102, 241, 0.1); }
            .message-preview { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); }
        </style>
        <div class="table-card">
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Date</th>
                        <th>Name</th>
                        <th>Email</th>
                        <th>Message</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Messages}}
                    <tr {{if not .Read}}class="unread"{{end}}>
                        <td>{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td>
                        <td>{{if not .Read}}<strong>{{.Name}}</strong>{{else}}{{.Name}}{{end}}</td>
                        <td>{{if and .Email (not .IsSystem)}}<a href="mailto:{{.Email}}">{{.Email}}</a>{{else if .Email}}{{.Email}}{{else}}<em>N/A</em>{{end}}</td>
                        <td class="message-preview">{{.Message}}</td>
                        <td class="actions">
                            <a href="/cm/messages/{{.ID.Hex}}" class="btn btn-sm">View</a>
                            <form method="POST" action="/cm/messages/{{.ID.Hex}}/delete" style="display:inline" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this message?')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{else}}
                    <tr>
                        <td colspan="5" class="text-muted" style="text-align:center;padding:2rem;">No messages yet.</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"contact_message_view": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .Message.Subject}}{{.Message.Subject}}{{else}}Message from {{.Message.Name}}{{end}}</h1>
            <a href="/cm/messages" class="btn btn-outline">Back to Messages</a>
        </div>
        <style>
            .message-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--radius); padding: 2rem; margin-bottom: 1rem; }
            .message-meta { display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem; margin-bottom: 1.5rem; padding-bottom: 1.5rem; border-bottom: 1px solid var(--border); }
            .message-meta-item label { display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.25rem; }
            .message-meta-item span { font-size: 1rem; }
            .message-body { white-space: pre-wrap; line-height: 1.6; }
            .message-body-html { line-height: 1.6; }
            .message-body-html a { color: var(--primary); }
            .message-footer { margin-top: 1.5rem; padding-top: 1.5rem; border-top: 1px solid var(--border); font-size: 0.85rem; color: var(--text-muted); }
            .system-badge { display: inline-block; background: var(--primary); color: white; font-size: 0.7rem; padding: 0.15rem 0.5rem; border-radius: 4px; margin-left: 0.5rem; }
        </style>
        <div class="message-card">
            <div class="message-meta">
                <div class="message-meta-item">
                    <label>From</label>
                    <span>{{.Message.Name}}{{if .Message.IsSystem}}<span class="system-badge">System</span>{{end}}</span>
                </div>
                {{if not .Message.IsSystem}}
                <div class="message-meta-item">
                    <label>Email</label>
                    <span>{{if .Message.Email}}<a href="mailto:{{.Message.Email}}">{{.Message.Email}}</a>{{else}}<em>N/A</em>{{end}}</span>
                </div>
                {{end}}
                <div class="message-meta-item">
                    <label>Received</label>
                    <span>{{.Message.CreatedAt.Format "January 2, 2006 at 3:04 PM"}}</span>
                </div>
                {{if not .Message.IsSystem}}
                <div class="message-meta-item">
                    <label>IP Address</label>
                    <span>{{.Message.IPAddress}}</span>
                </div>
                {{end}}
            </div>
            {{if .Message.IsSystem}}
            <div class="message-body-html">{{safeHTML .Message.Message}}</div>
            {{else}}
            <div class="message-body">{{.Message.Message}}</div>
            {{end}}
            {{if not .Message.IsSystem}}
            <div class="message-footer">
                User Agent: {{.Message.UserAgent}}
            </div>
            {{end}}
        </div>
        <div class="form-actions">
            {{if not .Message.IsSystem}}
            <a href="mailto:{{.Message.Email}}?subject=Re: Your message" class="btn btn-primary">Reply via Email</a>
            {{end}}
            <form method="POST" action="/cm/messages/{{.Message.ID.Hex}}/delete" style="display:inline" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this message?')">
            {{.CSRFField}}
                <button type="submit" class="btn btn-danger">Delete</button>
            </form>
        </div>
    ` + adminLayoutEnd,

	"asset_library": adminLayoutStart + `
        <div class="page-header">
            <h1>Asset Library</h1>
            {{if index $.Can "asset.upload"}}<a href="/cm/assets/upload" class="btn btn-primary">Upload Asset</a>{{end}}
        </div>

        <div class="filter-bar" style="margin-bottom: 1rem;">
            <label style="margin-right: 0.5rem;">Filter by folder:</label>
            <select onchange="window.location.href='/cm/assets?folder=' + this.value" style="padding: 0.5rem; background: var(--bg-card); border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; color: var(--text);">
                <option value="">All folders</option>
                {{range .Folders}}
                <option value="{{.}}" {{if eq . $.CurrentFolder}}selected{{end}}>{{.}}</option>
                {{end}}
            </select>
        </div>

        {{if .Assets}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Preview</th>
                        <th>Filename</th>
                        <th>Serve Path</th>
                        <th>Type</th>
                        <th>Size</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Assets}}
                    <tr>
                        <td style="width: 60px;">
                            {{if or (eq .MimeType "image/png") (eq .MimeType "image/jpeg") (eq .MimeType "image/gif") (eq .MimeType "image/webp") (eq .MimeType "image/svg+xml")}}
                            <img src="{{.ServePath}}" alt="{{.Filename}}" style="max-width: 50px; max-height: 50px; border-radius: 4px;">
                            {{else}}
                            <span style="font-size: 1.5rem;">📄</span>
                            {{end}}
                        </td>
                        <td>{{.Filename}}</td>
                        <td><code>{{.ServePath}}</code></td>
                        <td>{{.MimeType}}</td>
                        <td>{{if lt .Size 1024}}{{.Size}} B{{else if lt .Size 1048576}}{{divide .Size 1024}} KB{{else}}{{divide .Size 1048576}} MB{{end}}</td>
                        <td>
                            <a href="{{.ServePath}}" target="_blank" class="btn btn-sm">View</a>
                            <button onclick="copyToClipboard('{{.ServePath}}')" class="btn btn-sm btn-secondary">Copy URL</button>
                            <form method="POST" action="/cm/assets/{{.ID.Hex}}/delete" style="display:inline" onsubmit="return confirmDelete(this, 'Are you sure you want to delete this asset?')">
            {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="empty-state">
            <p>No assets yet. <a href="/cm/assets/upload">Upload your first asset</a></p>
        </div>
        {{end}}

        <script>
        function copyToClipboard(text) {
            navigator.clipboard.writeText(window.location.origin + text).then(function() {
                showAlert('URL copied to clipboard!', 'Copied');
            });
        }
        </script>
    ` + adminLayoutEnd,

	"broken_links": adminLayoutStart + `
        <div class="page-header">
            <h1>🔗 Broken Link Finder</h1>
        </div>

        <div class="info-card" style="margin-bottom: 1.5rem;">
            <p style="margin: 0; color: var(--text-muted);">
                This tool scans all published content for broken links, including both internal pages and external URLs.
                Links with redirects are considered valid.
            </p>
        </div>

        <!-- Progress Section -->
        <div id="scan-progress" style="margin-bottom: 1.5rem;">
            <div style="background: var(--bg-card); padding: 1.5rem; border-radius: 8px;">
                <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem;">
                    <div class="spinner" style="width: 24px; height: 24px; border: 3px solid var(--bg-dark); border-top-color: var(--primary); border-radius: 50%; animation: spin 1s linear infinite;"></div>
                    <div style="flex: 1; min-width: 0;">
                        <div id="progress-text" style="font-weight: 500;">Starting scan...</div>
                        <div id="progress-path" style="font-size: 0.875rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"></div>
                        <div id="progress-checking" style="font-size: 0.75rem; color: var(--primary); margin-top: 0.25rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"></div>
                    </div>
                </div>
                <div style="background: var(--bg-dark); height: 8px; border-radius: 4px; overflow: hidden;">
                    <div id="progress-bar" style="background: var(--primary); height: 100%; width: 0%; transition: width 0.2s;"></div>
                </div>
                <div id="progress-links" style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.5rem;">Links checked: 0</div>
            </div>
        </div>

        <!-- Fix Link Modal -->
        <div id="fix-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); z-index: 1000; align-items: center; justify-content: center;">
            <div style="background: var(--bg-card); border-radius: 12px; padding: 1.5rem; max-width: 600px; width: 90%; max-height: 80vh; overflow-y: auto;">
                <h3 style="margin: 0 0 1rem 0;">Fix Broken Link</h3>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.5rem; color: var(--text-muted); font-size: 0.875rem;">Page</label>
                    <div id="fix-page-title" style="font-weight: 500;"></div>
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.5rem; color: var(--text-muted); font-size: 0.875rem;">Field</label>
                    <div id="fix-field-name" style="font-weight: 500;"></div>
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.5rem; color: var(--text-muted); font-size: 0.875rem;">Current Link (broken)</label>
                    <code id="fix-old-url" style="display: block; background: rgba(255,107,107,0.2); color: var(--danger); padding: 0.5rem; border-radius: 4px; word-break: break-all;"></code>
                </div>
                <div style="margin-bottom: 1.5rem;">
                    <label for="fix-new-url" style="display: block; margin-bottom: 0.5rem; color: var(--text-muted); font-size: 0.875rem;">New Link</label>
                    <input type="text" id="fix-new-url" style="width: 100%; padding: 0.75rem; background: var(--bg-dark); border: 1px solid rgba(255,255,255,0.1); border-radius: 8px; color: var(--text); font-size: 1rem;" placeholder="Enter the corrected URL">
                    <small style="color: var(--text-muted); display: block; margin-top: 0.25rem;">Leave empty to remove the link entirely</small>
                </div>
                <div id="fix-status" style="display: none; margin-bottom: 1rem; padding: 0.75rem; border-radius: 6px;"></div>
                <div style="display: flex; gap: 0.5rem; justify-content: flex-end;">
                    <button id="fix-cancel" class="btn btn-secondary">Cancel</button>
                    <button id="fix-save" class="btn btn-primary">Save Fix</button>
                </div>
            </div>
        </div>

        <style>
            @keyframes spin {
                to { transform: rotate(360deg); }
            }
        </style>

        <div class="stats-row" style="display: flex; gap: 1rem; margin-bottom: 1.5rem;">
            <div class="stat-card" style="background: var(--bg-card); padding: 1rem 1.5rem; border-radius: 8px; flex: 1;">
                <div id="stat-scanned" style="font-size: 2rem; font-weight: bold; color: var(--primary);">0</div>
                <div style="color: var(--text-muted); font-size: 0.875rem;">Pages Scanned</div>
            </div>
            <div class="stat-card" style="background: var(--bg-card); padding: 1rem 1.5rem; border-radius: 8px; flex: 1;">
                <div id="stat-links" style="font-size: 2rem; font-weight: bold; color: var(--primary);">0</div>
                <div style="color: var(--text-muted); font-size: 0.875rem;">Links Checked</div>
            </div>
            <div class="stat-card" style="background: var(--bg-card); padding: 1rem 1.5rem; border-radius: 8px; flex: 1;">
                <div id="stat-pages-broken" style="font-size: 2rem; font-weight: bold; color: var(--success);">0</div>
                <div style="color: var(--text-muted); font-size: 0.875rem;">Pages with Issues</div>
            </div>
            <div class="stat-card" style="background: var(--bg-card); padding: 1rem 1.5rem; border-radius: 8px; flex: 1;">
                <div id="stat-total-broken" style="font-size: 2rem; font-weight: bold; color: var(--success);">0</div>
                <div style="color: var(--text-muted); font-size: 0.875rem;">Broken Links</div>
            </div>
        </div>

        <div id="results-container">
            <div class="table-container" id="results-table" style="display: none;">
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Page</th>
                            <th>Broken Links</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="results-body">
                    </tbody>
                </table>
            </div>
        </div>

        <div id="no-results" class="empty-state" style="display: none; background: rgba(72, 187, 120, 0.1); border: 1px solid rgba(72, 187, 120, 0.3);">
            <p style="color: var(--success); font-size: 1.25rem; margin: 0;">✓ No broken links found!</p>
            <p style="color: var(--text-muted); margin-top: 0.5rem;">All links in your published content are valid.</p>
        </div>

        <script>
        (function() {
            var pagesWithBrokenLinks = 0;
            var totalBrokenLinks = 0;
            var currentFix = null;

            function escapeHtml(text) {
                var div = document.createElement('div');
                div.textContent = text;
                return div.innerHTML;
            }

            function formatLinkError(link) {
                var details = '';
                if (link.status) {
                    details = ' <span style="background: rgba(255,107,107,0.3); padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.7rem;">HTTP ' + link.status + '</span>';
                } else if (link.error) {
                    var shortError = link.error;
                    if (shortError.length > 40) shortError = shortError.substring(0, 40) + '...';
                    details = ' <span style="background: rgba(255,107,107,0.3); padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.7rem;" title="' + escapeHtml(link.error) + '">' + escapeHtml(shortError) + '</span>';
                }
                return details;
            }

            function openFixModal(contentId, title, field, oldUrl, linkElement) {
                currentFix = { contentId: contentId, field: field, oldUrl: oldUrl, linkElement: linkElement };
                document.getElementById('fix-page-title').textContent = title;
                document.getElementById('fix-field-name').textContent = field;
                document.getElementById('fix-old-url').textContent = oldUrl;
                document.getElementById('fix-new-url').value = oldUrl;
                document.getElementById('fix-status').style.display = 'none';
                document.getElementById('fix-modal').style.display = 'flex';
                document.getElementById('fix-new-url').focus();
                document.getElementById('fix-new-url').select();
            }

            function closeFixModal() {
                document.getElementById('fix-modal').style.display = 'none';
                currentFix = null;
            }

            function showFixStatus(message, isError) {
                var status = document.getElementById('fix-status');
                status.textContent = message;
                status.style.display = 'block';
                status.style.background = isError ? 'rgba(255,107,107,0.2)' : 'rgba(72,187,120,0.2)';
                status.style.color = isError ? 'var(--danger)' : 'var(--success)';
            }

            function saveFix() {
                if (!currentFix) return;

                var newUrl = document.getElementById('fix-new-url').value.trim();
                var saveBtn = document.getElementById('fix-save');
                saveBtn.disabled = true;
                saveBtn.textContent = 'Saving...';

                fetch('/cm/tools/broken-links/fix', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': '{{.CSRFToken}}' },
                    body: JSON.stringify({
                        contentId: currentFix.contentId,
                        field: currentFix.field,
                        oldUrl: currentFix.oldUrl,
                        newUrl: newUrl
                    })
                })
                .then(function(resp) {
                    return resp.json().then(function(data) {
                        if (!resp.ok) throw new Error(data.error || 'Failed to save');
                        return data;
                    });
                })
                .then(function(data) {
                    showFixStatus('Link fixed successfully! Version ' + data.version + ' created.', false);
                    // Update the UI to show the link is fixed
                    if (currentFix.linkElement) {
                        currentFix.linkElement.style.background = 'rgba(72,187,120,0.2)';
                        currentFix.linkElement.style.color = 'var(--success)';
                        currentFix.linkElement.textContent = newUrl || '(removed)';
                        // Remove the fix button
                        var fixBtn = currentFix.linkElement.parentElement.querySelector('.fix-btn');
                        if (fixBtn) fixBtn.remove();
                    }
                    setTimeout(closeFixModal, 1500);
                })
                .catch(function(err) {
                    showFixStatus('Error: ' + err.message, true);
                })
                .finally(function() {
                    saveBtn.disabled = false;
                    saveBtn.textContent = 'Save Fix';
                });
            }

            // Set up modal event listeners
            document.getElementById('fix-cancel').addEventListener('click', closeFixModal);
            document.getElementById('fix-save').addEventListener('click', saveFix);
            document.getElementById('fix-modal').addEventListener('click', function(e) {
                if (e.target === this) closeFixModal();
            });
            document.getElementById('fix-new-url').addEventListener('keydown', function(e) {
                if (e.key === 'Enter') saveFix();
                if (e.key === 'Escape') closeFixModal();
            });

            function addResult(result) {
                var table = document.getElementById('results-table');
                var tbody = document.getElementById('results-body');
                table.style.display = 'block';

                pagesWithBrokenLinks++;
                totalBrokenLinks += result.brokenLinks.length;

                // Update stats with danger color
                var statPagesBroken = document.getElementById('stat-pages-broken');
                var statTotalBroken = document.getElementById('stat-total-broken');
                statPagesBroken.textContent = pagesWithBrokenLinks;
                statPagesBroken.style.color = 'var(--danger)';
                statTotalBroken.textContent = totalBrokenLinks;
                statTotalBroken.style.color = 'var(--danger)';

                var linksHtml = result.brokenLinks.map(function(link, idx) {
                    var isExternal = link.url.startsWith('http') || link.url.startsWith('//');
                    var icon = isExternal ? '🌐' : '📄';
                    var linkId = 'link-' + result.id + '-' + idx;
                    return '<div style="margin-bottom: 0.5rem;">' +
                        '<span style="margin-right: 0.25rem;">' + icon + '</span>' +
                        '<code id="' + linkId + '" style="background: rgba(255,107,107,0.2); color: var(--danger); padding: 0.25rem 0.5rem; border-radius: 4px; word-break: break-all;">' +
                        escapeHtml(link.url) + '</code>' +
                        formatLinkError(link) +
                        '<span style="color: var(--text-muted); font-size: 0.75rem; margin-left: 0.5rem;">in: ' + escapeHtml(link.field) + '</span>' +
                        '</div>';
                }).join('');

                // Build fix buttons for each broken link
                var fixButtonsHtml = result.brokenLinks.map(function(link, idx) {
                    var linkId = 'link-' + result.id + '-' + idx;
                    return '<button class="btn btn-small fix-btn" data-content-id="' + escapeHtml(result.id) + '" data-title="' + escapeHtml(result.title) + '" data-field="' + escapeHtml(link.field) + '" data-url="' + escapeHtml(link.url) + '" data-link-id="' + linkId + '" style="margin-bottom: 0.25rem;">Fix</button>';
                }).join(' ');

                var row = document.createElement('tr');
                row.innerHTML = '<td><strong>' + escapeHtml(result.title) + '</strong>' +
                    '<div style="font-size: 0.875rem; color: var(--text-muted);">' + escapeHtml(result.path) + '</div></td>' +
                    '<td>' + linksHtml + '</td>' +
                    '<td style="white-space: nowrap;">' +
                    '<a href="' + escapeHtml(result.path) + '" target="_blank" class="btn btn-small">View</a> ' +
                    '<a href="/cm/content/' + escapeHtml(result.id) + '" class="btn btn-small btn-secondary">Edit</a> ' +
                    fixButtonsHtml + '</td>';

                // Add click handlers for fix buttons
                row.querySelectorAll('.fix-btn').forEach(function(btn) {
                    btn.addEventListener('click', function() {
                        var linkElement = document.getElementById(this.dataset.linkId);
                        openFixModal(this.dataset.contentId, this.dataset.title, this.dataset.field, this.dataset.url, linkElement);
                    });
                });

                tbody.appendChild(row);
            }

            function startScan() {
                var eventSource = new EventSource('/api/tools/broken-links/scan');

                eventSource.addEventListener('total', function(e) {
                    var data = JSON.parse(e.data);
                    document.getElementById('progress-text').textContent = 'Scanning ' + data.total + ' pages...';
                });

                eventSource.addEventListener('progress', function(e) {
                    var data = JSON.parse(e.data);
                    var percent = Math.round((data.current / data.total) * 100);
                    document.getElementById('progress-bar').style.width = percent + '%';
                    document.getElementById('progress-text').textContent = 'Scanning page ' + data.current + ' of ' + data.total + '...';
                    document.getElementById('progress-path').textContent = data.title + ' (' + data.path + ')';
                    document.getElementById('stat-scanned').textContent = data.current;

                    if (data.linksChecked) {
                        document.getElementById('progress-links').textContent = 'Links checked: ' + data.linksChecked;
                        document.getElementById('stat-links').textContent = data.linksChecked;
                    }

                    if (data.checking) {
                        document.getElementById('progress-checking').textContent = 'Checking: ' + data.checking;
                    } else {
                        document.getElementById('progress-checking').textContent = '';
                    }
                });

                eventSource.addEventListener('result', function(e) {
                    var result = JSON.parse(e.data);
                    addResult(result);
                });

                eventSource.addEventListener('complete', function(e) {
                    var data = JSON.parse(e.data);
                    eventSource.close();

                    // Hide progress section
                    document.getElementById('scan-progress').style.display = 'none';

                    // Update final stats
                    document.getElementById('stat-scanned').textContent = data.totalPages;
                    document.getElementById('stat-links').textContent = data.totalLinksChecked || 0;

                    // Show "no results" message if no broken links
                    if (data.pagesWithBrokenLinks === 0) {
                        document.getElementById('no-results').style.display = 'block';
                    }
                });

                eventSource.addEventListener('error', function(e) {
                    try {
                        var data = JSON.parse(e.data);
                        document.getElementById('progress-text').textContent = 'Error: ' + data;
                    } catch(err) {
                        document.getElementById('progress-text').textContent = 'Connection error';
                    }
                    eventSource.close();
                });

                eventSource.onerror = function() {
                    eventSource.close();
                    document.getElementById('scan-progress').style.display = 'none';
                };
            }

            startScan();
        })();
        </script>
    ` + adminLayoutEnd,

	"asset_upload": adminLayoutStart + `
        <div class="page-header">
            <h1>Upload Asset</h1>
            <a href="/cm/assets" class="btn btn-secondary">Back to Library</a>
        </div>

        <form method="POST" action="/cm/assets/upload" enctype="multipart/form-data" class="form-card">
            {{.CSRFField}}
            <div class="form-group">
                <label for="file">File</label>
                <input type="file" id="file" name="file" required style="padding: 0.75rem; background: var(--bg-dark); border: 1px solid rgba(255,255,255,0.1); border-radius: 8px; color: var(--text);">
            </div>

            <div class="form-group">
                <label for="serve_path">Serve Path (URL where the file will be accessible)</label>
                <input type="text" id="serve_path" name="serve_path" placeholder="e.g., /favicon.png or /images/logo.png" required>
                <small style="color: var(--text-muted);">The full URL path where this file will be served. Use / for root level files like favicons.</small>
            </div>

            <div class="form-group">
                <label for="description">Description (optional)</label>
                <input type="text" id="description" name="description" placeholder="Brief description of the asset">
            </div>

            <div class="form-actions">
                <button type="submit" class="btn btn-primary">Upload Asset</button>
            </div>
        </form>

        <script>
        // Auto-suggest serve path from filename
        document.getElementById('file').addEventListener('change', function(e) {
            var servePathInput = document.getElementById('serve_path');
            if (servePathInput.value === '' && e.target.files.length > 0) {
                servePathInput.value = '/' + e.target.files[0].name;
            }
        });
        </script>
    ` + adminLayoutEnd,

	"api_keys": adminLayoutStart + `
        <div class="page-header">
            <h1>API Keys</h1>
            <a href="/cm/api-keys/new" class="btn btn-primary">+ New API Key</a>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}

        <p class="help-text" style="margin-bottom: 1.5rem;">API keys provide programmatic access to the LightCMS REST API. Use them with the CLI tool, MCP server, or custom integrations. A key acts as its owner, limited by its access: <strong>Sandbox</strong> keys change things only in forks they created, which a person then reviews and merges; <strong>Full</strong> keys can do anything their owner's role can. Only an admin can let a key merge its own forks.</p>

        {{if .APIKeys}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        {{if .SeeAll}}<th>Owner</th>{{end}}
                        <th>Key Prefix</th>
                        <th>Access</th>
                        <th>Created</th>
                        <th>Last Used</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody>
                    {{range .APIKeys}}
                    <tr>
                        <td><strong>{{.Key.Name}}</strong>{{if .Key.Description}}<div class="help-text">{{.Key.Description}}</div>{{end}}</td>
                        {{if $.SeeAll}}<td>{{.OwnerEmail}}</td>{{end}}
                        <td><code>{{.Key.Prefix}}...</code></td>
                        <td>
                            <form method="POST" action="/cm/api-keys/{{.Key.ID.Hex}}/access" class="access-form">
                                {{$.CSRFField}}
                                <select name="access" aria-label="Access for {{.Key.Name}}">
                                    <option value="sandbox"{{if .Key.SandboxOnly}} selected{{end}}>Sandbox</option>
                                    <option value="full"{{if not .Key.SandboxOnly}} selected{{end}}>Full</option>
                                </select>
                                {{if .MergeEditable}}
                                <input type="hidden" name="merge_shown" value="1">
                                <label class="inline-check"><input type="checkbox" name="can_merge_own_forks" value="1"{{if .Key.CanMergeOwnForks}} checked{{end}}> Can merge its own forks</label>
                                {{else if .Key.CanMergeOwnForks}}
                                <span class="help-text">Can merge its own forks</span>
                                {{end}}
                                <button type="submit" class="btn btn-outline btn-sm">Save</button>
                            </form>
                        </td>
                        <td>{{.Key.CreatedAt.Format "Jan 2, 2006"}}</td>
                        <td>{{if .Key.LastUsedAt}}{{.Key.LastUsedAt.Format "Jan 2, 2006 3:04 PM"}}{{else}}<em>Never</em>{{end}}</td>
                        <td>
                            <form method="POST" action="/cm/api-keys/{{.Key.ID.Hex}}/delete" style="display:inline;" data-confirm="Are you sure you want to delete the API key '{{.Key.Name}}'? Any integrations using this key will stop working." data-confirm-title="Delete API Key">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-danger btn-sm delete-btn">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        <style>
            .access-form { display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; }
            .access-form select { width: auto; }
            .inline-check { display: inline-flex; gap: 0.35rem; align-items: center; font-size: 0.85rem; margin: 0; }
        </style>
        {{else}}
        <div class="empty-state">
            <p>No API keys yet. Create one to enable programmatic access.</p>
        </div>
        {{end}}
    ` + adminLayoutEnd,

	"connected_apps": adminLayoutStart + `
        <div class="page-header">
            <h1>Connected Apps</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}

        <p class="help-text" style="margin-bottom: 1.5rem;">Apps you connected by signing in (for example an AI assistant using this site's MCP server). Each one acts as the person who connected it, limited by its access: <strong>Sandbox</strong> apps change things only in forks they created, which a person then reviews and merges; <strong>Full</strong> apps can do anything that person's role can. Only an admin can let an app merge its own forks. Disconnecting stops an app at once.</p>

        {{if .Connections}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>App</th>
                        {{if .SeeAll}}<th>Connected by</th>{{end}}
                        <th>Access</th>
                        <th>Connected</th>
                        <th>Last Used</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Connections}}
                    <tr>
                        <td><strong>{{.Grant.ClientName}}</strong></td>
                        {{if $.SeeAll}}<td>{{.OwnerEmail}}</td>{{end}}
                        <td>
                            <form method="POST" action="/cm/connected-apps/{{.Grant.ID.Hex}}/access" class="access-form">
                                {{$.CSRFField}}
                                <select name="access" aria-label="Access for {{.Grant.ClientName}}">
                                    <option value="sandbox"{{if .Grant.SandboxOnly}} selected{{end}}>Sandbox</option>
                                    <option value="full"{{if not .Grant.SandboxOnly}} selected{{end}}>Full</option>
                                </select>
                                {{if .MergeEditable}}
                                <input type="hidden" name="merge_shown" value="1">
                                <label class="inline-check"><input type="checkbox" name="can_merge_own_forks" value="1"{{if .Grant.CanMergeOwnForks}} checked{{end}}> Can merge its own forks</label>
                                {{else if .Grant.CanMergeOwnForks}}
                                <span class="help-text">Can merge its own forks</span>
                                {{end}}
                                <button type="submit" class="btn btn-outline btn-sm">Save</button>
                            </form>
                        </td>
                        <td>{{.Grant.CreatedAt.Format "Jan 2, 2006"}}</td>
                        <td>{{if .Grant.LastUsedAt}}{{.Grant.LastUsedAt.Format "Jan 2, 2006 3:04 PM"}}{{else}}<em>Never</em>{{end}}</td>
                        <td>
                            <form method="POST" action="/cm/connected-apps/{{.Grant.ID.Hex}}/revoke" style="display:inline;" data-confirm="Disconnect '{{.Grant.ClientName}}'? It stops working at once and has to be connected again to come back." data-confirm-title="Disconnect App">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-danger btn-sm delete-btn">Disconnect</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        <style>
            .access-form { display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; }
            .access-form select { width: auto; }
            .inline-check { display: inline-flex; gap: 0.35rem; align-items: center; font-size: 0.85rem; margin: 0; }
        </style>
        {{else}}
        <div class="empty-state">
            <p>No apps are connected.</p>
        </div>
        {{end}}
    ` + adminLayoutEnd,

	"api_key_new": adminLayoutStart + `
        <div class="page-header">
            <h1>New API Key</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}

        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-section">
                <div class="form-group">
                    <label for="name">Name <span class="required">*</span></label>
                    <input type="text" id="name" name="name" required placeholder="e.g., CLI Access, MCP Server, CI/CD">
                </div>
                <div class="form-group">
                    <label for="description">Description</label>
                    <input type="text" id="description" name="description" placeholder="What this key is used for">
                </div>
                <div class="form-group">
                    <label>Access</label>
                    <label class="inline-choice"><input type="radio" name="access" value="sandbox" checked> <span><strong>Sandbox</strong> (recommended for agents). It can read the site, and it can change things only in forks it creates. Nothing goes live until a person merges them.</span></label>
                    <label class="inline-choice"><input type="radio" name="access" value="full"> <span><strong>Full</strong>. It can do anything your role can, directly on the live site.</span></label>
                    {{if index $.Can "fork.merge"}}
                    <label class="inline-choice"><input type="checkbox" name="can_merge_own_forks" value="1"> <span><strong>Let it merge its own forks.</strong> Its changes then go live without anyone reviewing them first.</span></label>
                    {{end}}
                </div>
            </div>
            <div class="form-actions">
                <a href="/cm/api-keys" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">Create API Key</button>
            </div>
        </form>
        <style>
            .inline-choice { display: flex; gap: 0.5rem; align-items: flex-start; font-weight: 400; margin: 0.4rem 0; }
            .inline-choice input { width: auto; margin-top: 0.3rem; }
        </style>
    ` + adminLayoutEnd,

	"api_key_created": adminLayoutStart + `
        <div class="page-header">
            <h1>API Key Created</h1>
        </div>

        <div class="form-card">
            <div class="form-section">
                <div style="background: rgba(16, 185, 129, 0.1); border: 1px solid rgba(16, 185, 129, 0.3); border-radius: var(--radius); padding: 1.5rem; margin-bottom: 1.5rem;">
                    <p style="margin: 0 0 0.5rem; color: #10b981; font-weight: 600;">Your API key has been created</p>
                    <p style="margin: 0 0 1rem; color: var(--text-muted);">Copy this key now — it will not be shown again.</p>
                    <div style="display: flex; align-items: center; gap: 0.75rem;">
                        <code id="api-key-value" style="flex: 1; background: var(--bg-primary); padding: 0.75rem 1rem; border-radius: 6px; font-size: 1rem; word-break: break-all; border: 1px solid var(--border);">{{.RawKey}}</code>
                        <button type="button" class="btn btn-primary" onclick="copyKey()" id="copy-btn">Copy</button>
                    </div>
                </div>

                <table style="width: 100%;">
                    <tr>
                        <td style="padding: 0.5rem 0; color: var(--text-muted); width: 120px;">Name</td>
                        <td style="padding: 0.5rem 0;"><strong>{{.APIKey.Name}}</strong></td>
                    </tr>
                    {{if .APIKey.Description}}
                    <tr>
                        <td style="padding: 0.5rem 0; color: var(--text-muted);">Description</td>
                        <td style="padding: 0.5rem 0;">{{.APIKey.Description}}</td>
                    </tr>
                    {{end}}
                    <tr>
                        <td style="padding: 0.5rem 0; color: var(--text-muted);">Prefix</td>
                        <td style="padding: 0.5rem 0;"><code>{{.APIKey.Prefix}}...</code></td>
                    </tr>
                </table>
            </div>

            <div class="form-actions">
                <a href="/cm/api-keys" class="btn btn-primary">Done</a>
            </div>
        </div>

        <script>
        function copyKey() {
            var key = document.getElementById('api-key-value').textContent;
            navigator.clipboard.writeText(key).then(function() {
                var btn = document.getElementById('copy-btn');
                btn.textContent = 'Copied!';
                btn.style.background = '#10b981';
                setTimeout(function() {
                    btn.textContent = 'Copy';
                    btn.style.background = '';
                }, 2000);
            });
        }
        </script>
    ` + adminLayoutEnd,

	"search_tool": adminLayoutStart + `
        <div class="page-header">
            <h1>🔍 End User Search</h1>
        </div>

        {{if not .SearchEnabled}}
        <div class="info-card" style="margin-bottom: 1.5rem; border-color: rgba(245, 158, 11, 0.3);">
            <p style="margin: 0; color: var(--warning);">
                <strong>Search not configured.</strong> Set the <code>VOYAGE_API_KEY</code> environment variable (or <code>voyage_api_key</code> in config JSON) to enable semantic search.
                Get an API key at <a href="https://dash.voyageai.com/" target="_blank" style="color: var(--primary);">dash.voyageai.com</a>.
            </p>
        </div>
        {{end}}

        <!-- Embedding Status -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1.5rem;">
            <div class="info-card">
                <div style="font-size: 0.875rem; color: var(--text-muted);">Published Pages</div>
                <div style="font-size: 1.5rem; font-weight: 600; color: var(--text);">{{.TotalContent}}</div>
            </div>
            <div class="info-card">
                <div style="font-size: 0.875rem; color: var(--text-muted);">With Embeddings</div>
                <div style="font-size: 1.5rem; font-weight: 600; color: var(--success);">{{.WithEmbedding}}</div>
            </div>
            <div class="info-card">
                <div style="font-size: 0.875rem; color: var(--text-muted);">Needs Indexing</div>
                <div style="font-size: 1.5rem; font-weight: 600; color: var(--warning);" id="needs-indexing">{{if .TotalContent}}{{subtract .TotalContent .WithEmbedding}}{{else}}0{{end}}</div>
            </div>
        </div>

        {{if .SearchEnabled}}
        <div style="margin-bottom: 1.5rem;">
            <form method="POST" action="/cm/tools/search/reindex" id="reindex-form" style="display: inline;">
                {{.CSRFField}}
                <button type="button" class="btn" id="reindex-btn" style="background: var(--primary); color: white;">
                    Reindex All Content
                </button>
                <span id="reindex-status" style="margin-left: 1rem; color: var(--text-muted);"></span>
            </form>
        </div>
        {{end}}

        <!-- Search Ranking Configuration -->
        {{if .SearchRankingConfig}}
        <div class="info-card" style="margin-bottom: 1.5rem;">
            <h2 style="margin-top: 0; font-size: 1.25rem;">Search Ranking</h2>
            <p style="color: var(--text-muted); margin-bottom: 1.25rem; font-size: 0.875rem;">
                Controls how results are prioritised. Nav-linked pages are detected automatically from your header HTML.
                Leave a field blank or at zero to disable that factor.
            </p>
            {{if .SavedConfig}}<div style="margin-bottom: 1rem; padding: 0.625rem 1rem; background: rgba(34,197,94,0.1); border: 1px solid rgba(34,197,94,0.3); border-radius: 8px; color: var(--success); font-size: 0.875rem;">Settings saved.</div>{{end}}
            <form method="POST" action="/cm/tools/search/config">
                {{.CSRFField}}
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.25rem; margin-bottom: 1.25rem;">
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Title Match Boost</label>
                        <input type="number" name="title_boost" step="0.01" min="0" max="1"
                            value="{{.SearchRankingConfig.TitleBoost}}"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Added when query appears in page title (default 0.20)</div>
                    </div>
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Nav Page Boost</label>
                        <input type="number" name="nav_boost" step="0.01" min="-1" max="1"
                            value="{{.SearchRankingConfig.NavBoost}}"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Boost for pages linked from site navigation (default 0.15)</div>
                    </div>
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Template Boost Score</label>
                        <input type="number" name="boost_template_score" step="0.01" min="-1" max="1"
                            value="{{.SearchRankingConfig.BoostTemplateScore}}"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Boost for pages using a boosted template (default 0.05)</div>
                    </div>
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Demotion Score</label>
                        <input type="number" name="demote_score" step="0.01" min="-1" max="1"
                            value="{{.SearchRankingConfig.DemoteScore}}"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Score penalty for demoted paths (default -0.05)</div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.25rem; margin-bottom: 1.25rem;">
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Boosted Templates <span style="font-weight: normal;">(one per line)</span></label>
                        <textarea name="boost_templates" rows="4"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 0.875rem; resize: vertical;">{{range .SearchRankingConfig.BoostTemplates}}{{.}}
{{end}}</textarea>
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Template name substrings that get a ranking boost (e.g. concept)</div>
                    </div>
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Demoted Path Prefixes <span style="font-weight: normal;">(one per line)</span></label>
                        <textarea name="demote_path_prefixes" rows="4"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 0.875rem; resize: vertical;">{{range .SearchRankingConfig.DemotePathPrefixes}}{{.}}
{{end}}</textarea>
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">URL path prefixes to rank lower (e.g. /videos/)</div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.25rem; margin-bottom: 1.25rem;">
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Boosted Pages <span style="font-weight: normal;">(one exact path per line)</span></label>
                        <textarea name="boost_paths" rows="4"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 0.875rem; resize: vertical;">{{range .SearchRankingConfig.BoostPaths}}{{.}}
{{end}}</textarea>
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Exact page paths to always rank higher (e.g. /about)</div>
                    </div>
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Boost Score for Pages</label>
                        <input type="number" name="boost_path_score" step="0.01" min="0" max="1"
                            value="{{.SearchRankingConfig.BoostPathScore}}"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Score bonus for boosted pages (default 0.15)</div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.25rem; margin-bottom: 1.25rem;">
                    <div>
                        <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Demoted Pages <span style="font-weight: normal;">(one exact path per line)</span></label>
                        <textarea name="demote_paths" rows="4"
                            style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 0.875rem; resize: vertical;">{{range .SearchRankingConfig.DemotePaths}}{{.}}
{{end}}</textarea>
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Exact page paths to always rank lower (e.g. /thank-you). Uses the Demotion Score above.</div>
                    </div>
                </div>
                {{if index $.Can "settings.edit"}}<button type="submit" class="btn btn-primary">Save Ranking Config</button>{{else}}<span style="color: var(--text-muted); font-size: 0.9rem;">Read-only: your role cannot change these settings.</span>{{end}}
            </form>
        </div>
        {{end}}

        <!-- Test Search -->
        <div class="info-card" style="margin-bottom: 1.5rem;">
            <h2 style="margin-top: 0; font-size: 1.25rem;">Test Search</h2>
            <div style="display: flex; gap: 0.75rem; margin-bottom: 1rem; flex-wrap: wrap;">
                <div style="flex: 1; min-width: 200px; position: relative;">
                    <input type="text" id="search-query" placeholder="Enter search query..." autocomplete="off"
                        style="width: 100%; padding: 0.625rem 1rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; color: var(--text); font-size: 0.9375rem;">
                    <div id="suggest-dropdown" style="display: none; position: absolute; top: 100%; left: 0; right: 0; z-index: 100; background: var(--bg-card); border: 1px solid var(--border); border-radius: 0 0 8px 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.3); max-height: 320px; overflow-y: auto;"></div>
                </div>
                <select id="search-mode" style="padding: 0.625rem 1rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; color: var(--text);">
                    <option value="hybrid">Hybrid</option>
                    <option value="exact">Exact Match</option>
                    <option value="semantic">Semantic</option>
                </select>
                <button type="button" class="btn" id="search-btn" style="background: var(--primary); color: white;">Search</button>
            </div>
            <div id="search-results" style="display: none;">
                <div id="search-count" style="font-size: 0.875rem; color: var(--text-muted); margin-bottom: 0.75rem;"></div>
                <div id="search-list"></div>
            </div>
        </div>

        <!-- Integration Instructions -->
        <div class="info-card">
            <h2 style="margin-top: 0; font-size: 1.25rem;">Integration Guide</h2>
            <p style="color: var(--text-muted); margin-bottom: 1rem;">Add search to your website using the public search API endpoint.</p>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">API Endpoint</h3>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem; margin-bottom: 1rem;"><code>GET {{.BaseURL}}/api/search?q=YOUR_QUERY&mode=hybrid&limit=10</code></pre>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">Parameters</h3>
            <table style="width: 100%; margin-bottom: 1rem; font-size: 0.875rem;">
                <tr style="border-bottom: 1px solid var(--border);">
                    <td style="padding: 0.5rem; font-weight: 500;">q</td>
                    <td style="padding: 0.5rem; color: var(--text-muted);">Search query (required)</td>
                </tr>
                <tr style="border-bottom: 1px solid var(--border);">
                    <td style="padding: 0.5rem; font-weight: 500;">mode</td>
                    <td style="padding: 0.5rem; color: var(--text-muted);"><code>hybrid</code> (default), <code>exact</code>, or <code>semantic</code></td>
                </tr>
                <tr>
                    <td style="padding: 0.5rem; font-weight: 500;">limit</td>
                    <td style="padding: 0.5rem; color: var(--text-muted);">Max results 1-50 (default 10)</td>
                </tr>
            </table>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">Response</h3>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem; margin-bottom: 1rem;"><code>{
  "query": "your search",
  "mode": "hybrid",
  "total": 3,
  "results": [
    {
      "id": "...",
      "title": "Page Title",
      "full_path": "/my-page",
      "snippet": "...matching text around the query...",
      "score": 0.95,
      "match_type": "both"
    }
  ]
}</code></pre>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">Typeahead Suggest API</h3>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem; margin-bottom: 0.5rem;"><code>GET {{.BaseURL}}/api/search/suggest?q=PREFIX&amp;limit=8</code></pre>
            <table style="width: 100%; margin-bottom: 1rem; font-size: 0.875rem;">
                <tr style="border-bottom: 1px solid var(--border);">
                    <td style="padding: 0.5rem; font-weight: 500;">q</td>
                    <td style="padding: 0.5rem; color: var(--text-muted);">Prefix to match (required, min 2 chars)</td>
                </tr>
                <tr>
                    <td style="padding: 0.5rem; font-weight: 500;">limit</td>
                    <td style="padding: 0.5rem; color: var(--text-muted);">Max suggestions 1–20 (default 8)</td>
                </tr>
            </table>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem; margin-bottom: 1rem;"><code>{
  "keywords": ["game design", "game mechanics", ...],
  "pages": [
    {"title": "About Jon Radoff", "path": "/about"},
    {"title": "Game Design", "path": "/concepts/game-design"}
  ]
}</code></pre>
            <p style="color: var(--text-muted); margin-bottom: 1rem; font-size: 0.875rem;">
                <strong>keywords</strong> are extracted from titles/descriptions of published content and updated on publish/delete.
                Clicking a keyword triggers a full search for that term.<br>
                <strong>pages</strong> are direct-navigation results, ranked by: nav-linked &rsaquo; boosted-template &rsaquo; title-starts-with &rsaquo; title-contains &rsaquo; demoted paths.
                Clicking a page navigates directly without a search round-trip.
            </p>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">Full Search + Suggest Together (JavaScript)</h3>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem; margin-bottom: 1rem;"><code>&lt;input type="text" id="q" placeholder="Search..." autocomplete="off"&gt;
&lt;ul id="suggest"&gt;&lt;/ul&gt;
&lt;div id="results"&gt;&lt;/div&gt;

&lt;script&gt;
const input = document.getElementById('q');
const suggest = document.getElementById('suggest');
const results = document.getElementById('results');
let timer;

// Typeahead: show suggestions while typing
input.addEventListener('input', () => {
  clearTimeout(timer);
  const q = input.value.trim();
  if (q.length &lt; 2) { suggest.replaceChildren(); return; }
  timer = setTimeout(async () => {
    const r = await fetch('/api/search/suggest?q=' + encodeURIComponent(q) + '&amp;limit=8');
    const d = await r.json();
    suggest.replaceChildren(
      ...(d.pages || []).map(p =&gt; item('&#x1F4C4; ' + p.title, p.path)),
      ...(d.keywords || []).map(k =&gt; item('&#x1F50D; ' + k, '#', e =&gt; { e.preventDefault(); doSearch(k); })),
    );
  }, 200);
});

// Titles and keywords are text, never markup: build elements, set textContent
function item(text, href, onClick) {
  const li = document.createElement('li');
  const a = document.createElement('a');
  a.textContent = text;
  a.href = href;
  if (onClick) a.addEventListener('click', onClick);
  li.appendChild(a);
  return li;
}

// Full search on Enter
input.addEventListener('keydown', e =&gt; { if (e.key === 'Enter') doSearch(input.value); });

async function doSearch(q) {
  suggest.replaceChildren();
  const r = await fetch('/api/search?q=' + encodeURIComponent(q) + '&amp;mode=hybrid&amp;limit=10');
  const d = await r.json();
  results.replaceChildren(...(d.results || []).map(function(r) {
    const div = document.createElement('div');
    const a = document.createElement('a');
    const strong = document.createElement('strong');
    const p = document.createElement('p');
    a.href = r.full_path;
    strong.textContent = r.title;
    p.textContent = r.snippet;
    a.appendChild(strong);
    div.append(a, p);
    return div;
  }));
  if (!results.firstChild) results.textContent = 'No results.';
}
&lt;/script&gt;</code></pre>

            <h3 style="font-size: 1rem; margin-bottom: 0.5rem;">MongoDB Atlas Vector Search Index</h3>
            <p style="color: var(--text-muted); margin-bottom: 0.5rem;">
                For semantic search, create a vector search index on the <code>content</code> collection in MongoDB Atlas:
            </p>
            <ol style="color: var(--text-muted); margin-bottom: 0.5rem; padding-left: 1.5rem;">
                <li>Go to Atlas &rarr; Database &rarr; Browse Collections &rarr; <code>content</code> collection</li>
                <li>Click "Search Indexes" tab &rarr; "Create Search Index"</li>
                <li>Select "Atlas Vector Search" &rarr; JSON Editor</li>
                <li>Index name: <code>content_vector_search</code></li>
            </ol>
            <pre style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.875rem;"><code>{
  "fields": [
    {
      "type": "vector",
      "path": "embedding",
      "numDimensions": 1024,
      "similarity": "cosine"
    },
    {
      "type": "filter",
      "path": "published"
    },
    {
      "type": "filter",
      "path": "deleted"
    }
  ]
}</code></pre>
        </div>

        <script>
        // Search
        document.getElementById('search-btn').addEventListener('click', doSearch);
        document.getElementById('search-query').addEventListener('keydown', function(e) {
            if (e.key === 'Enter') doSearch();
        });

        async function doSearch() {
            const query = document.getElementById('search-query').value.trim();
            if (!query) return;

            const mode = document.getElementById('search-mode').value;
            const btn = document.getElementById('search-btn');
            btn.disabled = true;
            btn.textContent = 'Searching...';

            try {
                const res = await fetch('/cm/tools/search/test?q=' + encodeURIComponent(query) + '&mode=' + mode);
                const data = await res.json();

                document.getElementById('search-results').style.display = 'block';
                document.getElementById('search-count').textContent = data.total + ' result(s) found';

                const list = document.getElementById('search-list');
                if (!data.results || data.results.length === 0) {
                    list.innerHTML = '<div style="color: var(--text-muted); padding: 1rem;">No results found.</div>';
                } else {
                    list.innerHTML = data.results.map(function(r) {
                        return '<div style="padding: 0.75rem; margin-bottom: 0.5rem; background: var(--bg-dark); border-radius: 8px;">' +
                            '<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.25rem;">' +
                                '<a href="' + r.full_path + '" target="_blank" style="font-weight: 500; color: var(--primary);">' + escapeHtml(r.title) + '</a>' +
                                '<span style="font-size: 0.75rem; padding: 0.125rem 0.5rem; border-radius: 4px; background: ' +
                                    (r.match_type === 'both' ? 'var(--success)' : r.match_type === 'semantic' ? 'var(--primary)' : 'var(--warning)') +
                                    '; color: white;">' + r.match_type + '</span>' +
                            '</div>' +
                            '<div style="font-size: 0.875rem; color: var(--text-muted);">' + r.full_path + '</div>' +
                            '<div style="font-size: 0.875rem; color: var(--text); margin-top: 0.25rem;">' + escapeHtml(r.snippet) + '</div>' +
                            '<div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Score: ' + r.score.toFixed(4) + '</div>' +
                        '</div>';
                    }).join('');
                }
            } catch (err) {
                document.getElementById('search-results').style.display = 'block';
                document.getElementById('search-count').textContent = 'Error: ' + err.message;
                document.getElementById('search-list').innerHTML = '';
            } finally {
                btn.disabled = false;
                btn.textContent = 'Search';
            }
        }

        function escapeHtml(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Typeahead suggest
        var suggestTimer = null;
        var searchInput = document.getElementById('search-query');
        var suggestDropdown = document.getElementById('suggest-dropdown');

        searchInput.addEventListener('input', function() {
            clearTimeout(suggestTimer);
            var q = searchInput.value.trim();
            if (q.length < 2) {
                suggestDropdown.style.display = 'none';
                return;
            }
            suggestTimer = setTimeout(function() { fetchSuggestions(q); }, 200);
        });

        searchInput.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                suggestDropdown.style.display = 'none';
            }
        });

        document.addEventListener('click', function(e) {
            if (!searchInput.contains(e.target) && !suggestDropdown.contains(e.target)) {
                suggestDropdown.style.display = 'none';
            }
        });

        async function fetchSuggestions(q) {
            try {
                var res = await fetch('/api/search/suggest?q=' + encodeURIComponent(q) + '&limit=8');
                var data = await res.json();
                renderSuggestions(data);
            } catch (err) {
                suggestDropdown.style.display = 'none';
            }
        }

        function renderSuggestions(data) {
            var html = '';
            if (data.keywords && data.keywords.length > 0) {
                html += '<div style="padding: 0.375rem 0.75rem; font-size: 0.7rem; text-transform: uppercase; color: var(--text-muted); letter-spacing: 0.05em;">Keywords</div>';
                data.keywords.forEach(function(kw) {
                    html += '<div class="suggest-item" data-type="keyword" data-value="' + escapeHtml(kw) + '" style="padding: 0.5rem 0.75rem; cursor: pointer; font-size: 0.875rem; color: var(--text);">' +
                        '<span style="margin-right: 0.5rem; color: var(--text-muted);">&#x1F50D;</span>' + escapeHtml(kw) + '</div>';
                });
            }
            if (data.pages && data.pages.length > 0) {
                html += '<div style="padding: 0.375rem 0.75rem; font-size: 0.7rem; text-transform: uppercase; color: var(--text-muted); letter-spacing: 0.05em; border-top: 1px solid var(--border);">Pages</div>';
                data.pages.forEach(function(p) {
                    html += '<div class="suggest-item" data-type="page" data-path="' + escapeHtml(p.path) + '" data-value="' + escapeHtml(p.title) + '" style="padding: 0.5rem 0.75rem; cursor: pointer; font-size: 0.875rem; color: var(--text);">' +
                        '<span style="margin-right: 0.5rem; color: var(--text-muted);">&#x1F4C4;</span>' + escapeHtml(p.title) +
                        '<span style="float: right; font-size: 0.75rem; color: var(--text-muted);">' + escapeHtml(p.path) + '</span></div>';
                });
            }
            if (!html) {
                suggestDropdown.style.display = 'none';
                return;
            }
            suggestDropdown.innerHTML = html;
            suggestDropdown.style.display = 'block';

            // Add hover + click handlers
            suggestDropdown.querySelectorAll('.suggest-item').forEach(function(item) {
                item.addEventListener('mouseenter', function() { this.style.background = 'var(--bg-hover)'; });
                item.addEventListener('mouseleave', function() { this.style.background = 'transparent'; });
                item.addEventListener('click', function() {
                    if (this.dataset.type === 'page') {
                        window.open(this.dataset.path, '_blank');
                    } else {
                        searchInput.value = this.dataset.value;
                        suggestDropdown.style.display = 'none';
                        doSearch();
                    }
                });
            });
        }

        // Reindex
        var reindexBtn = document.getElementById('reindex-btn');
        if (reindexBtn) {
            reindexBtn.addEventListener('click', async function() {
                reindexBtn.disabled = true;
                reindexBtn.textContent = 'Reindexing...';
                document.getElementById('reindex-status').textContent = 'This may take a moment...';

                try {
                    const res = await fetch('/cm/tools/search/reindex', {
                        method: 'POST',
                        headers: {
                            'X-CSRF-Token': '{{.CSRFToken}}'
                        }
                    });
                    const data = await res.json();
                    document.getElementById('reindex-status').textContent =
                        'Done! ' + data.processed + ' pages indexed' +
                        (data.errors > 0 ? ', ' + data.errors + ' errors' : '');
                    document.getElementById('needs-indexing').textContent = '0';
                } catch (err) {
                    document.getElementById('reindex-status').textContent = 'Error: ' + err.message;
                } finally {
                    reindexBtn.disabled = false;
                    reindexBtn.textContent = 'Reindex All Content';
                }
            });
        }
        </script>
    ` + adminLayoutEnd,

	"chat_widget_tool": adminLayoutStart + `
        <div class="page-header">
            <h1>💬 Chat Widget</h1>
        </div>

        <!-- AI status -->
        <div style="display: flex; gap: 1rem; margin-bottom: 1.5rem; flex-wrap: wrap;">
            <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.875rem; background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; font-size: 0.8125rem;">
                {{if .SemanticEnabled}}
                <span style="width: 8px; height: 8px; border-radius: 50%; background: #4ade80; flex-shrink: 0;"></span>
                <span style="color: var(--text-muted);">Semantic search <strong style="color: var(--text);">enabled</strong></span>
                {{else}}
                <span style="width: 8px; height: 8px; border-radius: 50%; background: #f87171; flex-shrink: 0;"></span>
                <span style="color: var(--text-muted);">Semantic search <strong style="color: var(--text);">disabled</strong> — set <code>VOYAGE_API_KEY</code></span>
                {{end}}
            </div>
            <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.875rem; background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; font-size: 0.8125rem;">
                {{if .HaikuEnabled}}
                <span style="width: 8px; height: 8px; border-radius: 50%; background: #4ade80; flex-shrink: 0;"></span>
                <span style="color: var(--text-muted);">AI answers <strong style="color: var(--text);">enabled</strong> (Claude Haiku)</span>
                {{else}}
                <span style="width: 8px; height: 8px; border-radius: 50%; background: #fbbf24; flex-shrink: 0;"></span>
                <span style="color: var(--text-muted);">AI answers <strong style="color: var(--text);">disabled</strong> — set <code>ANTHROPIC_API_KEY</code></span>
                {{end}}
            </div>
        </div>

        {{if .Saved}}<div style="margin-bottom: 1.25rem; padding: 0.625rem 1rem; background: rgba(34,197,94,0.1); border: 1px solid rgba(34,197,94,0.3); border-radius: 8px; color: var(--success); font-size: 0.875rem;">Settings saved.</div>{{end}}

        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem; align-items: start;">

            <!-- Left: Configuration -->
            <div>
                <div class="info-card" style="margin-bottom: 1.5rem;">
                    <h2 style="margin-top: 0; font-size: 1.25rem;">Configuration</h2>
                    <form method="POST" action="/cm/tools/chat/config">
                        {{.CSRFField}}

                        <!-- Enable/disable -->
                        <div style="margin-bottom: 1.25rem; display: flex; align-items: center; gap: 0.75rem;">
                            <label style="display: flex; align-items: center; gap: 0.625rem; cursor: pointer; font-size: 0.9375rem; color: var(--text);">
                                <input type="hidden" name="enabled" value="0">
                                <input type="checkbox" name="enabled" value="1" {{if .ChatConfig.Enabled}}checked{{end}}
                                    style="width: 18px; height: 18px; accent-color: var(--primary); cursor: pointer;">
                                Enable chat widget on public site
                            </label>
                        </div>

                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1rem;">
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Widget Title</label>
                                <input type="text" name="widget_title" value="{{.ChatConfig.WidgetTitle}}"
                                    style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                            </div>
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Position</label>
                                <select name="position" style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                                    <option value="bottom-right" {{if eq .ChatConfig.Position "bottom-right"}}selected{{end}}>Bottom Right</option>
                                    <option value="bottom-left" {{if eq .ChatConfig.Position "bottom-left"}}selected{{end}}>Bottom Left</option>
                                </select>
                            </div>
                        </div>

                        <div style="margin-bottom: 1rem;">
                            <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Welcome Message</label>
                            <textarea name="welcome_message" rows="2"
                                style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); resize: vertical;">{{.ChatConfig.WelcomeMessage}}</textarea>
                        </div>

                        <div style="margin-bottom: 1rem;">
                            <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Input Placeholder</label>
                            <input type="text" name="placeholder" value="{{.ChatConfig.Placeholder}}"
                                style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                        </div>

                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1rem;">
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Primary Color</label>
                                <div style="display: flex; gap: 0.5rem; align-items: center;">
                                    <input type="color" name="primary_color" value="{{.ChatConfig.PrimaryColor}}" id="color-picker"
                                        style="width: 44px; height: 36px; padding: 2px; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; cursor: pointer;">
                                    <input type="text" id="color-text" value="{{.ChatConfig.PrimaryColor}}"
                                        style="flex: 1; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 0.875rem;"
                                        placeholder="#6366f1" maxlength="7">
                                </div>
                            </div>
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Max Results</label>
                                <input type="number" name="max_results" value="{{.ChatConfig.MaxResults}}" min="1" max="10"
                                    style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                                <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Results per query (1–10)</div>
                            </div>
                        </div>

                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1.25rem;">
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Rate Limit (per IP)</label>
                                <input type="number" name="rate_limit_per_ip" value="{{.ChatConfig.RateLimitPerIP}}" min="1" max="60"
                                    style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                                <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Queries/min per visitor</div>
                            </div>
                            <div>
                                <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">Rate Limit (global)</label>
                                <input type="number" name="rate_limit_global" value="{{.ChatConfig.RateLimitGlobal}}" min="1" max="300"
                                    style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text);">
                                <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">Total queries/min site-wide</div>
                            </div>
                        </div>

                        {{if index $.Can "settings.edit"}}<button type="submit" class="btn btn-primary">Save Configuration</button>{{else}}<span style="color: var(--text-muted); font-size: 0.9rem;">Read-only: your role cannot change these settings.</span>{{end}}
                    </form>
                </div>

                <!-- AI Prompts -->
                {{if .HaikuEnabled}}
                <div class="info-card" style="margin-bottom: 1.5rem;">
                    <h2 style="margin-top: 0; font-size: 1.25rem;">AI Prompts</h2>
                    <p style="color: var(--text-muted); font-size: 0.875rem; margin-bottom: 1rem;">
                        Customize what the AI is told. Use <code>{siteName}</code> in either field.
                        In the user prompt template, use <code>{excerpts}</code> for the retrieved page snippets and <code>{question}</code> for the visitor's query.
                    </p>
                    <form method="POST" action="/cm/tools/chat/config">
                        {{.CSRFField}}
                        <!-- hidden fields to preserve all other settings -->
                        <input type="hidden" name="enabled" value="{{if .ChatConfig.Enabled}}1{{else}}0{{end}}">
                        <input type="hidden" name="widget_title" value="{{.ChatConfig.WidgetTitle}}">
                        <input type="hidden" name="welcome_message" value="{{.ChatConfig.WelcomeMessage}}">
                        <input type="hidden" name="placeholder" value="{{.ChatConfig.Placeholder}}">
                        <input type="hidden" name="primary_color" value="{{.ChatConfig.PrimaryColor}}">
                        <input type="hidden" name="position" value="{{.ChatConfig.Position}}">
                        <input type="hidden" name="max_results" value="{{.ChatConfig.MaxResults}}">
                        <input type="hidden" name="rate_limit_per_ip" value="{{.ChatConfig.RateLimitPerIP}}">
                        <input type="hidden" name="rate_limit_global" value="{{.ChatConfig.RateLimitGlobal}}">

                        <div style="margin-bottom: 1rem;">
                            <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">System Prompt</label>
                            <textarea name="system_prompt" rows="8"
                                style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); resize: vertical; font-size: 0.8125rem; line-height: 1.5; font-family: inherit;">{{.ChatConfig.SystemPrompt}}</textarea>
                        </div>

                        <div style="margin-bottom: 1.25rem;">
                            <label style="display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.375rem;">User Prompt Template</label>
                            <textarea name="user_prompt_template" rows="6"
                                style="width: 100%; padding: 0.5rem 0.75rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); resize: vertical; font-size: 0.8125rem; line-height: 1.5; font-family: monospace;">{{.ChatConfig.UserPromptTemplate}}</textarea>
                        </div>

                        <div style="display: flex; align-items: center; gap: 1rem;">
                            {{if index $.Can "settings.edit"}}<button type="submit" class="btn btn-primary">Save Prompts</button>{{else}}<span style="color: var(--text-muted); font-size: 0.9rem;">Read-only: your role cannot change these settings.</span>{{end}}
                            <button type="button" class="btn btn-outline btn-sm" id="reset-prompts-btn">Reset to defaults</button>
                        </div>
                    </form>
                </div>
                {{end}}

                <!-- Embed Code -->
                <div class="info-card">
                    <h2 style="margin-top: 0; font-size: 1.25rem;">Embed Code</h2>
                    <p style="color: var(--text-muted); font-size: 0.875rem; margin-bottom: 1rem;">
                        Add this to your site's <code>&lt;head&gt;</code> or just before <code>&lt;/body&gt;</code>.
                        The widget loads asynchronously and won't block page rendering.
                    </p>
                    <pre id="embed-code" style="background: var(--bg-dark); padding: 1rem; border-radius: 8px; overflow-x: auto; font-size: 0.8125rem; margin-bottom: 0.75rem; white-space: pre-wrap; word-break: break-all;"><code>&lt;script src="{{.BaseURL}}/static/js/chat-widget.js" async&gt;&lt;/script&gt;</code></pre>
                    <button type="button" class="btn btn-outline btn-sm" id="copy-embed-btn" style="font-size: 0.8125rem;">Copy</button>

                    <div style="margin-top: 1.25rem; padding-top: 1.25rem; border-top: 1px solid var(--border);">
                        <h3 style="font-size: 0.9375rem; margin-bottom: 0.5rem;">API Endpoints</h3>
                        <p style="color: var(--text-muted); font-size: 0.8125rem; margin-bottom: 0.75rem;">
                            Use these directly to build custom search UIs or integrations.
                        </p>
                        <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                            <div style="background: var(--bg-dark); border-radius: 6px; padding: 0.5rem 0.75rem; font-size: 0.8125rem; font-family: monospace;">
                                <span style="color: var(--success);">GET</span>
                                <span style="color: var(--text); margin-left: 0.5rem;">{{.BaseURL}}/api/chat?q={query}</span>
                            </div>
                            <div style="background: var(--bg-dark); border-radius: 6px; padding: 0.5rem 0.75rem; font-size: 0.8125rem; font-family: monospace;">
                                <span style="color: var(--success);">GET</span>
                                <span style="color: var(--text); margin-left: 0.5rem;">{{.BaseURL}}/api/chat/config</span>
                            </div>
                        </div>
                        <p style="color: var(--text-muted); font-size: 0.8125rem; margin-top: 0.75rem;">
                            Both endpoints return JSON and include <code>Access-Control-Allow-Origin: *</code> for cross-origin use.
                        </p>
                    </div>
                </div>
            </div>

            <!-- Right: Live Test -->
            <div>
                <div class="info-card" style="position: sticky; top: 1.5rem;">
                    <h2 style="margin-top: 0; font-size: 1.25rem;">Live Test</h2>
                    <p style="color: var(--text-muted); font-size: 0.875rem; margin-bottom: 1rem;">
                        Try the widget here. This calls the same <code>/api/chat</code> endpoint visitors will use.
                        Works even when the widget is disabled on the public site.
                    </p>

                    <!-- Inline chat panel (same look as floating widget) -->
                    <div id="admin-chat-preview" style="border: 1px solid var(--border); border-radius: 14px; overflow: hidden; background: #fff; display: flex; flex-direction: column; max-height: 480px;">
                        <div id="admin-chat-header" style="padding: 0.875rem 1rem; display: flex; align-items: center; justify-content: space-between;">
                            <span id="admin-chat-title" style="font-weight: 600; font-size: 0.9375rem; color: #fff;">{{.ChatConfig.WidgetTitle}}</span>
                        </div>
                        <div id="admin-chat-body" style="flex: 1; overflow-y: auto; padding: 1rem; min-height: 140px; display: flex; flex-direction: column; gap: 0.75rem; background: #fff;">
                            <p style="color: #64748b; font-size: 0.875rem; line-height: 1.5; margin: 0;">{{.ChatConfig.WelcomeMessage}}</p>
                        </div>
                        <div style="padding: 0.75rem; border-top: 1px solid #f1f5f9; display: flex; gap: 0.5rem; background: #fff;">
                            <input id="admin-chat-input" type="text" placeholder="{{.ChatConfig.Placeholder}}" autocomplete="off" maxlength="200"
                                style="flex: 1; border: 1px solid #e2e8f0; border-radius: 8px; padding: 0.5rem 0.75rem; font-size: 0.875rem; outline: none; color: #1e293b; background: #f8fafc; transition: border-color 0.15s;">
                            <button id="admin-chat-send"
                                style="color: #fff; border: none; border-radius: 8px; padding: 0.5rem 0.875rem; cursor: pointer; font-size: 0.875rem; font-weight: 500; white-space: nowrap; transition: opacity 0.15s;">
                                Ask
                            </button>
                        </div>
                    </div>
                </div>
            </div>

        </div>

        <script>
        (function() {
            // Sync color picker ↔ text input ↔ live preview
            var colorPicker = document.getElementById('color-picker');
            var colorText = document.getElementById('color-text');
            var header = document.getElementById('admin-chat-header');
            var sendBtn = document.getElementById('admin-chat-send');

            function applyColor(hex) {
                if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return;
                header.style.background = hex;
                sendBtn.style.background = hex;
            }
            applyColor(colorPicker.value);

            colorPicker.addEventListener('input', function() {
                colorText.value = colorPicker.value;
                applyColor(colorPicker.value);
            });
            colorText.addEventListener('input', function() {
                var v = colorText.value.trim();
                if (/^#[0-9a-fA-F]{6}$/.test(v)) {
                    colorPicker.value = v;
                    // sync hidden primary_color field
                    document.querySelector('input[name="primary_color"]').value = v;
                    applyColor(v);
                }
            });
            colorPicker.addEventListener('change', function() {
                document.querySelector('input[name="primary_color"]').value = colorPicker.value;
            });

            // Widget title live update
            var titleInput = document.querySelector('input[name="widget_title"]');
            var chatTitle = document.getElementById('admin-chat-title');
            if (titleInput) {
                titleInput.addEventListener('input', function() {
                    chatTitle.textContent = titleInput.value || 'Chat with Site';
                });
            }

            // Placeholder live update
            var placeholderInput = document.querySelector('input[name="placeholder"]');
            var chatInput = document.getElementById('admin-chat-input');
            if (placeholderInput) {
                placeholderInput.addEventListener('input', function() {
                    chatInput.placeholder = placeholderInput.value || 'Ask a question...';
                });
            }

            // Reset prompts to defaults
            var resetBtn = document.getElementById('reset-prompts-btn');
            if (resetBtn) {
                var defaultSystemPrompt = "You are a friendly, knowledgeable assistant for {siteName}. Use the provided page excerpts to answer questions conversationally and helpfully. Synthesize a clear, direct answer — don't just describe what the pages say. Keep answers concise (2-4 sentences). If the excerpts don't contain enough to answer confidently, say so briefly and naturally.";
                var defaultUserPrompt = 'Here are relevant excerpts from the site:\n\n{excerpts}\nQuestion: {question}';
                resetBtn.addEventListener('click', function() {
                    var sp = document.querySelector('textarea[name="system_prompt"]');
                    var up = document.querySelector('textarea[name="user_prompt_template"]');
                    if (sp) sp.value = defaultSystemPrompt;
                    if (up) up.value = defaultUserPrompt;
                });
            }

            // Copy embed code button
            var copyBtn = document.getElementById('copy-embed-btn');
            copyBtn.addEventListener('click', function() {
                var code = document.querySelector('#embed-code code').textContent;
                navigator.clipboard.writeText(code).then(function() {
                    copyBtn.textContent = 'Copied!';
                    setTimeout(function() { copyBtn.textContent = 'Copy'; }, 2000);
                });
            });

            // Live test chat
            var body = document.getElementById('admin-chat-body');

            function escHtml(str) {
                return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
            }

            function renderMarkdown(text) {
                var s = text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
                s = s.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
                s = s.replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
                s = s.replace(/\[([^\]]+)\]\((https?:\/\/[^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
                s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');
                s = s.replace(/(?<!href=")(https?:\/\/[^\s<>")\]]+)/g, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>');
                s = s.replace(/(?<![="<\/])(\/[a-zA-Z][a-zA-Z0-9\-_\/]*)/g, '<a href="$1">$1</a>');
                s = s.replace(/\n/g, '<br>');
                return s;
            }

            function renderSources(results, color) {
                var el = document.createElement('div');
                var html = '<div style="font-size:0.75rem;font-weight:600;color:#94a3b8;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:0.375rem;">Sources</div>';
                results.forEach(function(r) {
                    html += '<a href="' + escHtml(r.url) + '" target="_blank" style="font-size:0.8125rem;text-decoration:none;display:flex;align-items:center;gap:0.375rem;padding:0.3rem 0;border-bottom:1px solid #f1f5f9;line-height:1.3;">' +
                        '<span style="opacity:0.5;flex-shrink:0;font-size:0.7rem;">&#8599;</span>' +
                        '<span class="acq-source-title" style="font-weight:500;color:#1e293b;">' + escHtml(r.title) + '</span>' +
                        '</a>';
                });
                el.innerHTML = html;
                // Hover colour through listeners: the configured colour is never written into an inline handler
                el.querySelectorAll('.acq-source-title').forEach(function(t) {
                    t.addEventListener('mouseover', function() { t.style.color = color; });
                    t.addEventListener('mouseout', function() { t.style.color = '#1e293b'; });
                });
                el.lastElementChild && (el.lastElementChild.style.borderBottom = 'none');
                return el;
            }

            function doQuery() {
                var q = chatInput.value.trim();
                if (!q) return;
                sendBtn.disabled = true;

                var dotColor = colorPicker.value;
                body.innerHTML = '<div id="acq-loading" style="display:flex;gap:4px;justify-content:center;align-items:center;padding:1rem 0;">' +
                    '<div style="width:8px;height:8px;border-radius:50%;background:' + dotColor + ';animation:lca-bounce 0.9s infinite"></div>' +
                    '<div style="width:8px;height:8px;border-radius:50%;background:' + dotColor + ';animation:lca-bounce 0.9s 0.15s infinite"></div>' +
                    '<div style="width:8px;height:8px;border-radius:50%;background:' + dotColor + ';animation:lca-bounce 0.9s 0.3s infinite"></div>' +
                    '</div><style>@keyframes lca-bounce{0%,80%,100%{transform:scale(0.7);opacity:.5}40%{transform:scale(1);opacity:1}}</style>';

                var loadingRemoved = false;
                var answerEl = null;
                var rawAnswer = '';

                function removeLoading() {
                    if (!loadingRemoved) {
                        var l = document.getElementById('acq-loading');
                        if (l) l.remove();
                        loadingRemoved = true;
                    }
                }

                function ensureAnswer() {
                    if (!answerEl) {
                        removeLoading();
                        answerEl = document.createElement('div');
                        answerEl.style.cssText = 'font-size:0.875rem;line-height:1.6;color:#1e293b;background:#f8fafc;border-radius:10px;padding:0.75rem;word-break:break-word;';
                        body.appendChild(answerEl);
                    }
                    return answerEl;
                }

                function handleEvent(evt) {
                    if (evt.type === 'token' && evt.text) {
                        rawAnswer += evt.text;
                        ensureAnswer().innerHTML = renderMarkdown(rawAnswer);
                        body.scrollTop = body.scrollHeight;
                    } else if (evt.type === 'sources') {
                        removeLoading();
                        var results = evt.results || [];
                        if (results.length === 0 && !answerEl) {
                            body.innerHTML = '<p style="color:#94a3b8;font-size:0.875rem;text-align:center;padding:1rem 0;margin:0;">No results found. Try rephrasing your question.</p>';
                            return;
                        }
                        if (results.length > 0) {
                            body.appendChild(renderSources(results, colorPicker.value));
                            body.scrollTop = body.scrollHeight;
                        }
                    } else if (evt.type === 'done') {
                        sendBtn.disabled = false;
                        chatInput.value = '';
                        chatInput.focus();
                    }
                }

                fetch('/api/chat?q=' + encodeURIComponent(q))
                    .then(function(res) {
                        if (!res.ok || !res.body) throw new Error('HTTP ' + res.status);
                        var reader = res.body.getReader();
                        var decoder = new TextDecoder();
                        var buf = '';
                        function pump() {
                            return reader.read().then(function(result) {
                                if (result.done) { sendBtn.disabled = false; return; }
                                buf += decoder.decode(result.value, {stream: true});
                                var parts = buf.split('\n\n');
                                buf = parts.pop();
                                for (var i = 0; i < parts.length; i++) {
                                    var p = parts[i].trim();
                                    if (p.indexOf('data: ') === 0) {
                                        try { handleEvent(JSON.parse(p.slice(6))); } catch(e) {}
                                    }
                                }
                                return pump();
                            });
                        }
                        return pump();
                    })
                    .catch(function() {
                        sendBtn.disabled = false;
                        body.innerHTML = '<p style="color:#f87171;font-size:0.875rem;text-align:center;padding:1rem 0;margin:0;">Request failed. Check that the server is running.</p>';
                    });
            }

            sendBtn.addEventListener('click', doQuery);
            chatInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter') doQuery();
            });
            chatInput.addEventListener('focus', function() {
                chatInput.style.borderColor = colorPicker.value;
                chatInput.style.background = '#fff';
            });
            chatInput.addEventListener('blur', function() {
                chatInput.style.borderColor = '#e2e8f0';
                chatInput.style.background = '#f8fafc';
            });
        })();
        </script>
    ` + adminLayoutEnd,

	"force_change_password": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Change Password - LightCMS</title>
    <link rel="icon" type="image/x-icon" href="/static/images/favicon.ico">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Inter', system-ui, sans-serif;
            background: linear-gradient(135deg, #0f172a 0%, #1e1b4b 50%, #0f172a 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1rem;
        }
        .card {
            background: rgba(30, 27, 75, 0.5);
            backdrop-filter: blur(20px);
            border: 1px solid rgba(99, 102, 241, 0.2);
            border-radius: 24px;
            padding: 3rem;
            width: 100%;
            max-width: 420px;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        h2 { color: #f1f5f9; margin-bottom: 0.5rem; font-family: 'Space Grotesk', sans-serif; }
        .info { color: #94a3b8; margin-bottom: 1.5rem; font-size: 0.9rem; line-height: 1.5; }
        .error {
            background: rgba(239, 68, 68, 0.1);
            border: 1px solid rgba(239, 68, 68, 0.3);
            color: #f87171;
            padding: 0.75rem 1rem;
            border-radius: 8px;
            margin-bottom: 1.5rem;
            font-size: 0.9rem;
        }
        label { display: block; color: #e2e8f0; margin-bottom: 0.5rem; font-weight: 500; }
        input[type="password"] {
            width: 100%;
            padding: 0.875rem 1rem;
            background: rgba(15, 23, 42, 0.5);
            border: 1px solid rgba(99, 102, 241, 0.3);
            border-radius: 12px;
            color: #f1f5f9;
            font-size: 1rem;
            margin-bottom: 1rem;
        }
        input[type="password"]:focus { outline: none; border-color: #6366f1; box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2); }
        button {
            width: 100%;
            padding: 0.875rem;
            background: linear-gradient(135deg, #6366f1, #8b5cf6);
            border: none;
            border-radius: 12px;
            color: white;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            margin-top: 0.5rem;
        }
        button:hover { transform: translateY(-2px); box-shadow: 0 10px 20px -10px rgba(99, 102, 241, 0.5); }
    </style>
</head>
<body>
    <div class="card">
        <h2>Change Your Password</h2>
        <p class="info">You must change your temporary password before continuing.</p>
        {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
        <form method="POST" action="/cm/change-password">
            {{.CSRFField}}
            <label for="current_password">Current Password</label>
            <input type="password" id="current_password" name="current_password" required autofocus>
            <label for="new_password">New Password</label>
            <input type="password" id="new_password" name="new_password" required>
            <label for="confirm_password">Confirm New Password</label>
            <input type="password" id="confirm_password" name="confirm_password" required>
            <button type="submit">Change Password</button>
        </form>
    </div>
</body>
</html>`,

	"users_list": adminLayoutStart + `
        <div class="content-section">
            <div class="section-header">
                <h1>Users</h1>
                <a href="/cm/users/new" class="btn btn-primary">+ New User</a>
            </div>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Email</th>
                        <th>Display Name</th>
                        <th>Role</th>
                        <th>Status</th>
                        <th>Last Login</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Users}}
                    <tr>
                        <td>{{.Email}}</td>
                        <td>{{.DisplayName}}</td>
                        <td><span class="badge badge-{{.Role}}">{{.Role}}</span></td>
                        <td>{{if .Disabled}}<span style="color:#f87171;">Disabled</span>{{else}}<span style="color:#4ade80;">Active</span>{{end}}</td>
                        <td>{{if .LastLoginAt}}{{.LastLoginAt.Format "Jan 2, 2006 15:04"}}{{else}}Never{{end}}</td>
                        <td>
                            <a href="/cm/users/{{.ID.Hex}}" class="btn btn-small">Edit</a>
                        </td>
                    </tr>
                {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"user_new": adminLayoutStart + `
        <div class="content-section">
            <h1>Create New User</h1>
            {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
            <form method="POST" action="/cm/users/new" class="form-card">
                {{.CSRFField}}
                <div class="form-group">
                    <label for="email">Email *</label>
                    <input type="email" id="email" name="email" required>
                </div>
                <div class="form-group">
                    <label for="display_name">Display Name</label>
                    <input type="text" id="display_name" name="display_name">
                </div>
                <div class="form-group">
                    <label for="role">Role *</label>
                    <select id="role" name="role" required>
                        <option value="viewer">Viewer (read-only)</option>
                        <option value="contributor">Contributor (create &amp; submit for approval)</option>
                        <option value="editor" selected>Editor (content management)</option>
                        <option value="admin">Admin (full access)</option>
                    </select>
                </div>
                <div class="form-actions">
                    <button type="submit" class="btn btn-primary">Create User</button>
                    <a href="/cm/users" class="btn btn-outline">Cancel</a>
                </div>
            </form>
        </div>
    ` + adminLayoutEnd,

	"user_created": adminLayoutStart + `
        <div class="content-section">
            <h1>User Created</h1>
            <div class="success-card" style="background: rgba(34, 197, 94, 0.1); border: 1px solid rgba(34, 197, 94, 0.3); padding: 1.5rem; border-radius: 12px; margin-bottom: 1.5rem;">
                <p><strong>{{.NewUser.Email}}</strong> has been created with the role <strong>{{.NewUser.Role}}</strong>.</p>
                <p style="margin-top: 1rem;">Temporary password (shown once):</p>
                <code style="display: block; padding: 1rem; background: rgba(15, 23, 42, 0.5); border-radius: 8px; margin-top: 0.5rem; font-size: 1.1rem; color: #4ade80; user-select: all;">{{.TempPassword}}</code>
                <p style="margin-top: 1rem; color: #94a3b8; font-size: 0.9rem;">The user will be required to change this password on first login.</p>
            </div>
            <a href="/cm/users" class="btn btn-primary">Done</a>
        </div>
    ` + adminLayoutEnd,

	"user_edit": adminLayoutStart + `
        <div class="content-section">
            <h1>Edit User: {{.EditUser.Email}}</h1>
            {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
            <form method="POST" action="/cm/users/{{.EditUser.ID.Hex}}" class="form-card">
                {{.CSRFField}}
                <div class="form-group">
                    <label>Email</label>
                    <input type="text" value="{{.EditUser.Email}}" disabled style="opacity: 0.6;">
                </div>
                <div class="form-group">
                    <label for="display_name">Display Name</label>
                    <input type="text" id="display_name" name="display_name" value="{{.EditUser.DisplayName}}">
                </div>
                <div class="form-group">
                    <label for="role">Role</label>
                    <select id="role" name="role">
                        <option value="viewer" {{if eq .EditUser.Role "viewer"}}selected{{end}}>Viewer</option>
                        <option value="contributor" {{if eq .EditUser.Role "contributor"}}selected{{end}}>Contributor</option>
                        <option value="editor" {{if eq .EditUser.Role "editor"}}selected{{end}}>Editor</option>
                        <option value="admin" {{if eq .EditUser.Role "admin"}}selected{{end}}>Admin</option>
                    </select>
                </div>
                <div class="form-actions">
                    <button type="submit" class="btn btn-primary">Save Changes</button>
                    <a href="/cm/users" class="btn btn-outline">Cancel</a>
                </div>
            </form>
            <div style="margin-top: 2rem; padding-top: 1.5rem; border-top: 1px solid rgba(99, 102, 241, 0.2);">
                <h3 style="margin-bottom: 1rem;">Account Actions</h3>
                <div style="display: flex; gap: 1rem; flex-wrap: wrap;">
                    <form method="POST" action="/cm/users/{{.EditUser.ID.Hex}}/toggle-disabled" style="display:inline;"{{if not .EditUser.Disabled}} data-confirm="Disable the account {{.EditUser.Email}}? They will be signed out and unable to sign in until it is enabled again." data-confirm-title="Disable Account"{{end}}>
                        {{.CSRFField}}
                        <button type="submit" class="btn btn-outline">{{if .EditUser.Disabled}}Enable Account{{else}}Disable Account{{end}}</button>
                    </form>
                    <form method="POST" action="/cm/users/{{.EditUser.ID.Hex}}/reset-password" style="display:inline;" data-confirm="Reset the password for {{.EditUser.Email}}? Their current password stops working and they get a temporary one." data-confirm-title="Reset Password">
                        {{.CSRFField}}
                        <button type="submit" class="btn btn-outline">Reset Password</button>
                    </form>
                </div>
            </div>
        </div>
    ` + adminLayoutEnd,

	"user_password_reset": adminLayoutStart + `
        <div class="content-section">
            <h1>Password Reset</h1>
            <div class="success-card" style="background: rgba(34, 197, 94, 0.1); border: 1px solid rgba(34, 197, 94, 0.3); padding: 1.5rem; border-radius: 12px; margin-bottom: 1.5rem;">
                <p>Password for <strong>{{.TargetUser.Email}}</strong> has been reset.</p>
                <p style="margin-top: 1rem;">New temporary password (shown once):</p>
                <code style="display: block; padding: 1rem; background: rgba(15, 23, 42, 0.5); border-radius: 8px; margin-top: 0.5rem; font-size: 1.1rem; color: #4ade80; user-select: all;">{{.TempPassword}}</code>
                <p style="margin-top: 1rem; color: #94a3b8; font-size: 0.9rem;">The user will be required to change this password on next login.</p>
            </div>
            <a href="/cm/users" class="btn btn-primary">Done</a>
        </div>
    ` + adminLayoutEnd,

	"analytics": adminLayoutStart + `
        <style>
            .analytics-card { background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; }
            .analytics-label { font-size: 0.75rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
            .analytics-value { font-size: 2rem; font-weight: 700; margin-top: 0.25rem; }
            .chart-box { background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; margin-bottom: 1.5rem; }
            .chart-title { margin: 0 0 1rem 0; font-size: 0.875rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
            .vchart { display: flex; align-items: flex-end; height: 180px; position: relative; overflow: hidden; }
            .vchart-bar { flex: 1; min-width: 0; background: #60a5fa; border-radius: 1px 1px 0 0; position: relative; transition: background 0.1s; cursor: pointer; }
            .vchart-bar:hover { background: #93c5fd; }
            .vchart-bar[data-v="0"] { background: transparent; }
            .vchart-bar[data-v="0"]:hover { background: rgba(148,163,184,0.15); }
            .vchart-tooltip { display: none; position: absolute; bottom: calc(100% + 6px); left: 50%; transform: translateX(-50%); background: #1e293b; border: 1px solid #475569; color: #f1f5f9; padding: 6px 10px; border-radius: 6px; font-size: 12px; white-space: nowrap; z-index: 10; pointer-events: none; box-shadow: 0 4px 12px rgba(0,0,0,0.4); }
            .vchart-tooltip::after { content: ''; position: absolute; top: 100%; left: 50%; transform: translateX(-50%); border: 5px solid transparent; border-top-color: #475569; }
            .vchart-bar:hover .vchart-tooltip { display: block; }
            .vchart-y { position: absolute; left: 0; right: 0; border-top: 1px solid #475569; pointer-events: none; }
            .vchart-y span { position: absolute; right: calc(100% + 4px); top: -7px; font-size: 10px; color: #94a3b8; font-family: monospace; }
            .vchart-wrap { position: relative; padding-left: 40px; }
            .vchart-trend { position: absolute; top: 0; left: 0; width: 100%; height: 100%; pointer-events: none; }
            .vchart-trend line { stroke: #f59e0b; stroke-width: 2; stroke-dasharray: 6 3; opacity: 0.85; }
            .trend-legend { display: inline-flex; align-items: center; gap: 0.375rem; font-size: 0.7rem; color: #94a3b8; margin-left: 0.75rem; vertical-align: middle; }
            .trend-legend-line { width: 18px; height: 0; border-top: 2px dashed #f59e0b; opacity: 0.85; }
            .vchart-xlabels { position: relative; height: 16px; padding-left: 40px; margin-top: 4px; }
            .vchart-xlabels span { position: absolute; font-size: 10px; color: #94a3b8; font-family: monospace; white-space: nowrap; transform: translateX(-50%); }
            .stat-row { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; font-size: 0.8125rem; }
            .stat-label { width: 220px; min-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #e2e8f0; }
            .stat-label a { color: #60a5fa; text-decoration: none; }
            .stat-label a:hover { text-decoration: underline; }
            .stat-bar-wrap { flex: 1; height: 22px; background: #1e293b; border-radius: 4px; overflow: hidden; }
            .stat-bar { height: 100%; border-radius: 4px; min-width: 2px; }
            .stat-bar-blue { background: #60a5fa; }
            .stat-bar-green { background: #4ade80; }
            .stat-count { width: 70px; text-align: right; color: #94a3b8; font-family: monospace; font-size: 0.75rem; }
            .stat-actions { display: flex; gap: 0.375rem; width: 70px; flex-shrink: 0; }
            .stat-actions a { color: #94a3b8; text-decoration: none; font-size: 0.75rem; padding: 2px 5px; border-radius: 3px; border: 1px solid #475569; }
            .stat-actions a:hover { color: #e2e8f0; border-color: #60a5fa; }
            .stat-empty { color: var(--text-muted); font-size: 0.875rem; padding: 1rem 0; }
            .ref-tab { padding: 0.25rem 0.625rem; font-size: 0.75rem; background: transparent; border: 1px solid #475569; color: #94a3b8; cursor: pointer; border-radius: 4px; }
            .ref-tab:hover { color: #e2e8f0; border-color: #60a5fa; }
            .ref-tab.active { background: #60a5fa; color: #fff; border-color: #60a5fa; }
            .two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem; }
            @media (max-width: 900px) { .two-col { grid-template-columns: 1fr; } }
        </style>

        <div class="content-section">
            <h1>Site Analytics <a href="/cm/analytics/ai?range={{.Range}}" class="btn btn-secondary" style="padding: 0.3rem 0.7rem; font-size: 0.8rem; vertical-align: middle; margin-left: 0.5rem;">🤖 AI traffic</a></h1>
            <div style="display: flex; gap: 0.5rem; margin-bottom: 1.5rem;">
                <a href="/cm/analytics?range=24h" class="btn {{if eq .Range "24h"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">24 Hours</a>
                <a href="/cm/analytics?range=7d" class="btn {{if eq .Range "7d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">7 Days</a>
                <a href="/cm/analytics?range=30d" class="btn {{if eq .Range "30d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">30 Days</a>
                <a href="/cm/analytics?range=60d" class="btn {{if eq .Range "60d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">60 Days</a>
                <a href="/cm/analytics?range=90d" class="btn {{if eq .Range "90d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">90 Days</a>
                <form method="GET" action="/cm/analytics" style="display: inline-flex; gap: 6px; align-items: center; margin-left: 8px;">
                    <input type="hidden" name="range" value="custom">
                    <input type="date" name="start" value="{{.RangeStart}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <span style="color: var(--text-muted);">–</span>
                    <input type="date" name="end" value="{{.RangeEnd}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <button type="submit" class="btn {{if eq .Range "custom"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">Apply</button>
                </form>
            </div>

            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 1rem; margin-bottom: 2rem;">
                <div class="analytics-card">
                    <div class="analytics-label">Uptime</div>
                    <div class="analytics-value" style="color: {{if ge .UptimePercent 99.0}}#4ade80{{else if ge .UptimePercent 95.0}}#facc15{{else}}#f87171{{end}};">{{printf "%.1f" .UptimePercent}}%</div>
                </div>
                <div class="analytics-card">
                    <div class="analytics-label">All Visitors</div>
                    <div class="analytics-value">{{.TotalVisitors}}</div>
                </div>
                <div class="analytics-card">
                    <div class="analytics-label">Non-Bot Visitors</div>
                    <div class="analytics-value">{{.HumanVisitors}}</div>
                </div>
                <div class="analytics-card">
                    <div class="analytics-label">Trend</div>
                    <div id="trendPctCard" class="analytics-value">&mdash;</div>
                </div>
                <div class="analytics-card">
                    <div class="analytics-label">Peak Hour</div>
                    <div style="font-size: 1.25rem; font-weight: 700; margin-top: 0.25rem;">{{if .PeakHour}}{{.PeakVisitors}} visitors<div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 400;">{{.PeakHour}}</div>{{else}}&mdash;{{end}}</div>
                </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 280px; gap: 1.5rem; margin-bottom: 1.5rem;">
                <div class="chart-box" style="margin-bottom:0;">
                    <div style="display:flex;align-items:center;gap:1rem;margin-bottom:1rem;">
                        <h3 class="chart-title" style="margin:0;">Unique Visitors per Hour</h3>
                        <div style="display:flex;gap:2px;">
                            <button class="ref-tab active" data-vchart-tab="human" onclick="switchVisitorTab('human')">Excluding Bots</button>
                            <button class="ref-tab" data-vchart-tab="bot" onclick="switchVisitorTab('bot')">Bots Only</button>
                            <button class="ref-tab" data-vchart-tab="all" onclick="switchVisitorTab('all')">All Visitors</button>
                        </div>
                        <span id="trendLegend" class="trend-legend" style="display:none;"><span class="trend-legend-line"></span> Trend</span>
                    </div>
                    <div class="vchart-wrap">
                        <div id="visitorsChart" class="vchart">
                            <svg id="trendLine" class="vchart-trend"></svg>
                        </div>
                    </div>
                    <div id="visitorsXLabels" class="vchart-xlabels"></div>
                </div>
                <div class="chart-box" style="margin-bottom:0;">
                    <h3 class="chart-title">Browsers</h3>
                    <canvas id="uaPieChart" width="240" height="240" style="display:block;margin:0 auto;"></canvas>
                    <div id="uaLegend" style="margin-top: 0.75rem; font-size: 0.75rem; color: #94a3b8;"></div>
                    <div id="uaEmpty" class="stat-empty" style="display:none;">No browser data yet.</div>
                </div>
            </div>

            <div class="two-col">
                <div class="chart-box" style="margin-bottom:0;">
                    <h3 class="chart-title">Top Pages</h3>
                    <div id="topPages"></div>
                    <div id="topPagesEmpty" class="stat-empty" style="display:none;">No page view data yet.</div>
                </div>
                <div class="chart-box" style="margin-bottom:0;">
                    <div style="display:flex;align-items:center;gap:1rem;margin-bottom:1rem;">
                        <h3 class="chart-title" style="margin:0;">Top Referrers</h3>
                        <div class="ref-tabs" style="display:flex;gap:2px;">
                            <button class="ref-tab active" data-ref-tab="human" onclick="switchRefTab('human')">Non-bot</button>
                            <button class="ref-tab" data-ref-tab="bot" onclick="switchRefTab('bot')">Bots</button>
                            <button class="ref-tab" data-ref-tab="all" onclick="switchRefTab('all')">All</button>
                        </div>
                    </div>
                    <div id="topReferrers"></div>
                    <div id="topReferrersEmpty" class="stat-empty" style="display:none;">No referrer data yet.</div>
                </div>
            </div>

            <div class="chart-box" style="margin-top: 1.5rem;">
                <h3 class="chart-title">Uptime</h3>
                <div id="uptimeStrip" style="display: flex; gap: 1px; flex-wrap: wrap;"></div>
                <div style="display: flex; gap: 1rem; margin-top: 0.75rem; font-size: 0.75rem; color: var(--text-muted);">
                    <span><span style="display: inline-block; width: 10px; height: 10px; background: #4ade80; border-radius: 2px; vertical-align: middle;"></span> Online</span>
                    <span><span style="display: inline-block; width: 10px; height: 10px; background: #f87171; border-radius: 2px; vertical-align: middle;"></span> Offline</span>
                    <span><span style="display: inline-block; width: 10px; height: 10px; background: #334155; border-radius: 2px; vertical-align: middle;"></span> No data</span>
                </div>
            </div>
        </div>

        <script>
        (function() {
            var stats = JSON.parse('{{.StatsJSON}}');
            var topPagesData = {
                human: JSON.parse('{{.TopPagesHumanJSON}}'),
                bot: JSON.parse('{{.TopPagesBotJSON}}'),
                all: JSON.parse('{{.TopPagesAllJSON}}')
            };
            var refData = {
                human: JSON.parse('{{.RefHumanJSON}}'),
                bot: JSON.parse('{{.RefBotJSON}}'),
                all: JSON.parse('{{.RefAllJSON}}')
            };
            var userAgents = JSON.parse('{{.UserAgentsJSON}}');
            var range = '{{.Range}}';

            var byHour = {};
            stats.forEach(function(s) { byHour[new Date(s.hour).getTime()] = s; });

            var now = new Date();
            now.setUTCMinutes(0, 0, 0);
            var hours = range === '30d' ? 720 : range === '7d' ? 168 : 24;
            var nowKey = now.getTime();
            var labels = [], uptimeData = [], isCurrentHour = [];
            // Build per-tab visitor arrays
            var visitorSets = { human: [], bot: [], all: [] };

            for (var i = hours - 1; i >= 0; i--) {
                var h = new Date(now.getTime() - i * 3600000);
                var key = h.getTime();
                var s = byHour[key];
                var label;
                if (range === '24h') label = h.getUTCHours() + ':00';
                else if (range === '7d') label = (h.getUTCMonth()+1) + '/' + h.getUTCDate() + ' ' + h.getUTCHours() + ':00';
                else label = (h.getUTCMonth()+1) + '/' + h.getUTCDate();
                labels.push(label);
                visitorSets.all.push(s ? s.visitor_count : 0);
                visitorSets.human.push(s ? s.visitor_count_human : 0);
                visitorSets.bot.push(s ? s.visitor_count_bot : 0);
                uptimeData.push(s ? s.uptime_pings : -1);
                isCurrentHour.push(key === nowKey);
            }

            // --- X-axis label positions (computed once, reused on tab switch) ---
            var labelTexts = [];
            if (range === '24h') {
                for (var i = 0; i < hours; i++) {
                    var h = new Date(now.getTime() - (hours - 1 - i) * 3600000);
                    if (h.getUTCHours() % 4 === 0) labelTexts.push({ idx: i, text: labels[i] });
                }
            } else if (range === '7d') {
                var lastDay = -1;
                for (var i = 0; i < hours; i++) {
                    var h = new Date(now.getTime() - (hours - 1 - i) * 3600000);
                    var day = h.getUTCDate();
                    if (day !== lastDay) {
                        labelTexts.push({ idx: i, text: (h.getUTCMonth()+1) + '/' + day });
                        lastDay = day;
                    }
                }
            } else {
                var lastDay = -1, dayCount = 0;
                for (var i = 0; i < hours; i++) {
                    var h = new Date(now.getTime() - (hours - 1 - i) * 3600000);
                    var day = h.getUTCDate();
                    if (day !== lastDay) {
                        if (dayCount % 5 === 0) labelTexts.push({ idx: i, text: (h.getUTCMonth()+1) + '/' + day });
                        lastDay = day;
                        dayCount++;
                    }
                }
            }

            // --- Render visitor chart for a given tab ---
            function renderVisitorChart(which) {
                var visitors = visitorSets[which];
                var maxV = Math.max.apply(null, visitors) || 1;
                var chart = document.getElementById('visitorsChart');
                var svg = document.getElementById('trendLine');
                // Clear existing bars and grid lines (keep the SVG)
                var children = chart.children;
                for (var i = children.length - 1; i >= 0; i--) {
                    if (children[i] !== svg) chart.removeChild(children[i]);
                }
                // Clear SVG
                svg.innerHTML = '';
                svg.removeAttribute('viewBox');
                document.getElementById('trendLegend').style.display = 'none';
                // Reset trend KPI so a tab without enough data doesn't keep the previous tab's value
                var trendCard = document.getElementById('trendPctCard');
                trendCard.textContent = '—';
                trendCard.style.color = '';

                var barGap = visitors.length <= 48 ? 2 : visitors.length <= 168 ? 1 : 0;
                chart.style.gap = barGap + 'px';
                // Grid lines
                for (var g = 0; g < 5; g++) {
                    var pct = (g / 4) * 100;
                    var line = document.createElement('div');
                    line.className = 'vchart-y';
                    line.style.bottom = (100 - pct) + '%';
                    var lbl = document.createElement('span');
                    lbl.textContent = Math.round(maxV - (maxV / 4) * g);
                    line.appendChild(lbl);
                    chart.insertBefore(line, svg);
                }
                // Bars
                visitors.forEach(function(v, i) {
                    var bar = document.createElement('div');
                    bar.className = 'vchart-bar';
                    bar.setAttribute('data-v', v);
                    bar.style.height = Math.max(v > 0 ? 2 : 0, maxV > 0 ? (v / maxV * 100) : 0) + '%';
                    var tip = document.createElement('div');
                    tip.className = 'vchart-tooltip';
                    tip.innerHTML = '<strong>' + v.toLocaleString() + ' unique' + (v !== 1 ? 's' : '') + '</strong><br>' + labels[i] + ' UTC';
                    bar.appendChild(tip);
                    chart.insertBefore(bar, svg);
                });
                // X labels
                var xlabels = document.getElementById('visitorsXLabels');
                xlabels.innerHTML = '';
                labelTexts.forEach(function(lt) {
                    var sp = document.createElement('span');
                    sp.textContent = lt.text;
                    sp.style.left = ((lt.idx + 0.5) / visitors.length * 100) + '%';
                    xlabels.appendChild(sp);
                });
                // Trend line
                var points = [];
                for (var i = 0; i < visitors.length; i++) {
                    if (visitors[i] > 0) points.push({ x: i, y: visitors[i] });
                }
                if (points.length >= 2) {
                    var n = points.length;
                    var sumX = 0, sumY = 0, sumXY = 0, sumX2 = 0;
                    for (var i = 0; i < n; i++) {
                        sumX += points[i].x;
                        sumY += points[i].y;
                        sumXY += points[i].x * points[i].y;
                        sumX2 += points[i].x * points[i].x;
                    }
                    var denom = n * sumX2 - sumX * sumX;
                    if (denom !== 0) {
                        var m = (n * sumXY - sumX * sumY) / denom;
                        var bCoeff = (sumY - m * sumX) / n;
                        var firstIdx = points[0].x;
                        var lastIdx = points[n - 1].x;
                        var y1 = m * firstIdx + bCoeff;
                        var y2 = m * lastIdx + bCoeff;
                        var total = visitors.length;
                        var x1Pct = (firstIdx + 0.5) / total * 100;
                        var x2Pct = (lastIdx + 0.5) / total * 100;
                        var y1Pct = Math.max(0, Math.min(100, 100 - (y1 / maxV * 100)));
                        var y2Pct = Math.max(0, Math.min(100, 100 - (y2 / maxV * 100)));
                        svg.setAttribute('viewBox', '0 0 100 100');
                        svg.setAttribute('preserveAspectRatio', 'none');
                        var tl = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                        tl.setAttribute('x1', x1Pct);
                        tl.setAttribute('y1', y1Pct);
                        tl.setAttribute('x2', x2Pct);
                        tl.setAttribute('y2', y2Pct);
                        tl.setAttribute('vector-effect', 'non-scaling-stroke');
                        svg.appendChild(tl);
                        document.getElementById('trendLegend').style.display = '';
                        // Update trend % KPI card
                        if (y1 > 0) {
                            var pctChange = ((y2 - y1) / y1 * 100);
                            var sign = pctChange >= 0 ? '+' : '';
                            trendCard.textContent = sign + pctChange.toFixed(1) + '%';
                            trendCard.style.color = pctChange > 0 ? '#4ade80' : pctChange < 0 ? '#f87171' : '#e2e8f0';
                        }
                    }
                }
            }

            // Tab switching
            window.switchVisitorTab = function(which) {
                document.querySelectorAll('[data-vchart-tab]').forEach(function(b) { b.classList.remove('active'); });
                document.querySelector('[data-vchart-tab="'+which+'"]').classList.add('active');
                renderVisitorChart(which);
                renderTopPages(which);
            };
            // Initial render with human (excluding bots) as default
            renderVisitorChart('human');

            // --- Uptime strip ---
            var strip = document.getElementById('uptimeStrip');
            var cellSize = range === '30d' ? '3px' : range === '7d' ? '6px' : '16px';
            uptimeData.forEach(function(pings, idx) {
                var cell = document.createElement('div');
                var color;
                if (isCurrentHour[idx]) color = '#4ade80';
                else if (pings > 0) color = '#4ade80';
                else if (pings === 0) color = '#f87171';
                else color = '#334155';
                cell.style.cssText = 'width:' + cellSize + ';height:20px;background:' + color + ';border-radius:2px;flex-shrink:0;';
                var tt = labels[idx] + ' UTC';
                if (isCurrentHour[idx]) tt += ' \u2014 online (current)';
                else if (pings > 0) tt += ' \u2014 ' + pings + ' pings';
                else if (pings === 0) tt += ' \u2014 DOWN';
                else tt += ' \u2014 no data';
                cell.title = tt;
                strip.appendChild(cell);
            });

            // --- Top Pages (with links to page detail and to view page) ---
            function renderStatList(data, container, emptyEl, opts) {
                if (!data || data.length === 0) { emptyEl.style.display = 'block'; return; }
                var maxVal = data[0][opts.valKey];
                data.forEach(function(item) {
                    var row = document.createElement('div');
                    row.className = 'stat-row';

                    var label = document.createElement('div');
                    label.className = 'stat-label';
                    if (opts.labelLink) {
                        var a = document.createElement('a');
                        a.href = opts.labelLink(item);
                        a.textContent = item[opts.labelKey];
                        a.title = item[opts.labelKey];
                        if (opts.labelTarget) a.target = opts.labelTarget;
                        label.appendChild(a);
                    } else {
                        label.textContent = item[opts.labelKey];
                        label.title = item[opts.labelKey];
                    }

                    var barWrap = document.createElement('div');
                    barWrap.className = 'stat-bar-wrap';
                    var bar = document.createElement('div');
                    bar.className = 'stat-bar ' + (opts.barClass || 'stat-bar-blue');
                    bar.style.width = (item[opts.valKey] / maxVal * 100) + '%';
                    barWrap.appendChild(bar);

                    var count = document.createElement('div');
                    count.className = 'stat-count';
                    count.textContent = item[opts.valKey].toLocaleString() + ' ' + opts.valLabel;

                    row.appendChild(label);
                    row.appendChild(barWrap);
                    row.appendChild(count);

                    if (opts.actions) {
                        var acts = document.createElement('div');
                        acts.className = 'stat-actions';
                        opts.actions(item).forEach(function(ac) {
                            var a = document.createElement('a');
                            a.href = ac.href;
                            a.textContent = ac.text;
                            a.title = ac.title || '';
                            if (ac.target) a.target = ac.target;
                            acts.appendChild(a);
                        });
                        row.appendChild(acts);
                    }

                    container.appendChild(row);
                });
            }

            function renderTopPages(which) {
                var container = document.getElementById('topPages');
                var emptyEl = document.getElementById('topPagesEmpty');
                container.innerHTML = '';
                emptyEl.style.display = 'none';
                renderStatList(topPagesData[which], container, emptyEl, {
                    labelKey: 'path', valKey: 'views', valLabel: 'views', barClass: 'stat-bar-blue',
                    labelLink: function(p) { return '/cm/analytics/page?path=' + encodeURIComponent(p.path) + '&range=' + range; },
                    actions: function(p) {
                        var acts = [{ href: p.path, text: '\u2197', title: 'View page', target: '_blank' }];
                        if (p.edit_id) acts.push({ href: '/cm/content/' + p.edit_id, text: '\u270E', title: 'Edit page' });
                        return acts;
                    }
                });
            }
            renderTopPages('human');

            // --- Top Referrers (tabbed: human/bot/all) ---
            function renderRefs(which) {
                var container = document.getElementById('topReferrers');
                var emptyEl = document.getElementById('topReferrersEmpty');
                container.innerHTML = '';
                emptyEl.style.display = 'none';
                renderStatList(refData[which], container, emptyEl, {
                    labelKey: 'domain', valKey: 'hits', valLabel: 'hits', barClass: 'stat-bar-green',
                    labelLink: function(r) { return '/cm/analytics/referrer?referrer=' + encodeURIComponent(r.domain) + '&range=' + range; }
                });
            }
            window.switchRefTab = function(which) {
                document.querySelectorAll('.ref-tab').forEach(function(b) { b.classList.remove('active'); });
                document.querySelector('[data-ref-tab="'+which+'"]').classList.add('active');
                renderRefs(which);
            };
            renderRefs('human');

            // --- User Agent Donut Chart with hover ---
            (function() {
                var canvas = document.getElementById('uaPieChart');
                var legend = document.getElementById('uaLegend');
                var emptyEl = document.getElementById('uaEmpty');
                if (!userAgents || userAgents.length === 0) {
                    canvas.style.display = 'none';
                    emptyEl.style.display = 'block';
                    return;
                }
                var colors = ['#60a5fa','#4ade80','#facc15','#f87171','#a78bfa','#fb923c','#2dd4bf','#e879f9','#94a3b8','#67e8f9','#fda4af','#86efac'];
                var total = 0;
                userAgents.forEach(function(u) { total += u.hits; });
                var cx = 120, cy = 110, R = 90, innerR = R * 0.55;

                // Pre-compute slice angles
                var slices = [];
                var angle = -Math.PI / 2;
                userAgents.forEach(function(u, i) {
                    var sweep = (u.hits / total) * Math.PI * 2;
                    slices.push({ start: angle, end: angle + sweep, ua: u, color: colors[i % colors.length], idx: i });
                    angle += sweep;
                });

                function drawChart(hoverIdx) {
                    var c = canvas.getContext('2d');
                    c.clearRect(0, 0, canvas.width, canvas.height);
                    slices.forEach(function(s) {
                        c.beginPath();
                        c.moveTo(cx, cy);
                        c.arc(cx, cy, s.idx === hoverIdx ? R + 4 : R, s.start, s.end);
                        c.closePath();
                        c.fillStyle = s.color;
                        c.globalAlpha = (hoverIdx >= 0 && s.idx !== hoverIdx) ? 0.5 : 1;
                        c.fill();
                        c.globalAlpha = 1;
                    });
                    // Center hole
                    c.beginPath();
                    c.arc(cx, cy, innerR, 0, Math.PI * 2);
                    c.fillStyle = '#1e293b';
                    c.fill();
                    // Center text
                    if (hoverIdx >= 0) {
                        var h = slices[hoverIdx].ua;
                        var pct = (h.hits / total * 100).toFixed(1);
                        c.fillStyle = slices[hoverIdx].color;
                        c.font = 'bold 15px Inter, sans-serif';
                        c.textAlign = 'center';
                        c.textBaseline = 'middle';
                        c.fillText(h.hits.toLocaleString(), cx, cy - 12);
                        c.fillStyle = '#e2e8f0';
                        c.font = '11px Inter, sans-serif';
                        c.fillText(h.category, cx, cy + 4);
                        c.fillStyle = '#94a3b8';
                        c.font = '10px Inter, sans-serif';
                        c.fillText(pct + '%', cx, cy + 18);
                    } else {
                        c.fillStyle = '#e2e8f0';
                        c.font = 'bold 16px Inter, sans-serif';
                        c.textAlign = 'center';
                        c.textBaseline = 'middle';
                        c.fillText(total.toLocaleString(), cx, cy - 6);
                        c.fillStyle = '#94a3b8';
                        c.font = '10px Inter, sans-serif';
                        c.fillText('views', cx, cy + 10);
                    }
                }
                drawChart(-1);

                function getSliceAt(e) {
                    var rect = canvas.getBoundingClientRect();
                    var mx = e.clientX - rect.left, my = e.clientY - rect.top;
                    var dx = mx - cx, dy = my - cy;
                    var dist = Math.sqrt(dx * dx + dy * dy);
                    if (dist < innerR || dist > R + 6) return -1;
                    var a = Math.atan2(dy, dx);
                    for (var i = 0; i < slices.length; i++) {
                        var s = slices[i];
                        // Normalize angles for comparison
                        var sa = s.start, ea = s.end;
                        var ta = a;
                        if (ta < sa) ta += Math.PI * 2;
                        if (ea < sa) ea += Math.PI * 2;
                        if (ta >= sa && ta < ea) return i;
                    }
                    return -1;
                }
                var lastHover = -1;
                canvas.addEventListener('mousemove', function(e) {
                    var idx = getSliceAt(e);
                    if (idx !== lastHover) { lastHover = idx; drawChart(idx); }
                    canvas.style.cursor = idx >= 0 ? 'pointer' : 'default';
                });
                canvas.addEventListener('mouseleave', function() {
                    lastHover = -1; drawChart(-1);
                    canvas.style.cursor = 'default';
                });

                // Legend
                var html = '';
                userAgents.forEach(function(u, i) {
                    var pct = total > 0 ? (u.hits / total * 100).toFixed(1) : '0';
                    html += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:3px;">';
                    html += '<span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:' + colors[i % colors.length] + ';flex-shrink:0;"></span>';
                    html += '<span style="flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">' + u.category + '</span>';
                    html += '<span style="color:#e2e8f0;">' + pct + '%</span>';
                    html += '</div>';
                });
                legend.innerHTML = html;
            })();
        })();
        </script>
    ` + adminLayoutEnd,

	"analytics_page": adminLayoutStart + `
        <style>
            .chart-box { background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; margin-bottom: 1.5rem; }
            .chart-title { margin: 0 0 1rem 0; font-size: 0.875rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
            .stat-row { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; font-size: 0.8125rem; }
            .stat-label { width: 220px; min-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #e2e8f0; }
            .stat-label a { color: #60a5fa; text-decoration: none; }
            .stat-label a:hover { text-decoration: underline; }
            .stat-bar-wrap { flex: 1; height: 22px; background: #1e293b; border-radius: 4px; overflow: hidden; }
            .stat-bar { height: 100%; border-radius: 4px; min-width: 2px; background: #4ade80; }
            .stat-count { width: 70px; text-align: right; color: #94a3b8; font-family: monospace; font-size: 0.75rem; }
            .stat-empty { color: var(--text-muted); font-size: 0.875rem; padding: 1rem 0; }
            .ref-tab { padding: 0.25rem 0.625rem; font-size: 0.75rem; background: transparent; border: 1px solid #475569; color: #94a3b8; cursor: pointer; border-radius: 4px; }
            .ref-tab:hover { color: #e2e8f0; border-color: #60a5fa; }
            .ref-tab.active { background: #60a5fa; color: #fff; border-color: #60a5fa; }
            .page-detail-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
            .page-detail-header h1 { margin: 0; font-size: 1.25rem; }
            .page-detail-path { font-family: monospace; color: #60a5fa; font-size: 0.875rem; max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
            .page-detail-meta { display: flex; gap: 1.5rem; font-size: 0.875rem; color: var(--text-muted); margin-bottom: 1.5rem; }
            .page-detail-meta strong { color: #e2e8f0; }
        </style>

        <div class="content-section">
            <div class="page-detail-header">
                <a href="/cm/analytics?range={{.Range}}" style="color: #94a3b8; text-decoration: none; font-size: 1.25rem;" title="Back to analytics">&larr;</a>
                <h1>Page Analytics</h1>
                <a href="{{.PagePath}}" target="_blank" class="page-detail-path" title="{{.PagePath}}">{{.PagePath}} &#x2197;</a>
                {{if .EditID}}<a href="/cm/content/{{.EditID}}" style="color: #94a3b8; text-decoration: none; font-size: 0.8rem; padding: 3px 8px; border: 1px solid #475569; border-radius: 4px;" title="Edit page">&#x270E; Edit</a>{{end}}
            </div>

            <div style="display: flex; gap: 0.5rem; margin-bottom: 1.5rem;">
                <a href="/cm/analytics/page?path={{.PagePath}}&range=24h" class="btn {{if eq .Range "24h"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">24 Hours</a>
                <a href="/cm/analytics/page?path={{.PagePath}}&range=7d" class="btn {{if eq .Range "7d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">7 Days</a>
                <a href="/cm/analytics/page?path={{.PagePath}}&range=30d" class="btn {{if eq .Range "30d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">30 Days</a>
                <a href="/cm/analytics/page?path={{.PagePath}}&range=60d" class="btn {{if eq .Range "60d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">60 Days</a>
                <a href="/cm/analytics/page?path={{.PagePath}}&range=90d" class="btn {{if eq .Range "90d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">90 Days</a>
                <form method="GET" action="/cm/analytics/page" style="display: inline-flex; gap: 6px; align-items: center; margin-left: 8px;">
                    <input type="hidden" name="range" value="custom">
                    <input type="hidden" name="path" value="{{.PagePath}}">
                    <input type="date" name="start" value="{{.RangeStart}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <span style="color: var(--text-muted);">–</span>
                    <input type="date" name="end" value="{{.RangeEnd}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <button type="submit" class="btn {{if eq .Range "custom"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">Apply</button>
                </form>
            </div>

            <div class="page-detail-meta">
                <span>Total views: <strong>{{.TotalViews}}</strong></span>
            </div>

            <div class="chart-box">
                <div style="display:flex;align-items:center;gap:1rem;margin-bottom:1rem;">
                    <h3 class="chart-title" style="margin:0;">Referrers</h3>
                    <div class="ref-tabs" style="display:flex;gap:2px;">
                        <button class="ref-tab active" data-ref-tab="human" onclick="switchRefTab('human')">Non-bot</button>
                        <button class="ref-tab" data-ref-tab="bot" onclick="switchRefTab('bot')">Bots</button>
                        <button class="ref-tab" data-ref-tab="all" onclick="switchRefTab('all')">All</button>
                    </div>
                </div>
                <div id="pageReferrers"></div>
                <div id="pageReferrersEmpty" class="stat-empty" style="display:none;">No referrer data for this page yet.</div>
            </div>
        </div>

        <script>
        (function() {
            var refData = {
                human: JSON.parse('{{.RefHumanJSON}}'),
                bot: JSON.parse('{{.RefBotJSON}}'),
                all: JSON.parse('{{.RefAllJSON}}')
            };
            function renderRefs(which) {
                var referrers = refData[which];
                var container = document.getElementById('pageReferrers');
                var emptyEl = document.getElementById('pageReferrersEmpty');
                container.innerHTML = '';
                emptyEl.style.display = 'none';
                if (!referrers || referrers.length === 0) { emptyEl.style.display = 'block'; return; }
                var maxHits = referrers[0].hits;
                referrers.forEach(function(r) {
                    var row = document.createElement('div');
                    row.className = 'stat-row';
                    var label = document.createElement('div');
                    label.className = 'stat-label';
                    var a = document.createElement('a');
                    a.href = '/cm/analytics/referrer?referrer=' + encodeURIComponent(r.domain) + '&range={{.Range}}';
                    a.textContent = r.domain;
                    a.title = r.domain;
                    label.appendChild(a);
                    var barWrap = document.createElement('div');
                    barWrap.className = 'stat-bar-wrap';
                    var bar = document.createElement('div');
                    bar.className = 'stat-bar';
                    bar.style.width = (r.hits / maxHits * 100) + '%';
                    barWrap.appendChild(bar);
                    var count = document.createElement('div');
                    count.className = 'stat-count';
                    count.textContent = r.hits.toLocaleString() + ' hits';
                    row.appendChild(label);
                    row.appendChild(barWrap);
                    row.appendChild(count);
                    container.appendChild(row);
                });
            }
            window.switchRefTab = function(which) {
                document.querySelectorAll('.ref-tab').forEach(function(b) { b.classList.remove('active'); });
                document.querySelector('[data-ref-tab="'+which+'"]').classList.add('active');
                renderRefs(which);
            };
            renderRefs('human');
        })();
        </script>
    ` + adminLayoutEnd,

	"analytics_referrer": adminLayoutStart + `
        <style>
            .chart-box { background: var(--bg-dark); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem; margin-bottom: 1.5rem; }
            .chart-title { margin: 0 0 1rem 0; font-size: 0.875rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
            .stat-row { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; font-size: 0.8125rem; }
            .stat-label { width: 220px; min-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #e2e8f0; }
            .stat-label a { color: #60a5fa; text-decoration: none; }
            .stat-label a:hover { text-decoration: underline; }
            .stat-bar-wrap { flex: 1; height: 22px; background: #1e293b; border-radius: 4px; overflow: hidden; }
            .stat-bar { height: 100%; border-radius: 4px; min-width: 2px; background: #60a5fa; }
            .stat-count { width: 70px; text-align: right; color: #94a3b8; font-family: monospace; font-size: 0.75rem; }
            .stat-actions { display: flex; gap: 0.375rem; width: 60px; flex-shrink: 0; }
            .stat-actions a { color: #94a3b8; text-decoration: none; font-size: 0.75rem; padding: 2px 5px; border-radius: 3px; border: 1px solid #475569; }
            .stat-actions a:hover { color: #e2e8f0; border-color: #60a5fa; }
            .stat-empty { color: var(--text-muted); font-size: 0.875rem; padding: 1rem 0; }
            .ref-tab { padding: 0.25rem 0.625rem; font-size: 0.75rem; background: transparent; border: 1px solid #475569; color: #94a3b8; cursor: pointer; border-radius: 4px; }
            .ref-tab:hover { color: #e2e8f0; border-color: #60a5fa; }
            .ref-tab.active { background: #60a5fa; color: #fff; border-color: #60a5fa; }
            .ref-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
            .ref-header h1 { margin: 0; font-size: 1.25rem; }
            .ref-domain { font-family: monospace; color: #4ade80; font-size: 1rem; }
            .ref-meta { display: flex; gap: 1.5rem; font-size: 0.875rem; color: var(--text-muted); margin-bottom: 1.5rem; align-items: center; flex-wrap: wrap; }
            .ref-meta strong { color: #e2e8f0; }
            .ref-visit-link { display: inline-flex; align-items: center; gap: 0.375rem; color: #60a5fa; text-decoration: none; font-size: 0.875rem; padding: 0.375rem 0.75rem; border: 1px solid #475569; border-radius: 6px; }
            .ref-visit-link:hover { border-color: #60a5fa; color: #93c5fd; }
        </style>

        <div class="content-section">
            <div class="ref-header">
                <a href="/cm/analytics?range={{.Range}}" style="color: #94a3b8; text-decoration: none; font-size: 1.25rem;" title="Back to analytics">&larr;</a>
                <h1>Referrer Report</h1>
                <span class="ref-domain">{{.Referrer}}</span>
            </div>

            <div style="display: flex; gap: 0.5rem; margin-bottom: 1.5rem;">
                <a href="/cm/analytics/referrer?referrer={{.Referrer}}&range=24h" class="btn {{if eq .Range "24h"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">24 Hours</a>
                <a href="/cm/analytics/referrer?referrer={{.Referrer}}&range=7d" class="btn {{if eq .Range "7d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">7 Days</a>
                <a href="/cm/analytics/referrer?referrer={{.Referrer}}&range=30d" class="btn {{if eq .Range "30d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">30 Days</a>
                <a href="/cm/analytics/referrer?referrer={{.Referrer}}&range=60d" class="btn {{if eq .Range "60d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">60 Days</a>
                <a href="/cm/analytics/referrer?referrer={{.Referrer}}&range=90d" class="btn {{if eq .Range "90d"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">90 Days</a>
                <form method="GET" action="/cm/analytics/referrer" style="display: inline-flex; gap: 6px; align-items: center; margin-left: 8px;">
                    <input type="hidden" name="range" value="custom">
                    <input type="hidden" name="referrer" value="{{.Referrer}}">
                    <input type="date" name="start" value="{{.RangeStart}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <span style="color: var(--text-muted);">–</span>
                    <input type="date" name="end" value="{{.RangeEnd}}" required style="padding: 0.3rem 0.5rem; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 0.8rem;">
                    <button type="submit" class="btn {{if eq .Range "custom"}}btn-primary{{else}}btn-secondary{{end}}" style="padding: 0.375rem 0.75rem; font-size: 0.875rem;">Apply</button>
                </form>
            </div>

            <div class="ref-meta">
                <span id="refHitsLabel">Total hits: <strong>{{.HitsHuman}}</strong></span>
                {{if and (ne .Referrer "(direct)") (ne .Referrer "(internal)")}}
                <a href="https://{{.Referrer}}" target="_blank" class="ref-visit-link">&#x2197; Visit {{.Referrer}}</a>
                {{end}}
            </div>

            <div class="chart-box">
                <div style="display:flex;align-items:center;gap:1rem;margin-bottom:1rem;">
                    <h3 class="chart-title" style="margin:0;">Top Pages from This Referrer</h3>
                    <div class="ref-tabs" style="display:flex;gap:2px;">
                        <button class="ref-tab active" data-ref-tab="human" onclick="switchRefTab('human')">Non-bot</button>
                        <button class="ref-tab" data-ref-tab="bot" onclick="switchRefTab('bot')">Bots</button>
                        <button class="ref-tab" data-ref-tab="all" onclick="switchRefTab('all')">All</button>
                    </div>
                </div>
                <div id="refPages"></div>
                <div id="refPagesEmpty" class="stat-empty" style="display:none;">No page data for this referrer yet.</div>
            </div>
        </div>

        <script>
        (function() {
            var pagesData = {
                human: JSON.parse('{{.PagesHumanJSON}}'),
                bot: JSON.parse('{{.PagesBotJSON}}'),
                all: JSON.parse('{{.PagesAllJSON}}')
            };
            var hitsData = { human: {{.HitsHuman}}, bot: {{.HitsBot}}, all: {{.HitsAll}} };
            var range = '{{.Range}}';

            function renderPages(which) {
                var topPages = pagesData[which];
                var container = document.getElementById('refPages');
                var emptyEl = document.getElementById('refPagesEmpty');
                container.innerHTML = '';
                emptyEl.style.display = 'none';
                document.getElementById('refHitsLabel').innerHTML = 'Total hits: <strong>' + hitsData[which].toLocaleString() + '</strong>';
                if (!topPages || topPages.length === 0) { emptyEl.style.display = 'block'; return; }
                var maxViews = topPages[0].views;
                topPages.forEach(function(p) {
                    var row = document.createElement('div');
                    row.className = 'stat-row';
                    var label = document.createElement('div');
                    label.className = 'stat-label';
                    var link = document.createElement('a');
                    link.href = '/cm/analytics/page?path=' + encodeURIComponent(p.path) + '&range=' + range;
                    link.textContent = p.path;
                    link.title = p.path;
                    label.appendChild(link);
                    var barWrap = document.createElement('div');
                    barWrap.className = 'stat-bar-wrap';
                    var bar = document.createElement('div');
                    bar.className = 'stat-bar';
                    bar.style.width = (p.views / maxViews * 100) + '%';
                    barWrap.appendChild(bar);
                    var count = document.createElement('div');
                    count.className = 'stat-count';
                    count.textContent = p.views.toLocaleString() + ' views';
                    var acts = document.createElement('div');
                    acts.className = 'stat-actions';
                    var viewLink = document.createElement('a');
                    viewLink.href = p.path;
                    viewLink.target = '_blank';
                    viewLink.textContent = '\u2197';
                    viewLink.title = 'View page';
                    acts.appendChild(viewLink);
                    if (p.edit_id) {
                        var editLink = document.createElement('a');
                        editLink.href = '/cm/content/' + p.edit_id;
                        editLink.textContent = '\u270E';
                        editLink.title = 'Edit page';
                        acts.appendChild(editLink);
                    }
                    row.appendChild(label);
                    row.appendChild(barWrap);
                    row.appendChild(count);
                    row.appendChild(acts);
                    container.appendChild(row);
                });
            }
            window.switchRefTab = function(which) {
                document.querySelectorAll('.ref-tab').forEach(function(b) { b.classList.remove('active'); });
                document.querySelector('[data-ref-tab="'+which+'"]').classList.add('active');
                renderPages(which);
            };
            renderPages('human');
        })();
        </script>
    ` + adminLayoutEnd,

	"audit_log": adminLayoutStart + `
        <div class="content-section">
            <h1>Audit Log</h1>
            <div style="margin-bottom: 1rem;">
                <form method="GET" action="/cm/audit" style="display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: end;">
                    <div>
                        <label style="font-size: 0.75rem; display: block; margin-bottom: 0.2rem; color: var(--text-muted);">Action</label>
                        <select name="action" style="padding: 0.375rem 0.625rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 0.875rem;">
                            <option value="">All</option>
                            <option value="login.success">Login</option>
                            <option value="login.failure">Failed Login</option>
                            <option value="password.change">Password Change</option>
                            <option value="user.create">User Created</option>
                            <option value="content.create">Content Created</option>
                            <option value="content.update">Content Updated</option>
                            <option value="content.delete">Content Deleted</option>
                            <option value="chat.query">Chat Query</option>
                        </select>
                    </div>
                    <div>
                        <label style="font-size: 0.75rem; display: block; margin-bottom: 0.2rem; color: var(--text-muted);">Since</label>
                        <input type="date" name="since" style="padding: 0.375rem 0.625rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 0.875rem;">
                    </div>
                    <div>
                        <label style="font-size: 0.75rem; display: block; margin-bottom: 0.2rem; color: var(--text-muted);">Until</label>
                        <input type="date" name="until" style="padding: 0.375rem 0.625rem; background: var(--bg-dark); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 0.875rem;">
                    </div>
                    <button type="submit" class="btn btn-primary" style="height: fit-content; padding: 0.375rem 0.875rem; font-size: 0.875rem;">Filter</button>
                </form>
            </div>
            <p style="color: var(--text-muted); font-size: 0.8rem; margin-bottom: 0.75rem;">{{.Total}} entries &middot; page {{.CurrentPage}} of {{.TotalPages}}</p>
            <table class="data-table" style="font-size: 0.8125rem;">
                <thead>
                    <tr>
                        <th style="width: 9rem;">Time</th>
                        <th style="width: 11rem;">User</th>
                        <th style="width: 10rem;">Action</th>
                        <th style="width: 8rem;">Resource</th>
                        <th>Details</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Logs}}
                    <tr style="vertical-align: top;">
                        <td style="white-space: nowrap; padding: 0.4rem 0.625rem; color: var(--text-muted);">{{.CreatedAt.Format "Jan 2 15:04:05"}}</td>
                        <td style="padding: 0.4rem 0.625rem; max-width: 11rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{.UserEmail}}{{if .ViaAPI}} <span style="font-size:0.7rem;color:var(--text-muted);opacity:0.7;">(api)</span>{{end}}</td>
                        <td style="padding: 0.4rem 0.625rem;"><code style="font-size: 0.75rem;">{{.Action}}</code></td>
                        <td style="padding: 0.4rem 0.625rem;">{{.Resource}}{{if .ResourceID}}<br><span style="color:var(--text-muted);font-size:0.7rem;font-family:monospace;">{{slice .ResourceID 0 8}}…</span>{{end}}</td>
                        <td style="padding: 0.4rem 0.625rem;">
                            {{if .Details}}
                            <span class="audit-details-preview">{{range $k, $v := .Details}}<span style="color:var(--text-muted);">{{$k}}:</span> {{$v}} {{end}}</span>
                            <a href="#" class="audit-more-link" style="display:none; font-size:0.75rem; color:var(--primary); white-space:nowrap;" data-details="{{range $k, $v := .Details}}{{$k}}: {{$v}}&#10;{{end}}">More ›</a>
                            {{end}}
                        </td>
                    </tr>
                {{end}}
                </tbody>
            </table>
            {{if gt .TotalPages 1}}
            <div style="margin-top: 0.75rem; display: flex; gap: 0.5rem;">
                {{if gt .CurrentPage 1}}<a href="/cm/audit?page={{subtract .CurrentPage 1}}" class="btn btn-outline">Previous</a>{{end}}
                {{if lt .CurrentPage .TotalPages}}<a href="/cm/audit?page={{add .CurrentPage 1}}" class="btn btn-outline">Next</a>{{end}}
            </div>
            {{end}}
        </div>

        <!-- Rate Limits section -->
        {{if .LoginAttempts}}
        <div style="margin-top:2rem;">
            <h2 style="margin-bottom:1rem;">Rate Limits</h2>
            <div class="table-container">
                <table style="font-size:0.8125rem;">
                    <thead>
                        <tr>
                            <th>IP Address</th>
                            <th>Attempts</th>
                            <th>Last Attempt</th>
                            <th>Locked Until</th>
                            <th>Action</th>
                        </tr>
                    </thead>
                    <tbody>
                    {{range .LoginAttempts}}
                    <tr>
                        <td><code>{{.IP}}</code></td>
                        <td>{{.Attempts}}</td>
                        <td style="color:var(--text-muted);">{{.LastAttempt.Format "Jan 2 15:04:05"}}</td>
                        <td>{{if .LockedUntil}}<span class="content-status draft">{{.LockedUntil.Format "15:04:05"}}</span>{{else}}-{{end}}</td>
                        <td>
                            <form method="POST" action="/cm/audit/ratelimits/{{.IP}}/clear" style="display:inline;">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-secondary">Clear</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                    </tbody>
                </table>
            </div>
        </div>
        {{end}}

        <!-- Details modal -->
        <div id="audit-modal" style="display:none; position:fixed; inset:0; background:rgba(0,0,0,0.6); z-index:1000; align-items:center; justify-content:center;">
            <div style="background:var(--bg-card); border:1px solid var(--border); border-radius:12px; padding:1.5rem; max-width:560px; width:90%; max-height:80vh; overflow-y:auto; position:relative;">
                <button onclick="document.getElementById('audit-modal').style.display='none'" style="position:absolute; top:0.75rem; right:1rem; background:none; border:none; color:var(--text-muted); font-size:1.25rem; cursor:pointer;">&times;</button>
                <h3 style="margin-bottom:1rem; font-size:1rem;">Entry Details</h3>
                <pre id="audit-modal-body" style="white-space:pre-wrap; font-size:0.8125rem; color:var(--text); font-family:monospace; line-height:1.6;"></pre>
            </div>
        </div>

        <style>
        .audit-details-preview {
            display: -webkit-box;
            -webkit-line-clamp: 2;
            -webkit-box-orient: vertical;
            overflow: hidden;
            font-size: 0.8rem;
            color: var(--text-muted);
            line-height: 1.4;
        }
        .audit-details-preview.clipped + .audit-more-link { display: inline !important; }
        </style>
        <script>
        // Show "More ›" only on rows whose preview text actually overflows 2 lines
        document.querySelectorAll('.audit-details-preview').forEach(function(el) {
            if (el.scrollHeight > el.clientHeight + 2) {
                el.classList.add('clipped');
            }
        });
        // Modal open
        document.querySelectorAll('.audit-more-link').forEach(function(a) {
            a.addEventListener('click', function(e) {
                e.preventDefault();
                document.getElementById('audit-modal-body').textContent = this.dataset.details;
                var modal = document.getElementById('audit-modal');
                modal.style.display = 'flex';
            });
        });
        // Close on backdrop click
        document.getElementById('audit-modal').addEventListener('click', function(e) {
            if (e.target === this) this.style.display = 'none';
        });
        </script>
    ` + adminLayoutEnd,

	"snippets_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Snippets</h1>
            {{if index $.Can "template.edit"}}<a href="/cm/snippets/new" class="btn btn-primary">+ New Snippet</a>{{end}}
        </div>
        <p class="help-text" style="margin-bottom: 1.5rem;">Snippets are reusable HTML templates used to render individual items in <code>lc:query</code> index pages. Reference them by name in your template layouts.</p>

        {{if .Snippets}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Updated</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Snippets}}
                    <tr>
                        <td><strong><code>{{.Name}}</code></strong></td>
                        <td>{{.UpdatedAt.Format "Jan 2, 2006"}}</td>
                        <td style="display:flex; gap:0.5rem; justify-content:flex-end;">
                            <a href="/cm/snippets/{{.ID.Hex}}" class="btn btn-outline btn-sm">Edit</a>
                            <form method="POST" action="/cm/snippets/{{.ID.Hex}}/delete" style="display:inline;" data-confirm="Delete snippet '{{.Name}}'? Pages that include it will render without it." data-confirm-title="Delete Snippet">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-danger btn-sm delete-btn">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="empty-state">
            <p>No snippets yet. Create one to use with <code>lc:query</code> directives in your templates.</p>
        </div>
        {{end}}
    ` + adminLayoutEnd,

	"snippet_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{.Title}}</h1>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}

        <form method="POST" class="form-card">
            {{.CSRFField}}
            <div class="form-section">
                <div class="form-group">
                    <label for="name">Name <span class="required">*</span></label>
                    <input type="text" id="name" name="name" required placeholder="e.g., glossary-card, blog-card" value="{{if .Snippet}}{{.Snippet.Name}}{{end}}">
                    <p class="help-text">Used in <code>lc:query</code> directives: <code>snippet="your-name-here"</code></p>
                </div>
                <div class="form-group">
                    <label for="html">HTML Template <span class="required">*</span></label>
                    <textarea id="html" name="html" rows="20" style="font-family: monospace; font-size: 0.875rem;">{{if .Snippet}}{{.Snippet.HTML}}{{end}}</textarea>
                    <p class="help-text">Go template. Available fields: <code>.Title</code>, <code>.FullPath</code>, <code>.Tags</code>, <code>.MetaDescription</code>, <code>.Category</code>, <code>.Data</code> (map of template field values).</p>
                </div>
            </div>
            <div class="form-actions">
                <a href="/cm/snippets" class="btn btn-outline">Cancel</a>
                <button type="submit" class="btn btn-primary">{{if .Snippet}}Save Changes{{else}}Create Snippet{{end}}</button>
            </div>
        </form>
    ` + adminLayoutEnd,

	"forks_list": adminLayoutStart + `
        <div class="page-header">
            <h1>Content Forks</h1>
            <a href="/cm/forks/new" class="btn btn-primary">+ New Fork</a>
        </div>
        <p class="help-text" style="margin-bottom:1.5rem;">Forks are isolated workspaces where you can draft changes to multiple pages and preview them before merging into the live site. Editors can create and work in forks; only admins can merge.</p>
        {{if .ForkPageID}}
        <div class="alert alert-info" style="margin-bottom:1.5rem;background:rgba(99,102,241,0.15);border:1px solid rgba(99,102,241,0.3);padding:1rem;border-radius:var(--radius)">
            <strong>Select a fork</strong> to add this page to, or <a href="/cm/forks/new">create a new fork</a>.
        </div>
        {{end}}
        {{if .Forks}}
        <div class="table-container">
            <table class="table">
                <thead><tr>
                    <th>Name</th>
                    <th>Description</th>
                    <th>Pages</th>
                    <th>Status</th>
                    <th>Created By</th>
                    <th>Created</th>
                    <th>Actions</th>
                </tr></thead>
                <tbody>
                {{range .Forks}}
                <tr>
                    <td><a href="/cm/forks/{{.ID.Hex}}">{{.Name}}</a></td>
                    <td style="color:var(--text-muted)">{{.Description}}</td>
                    <td>{{.PageCount}}</td>
                    <td><span class="badge badge-{{if eq .Status "active"}}success{{else if eq .Status "merged"}}info{{else}}warning{{end}}">{{.Status}}</span></td>
                    <td style="color:var(--text-muted);font-size:0.85rem">{{.CreatedByEmail}}</td>
                    <td style="color:var(--text-muted);font-size:0.85rem">{{.CreatedAt.Format "Jan 2, 2006"}}</td>
                    <td>
                        <a href="/cm/forks/{{.ID.Hex}}" class="btn btn-sm btn-outline">View</a>
                        {{if eq .Status "active"}}
                        <a href="/cm/forks/{{.ID.Hex}}/preview" class="btn btn-sm">👁 Preview</a>
                        {{if $.ForkPageID}}
                        <form method="POST" action="/cm/forks/{{.ID.Hex}}/fork-page" style="display:inline">
                            {{$.CSRFField}}
                            <input type="hidden" name="content_id" value="{{$.ForkPageID}}">
                            <button type="submit" class="btn btn-sm btn-primary">+ Add to this fork</button>
                        </form>
                        {{end}}
                        {{end}}
                    </td>
                </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="empty-state">
            <p>No forks yet. Create one to start staging changes.</p>
            <a href="/cm/forks/new" class="btn btn-primary">Create your first fork</a>
        </div>
        {{end}}
    ` + adminLayoutEnd,

	"fork_form": adminLayoutStart + `
        <div class="page-header">
            <h1>New Fork</h1>
            <a href="/cm/forks" class="btn btn-outline">← Back</a>
        </div>
        {{if .Error}}<div class="alert alert-danger">{{.Error}}</div>{{end}}
        <div class="card">
            <form method="POST" action="/cm/forks/new">
                {{.CSRFField}}
                <div class="form-group">
                    <label class="form-label">Fork Name *</label>
                    <input type="text" name="name" class="form-control" placeholder="e.g. Q2 Redesign, Holiday Campaign" required autofocus>
                    <span class="help-text">A short name to identify this set of changes.</span>
                </div>
                <div class="form-group">
                    <label class="form-label">Description</label>
                    <textarea name="description" class="form-control" rows="3" placeholder="Optional notes about what this fork is for"></textarea>
                </div>
                <div class="form-actions">
                    <a href="/cm/forks" class="btn btn-outline">Cancel</a>
                    <button type="submit" class="btn btn-primary">Create Fork</button>
                </div>
            </form>
        </div>
    ` + adminLayoutEnd,

	"fork_detail": adminLayoutStart + `
        <style>
            /* The action buttons keep their size on one line. A name too long
               to sit beside them takes the full width and the buttons drop
               below it as a row; a short name keeps them on its right. */
            .fork-header { flex-wrap: wrap; gap: 1rem; }
            .fork-header-title { flex: 1 1 auto; min-width: 0; max-width: 100%; }
            .fork-header-title h1, .fork-header-title p { overflow-wrap: anywhere; }
            .fork-header-actions { display: flex; gap: 0.75rem; align-items: center; flex-wrap: wrap; justify-content: flex-end; flex-shrink: 0; max-width: 100%; margin-left: auto; }
            .fork-header-actions .btn { white-space: nowrap; }
        </style>
        <div class="page-header fork-header">
            <div class="fork-header-title">
                <h1>🌿 {{.Fork.Name}}</h1>
                {{if .Fork.Description}}<p style="color:var(--text-muted);margin-top:0.25rem">{{.Fork.Description}}</p>{{end}}
            </div>
            <div class="fork-header-actions">
                {{if eq .Fork.Status "active"}}
                <a href="/cm/forks/{{.Fork.ID.Hex}}/preview" class="btn btn-secondary">👁 Start Preview</a>
                {{if .CanMerge}}
                <form method="POST" action="/cm/forks/{{.Fork.ID.Hex}}/merge" style="display:flex;gap:0.75rem;align-items:center;margin:0" data-confirm="Merge this fork into the live site? This will update all matching pages and cannot be undone." data-confirm-title="Merge fork">
                    {{.CSRFField}}
                    <label class="checkbox-label" style="margin:0;font-size:0.85rem;white-space:nowrap" title="New pages are created as drafts unless this is checked. Pages on hold always stay drafts.">
                        <input type="checkbox" name="publish_new">
                        Publish new pages
                    </label>
                    <button type="submit" class="btn btn-primary">Merge into Live →</button>
                </form>
                <form method="POST" action="/cm/forks/{{.Fork.ID.Hex}}/archive" style="margin:0" data-confirm="Archive this fork without merging?" data-confirm-title="Archive fork">
                    {{.CSRFField}}
                    <button type="submit" class="btn btn-outline">Archive</button>
                </form>
                {{end}}
                {{end}}
                <a href="/cm/forks" class="btn btn-outline">← All Forks</a>
            </div>
        </div>

        <div style="display:flex;gap:1rem;margin-bottom:1.5rem;flex-wrap:wrap">
            {{if and (eq .Fork.Status "merged") (not .Pages)}}
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:2rem;font-weight:700;color:var(--primary)">{{.Fork.MergedUpdated}}</div>
                <div style="color:var(--text-muted);font-size:0.85rem">Pages updated</div>
            </div>
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:2rem;font-weight:700;color:var(--primary)">{{.Fork.MergedCreated}}</div>
                <div style="color:var(--text-muted);font-size:0.85rem">Pages created</div>
            </div>
            {{else}}
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:2rem;font-weight:700;color:var(--primary)">{{len .Pages}}</div>
                <div style="color:var(--text-muted);font-size:0.85rem">Pages in fork</div>
            </div>
            {{end}}
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:1rem;font-weight:600">
                    <span class="badge badge-{{if eq .Fork.Status "active"}}success{{else if eq .Fork.Status "merged"}}info{{else}}warning{{end}}">{{.Fork.Status}}</span>
                </div>
                <div style="color:var(--text-muted);font-size:0.85rem;margin-top:0.5rem">Status</div>
            </div>
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:0.9rem;font-weight:500">{{.Fork.CreatedByEmail}}</div>
                <div style="color:var(--text-muted);font-size:0.85rem;margin-top:0.25rem">{{.Fork.CreatedAt.Format "Jan 2, 2006"}}</div>
                <div style="color:var(--text-muted);font-size:0.75rem">Created by</div>
            </div>
            {{if .Fork.MergedByEmail}}
            <div class="card" style="flex:1;min-width:160px;padding:1rem;text-align:center">
                <div style="font-size:0.9rem;font-weight:500">{{.Fork.MergedByEmail}}</div>
                <div style="color:var(--text-muted);font-size:0.85rem;margin-top:0.25rem">{{.Fork.MergedAt.Format "Jan 2, 2006"}}</div>
                <div style="color:var(--text-muted);font-size:0.75rem">Merged by</div>
            </div>
            {{end}}
        </div>

        <div class="page-header" style="margin-top:2rem">
            <h2 style="font-size:1.25rem">Pages in this fork</h2>
            {{if eq .Fork.Status "active"}}
            <a href="/cm/content?fork={{.Fork.ID.Hex}}" class="btn btn-outline btn-sm">+ Fork a live page</a>
            {{end}}
        </div>

        {{if .Pages}}
        <div class="table-container">
            <table class="table">
                <thead><tr>
                    <th>Page</th>
                    <th>Path</th>
                    <th>Template</th>
                    <th>Last Updated</th>
                    {{if eq .Fork.Status "active"}}<th>Actions</th>{{end}}
                </tr></thead>
                <tbody>
                {{range .Pages}}
                <tr>
                    <td><strong>{{.Title}}</strong></td>
                    <td style="font-family:monospace;font-size:0.85rem;color:var(--text-muted)">{{.FullPath}}</td>
                    <td style="color:var(--text-muted);font-size:0.85rem">{{.TemplateName}}</td>
                    <td style="color:var(--text-muted);font-size:0.85rem">{{.UpdatedAt.Format "Jan 2, 2006 15:04"}}</td>
                    {{if eq $.Fork.Status "active"}}
                    <td>
                        <a href="/cm/content/{{.ID.Hex}}?fork={{$.Fork.ID.Hex}}" class="btn btn-sm btn-outline">Edit</a>
                        <form method="POST" action="/cm/forks/{{$.Fork.ID.Hex}}/pages/{{.ID.Hex}}/remove" style="display:inline" data-confirm="Remove this page from the fork?" data-confirm-title="Remove page">
                            {{$.CSRFField}}
                            <button type="submit" class="btn btn-sm btn-danger">Remove</button>
                        </form>
                    </td>
                    {{end}}
                </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="empty-state">
            {{if eq .Fork.Status "merged"}}
            <p>This fork was merged: {{.Fork.MergedUpdated}} updated, {{.Fork.MergedCreated}} created. Its page copies were removed after the merge — the changes now live in the site's pages.</p>
            {{else}}
            <p>No pages in this fork yet.</p>
            {{end}}
            {{if eq .Fork.Status "active"}}
            <p style="color:var(--text-muted);font-size:0.9rem">Go to <a href="/cm/content">Content</a>, open a page, and click "Fork to workspace" to add it here.</p>
            {{end}}
        </div>
        {{end}}
    ` + adminLayoutEnd,

	"fork_merge_result": adminLayoutStart + `
        <div class="page-header">
            <h1>Merge Complete</h1>
            <a href="/cm/forks" class="btn btn-outline">← All Forks</a>
        </div>
        {{if .Error}}
        <div class="alert alert-danger">{{.Error}}</div>
        {{else}}
        <div class="alert alert-success" style="margin-bottom:1.5rem">
            Fork successfully merged into the live site.
        </div>
        <div style="display:flex;gap:1rem;flex-wrap:wrap;margin-bottom:2rem">
            <div class="card" style="flex:1;min-width:140px;padding:1.25rem;text-align:center">
                <div style="font-size:2.5rem;font-weight:700;color:var(--success)">{{.Result.Updated}}</div>
                <div style="color:var(--text-muted)">Pages updated</div>
            </div>
            <div class="card" style="flex:1;min-width:140px;padding:1.25rem;text-align:center">
                <div style="font-size:2.5rem;font-weight:700;color:var(--primary)">{{.Result.Created}}</div>
                <div style="color:var(--text-muted)">Pages created</div>
            </div>
            <div class="card" style="flex:1;min-width:140px;padding:1.25rem;text-align:center">
                <div style="font-size:2.5rem;font-weight:700;color:{{if .Result.Conflicts}}#f59e0b{{else}}var(--success){{end}}">{{len .Result.Conflicts}}</div>
                <div style="color:var(--text-muted)">Conflicts (fork won)</div>
            </div>
        </div>
        {{if .Result.Created}}
        <p class="help-text" style="margin-bottom:1.5rem">{{if .PublishNew}}New pages were published.{{else}}New pages keep the publish state they had in the fork; drafts stay drafts until you publish them.{{end}}</p>
        {{end}}
        {{if .Result.NotPublished}}
        <div class="card" style="margin-bottom:1.5rem">
            <h3 style="margin-bottom:1rem">New pages left as drafts</h3>
            <table class="table">
                <thead><tr><th>Path</th><th>Reason</th></tr></thead>
                <tbody>
                {{range .Result.NotPublished}}
                <tr>
                    <td style="font-family:monospace;font-size:0.85rem"><a href="/cm/content/{{.ID.Hex}}">{{.Path}}</a></td>
                    <td>{{.Reason}}</td>
                </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
        {{if .Result.Conflicts}}
        <div class="card">
            <h3 style="margin-bottom:1rem;color:#f59e0b">⚠️ Conflicts — Fork Overwrote Live Changes</h3>
            <p style="color:var(--text-muted);margin-bottom:1rem;font-size:0.9rem">These pages were modified on the live site after the fork was created. The fork version was applied. Review these pages to ensure nothing was lost.</p>
            <table class="table">
                <thead><tr><th>Page</th><th>Path</th></tr></thead>
                <tbody>
                {{range .Result.Conflicts}}
                <tr>
                    <td>{{.LiveTitle}}</td>
                    <td style="font-family:monospace;font-size:0.85rem"><a href="{{.LivePath}}">{{.LivePath}}</a></td>
                </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
        {{end}}
    ` + adminLayoutEnd,

	"webhooks_list": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Webhooks</h1>
                <p class="page-subtitle">Receive HTTP notifications when content events occur.</p>
            </div>
            <div style="display:flex;gap:0.75rem;">
                <a href="/cm/webhooks/docs" class="btn btn-secondary">Docs</a>
                <a href="/cm/webhooks/new" class="btn btn-primary">+ New Webhook</a>
            </div>
        </div>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>URL</th>
                        <th>Secret</th>
                        <th>Events</th>
                        <th>Status</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Webhooks}}
                <tr>
                    <td>{{.Name}}</td>
                    <td style="font-family:monospace;font-size:0.8rem;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{.URL}}</td>
                    <td style="font-family:monospace;font-size:0.85rem;color:var(--text-muted);">••••••••</td>
                    <td style="font-size:0.8rem;">{{join .Events ", "}}</td>
                    <td>{{if .Active}}<span class="content-status published">Active</span>{{else}}<span class="content-status draft">Inactive</span>{{end}}</td>
                    <td class="actions">
                        <a href="/cm/webhooks/{{.ID.Hex}}/edit" class="btn btn-sm btn-secondary">Edit</a>
                        <a href="/cm/webhooks/{{.ID.Hex}}/deliveries" class="btn btn-sm btn-secondary">Deliveries</a>
                        <form method="POST" action="/cm/webhooks/{{.ID.Hex}}/delete" style="display:inline;" data-confirm="Delete this webhook?" data-confirm-title="Delete Webhook">
                            {{$.CSRFField}}
                            <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                        </form>
                    </td>
                </tr>
                {{else}}
                <tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:2rem;">No webhooks configured yet. <a href="/cm/webhooks/new">Create one</a>.</td></tr>
                {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"webhook_form": adminLayoutStart + `
        <div class="page-header">
            <h1>{{if .IsNew}}New Webhook{{else}}Edit Webhook{{end}}</h1>
            <a href="/cm/webhooks" class="btn btn-secondary">Back</a>
        </div>
        {{if .Error}}<div class="error-message">{{.Error}}</div>{{end}}
        {{if .Success}}<div class="success-message">{{.Success}}</div>{{end}}
        {{if .CreatedSecret}}
        <div style="background:rgba(239,68,68,0.1);border:2px solid rgba(239,68,68,0.4);border-radius:var(--radius);padding:1.25rem;margin-bottom:1.5rem;">
            <strong style="color:#f87171;">This secret will never be shown again. Save it now.</strong>
            <div style="margin-top:0.75rem;display:flex;align-items:center;gap:0.75rem;">
                <code id="webhook-secret-value" style="font-family:monospace;background:rgba(0,0,0,0.3);padding:0.75rem;border-radius:8px;word-break:break-all;font-size:0.9rem;flex:1;">{{.CreatedSecret}}</code>
                <button type="button" class="btn btn-secondary" onclick="navigator.clipboard.writeText(document.getElementById('webhook-secret-value').textContent)">Copy</button>
            </div>
        </div>
        {{end}}
        {{if .RegeneratedSecret}}
        <div style="background:rgba(239,68,68,0.1);border:2px solid rgba(239,68,68,0.4);border-radius:var(--radius);padding:1.25rem;margin-bottom:1.5rem;">
            <strong style="color:#f87171;">New secret generated — this will never be shown again. Save it now.</strong>
            <div style="margin-top:0.75rem;display:flex;align-items:center;gap:0.75rem;">
                <code id="webhook-secret-value" style="font-family:monospace;background:rgba(0,0,0,0.3);padding:0.75rem;border-radius:8px;word-break:break-all;font-size:0.9rem;flex:1;">{{.RegeneratedSecret}}</code>
                <button type="button" class="btn btn-secondary" onclick="navigator.clipboard.writeText(document.getElementById('webhook-secret-value').textContent)">Copy</button>
            </div>
        </div>
        {{end}}
        <form method="POST" action="{{if .IsNew}}/cm/webhooks{{else}}/cm/webhooks/{{.Webhook.ID.Hex}}{{end}}">
            {{.CSRFField}}
            <div class="form-group">
                <label>Name</label>
                <input type="text" name="name" value="{{if .Webhook}}{{.Webhook.Name}}{{end}}" placeholder="My Webhook" required>
            </div>
            <div class="form-group">
                <label>URL</label>
                <input type="text" name="url" value="{{if .Webhook}}{{.Webhook.URL}}{{end}}" placeholder="https://example.com/webhook" required>
            </div>
            {{if .Webhook}}
            <div class="form-group">
                <label>Secret (HMAC-SHA256 signing key)</label>
                <div style="display:flex;align-items:center;gap:0.75rem;">
                    <input type="text" value="••••••••" readonly style="flex:1;color:var(--text-muted);cursor:default;">
                    <button type="submit" form="regenerate-secret-form" class="btn btn-secondary">Regenerate Secret</button>
                </div>
                <p style="color:var(--text-muted);font-size:0.8rem;margin-top:0.25rem;">The secret is masked for security. Use the Regenerate button to create a new one.</p>
            </div>
            {{end}}
            <div class="form-group">
                <label>Events</label>
                <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:0.5rem;margin-top:0.5rem;">
                {{range split "content.create content.update content.publish content.unpublish content.delete comment.created content.pending_approval asset.pending_review" " "}}
                <label style="display:flex;align-items:center;gap:0.5rem;font-weight:normal;cursor:pointer;">
                    <input type="checkbox" name="events" value="{{.}}"
                        {{if $.Webhook}}{{$ev := .}}{{range $.Webhook.Events}}{{if eq . $ev}}checked{{end}}{{end}}{{end}}>
                    <code style="font-size:0.8rem;">{{.}}</code>
                </label>
                {{end}}
                </div>
            </div>
            <div class="form-group">
                <label style="display:flex;align-items:center;gap:0.75rem;cursor:pointer;">
                    <input type="checkbox" name="active" value="on" {{if .IsNew}}checked{{else if .Webhook}}{{if .Webhook.Active}}checked{{end}}{{end}}>
                    Active
                </label>
            </div>
            <div style="display:flex;gap:0.75rem;margin-top:1.5rem;">
                <button type="submit" class="btn btn-primary">{{if .IsNew}}Create Webhook{{else}}Save Changes{{end}}</button>
                <a href="/cm/webhooks" class="btn btn-secondary">Cancel</a>
            </div>
        </form>
        {{if .Webhook}}
        <!-- Separate form (a form cannot nest inside the edit form); the Regenerate button points here with form= -->
        <form id="regenerate-secret-form" method="POST" action="/cm/webhooks/{{.Webhook.ID.Hex}}/regenerate-secret" data-confirm="Regenerate secret? The old secret will stop working immediately." data-confirm-title="Regenerate Secret">
            {{.CSRFField}}
        </form>
        {{end}}
    ` + adminLayoutEnd,

	"webhook_deliveries": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Deliveries: {{.Webhook.Name}}</h1>
                <p class="page-subtitle">{{.Webhook.URL}}</p>
            </div>
            <a href="/cm/webhooks" class="btn btn-secondary">Back to Webhooks</a>
        </div>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Time</th>
                        <th>Event</th>
                        <th>Attempt</th>
                        <th>Status Code</th>
                        <th>Result</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Deliveries}}
                <tr>
                    <td style="white-space:nowrap;font-size:0.8rem;color:var(--text-muted);">{{.CreatedAt.Format "Jan 2 15:04:05"}}</td>
                    <td><code style="font-size:0.8rem;">{{.Event}}</code></td>
                    <td style="text-align:center;">{{.Attempt}}</td>
                    <td>{{if .StatusCode}}<code style="font-size:0.8rem;">{{.StatusCode}}</code>{{else}}-{{end}}</td>
                    <td>{{if .Success}}<span class="content-status published">Success</span>{{else}}<span class="content-status draft">{{if .Error}}{{.Error}}{{else}}Failed{{end}}</span>{{end}}</td>
                </tr>
                {{else}}
                <tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:2rem;">No deliveries recorded yet.</td></tr>
                {{end}}
                </tbody>
            </table>
        </div>
    ` + adminLayoutEnd,

	"webhook_docs": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Webhook Documentation</h1>
                <p class="page-subtitle">Everything you need to receive and verify LightCMS webhook events.</p>
            </div>
            <a href="/cm/webhooks" class="btn btn-secondary">Back to Webhooks</a>
        </div>
        <div style="max-width:800px;">
            <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:1.5rem;margin-bottom:1.5rem;">
                <h2 style="margin-bottom:1rem;">Events</h2>
                <table>
                    <thead><tr><th>Event</th><th>When it fires</th></tr></thead>
                    <tbody>
                        <tr><td><code>content.create</code></td><td>A new content item is created</td></tr>
                        <tr><td><code>content.update</code></td><td>An existing content item is updated</td></tr>
                        <tr><td><code>content.publish</code></td><td>Content is published (goes live)</td></tr>
                        <tr><td><code>content.unpublish</code></td><td>Content is unpublished (taken down)</td></tr>
                        <tr><td><code>content.delete</code></td><td>Content is deleted</td></tr>
                        <tr><td><code>comment.created</code></td><td>A discussion comment is posted on a content item</td></tr>
                        <tr><td><code>content.pending_approval</code></td><td>A contributor submits content for editorial approval</td></tr>
                        <tr><td><code>asset.pending_review</code></td><td>A contributor uploads an asset pending review</td></tr>
                    </tbody>
                </table>
            </div>
            <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:1.5rem;margin-bottom:1.5rem;">
                <h2 style="margin-bottom:1rem;">Payload Format</h2>
                <pre style="background:rgba(0,0,0,0.3);padding:1rem;border-radius:8px;font-size:0.85rem;overflow-x:auto;">{
  "event": "content.publish",
  "timestamp": "2026-03-24T10:00:00Z",
  "data": {
    "id": "60c72b2f9b1d8c001f647c1e",
    "title": "My Blog Post",
    "path": "/blog/my-blog-post"
  }
}</pre>
            </div>
            <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:1.5rem;margin-bottom:1.5rem;">
                <h2 style="margin-bottom:1rem;">Signature Verification</h2>
                <p style="margin-bottom:1rem;color:var(--text-muted);">Each request includes an <code>X-LightCMS-Signature</code> header (format: <code>sha256=&lt;hex&gt;</code>). Verify it to ensure the request is authentic.</p>
                <h3 style="margin:1rem 0 0.5rem;">Go</h3>
                <pre style="background:rgba(0,0,0,0.3);padding:1rem;border-radius:8px;font-size:0.85rem;overflow-x:auto;">func verify(secret, signature string, body []byte) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(signature))
}</pre>
                <h3 style="margin:1rem 0 0.5rem;">Python</h3>
                <pre style="background:rgba(0,0,0,0.3);padding:1rem;border-radius:8px;font-size:0.85rem;overflow-x:auto;">import hmac, hashlib
def verify(secret, signature, body):
    expected = "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)</pre>
                <h3 style="margin:1rem 0 0.5rem;">Node.js</h3>
                <pre style="background:rgba(0,0,0,0.3);padding:1rem;border-radius:8px;font-size:0.85rem;overflow-x:auto;">const crypto = require('crypto');
function verify(secret, signature, body) {
    const expected = 'sha256=' + crypto.createHmac('sha256', secret).update(body).digest('hex');
    return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(signature));
}</pre>
            </div>
        </div>
    ` + adminLayoutEnd,

	// ---------------------------------------------------------------------------
	// Import Pipeline templates
	// ---------------------------------------------------------------------------

	"imports": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Import Pipeline</h1>
                <p class="page-subtitle">Import content from RSS feeds, Markdown files, and CSV spreadsheets.</p>
            </div>
        </div>

        <!-- Configured RSS Sources -->
        <div style="margin-bottom:2rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem;">
                <h2 style="font-size:1.25rem;font-weight:600;">Configured RSS Sources</h2>
                <a href="/cm/imports/sources/new" class="btn btn-primary">+ Add RSS Source</a>
            </div>
            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Feed URL</th>
                            <th>Schedule</th>
                            <th>Last Run</th>
                            <th>Status</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                    {{range .Sources}}
                    <tr>
                        <td>{{.Name}}</td>
                        <td style="font-family:monospace;font-size:0.8rem;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="{{.URL}}">{{.URL}}</td>
                        <td>{{if .Schedule}}{{.Schedule}}{{else}}<span style="color:var(--text-muted)">manual</span>{{end}}</td>
                        <td>{{if .LastRunAt}}{{.LastRunAt.Format "2006-01-02 15:04"}}{{else}}<span style="color:var(--text-muted)">never</span>{{end}}</td>
                        <td>
                            {{if .Active}}<span class="content-status published">Active</span>{{else}}<span class="content-status draft">Inactive</span>{{end}}
                            {{if .LastRunStatus}}
                                {{if eq .LastRunStatus "ok"}}<span class="content-status published" style="margin-left:4px;">OK</span>{{else}}<span class="content-status" style="background:rgba(239,68,68,0.15);color:#f87171;margin-left:4px;">Failed</span>{{end}}
                            {{end}}
                        </td>
                        <td class="actions">
                            <a href="/cm/imports/sources/{{.ID.Hex}}/edit" class="btn btn-sm btn-secondary">Edit</a>
                            <form method="POST" action="/cm/imports/sources/{{.ID.Hex}}/trigger" style="display:inline;">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-primary">Run Now</button>
                            </form>
                            <form method="POST" action="/cm/imports/sources/{{.ID.Hex}}/delete" style="display:inline;" data-confirm="Delete this RSS source?" data-confirm-title="Delete RSS Source">
                                {{$.CSRFField}}
                                <button type="submit" class="btn btn-sm btn-danger">Delete</button>
                            </form>
                        </td>
                    </tr>
                    {{else}}
                    <tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:2rem;">No RSS sources configured. <a href="/cm/imports/sources/new">Add one</a>.</td></tr>
                    {{end}}
                    </tbody>
                </table>
            </div>
        </div>

        <!-- Recent Import Jobs -->
        <div>
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem;">
                <h2 style="font-size:1.25rem;font-weight:600;">Recent Import Jobs</h2>
                <div style="display:flex;gap:0.75rem;">
                    <a href="/cm/imports/markdown" class="btn btn-secondary">Import Markdown / ZIP</a>
                    <a href="/cm/imports/csv" class="btn btn-secondary">Import CSV</a>
                </div>
            </div>
            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>Started</th>
                            <th>Name / Type</th>
                            <th>Status</th>
                            <th>Created</th>
                            <th>Updated</th>
                            <th>Failed</th>
                            <th>Duration</th>
                            <th></th>
                        </tr>
                    </thead>
                    <tbody>
                    {{range .Jobs}}
                    <tr>
                        <td style="font-size:0.85rem;white-space:nowrap;">{{.StartedAt.Format "2006-01-02 15:04"}}</td>
                        <td>
                            <span style="font-weight:500;">{{.SourceName}}</span>
                            <span style="font-size:0.75rem;color:var(--text-muted);margin-left:4px;">[{{.Type}}]</span>
                        </td>
                        <td>
                            {{if eq (print .Status) "running"}}<span class="content-status" style="background:rgba(6,182,212,0.15);color:#06b6d4;">Running</span>
                            {{else if eq (print .Status) "done"}}<span class="content-status published">Done</span>
                            {{else if eq (print .Status) "failed"}}<span class="content-status" style="background:rgba(239,68,68,0.15);color:#f87171;">Failed</span>
                            {{else}}<span class="content-status draft">{{.Status}}</span>{{end}}
                        </td>
                        <td style="color:var(--success);">{{.Created}}</td>
                        <td style="color:var(--accent);">{{.Updated}}</td>
                        <td style="color:var(--danger);">{{.Failed}}</td>
                        <td style="font-size:0.8rem;color:var(--text-muted);">
                            {{if .FinishedAt}}{{.FinishedAt.Sub .StartedAt}}{{else}}&#8212;{{end}}
                        </td>
                        <td><a href="/cm/imports/{{.ID.Hex}}" class="btn btn-sm btn-secondary">View</a></td>
                    </tr>
                    {{else}}
                    <tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:2rem;">No import jobs yet.</td></tr>
                    {{end}}
                    </tbody>
                </table>
            </div>
        </div>
    ` + adminLayoutEnd,

	"import-job": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Import Job</h1>
                <p class="page-subtitle">{{.Job.SourceName}}</p>
            </div>
            <a href="/cm/imports" class="btn btn-secondary">Back to Imports</a>
        </div>

        <!-- Summary cards -->
        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:1rem;margin-bottom:2rem;">
            <div class="stat-card">
                <div class="stat-icon">&#9202;</div>
                <div class="stat-info">
                    <span class="stat-value" style="font-size:0.85rem;">{{.Job.StartedAt.Format "Jan 02 15:04:05"}}</span>
                    <span class="stat-label">Started</span>
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">&#9654;</div>
                <div class="stat-info">
                    <span class="stat-value" style="font-size:0.9rem;
                        {{if eq (print .Job.Status) "running"}}color:#06b6d4;
                        {{else if eq (print .Job.Status) "done"}}color:var(--success);
                        {{else}}color:var(--danger);{{end}}">{{.Job.Status}}</span>
                    <span class="stat-label">Status</span>
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">+</div>
                <div class="stat-info">
                    <span class="stat-value" style="color:var(--success);">{{.Job.Created}}</span>
                    <span class="stat-label">Created</span>
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">&#8635;</div>
                <div class="stat-info">
                    <span class="stat-value" style="color:var(--accent);">{{.Job.Updated}}</span>
                    <span class="stat-label">Updated</span>
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">!</div>
                <div class="stat-info">
                    <span class="stat-value" style="color:var(--danger);">{{.Job.Failed}}</span>
                    <span class="stat-label">Failed</span>
                </div>
            </div>
        </div>

        {{if .Job.ErrorMsg}}
        <div style="background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3);color:#f87171;padding:0.75rem 1rem;border-radius:8px;margin-bottom:1.5rem;">
            Error: {{.Job.ErrorMsg}}
        </div>
        {{end}}

        <!-- Log output -->
        <div style="background:rgba(0,0,0,0.4);border:1px solid var(--border);border-radius:12px;padding:1rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
                <h3 style="font-size:1rem;font-weight:600;">Log Output</h3>
                {{if eq (print .Job.Status) "running"}}
                <span id="live-badge" style="font-size:0.75rem;background:rgba(6,182,212,0.15);color:#06b6d4;padding:0.2rem 0.6rem;border-radius:9999px;">Live</span>
                {{end}}
            </div>
            <div id="log-output" style="font-family:'JetBrains Mono',monospace;font-size:0.8rem;line-height:1.6;max-height:600px;overflow-y:auto;white-space:pre-wrap;">{{range .Logs}}<span style="{{if eq (print .Level) "warn"}}color:#f59e0b;{{else if eq (print .Level) "error"}}color:#f87171;{{end}}">{{if .Path}}[{{.Path}}] {{end}}{{.Message}}
</span>{{end}}</div>
        </div>

        {{if eq (print .Job.Status) "running"}}
        <script>
        (function() {
            var logDiv = document.getElementById('log-output');
            var liveBadge = document.getElementById('live-badge');
            var evtSource = new EventSource('/cm/imports/{{.Job.ID.Hex}}/stream');

            function appendLine(level, path, msg) {
                var span = document.createElement('span');
                var text = (path ? '[' + path + '] ' : '') + msg + '\n';
                span.textContent = text;
                if (level === 'warn')  { span.style.color = '#f59e0b'; }
                if (level === 'error') { span.style.color = '#f87171'; }
                if (level === 'done')  { span.style.color = 'var(--success)'; span.style.fontWeight = 'bold'; }
                logDiv.appendChild(span);
                logDiv.scrollTop = logDiv.scrollHeight;
            }

            evtSource.onmessage = function(e) {
                var parts = e.data.split('|');
                var level = parts[0] || 'info';
                var path  = parts[1] || '';
                var msg   = parts.slice(2).join('|') || '';
                if (level === 'done') {
                    appendLine('done', '', 'Import complete (' + path + ')');
                    if (liveBadge) { liveBadge.style.display = 'none'; }
                    evtSource.close();
                    return;
                }
                appendLine(level, path, msg);
            };

            evtSource.onerror = function() {
                evtSource.close();
                if (liveBadge) { liveBadge.textContent = 'Disconnected'; }
            };
        })();
        </script>
        {{end}}
    ` + adminLayoutEnd,

	"import-source-form": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>{{if .IsNew}}New RSS Source{{else}}Edit RSS Source{{end}}</h1>
                <p class="page-subtitle">Configure a recurring RSS/Atom feed import.</p>
            </div>
        </div>
        {{if .Error}}<div style="background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3);color:#f87171;padding:0.75rem 1rem;border-radius:8px;margin-bottom:1.5rem;">{{.Error}}</div>{{end}}
        <div class="form-card" style="max-width:640px;">
            <form method="POST" action="{{if .IsNew}}/cm/imports/sources{{else}}/cm/imports/sources/{{.Source.ID.Hex}}{{end}}">
                {{.CSRFField}}
                <div class="form-group">
                    <label class="form-label">Name *</label>
                    <input type="text" name="name" class="form-input" required value="{{if .Source}}{{.Source.Name}}{{end}}" placeholder="e.g. TechCrunch RSS">
                </div>
                <div class="form-group">
                    <label class="form-label">Feed URL *</label>
                    <input type="url" name="url" class="form-input" required value="{{if .Source}}{{.Source.URL}}{{end}}" placeholder="https://example.com/feed.xml">
                </div>
                <div class="form-group">
                    <label class="form-label">Default Template</label>
                    <select name="template_id" class="form-select">
                        <option value="">&#8212; None &#8212;</option>
                        {{range .Templates}}
                        <option value="{{.ID.Hex}}"{{if and $.Source (eq $.Source.TemplateID .ID)}} selected{{end}}>{{.Name}}</option>
                        {{end}}
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Folder Path</label>
                    <input type="text" name="folder_path" class="form-input" value="{{if .Source}}{{.Source.FolderPath}}{{end}}" placeholder="/imports/rss">
                    <div class="form-help">Content will be created under this folder. Defaults to /imports.</div>
                </div>
                <div class="form-group">
                    <label class="form-label">Schedule</label>
                    <select name="schedule" class="form-select">
                        <option value="manual"{{if and .Source (eq .Source.Schedule "manual")}} selected{{end}}>Manual only</option>
                        <option value="hourly"{{if and .Source (eq .Source.Schedule "hourly")}} selected{{end}}>Hourly</option>
                        <option value="daily"{{if and .Source (eq .Source.Schedule "daily")}} selected{{end}}>Daily</option>
                        <option value="weekly"{{if and .Source (eq .Source.Schedule "weekly")}} selected{{end}}>Weekly</option>
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label" style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
                        <input type="checkbox" name="auto_publish" value="on"{{if and .Source .Source.AutoPublish}} checked{{end}}>
                        Auto-publish imported pages
                    </label>
                </div>
                <div class="form-group">
                    <label class="form-label" style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
                        <input type="checkbox" name="active" value="on"{{if or .IsNew (and .Source .Source.Active)}} checked{{end}}>
                        Active (include in scheduled runs)
                    </label>
                </div>
                <div style="display:flex;gap:0.75rem;margin-top:1.5rem;">
                    <button type="submit" class="btn btn-primary">{{if .IsNew}}Create Source{{else}}Save Changes{{end}}</button>
                    <a href="/cm/imports" class="btn btn-secondary">Cancel</a>
                </div>
            </form>
        </div>
    ` + adminLayoutEnd,

	"import-markdown-form": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Import Markdown / ZIP</h1>
                <p class="page-subtitle">Upload a .md, .markdown, or .zip file containing Markdown pages.</p>
            </div>
            <a href="/cm/imports" class="btn btn-secondary">Back</a>
        </div>
        <div class="form-card" style="max-width:600px;">
            <div style="background:rgba(6,182,212,0.08);border:1px solid rgba(6,182,212,0.2);border-radius:8px;padding:1rem;margin-bottom:1.5rem;font-size:0.88rem;color:var(--text-muted);">
                <strong style="color:var(--accent);">Supported frontmatter keys:</strong>
                <code style="display:block;margin-top:0.4rem;line-height:1.8;">title &bull; slug &bull; folder &bull; template &bull; published &bull; publish_at &bull; tags</code>
                <div style="margin-top:0.5rem;">Frontmatter is delimited by <code>---</code>. Any extra keys become content fields.</div>
            </div>
            <form method="POST" action="/cm/imports/markdown" enctype="multipart/form-data">
                {{.CSRFField}}
                <div class="form-group">
                    <label class="form-label">File *</label>
                    <input type="file" name="file" class="form-input" accept=".md,.markdown,.zip" required>
                    <div class="form-help">Single .md/.markdown file or a .zip archive containing multiple Markdown files.</div>
                </div>
                <div class="form-group">
                    <label class="form-label">Default Template</label>
                    <select name="default_template" class="form-select">
                        <option value="">&#8212; None &#8212;</option>
                        {{range .Templates}}
                        <option value="{{.Name}}">{{.Name}}</option>
                        {{end}}
                    </select>
                    <div class="form-help">Used when a file does not specify a template in frontmatter.</div>
                </div>
                <div class="form-group">
                    <label class="form-label">Default Folder</label>
                    <input type="text" name="default_folder" class="form-input" placeholder="/imports/markdown">
                    <div class="form-help">Used when a file does not specify a folder in frontmatter. Defaults to /imports.</div>
                </div>
                <div class="form-group">
                    <label class="form-label" style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
                        <input type="checkbox" name="auto_publish" value="on">
                        Auto-publish imported pages
                    </label>
                </div>
                <div style="display:flex;gap:0.75rem;margin-top:1.5rem;">
                    <button type="submit" class="btn btn-primary">Start Import</button>
                    <a href="/cm/imports" class="btn btn-secondary">Cancel</a>
                </div>
            </form>
        </div>
    ` + adminLayoutEnd,

	"import-csv-form": adminLayoutStart + `
        <div class="page-header">
            <div>
                <h1>Quick CSV Import</h1>
                <p class="page-subtitle">Import content rows from a CSV file.</p>
            </div>
            <a href="/cm/imports" class="btn btn-secondary">Back</a>
        </div>
        <div class="form-card" style="max-width:600px;">
            <div style="background:rgba(6,182,212,0.08);border:1px solid rgba(6,182,212,0.2);border-radius:8px;padding:1rem;margin-bottom:1.5rem;font-size:0.88rem;color:var(--text-muted);">
                All CSV columns will be imported as content fields. The title column will be used as the page title and to generate the URL slug.
            </div>
            <form method="POST" action="/cm/imports/csv" enctype="multipart/form-data">
                {{.CSRFField}}
                <div class="form-group">
                    <label class="form-label">CSV File *</label>
                    <input type="file" name="file" class="form-input" accept=".csv" required>
                </div>
                <div class="form-group">
                    <label class="form-label">Template</label>
                    <select name="template_id" class="form-select">
                        <option value="">&#8212; None &#8212;</option>
                        {{range .Templates}}
                        <option value="{{.ID.Hex}}">{{.Name}}</option>
                        {{end}}
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Folder Path</label>
                    <input type="text" name="folder_path" class="form-input" placeholder="/imports/csv">
                    <div class="form-help">Content will be created under this folder. Defaults to /imports.</div>
                </div>
                <div class="form-group">
                    <label class="form-label">Title Column</label>
                    <input type="text" name="title_column" class="form-input" value="title" placeholder="title">
                    <div class="form-help">Name of the CSV column to use as the page title and slug source. Default: "title".</div>
                </div>
                <div class="form-group">
                    <label class="form-label" style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
                        <input type="checkbox" name="auto_publish" value="on">
                        Auto-publish imported pages
                    </label>
                </div>
                <div style="display:flex;gap:0.75rem;margin-top:1.5rem;">
                    <button type="submit" class="btn btn-primary">Start Import</button>
                    <a href="/cm/imports" class="btn btn-secondary">Cancel</a>
                </div>
            </form>
        </div>
    ` + adminLayoutEnd,

	"approvals_page": adminLayoutStart + `
        <div class="page-header" style="display:flex;align-items:center;justify-content:space-between;">
            <h1>Approvals</h1>
        </div>

        <div class="form-section" style="margin-bottom:2rem;">
            <h2 style="font-size:1.1rem;margin-bottom:1rem;">My Queue</h2>
            {{if .MyQueue}}
            <div class="table-container">
                <table>
                    <thead><tr><th>Content</th><th>Path</th><th>Submitted By</th><th>Submitted</th><th>Actions</th></tr></thead>
                    <tbody>
                        {{range .MyQueue}}
                        <tr>
                            <td><a href="/cm/content/{{.ContentID.Hex}}">{{if .ContentTitle}}{{.ContentTitle}}{{else}}{{.ContentID.Hex}}{{end}}</a></td>
                            <td style="color:var(--muted);font-size:0.85rem;">{{.ContentPath}}</td>
                            <td style="font-size:0.9rem;">{{.SubmittedByEmail}}</td>
                            <td style="font-size:0.85rem;color:var(--muted);">{{.CreatedAt.Format "Jan 2, 3:04 PM"}}</td>
                            <td class="actions">
                                <button type="button" class="btn btn-sm btn-primary" onclick="approveRequest('{{.ID.Hex}}')">Approve</button>
                                <button type="button" class="btn btn-sm btn-secondary" onclick="openRejectModal('{{.ID.Hex}}')">Reject</button>
                            </td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{else}}
            <p style="color:var(--muted);">Nothing in your queue.</p>
            {{end}}
        </div>

        {{if .OtherPending}}
        <div class="form-section" style="margin-bottom:2rem;">
            <h2 style="font-size:1.1rem;margin-bottom:1rem;">Other Pending</h2>
            <div class="table-container">
                <table>
                    <thead><tr><th>Content</th><th>Path</th><th>Submitted By</th><th>Submitted</th></tr></thead>
                    <tbody>
                        {{range .OtherPending}}
                        <tr>
                            <td><a href="/cm/content/{{.ContentID.Hex}}">{{if .ContentTitle}}{{.ContentTitle}}{{else}}{{.ContentID.Hex}}{{end}}</a></td>
                            <td style="color:var(--muted);font-size:0.85rem;">{{.ContentPath}}</td>
                            <td style="font-size:0.9rem;">{{.SubmittedByEmail}}</td>
                            <td style="font-size:0.85rem;color:var(--muted);">{{.CreatedAt.Format "Jan 2, 3:04 PM"}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
        </div>
        {{end}}

        {{if and .CurrentUser (eq .CurrentUser.Role "admin")}}
        <div class="form-section">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
                <h2 style="font-size:1.1rem;margin:0;">Approval Workflows</h2>
                <button type="button" class="btn btn-primary btn-sm" onclick="document.getElementById('new-workflow-form').style.display='block';this.style.display='none';">+ New Workflow</button>
            </div>

            <div id="new-workflow-form" style="display:none;background:var(--bg-tertiary);border:1px solid var(--border);border-radius:var(--radius);padding:1.25rem;margin-bottom:1.5rem;">
                <h3 style="font-size:1rem;margin:0 0 1rem;">Create Workflow</h3>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                    <div>
                        <label style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:0.35rem;">Name *</label>
                        <input type="text" id="wf-name" placeholder="e.g. Blog Post Review" style="width:100%;padding:0.5rem 0.75rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);">
                    </div>
                    <div>
                        <label style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:0.35rem;">Mode</label>
                        <select id="wf-mode" style="width:100%;padding:0.5rem 0.75rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);">
                            <option value="concurrent">Concurrent (any approver)</option>
                            <option value="sequential">Sequential (in order)</option>
                        </select>
                    </div>
                    <div>
                        <label style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:0.35rem;">Trigger</label>
                        <select id="wf-trigger" onchange="toggleTriggerValue()" style="width:100%;padding:0.5rem 0.75rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);">
                            <option value="all_contributor">All contributors</option>
                            <option value="folder_path">Folder path</option>
                            <option value="template_id">Template ID</option>
                            <option value="tag">Tag</option>
                        </select>
                    </div>
                    <div id="wf-trigger-value-wrap">
                        <label style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:0.35rem;">Trigger Value <span style="font-size:0.8rem;">(folder path / template ID / tag)</span></label>
                        <input type="text" id="wf-trigger-value" placeholder="e.g. /blog or tag-name" style="width:100%;padding:0.5rem 0.75rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);">
                    </div>
                </div>
                <div style="margin-bottom:1rem;">
                    <label style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:0.35rem;">Approvers <span style="font-size:0.8rem;">(search by email)</span></label>
                    <div style="display:flex;gap:0.5rem;margin-bottom:0.5rem;">
                        <input type="text" id="wf-approver-search" placeholder="Type email to search..." style="flex:1;padding:0.5rem 0.75rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);" oninput="searchApprovers()">
                    </div>
                    <div id="wf-approver-results" style="display:none;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);max-height:120px;overflow-y:auto;margin-bottom:0.5rem;"></div>
                    <div id="wf-approvers-list" style="display:flex;flex-wrap:wrap;gap:0.4rem;"></div>
                </div>
                <div style="display:flex;gap:0.75rem;">
                    <button type="button" class="btn btn-primary btn-sm" onclick="createWorkflow()">Create Workflow</button>
                    <button type="button" class="btn btn-outline btn-sm" onclick="document.getElementById('new-workflow-form').style.display='none';document.querySelector('.btn-primary.btn-sm[onclick*=new-workflow]').style.display='';">Cancel</button>
                </div>
            </div>

            {{if .Workflows}}
            <div class="table-container">
                <table>
                    <thead><tr><th>Name</th><th>Trigger</th><th>Mode</th><th>Approvers</th><th>Actions</th></tr></thead>
                    <tbody>
                        {{range .Workflows}}
                        <tr>
                            <td>{{.Name}}</td>
                            <td style="font-size:0.85rem;color:var(--muted);">{{.Trigger}}{{if .TriggerValue}}: {{.TriggerValue}}{{end}}</td>
                            <td style="font-size:0.85rem;">{{.Mode}}</td>
                            <td style="font-size:0.85rem;color:var(--muted);">{{len .Approvers}}</td>
                            <td class="actions">
                                <button type="button" class="btn btn-sm btn-outline" onclick="deleteWorkflow('{{.ID.Hex}}')">Delete</button>
                            </td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{else}}
            <p style="color:var(--muted);font-size:0.9rem;">No custom workflows configured. By default, any editor can approve contributor submissions.</p>
            {{end}}
        </div>
        {{end}}

        <div id="reject-modal" style="display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.7);z-index:10000;align-items:center;justify-content:center;">
            <div style="background:var(--bg-card);border-radius:var(--radius);max-width:480px;width:90%;padding:1.5rem;border:1px solid var(--border);">
                <h3 style="margin:0 0 1rem;">Reject Request</h3>
                <p style="color:var(--muted);margin-bottom:0.75rem;font-size:0.9rem;">A comment is required and will be posted to the content's discussion thread.</p>
                <textarea id="reject-comment" rows="4" style="width:100%;padding:0.75rem;background:var(--bg-tertiary);border:1px solid var(--border);border-radius:var(--radius);color:var(--text);resize:vertical;box-sizing:border-box;"></textarea>
                <div style="display:flex;gap:0.75rem;justify-content:flex-end;margin-top:1rem;">
                    <button type="button" class="btn btn-outline" onclick="closeRejectModal()">Cancel</button>
                    <button type="button" class="btn btn-primary" onclick="submitReject()">Reject</button>
                </div>
            </div>
        </div>
        <script>
        var rejectingId = null;
        var wfApprovers = []; // [{user_id, email}]
        var allUsers = null;

        async function approveRequest(id) {
            if (!(await showConfirm('Approve this request?', 'Approve Request'))) return;
            var r = await fetch('/api/v1/approval-requests/'+id+'/approve', {method:'POST',headers:csrfHeaders({'Content-Type':'application/json'}),body:'{}'});
            if (r.ok) location.reload(); else { var e = await r.json().catch(()=>({error:'Failed'})); showAlert(dialogText(e.error||'Failed'), 'Approve Failed'); }
        }
        function openRejectModal(id) { rejectingId = id; document.getElementById('reject-comment').value=''; document.getElementById('reject-modal').style.display='flex'; }
        function closeRejectModal() { document.getElementById('reject-modal').style.display='none'; rejectingId=null; }
        async function submitReject() {
            var c = document.getElementById('reject-comment').value.trim();
            if (!c) { showAlert('Comment required', 'Reject Request'); return; }
            var r = await fetch('/api/v1/approval-requests/'+rejectingId+'/reject', {method:'POST',headers:csrfHeaders({'Content-Type':'application/json'}),body:JSON.stringify({comment:c})});
            if (r.ok) { closeRejectModal(); location.reload(); } else { var e = await r.json().catch(()=>({error:'Failed'})); showAlert(dialogText(e.error||'Failed'), 'Reject Failed'); }
        }
        async function deleteWorkflow(id) {
            if (!(await showConfirm('Delete this workflow?', 'Delete Workflow'))) return;
            var r = await fetch('/api/v1/approval-workflows/'+id, {method:'DELETE',headers:csrfHeaders()});
            if (r.ok) location.reload(); else showAlert('Failed', 'Delete Failed');
        }
        function toggleTriggerValue() {
            var t = document.getElementById('wf-trigger').value;
            document.getElementById('wf-trigger-value-wrap').style.display = t === 'all_contributor' ? 'none' : '';
        }
        async function loadAllUsers() {
            if (allUsers) return allUsers;
            var r = await fetch('/api/v1/users');
            if (r.ok) { allUsers = await r.json(); }
            return allUsers || [];
        }
        async function searchApprovers() {
            var q = document.getElementById('wf-approver-search').value.trim().toLowerCase();
            var res = document.getElementById('wf-approver-results');
            if (!q) { res.style.display='none'; return; }
            var users = await loadAllUsers();
            var matches = (users.users||users||[]).filter(u => (u.email||'').toLowerCase().includes(q) && !wfApprovers.find(a=>a.user_id===u.id));
            if (!matches.length) { res.style.display='none'; return; }
            res.style.display='block';
            // Built as DOM nodes: the email is shown as text and travels in a
            // data attribute, never inside an inline handler or HTML string.
            res.textContent = '';
            matches.slice(0,5).forEach(function(u) {
                var row = document.createElement('div');
                row.className = 'wf-approver-option';
                row.setAttribute('role', 'button');
                row.tabIndex = 0;
                row.dataset.userId = u.id || '';
                row.dataset.email = u.email || '';
                row.style.cssText = 'padding:0.5rem 0.75rem;cursor:pointer;font-size:0.9rem;';
                row.textContent = u.email || '';
                res.appendChild(row);
            });
        }
        function addApprover(id, email) {
            if (wfApprovers.find(a=>a.user_id===id)) return;
            wfApprovers.push({user_id:id,email:email});
            document.getElementById('wf-approver-search').value='';
            document.getElementById('wf-approver-results').style.display='none';
            renderApprovers();
        }
        function removeApprover(id) {
            wfApprovers = wfApprovers.filter(a=>a.user_id!==id);
            renderApprovers();
        }
        function renderApprovers() {
            var list = document.getElementById('wf-approvers-list');
            list.textContent = '';
            wfApprovers.forEach(function(a) {
                var chip = document.createElement('span');
                chip.className = 'wf-approver-chip';
                chip.style.cssText = 'display:inline-flex;align-items:center;gap:0.3rem;background:var(--bg-card);border:1px solid var(--border);border-radius:9999px;padding:0.2rem 0.6rem;font-size:0.85rem;';
                chip.appendChild(document.createTextNode(a.email));
                var x = document.createElement('button');
                x.type = 'button';
                x.className = 'wf-approver-remove';
                x.dataset.userId = a.user_id;
                x.setAttribute('aria-label', 'Remove approver');
                x.style.cssText = 'background:none;border:none;color:var(--muted);cursor:pointer;padding:0;font-size:1rem;line-height:1;';
                x.textContent = '\u00d7';
                chip.appendChild(x);
                list.appendChild(chip);
            });
        }
        // One listener for the approver picker (results and chips are rebuilt often)
        (function() {
            var res = document.getElementById('wf-approver-results');
            var list = document.getElementById('wf-approvers-list');
            if (!res || !list) return; // the workflow form is admin-only
            function pick(e) {
                var row = e.target.closest ? e.target.closest('.wf-approver-option') : null;
                if (row) addApprover(row.dataset.userId, row.dataset.email);
            }
            res.addEventListener('click', pick);
            res.addEventListener('keydown', function(e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(e); } });
            res.addEventListener('mouseover', function(e) { var row = e.target.closest('.wf-approver-option'); if (row) row.style.background = 'var(--bg-tertiary)'; });
            res.addEventListener('mouseout', function(e) { var row = e.target.closest('.wf-approver-option'); if (row) row.style.background = ''; });
            list.addEventListener('click', function(e) {
                var x = e.target.closest ? e.target.closest('.wf-approver-remove') : null;
                if (x) removeApprover(x.dataset.userId);
            });
        })();
        async function createWorkflow() {
            var name = document.getElementById('wf-name').value.trim();
            if (!name) { showAlert('Name is required', 'New Workflow'); return; }
            var trigger = document.getElementById('wf-trigger').value;
            var triggerValue = trigger !== 'all_contributor' ? document.getElementById('wf-trigger-value').value.trim() : '';
            var mode = document.getElementById('wf-mode').value;
            var approvers = wfApprovers.map(a => ({user_id: a.user_id}));
            var payload = {name:name, trigger:trigger, trigger_value:triggerValue, mode:mode, approvers:approvers};
            var r = await fetch('/api/v1/approval-workflows', {method:'POST',headers:csrfHeaders({'Content-Type':'application/json'}),body:JSON.stringify(payload)});
            if (r.ok) { location.reload(); } else { var e = await r.json().catch(()=>({error:'Failed'})); showAlert(dialogText(e.error||'Failed to create workflow'), 'Create Failed'); }
        }
        </script>
    ` + adminLayoutEnd,
}

const adminLayoutStart = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LightCMS Admin</title>
    <link rel="icon" type="image/x-icon" href="/static/images/favicon.ico">
    <link rel="icon" type="image/png" sizes="16x16" href="/static/images/favicon-16x16.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/static/images/favicon-32x32.png">
    <link rel="icon" type="image/png" sizes="48x48" href="/static/images/favicon-48x48.png">
    <link rel="apple-touch-icon" sizes="180x180" href="/static/images/apple-touch-icon.png">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        :root {
            --primary: #6366f1;
            --secondary: #8b5cf6;
            --accent: #06b6d4;
            --bg-dark: #0f172a;
            --bg-card: #1e1b4b;
            --bg-hover: #2e2a5a;
            --text: #f1f5f9;
            --text-muted: #94a3b8;
            --border: rgba(99, 102, 241, 0.2);
            --success: #10b981;
            --danger: #ef4444;
            --radius: 12px;
        }
        body {
            font-family: 'Inter', system-ui, sans-serif;
            background: var(--bg-dark);
            color: var(--text);
            min-height: 100vh;
        }
        a { color: var(--accent); text-decoration: none; }
        a:hover { text-decoration: underline; }

        .admin-layout {
            display: grid;
            grid-template-columns: 260px 1fr;
            min-height: 100vh;
        }
        .sidebar {
            background: linear-gradient(180deg, var(--bg-card) 0%, var(--bg-dark) 100%);
            border-right: 1px solid var(--border);
            padding: 1.5rem;
            position: sticky;
            top: 0;
            height: 100vh;
            overflow-y: auto;
        }
        .sidebar-logo {
            display: block;
        }
        .sidebar-logo img {
            height: 48px;
            width: auto;
        }
        .sidebar-version {
            font-size: 11px;
            color: var(--text-muted, #888);
            margin: 0.35rem 0 2rem 2px;
            letter-spacing: 0.03em;
        }
        .nav-section {
            margin-bottom: 1.5rem;
        }
        .nav-section-title {
            font-size: 0.7rem;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            color: var(--text-muted);
            margin-bottom: 0.5rem;
            padding-left: 0.75rem;
        }
        .nav-link {
            display: flex;
            align-items: center;
            gap: 0.75rem;
            padding: 0.75rem;
            border-radius: var(--radius);
            color: var(--text);
            transition: all 0.2s;
            text-decoration: none;
        }
        .nav-link:hover {
            background: var(--bg-hover);
            text-decoration: none;
        }
        .logout-btn {
            background: none;
            border: none;
            cursor: pointer;
            width: 100%;
            text-align: left;
            font: inherit;
        }
        .nav-link.active {
            background: linear-gradient(135deg, var(--primary), var(--secondary));
        }
        .nav-badge {
            background: #ef4444;
            color: white;
            font-size: 0.7rem;
            font-weight: 600;
            padding: 0.15rem 0.5rem;
            border-radius: 9999px;
            margin-left: auto;
        }

        .main-content {
            padding: 2rem;
            max-width: 1400px;
        }
        .page-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 2rem;
        }
        .page-header h1 {
            font-family: 'Space Grotesk', sans-serif;
            font-size: 2rem;
            font-weight: 600;
        }
        .page-subtitle {
            color: var(--text-muted);
            margin-bottom: 1.5rem;
        }

        .btn {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.625rem 1.25rem;
            border-radius: var(--radius);
            font-weight: 500;
            font-size: 0.9rem;
            cursor: pointer;
            border: none;
            transition: all 0.2s;
            text-decoration: none;
        }
        .btn:hover { text-decoration: none; transform: translateY(-1px); }
        .btn-primary {
            background: linear-gradient(135deg, var(--primary), var(--secondary));
            color: white;
        }
        .btn-secondary {
            background: var(--bg-hover);
            color: var(--text);
            border: 1px solid var(--border);
        }
        .btn-outline {
            background: transparent;
            color: var(--text);
            border: 1px solid var(--border);
        }
        .btn-danger {
            background: var(--danger);
            color: white;
        }
        .btn-sm {
            padding: 0.375rem 0.75rem;
            font-size: 0.8rem;
        }
        .toggle-html-btn {
            font-family: monospace;
            font-size: 0.75rem;
        }
        .toggle-html-btn.active {
            background: var(--primary);
            color: white;
            border-color: var(--primary);
        }

        .table-container {
            background: var(--bg-card);
            border-radius: var(--radius);
            border: 1px solid var(--border);
            overflow: hidden;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 1rem;
            text-align: left;
            border-bottom: 1px solid var(--border);
        }
        th {
            background: rgba(99, 102, 241, 0.1);
            font-weight: 600;
            font-size: 0.85rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        tr:hover td {
            background: var(--bg-hover);
        }
        .actions {
            display: flex;
            gap: 0.5rem;
            align-items: center;
        }
        .actions form { display: inline; }

        .status-badge {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            border-radius: 9999px;
            font-size: 0.75rem;
            font-weight: 500;
        }
        .status-badge.published {
            background: rgba(16, 185, 129, 0.2);
            color: var(--success);
        }
        .status-badge.draft {
            background: rgba(148, 163, 184, 0.2);
            color: var(--text-muted);
        }

        .form-card {
            background: var(--bg-card);
            border-radius: var(--radius);
            border: 1px solid var(--border);
            padding: 2rem;
        }
        .form-section {
            margin-bottom: 2rem;
            padding-bottom: 2rem;
            border-bottom: 1px solid var(--border);
        }
        .form-section h3 {
            margin-bottom: 1rem;
            font-size: 1.1rem;
        }
        .form-group {
            margin-bottom: 1.25rem;
        }
        .form-group label {
            display: block;
            margin-bottom: 0.5rem;
            font-weight: 500;
            color: var(--text);
        }
        .form-row {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
        }
        .help-text {
            font-size: 0.85rem;
            color: var(--text-muted);
            margin-bottom: 0.5rem;
        }
        input[type="text"], input[type="password"], input[type="email"],
        input[type="number"], input[type="date"], textarea, select {
            width: 100%;
            padding: 0.75rem 1rem;
            background: rgba(15, 23, 42, 0.5);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            color: var(--text);
            font-size: 1rem;
            font-family: inherit;
            transition: all 0.2s;
        }
        input:focus, textarea:focus, select:focus {
            outline: none;
            border-color: var(--primary);
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
        }
        textarea { resize: vertical; min-height: 100px; }
        .code-editor {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.9rem;
        }
        input[type="color"] {
            width: 60px;
            height: 40px;
            padding: 0;
            border: none;
            cursor: pointer;
        }
        .color-grid {
            display: flex;
            gap: 1.5rem;
            flex-wrap: wrap;
        }
        .color-grid .form-group {
            text-align: center;
        }
        .checkbox-group {
            margin-top: 1rem;
        }
        .checkbox-label {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            cursor: pointer;
        }
        .checkbox-label input[type="checkbox"] {
            width: 18px;
            height: 18px;
            cursor: pointer;
        }
        .form-actions {
            display: flex;
            gap: 1rem;
            justify-content: flex-end;
            margin-top: 2rem;
            padding-top: 1.5rem;
            border-top: 1px solid var(--border);
        }

        .field-row {
            display: grid;
            grid-template-columns: 1fr 1fr 120px 1fr 1fr auto auto;
            gap: 0.5rem;
            margin-bottom: 0.5rem;
            align-items: center;
        }
        .field-row input, .field-row select {
            padding: 0.5rem;
            font-size: 0.9rem;
        }

        .template-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 1.5rem;
        }
        .template-card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            padding: 1.5rem;
            transition: all 0.2s;
            text-decoration: none;
            color: var(--text);
            display: block;
        }
        .template-card:hover {
            border-color: var(--primary);
            transform: translateY(-2px);
            box-shadow: 0 10px 30px -10px rgba(99, 102, 241, 0.3);
            text-decoration: none;
        }
        .template-card h3 {
            margin-bottom: 0.5rem;
        }
        .template-card p {
            color: var(--text-muted);
            font-size: 0.9rem;
            margin-bottom: 1rem;
        }
        .template-category {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            background: rgba(99, 102, 241, 0.2);
            border-radius: 9999px;
            font-size: 0.75rem;
            color: var(--primary);
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            padding: 1.5rem;
            display: flex;
            align-items: center;
            gap: 1rem;
        }
        .stat-icon {
            font-size: 2rem;
        }
        .stat-value {
            display: block;
            font-size: 2rem;
            font-weight: 700;
            font-family: 'Space Grotesk', sans-serif;
        }
        .stat-label {
            color: var(--text-muted);
            font-size: 0.9rem;
        }

        .quick-actions {
            margin-bottom: 2rem;
        }
        .quick-actions h2 {
            margin-bottom: 1rem;
            font-size: 1.25rem;
        }
        .action-buttons {
            display: flex;
            gap: 1rem;
            flex-wrap: wrap;
        }

        .recent-content h2 {
            margin-bottom: 1rem;
            font-size: 1.25rem;
        }
        .content-list {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            overflow: hidden;
        }
        .content-item {
            display: grid;
            grid-template-columns: 1fr auto auto auto;
            gap: 1rem;
            padding: 1rem;
            border-bottom: 1px solid var(--border);
            color: var(--text);
            text-decoration: none;
            transition: background 0.2s;
            align-items: center;
        }
        .content-item:hover {
            background: var(--bg-hover);
            text-decoration: none;
        }
        .content-actions {
            display: flex;
            gap: 0.5rem;
        }
        .content-item:last-child {
            border-bottom: none;
        }
        .content-info {
            display: flex;
            flex-direction: column;
            gap: 0.25rem;
        }
        .content-title {
            font-weight: 500;
        }
        .content-slug {
            font-size: 0.75rem;
            color: var(--text-muted);
            font-family: 'JetBrains Mono', monospace;
        }
        .content-template {
            color: var(--text-muted);
            font-size: 0.9rem;
        }
        .content-status {
            padding: 0.25rem 0.75rem;
            border-radius: 9999px;
            font-size: 0.75rem;
        }
        .content-status.published {
            background: rgba(16, 185, 129, 0.2);
            color: var(--success);
        }
        .content-status.draft {
            background: rgba(148, 163, 184, 0.2);
            color: var(--text-muted);
        }

        .error-message {
            background: rgba(239, 68, 68, 0.1);
            border: 1px solid rgba(239, 68, 68, 0.3);
            color: #f87171;
            padding: 1rem;
            border-radius: var(--radius);
            margin-bottom: 1.5rem;
        }
        .success-message {
            background: rgba(16, 185, 129, 0.1);
            border: 1px solid rgba(16, 185, 129, 0.3);
            color: var(--success);
            padding: 1rem;
            border-radius: var(--radius);
            margin-bottom: 1.5rem;
        }

        .security-alert {
            background: linear-gradient(135deg, rgba(239, 68, 68, 0.15), rgba(251, 146, 60, 0.15));
            border: 2px solid #ef4444;
            border-radius: var(--radius);
            padding: 1.25rem;
            margin-bottom: 2rem;
            display: flex;
            align-items: center;
            gap: 1rem;
            animation: pulse-border 2s ease-in-out infinite;
        }
        @keyframes pulse-border {
            0%, 100% { border-color: #ef4444; }
            50% { border-color: #fb923c; }
        }
        .security-alert .alert-icon {
            font-size: 2rem;
        }
        .security-alert .alert-content {
            flex: 1;
            color: #fecaca;
        }
        .security-alert .alert-content strong {
            color: #fca5a5;
        }
        .security-alert .alert-content a {
            color: #fbbf24;
            font-weight: 600;
            text-decoration: underline;
        }
        .security-alert .alert-content a:hover {
            color: #fde047;
        }

        .password-requirements {
            list-style: none;
            padding: 0;
            color: var(--text-muted);
        }
        .password-requirements li {
            padding: 0.5rem 0;
            padding-left: 1.5rem;
            position: relative;
        }
        .password-requirements li::before {
            content: "•";
            position: absolute;
            left: 0;
            color: var(--primary);
        }

        @media (max-width: 768px) {
            .admin-layout {
                grid-template-columns: 1fr;
            }
            .sidebar {
                position: fixed;
                left: -100%;
                z-index: 100;
                transition: left 0.3s;
            }
            .sidebar.open {
                left: 0;
            }
            .field-row {
                grid-template-columns: 1fr 1fr;
            }
        }

        /* One dialog at a time: see dialogOpened(). Immediately — buttons and
           inputs have "transition: all", which would leave them showing for a
           moment after their modal is hidden. */
        body.dialog-open .modal-overlay,
        body.dialog-open .modal-overlay * {
            visibility: hidden !important;
            transition: none !important;
        }
    </style>
    <script>
    // csrfHeaders returns request headers carrying this page's CSRF token.
    // Every fetch() that changes state must send them, to /cm and to /api/v1
    // alike: a session-authenticated API request without the token is refused.
    //   fetch(url, {method: 'POST', headers: csrfHeaders({'Content-Type': 'application/json'}), body: ...})
    // html/template quotes and escapes the value in script context; do not pre-quote it
    var lcCSRFToken = {{.CSRFToken}};
    function csrfHeaders(extra) {
        var headers = {'X-CSRF-Token': lcCSRFToken};
        for (var name in (extra || {})) headers[name] = extra[name];
        return headers;
    }

    // showAlert and showConfirm take their message as HTML. Pass anything that
    // is not a fixed string (server errors, user input) through dialogText.
    function dialogText(text) {
        var el = document.createElement('div');
        el.textContent = text == null ? '' : String(text);
        return el.innerHTML;
    }

    // While a styled dialog (alert, confirm, delete, revert) is up it is the
    // only dialog on screen: the page's own modals (.modal-overlay — search,
    // replace preview) are hidden until it closes instead of showing through
    // around it, focus moves to the dialog's button so Enter cannot re-run
    // whatever is underneath, and Escape answers the dialog rather than
    // closing the modal below. Returns the function to call when it closes.
    var openDialogs = 0;
    function dialogOpened(focusBtn, onEscape) {
        var previous = document.activeElement;
        openDialogs++;
        document.body.classList.add('dialog-open');
        function onKey(e) {
            if (e.key !== 'Escape') return;
            e.preventDefault();
            e.stopPropagation();
            onEscape();
        }
        document.addEventListener('keydown', onKey, true);
        if (focusBtn) focusBtn.focus();
        return function() {
            document.removeEventListener('keydown', onKey, true);
            if (--openDialogs <= 0) {
                openDialogs = 0;
                document.body.classList.remove('dialog-open');
            }
            if (previous && previous.focus && document.contains(previous)) previous.focus();
        };
    }

    // Show info modal (the styled replacement for the browser's native alert)
    function showAlert(message, title, callback) {
        var modal = document.getElementById('info-modal');
        var msgEl = document.getElementById('info-modal-message');
        var titleEl = document.getElementById('info-modal-title');
        var okBtn = document.getElementById('info-ok-btn');

        titleEl.textContent = title || 'Information';
        msgEl.innerHTML = message;
        modal.style.display = 'flex';
        var closed = dialogOpened(okBtn, onOk);

        function cleanup() {
            modal.style.display = 'none';
            okBtn.removeEventListener('click', onOk);
            closed();
        }

        function onOk() {
            cleanup();
            if (callback) callback();
        }

        okBtn.addEventListener('click', onOk);
    }

    // Show confirm modal (the styled replacement for the browser's native confirm) - returns a Promise
    function showConfirm(message, title) {
        return new Promise(function(resolve) {
            var modal = document.getElementById('confirm-modal');
            var msgEl = document.getElementById('confirm-modal-message');
            var titleEl = document.getElementById('confirm-modal-title');
            var okBtn = document.getElementById('confirm-ok-btn');
            var cancelBtn = document.getElementById('confirm-cancel-btn');

            titleEl.textContent = title || 'Confirm';
            msgEl.innerHTML = message;
            modal.style.display = 'flex';
            var closed = dialogOpened(cancelBtn, onCancel); // Cancel is the safe default for Enter

            function cleanup() {
                modal.style.display = 'none';
                okBtn.removeEventListener('click', onOk);
                cancelBtn.removeEventListener('click', onCancel);
                closed();
            }

            function onOk() {
                cleanup();
                resolve(true);
            }

            function onCancel() {
                cleanup();
                resolve(false);
            }

            okBtn.addEventListener('click', onOk);
            cancelBtn.addEventListener('click', onCancel);
        });
    }

    // Forms with a data-confirm attribute ask through the styled confirm modal
    // before submitting (never a native dialog from an onsubmit/onclick handler).
    // data-confirm-title sets the modal title. The message is shown as text.
    document.addEventListener('submit', function(e) {
        var form = e.target;
        if (!form || !form.getAttribute) return;
        var message = form.getAttribute('data-confirm');
        if (!message) return;
        e.preventDefault();
        showConfirm(dialogText(message), form.getAttribute('data-confirm-title') || 'Confirm').then(function(confirmed) {
            if (confirmed) form.submit(); // form.submit() does not re-fire this handler
        });
    });
    </script>
</head>
<body>
    <div class="admin-layout">
        <aside class="sidebar">
            <a href="/cm" class="sidebar-logo"><img src="/static/images/lightcms-logo.png" alt="LightCMS"></a>
            {{if .AppVersion}}<div class="sidebar-version">v{{.AppVersion}}</div>{{end}}
            <nav>
                <div class="nav-section">
                    <div class="nav-section-title">Content</div>
                    <a href="/cm" class="nav-link">📊 Dashboard</a>
                    <a href="/cm/content" class="nav-link">📄 Content</a>
                    <a href="/cm/templates" class="nav-link">📋 Templates</a>
                    <a href="/cm/snippets" class="nav-link">✂️ Snippets</a>
                    <a href="/cm/collections" class="nav-link">📁 Collections</a>
                    <a href="/cm/folders" class="nav-link">🗂️ Folders</a>
                    <a href="/cm/forks" class="nav-link">🌿 Forks</a>
                    <a href="/cm/imports" class="nav-link">📥 Imports</a>
                    <a href="/cm/approvals" class="nav-link">✅ Approvals{{if .PendingApprovalCount}} <span class="nav-badge">{{.PendingApprovalCount}}</span>{{end}}</a>
                </div>
                <div class="nav-section">
                    <div class="nav-section-title">Media</div>
                    <a href="/cm/assets" class="nav-link">🖼️ Asset Library</a>
                </div>
                <div class="nav-section">
                    <div class="nav-section-title">Settings</div>
                    <a href="/cm/theme" class="nav-link">🎨 Theme</a>
                    <a href="/cm/redirects" class="nav-link">↪️ Redirects</a>
                    <a href="/cm/config" class="nav-link">⚙️ Configuration</a>
                    <a href="/cm/api-keys" class="nav-link">🔑 API Keys</a>
                    <a href="/cm/connected-apps" class="nav-link">🔌 Connected Apps</a>
                    <a href="/cm/security" class="nav-link">🔒 Security</a>
                    <a href="/cm/webhooks" class="nav-link">🔔 Webhooks</a>
                    {{if and .CurrentUser (eq .CurrentUser.Role "admin")}}
                    <a href="/cm/users" class="nav-link">👥 Users</a>
                    <a href="/cm/audit" class="nav-link">📜 Audit Log</a>
                    <a href="/cm/analytics" class="nav-link">📊 Analytics</a>
                    {{end}}
                </div>
                <div class="nav-section">
                    <div class="nav-section-title">Tools</div>
                    <a href="#" onclick="if(window.cpOpen){cpOpen();return false;}" class="nav-link">🤖 Copilot</a>
                    <a href="/cm/tools/agent" class="nav-link">🤵 CMS Agent</a>
                    <a href="/cm/tools/seo" class="nav-link">🧭 SEO &amp; AI</a>
                    <a href="/cm/tools/indexnow" class="nav-link">📡 IndexNow</a>
                    <a href="/cm/tools/search" class="nav-link">🔍 End User Search</a>
                    <a href="/cm/tools/chat" class="nav-link">💬 Chat Widget</a>
                    <a href="/cm/tools/broken-links" class="nav-link">🔗 Broken Link Finder</a>
                </div>
                <div class="nav-section">
                    <div class="nav-section-title">Inbox</div>
                    <a href="/cm/messages" class="nav-link">📬 Messages{{if .UnreadMessageCount}} <span class="nav-badge">{{.UnreadMessageCount}}</span>{{end}}</a>
                </div>
                <div class="nav-section">
                    <a href="/" target="_blank" class="nav-link">🌐 View Site</a>
                    {{if .CurrentUser}}<div style="color: #94a3b8; font-size: 0.75rem; padding: 0.25rem 0.75rem; word-break: break-all;">{{.CurrentUser.Email}}</div>{{end}}
                    <form method="POST" action="/cm/logout" style="margin: 0;">
                        {{.CSRFField}}
                        <button type="submit" class="nav-link logout-btn">🚪 Logout</button>
                    </form>
                </div>
            </nav>
        </aside>
        <main class="main-content">`

const adminLayoutEnd = `
        </main>
    </div>

    <!-- Delete confirmation modal -->
    <div id="delete-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
        <div style="background: #1e293b; border-radius: var(--radius); max-width: 450px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(239, 68, 68, 0.3);">
            <div style="padding: 1.5rem; border-bottom: 1px solid rgba(239, 68, 68, 0.2); background: #1a2332;">
                <h3 style="margin: 0; color: var(--danger);">Confirm Delete</h3>
            </div>
            <div style="padding: 1.5rem; background: #1e293b;">
                <p id="delete-modal-message" style="margin: 0; white-space: pre-line;">Are you sure you want to delete this item?</p>
            </div>
            <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(239, 68, 68, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                <button type="button" class="btn btn-outline" id="delete-cancel-btn">Cancel</button>
                <button type="button" class="btn" id="delete-confirm-btn" style="background: var(--danger); color: white;">Delete</button>
            </div>
        </div>
    </div>

    <!-- Info/Alert modal -->
    <div id="info-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
        <div style="background: #1e293b; border-radius: var(--radius); max-width: 450px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(99, 102, 241, 0.3);">
            <div style="padding: 1.5rem; border-bottom: 1px solid rgba(99, 102, 241, 0.2); background: #1a2332;">
                <h3 id="info-modal-title" style="margin: 0; color: var(--primary);">Information</h3>
            </div>
            <div style="padding: 1.5rem; background: #1e293b;">
                <p id="info-modal-message" style="margin: 0;"></p>
            </div>
            <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(99, 102, 241, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                <button type="button" class="btn" id="info-ok-btn" style="background: var(--primary); color: white;">OK</button>
            </div>
        </div>
    </div>

    <!-- Confirm modal -->
    <div id="confirm-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
        <div style="background: #1e293b; border-radius: var(--radius); max-width: 500px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(99, 102, 241, 0.3);">
            <div style="padding: 1.5rem; border-bottom: 1px solid rgba(99, 102, 241, 0.2); background: #1a2332;">
                <h3 id="confirm-modal-title" style="margin: 0; color: var(--primary);">Confirm</h3>
            </div>
            <div style="padding: 1.5rem; background: #1e293b;">
                <p id="confirm-modal-message" style="margin: 0;"></p>
            </div>
            <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(99, 102, 241, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                <button type="button" class="btn btn-outline" id="confirm-cancel-btn">Cancel</button>
                <button type="button" class="btn" id="confirm-ok-btn" style="background: var(--primary); color: white;">Confirm</button>
            </div>
        </div>
    </div>

    <!-- Revert confirmation modal -->
    <div id="revert-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); z-index: 10000; align-items: center; justify-content: center;">
        <div style="background: #1e293b; border-radius: var(--radius); max-width: 450px; width: 90%; box-shadow: 0 20px 60px rgba(0, 0, 0, 0.8); border: 1px solid rgba(245, 158, 11, 0.3);">
            <div style="padding: 1.5rem; border-bottom: 1px solid rgba(245, 158, 11, 0.2); background: #1a2332;">
                <h3 style="margin: 0; color: var(--warning);">Confirm Revert</h3>
            </div>
            <div style="padding: 1.5rem; background: #1e293b;">
                <p id="revert-modal-message" style="margin: 0; white-space: pre-line;"></p>
            </div>
            <div style="padding: 1rem 1.5rem; border-top: 1px solid rgba(245, 158, 11, 0.2); display: flex; gap: 0.75rem; justify-content: flex-end; background: #1a2332;">
                <button type="button" class="btn btn-outline" id="revert-cancel-btn">Cancel</button>
                <button type="button" class="btn" id="revert-confirm-btn" style="background: var(--warning); color: white;">Revert</button>
            </div>
        </div>
    </div>

    <script>
    // Revert confirmation modal
    var revertModalForm = null;
    function confirmRevert(form, version) {
        revertModalForm = form;
        var modal = document.getElementById('revert-modal');
        var msgEl = document.getElementById('revert-modal-message');
        var confirmBtn = document.getElementById('revert-confirm-btn');
        var cancelBtn = document.getElementById('revert-cancel-btn');

        // Shown as text (white-space: pre-line keeps the paragraph break)
        msgEl.textContent = 'Revert to version ' + version + '?\n\nA new version will be saved with the reverted content.';
        modal.style.display = 'flex';
        var closed = dialogOpened(cancelBtn, onCancel);

        function cleanup() {
            modal.style.display = 'none';
            confirmBtn.removeEventListener('click', onConfirm);
            cancelBtn.removeEventListener('click', onCancel);
            closed();
        }

        function onConfirm() {
            cleanup();
            if (revertModalForm) {
                revertModalForm.submit();
            }
        }

        function onCancel() {
            cleanup();
            revertModalForm = null;
        }

        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);

        return false; // Prevent form submission
    }

    // Delete confirmation modal
    var deleteModalForm = null;
    function confirmDelete(form, message) {
        deleteModalForm = form;
        var modal = document.getElementById('delete-modal');
        var msgEl = document.getElementById('delete-modal-message');
        var confirmBtn = document.getElementById('delete-confirm-btn');
        var cancelBtn = document.getElementById('delete-cancel-btn');

        // Shown as text; "\n" in a message is a line break (white-space: pre-line)
        msgEl.textContent = message || 'Are you sure you want to delete this item?';
        modal.style.display = 'flex';
        var closed = dialogOpened(cancelBtn, onCancel);

        function cleanup() {
            modal.style.display = 'none';
            confirmBtn.removeEventListener('click', onConfirm);
            cancelBtn.removeEventListener('click', onCancel);
            closed();
        }

        function onConfirm() {
            cleanup();
            if (deleteModalForm) {
                deleteModalForm.submit();
            }
        }

        function onCancel() {
            cleanup();
            deleteModalForm = null;
        }

        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);

        return false; // Prevent form submission
    }
    </script>

    {{if .CopilotEnabled}}
    <style>
    #cp-fab {
        position: fixed; right: 22px; bottom: 22px; z-index: 10500;
        width: 52px; height: 52px; border-radius: 50%; border: none; cursor: pointer;
        background: var(--primary, #4f6ef7); color: #fff; font-size: 24px;
        box-shadow: 0 6px 20px rgba(0,0,0,.35); transition: transform .15s;
    }
    #cp-fab:hover { transform: scale(1.08); }
    #cp-drawer {
        position: fixed; top: 0; right: 0; height: 100vh; width: min(480px, 96vw);
        background: var(--bg-card, #1e293b); border-left: 1px solid var(--border, #333);
        z-index: 10600; display: flex; flex-direction: row;
        transform: translateX(105%); transition: transform .22s ease, width .22s ease;
        box-shadow: -12px 0 40px rgba(0,0,0,.45);
    }
    #cp-drawer.open { transform: translateX(0); }
    #cp-drawer.full { width: 100vw; border-left: none; }
    #cp-side {
        width: 240px; flex: 0 0 240px; display: none; flex-direction: column;
        border-right: 1px solid var(--border, #333); background: rgba(0,0,0,.15);
    }
    #cp-drawer.full #cp-side, #cp-drawer.show-side #cp-side { display: flex; }
    #cp-search {
        margin: 10px; padding: 7px 10px; border: 1px solid var(--border, #444);
        border-radius: 8px; background: var(--bg, #111); color: var(--text, #eee);
        font: inherit; font-size: 12.5px;
    }
    #cp-sessions { flex: 1; overflow-y: auto; padding: 0 8px 10px; }
    .cp-sess {
        display: block; width: 100%; text-align: left; border: none; cursor: pointer;
        background: transparent; color: var(--text, #ddd); border-radius: 8px;
        padding: 8px 10px; margin-bottom: 2px; font: inherit; font-size: 12.5px; line-height: 1.35;
    }
    .cp-sess:hover { background: rgba(128,128,128,.12); }
    .cp-sess.active { background: rgba(79,110,247,.18); }
    .cp-sess .cp-sess-date { display: block; font-size: 10.5px; color: var(--text-muted, #888); margin-top: 1px; }
    .cp-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
    .cp-hbtn { border: 1px solid var(--border,#444); background: transparent; color: var(--text,#ddd);
        border-radius: 8px; cursor: pointer; font-size: 13px; padding: 4px 9px; }
    .cp-hbtn:hover { background: rgba(128,128,128,.12); }
    .cp-typing { display: inline-flex; gap: 4px; align-items: center; }
    .cp-typing span {
        width: 7px; height: 7px; border-radius: 50%;
        background: var(--text-muted, #999);
        animation: cp-bounce 1.2s infinite ease-in-out;
    }
    .cp-typing span:nth-child(2) { animation-delay: 0.15s; }
    .cp-typing span:nth-child(3) { animation-delay: 0.3s; }
    @keyframes cp-bounce {
        0%, 60%, 100% { transform: translateY(0); opacity: .45; }
        30% { transform: translateY(-5px); opacity: 1; }
    }
    .cp-status { margin-left: 8px; color: var(--text-muted, #888); font-size: 13px; }
    .cp-table { border-collapse: collapse; margin: 8px 0; font-size: 12.5px; width: 100%; }
    .cp-table th, .cp-table td { border: 1px solid var(--border, #444); padding: 4px 8px; text-align: left; vertical-align: top; }
    .cp-table th { background: rgba(128,128,128,.12); font-weight: 600; }
    .cp-table tr:nth-child(even) td { background: rgba(128,128,128,.05); }
    </style>
    <button id="cp-fab" title="Copilot">🤖</button>
    <div id="cp-drawer" aria-label="Copilot panel">
        <div id="cp-side">
            <input id="cp-search" type="search" placeholder="Search chats…">
            <div id="cp-sessions"></div>
        </div>
        <div class="cp-main">
            <div style="display:flex; align-items:center; gap:8px; padding:10px 14px; border-bottom:1px solid var(--border,#333);">
                <button id="cp-side-toggle" class="cp-hbtn" title="Chat history">☰</button>
                <strong style="flex:1;">🤖 Copilot</strong>
                <button id="cp-new" class="cp-hbtn" title="New chat">＋ New</button>
                <button id="cp-full" class="cp-hbtn" title="Toggle fullscreen">⛶</button>
                <button id="cp-close" class="cp-hbtn" title="Close">✕</button>
            </div>
            <div id="cp-log" style="flex:1; overflow-y:auto; padding:16px;"></div>
            <div style="border-top:1px solid var(--border,#333); padding:10px; display:flex; gap:8px;">
                <textarea id="cp-input" rows="2" placeholder="Ask the copilot…"
                    style="flex:1; resize:none; padding:9px; border:1px solid var(--border,#444); border-radius:8px; background:var(--bg,#111); color:var(--text,#eee); font:inherit; font-size:13px;"></textarea>
                <button id="cp-send" class="btn btn-primary" style="align-self:flex-end;">Send</button>
            </div>
        </div>
    </div>
    <script>
    (function() {
        const drawer = document.getElementById('cp-drawer');
        const fab = document.getElementById('cp-fab');
        const log = document.getElementById('cp-log');
        const input = document.getElementById('cp-input');
        const send = document.getElementById('cp-send');
        const newBtn = document.getElementById('cp-new');
        const sessList = document.getElementById('cp-sessions');
        const search = document.getElementById('cp-search');
        let messages = [];
        let renderLog = [];

        window.cpOpen = function() { drawer.classList.add('open'); input.focus(); };
        function cpClose() { drawer.classList.remove('open'); }
        fab.addEventListener('click', () => drawer.classList.contains('open') ? cpClose() : window.cpOpen());
        document.getElementById('cp-close').addEventListener('click', cpClose);
        document.getElementById('cp-full').addEventListener('click', () => drawer.classList.toggle('full'));
        document.getElementById('cp-side-toggle').addEventListener('click', () => drawer.classList.toggle('show-side'));
        document.addEventListener('keydown', e => { if (e.key === 'Escape') cpClose(); });
        if (new URLSearchParams(location.search).get('copilot') === '1') window.cpOpen();

        const STORE = 'lc_copilot_sessions';
        let sessionId = 'cs-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8);
        function loadStore() {
            try { return JSON.parse(localStorage.getItem(STORE)) || []; } catch (e) { return []; }
        }
        function saveSession() {
            if (!messages.length) return;
            let store = loadStore().filter(s => s.id !== sessionId);
            store.unshift({ id: sessionId, title: (messages[0].content || 'Chat').slice(0, 60), ts: Date.now(), messages: messages, renderLog: renderLog });
            if (store.length > 30) store = store.slice(0, 30);
            try { localStorage.setItem(STORE, JSON.stringify(store)); } catch (e) {}
            refreshHistory();
        }
        function matchesQuery(s, q) {
            if (!q) return true;
            if ((s.title || '').toLowerCase().indexOf(q) !== -1) return true;
            return (s.messages || []).some(m => (m.content || '').toLowerCase().indexOf(q) !== -1);
        }
        function refreshHistory() {
            const q = (search.value || '').trim().toLowerCase();
            const store = loadStore();
            sessList.innerHTML = '';
            store.filter(s => matchesQuery(s, q)).forEach(s => {
                const b = document.createElement('button');
                b.className = 'cp-sess' + (s.id === sessionId ? ' active' : '');
                const title = document.createElement('span');
                title.textContent = s.title || 'Chat';
                const date = document.createElement('span');
                date.className = 'cp-sess-date';
                date.textContent = new Date(s.ts).toLocaleString(undefined, {month:'short', day:'numeric', hour:'2-digit', minute:'2-digit'});
                b.appendChild(title); b.appendChild(date);
                b.addEventListener('click', () => { saveSession(); openSession(s.id); });
                sessList.appendChild(b);
            });
            if (!sessList.children.length) {
                const empty = document.createElement('div');
                empty.style.cssText = 'padding:10px; font-size:12px; color:var(--text-muted,#888);';
                empty.textContent = q ? 'No chats match.' : 'No previous chats yet.';
                sessList.appendChild(empty);
            }
        }
        function openSession(id) {
            const s = loadStore().find(x => x.id === id);
            if (!s) return;
            sessionId = s.id;
            messages = s.messages || [];
            renderLog = s.renderLog || [];
            log.innerHTML = '';
            renderLog.forEach(b => bubble(b.role, b.html, true));
            refreshHistory();
        }
        newBtn.addEventListener('click', () => {
            saveSession();
            sessionId = 'cs-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8);
            messages = []; renderLog = []; log.innerHTML = '';
            refreshHistory(); input.focus();
        });
        search.addEventListener('input', refreshHistory);
        refreshHistory();

        function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
        function mdInline(s) {
            return esc(s)
                .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
                .replace(/` + "`" + `([^` + "`" + `]+)` + "`" + `/g, '<code>$1</code>');
        }
        function isTableRow(line) { return /^\s*\|.*\|\s*$/.test(line); }
        function isTableSep(line) { return /^\s*\|?[\s:|-]+\|?\s*$/.test(line) && line.indexOf('-') !== -1; }
        function splitRow(line) {
            return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(c => c.trim());
        }
        function md(s) {
            const lines = s.split('\n');
            const out = [];
            let i = 0;
            while (i < lines.length) {
                if (isTableRow(lines[i]) && i + 1 < lines.length && isTableSep(lines[i + 1])) {
                    const head = splitRow(lines[i]);
                    i += 2;
                    const rows = [];
                    while (i < lines.length && isTableRow(lines[i])) { rows.push(splitRow(lines[i])); i++; }
                    let t = '<table class="cp-table"><thead><tr>';
                    head.forEach(h => t += '<th>' + mdInline(h) + '</th>');
                    t += '</tr></thead><tbody>';
                    rows.forEach(r => {
                        t += '<tr>';
                        for (let c = 0; c < head.length; c++) t += '<td>' + mdInline(r[c] || '') + '</td>';
                        t += '</tr>';
                    });
                    out.push(t + '</tbody></table>');
                    continue;
                }
                if (/^\s*[-*] /.test(lines[i])) {
                    let l = '<ul style="margin:6px 0 6px 18px;">';
                    while (i < lines.length && /^\s*[-*] /.test(lines[i])) {
                        l += '<li>' + mdInline(lines[i].replace(/^\s*[-*] /, '')) + '</li>';
                        i++;
                    }
                    out.push(l + '</ul>');
                    continue;
                }
                out.push(mdInline(lines[i]) + '<br>');
                i++;
            }
            return out.join('');
        }
        function bubble(role, html, restoring) {
            if (!restoring) renderLog.push({role: role, html: html});
            const div = document.createElement('div');
            div.style.cssText = 'margin-bottom:12px;max-width:92%;padding:9px 12px;border-radius:12px;line-height:1.5;font-size:13.5px;' +
                (role === 'user'
                    ? 'margin-left:auto;background:var(--primary,#4f6ef7);color:#fff;'
                    : 'background:var(--bg,#0f172a);border:1px solid var(--border,#333);');
            div.innerHTML = html;
            log.appendChild(div);
            log.scrollTop = log.scrollHeight;
            return div;
        }
        function actionChips(actions) {
            if (!actions || !actions.length) return '';
            return '<div style="margin-top:8px;">' + actions.map(a =>
                '<span style="display:inline-block;margin:2px 4px 0 0;padding:2px 10px;border-radius:999px;font-size:11.5px;background:rgba(80,160,80,.15);border:1px solid rgba(80,160,80,.4);">✓ ' + esc(a.summary) + '</span>'
            ).join('') + '</div>';
        }

        async function submit() {
            const text = input.value.trim();
            if (!text || send.disabled) return;
            input.value = '';
            bubble('user', esc(text));
            messages.push({role: 'user', content: text});
            send.disabled = true;
            const thinking = bubble('assistant',
                '<span class="cp-typing"><span></span><span></span><span></span></span>' +
                '<span class="cp-status">Thinking…</span>');
            const statusEl = thinking.querySelector('.cp-status');
            const phases = [
                [6000,  'Working — reading your site…'],
                [15000, 'Still working — running content tools…'],
                [35000, 'Long task — multiple edits may be in progress…'],
                [70000, 'Almost there — complex requests can take a couple of minutes…']
            ];
            const timers = phases.map(([ms, msg]) =>
                setTimeout(() => { if (statusEl) statusEl.textContent = msg; }, ms));
            try {
                const res = await fetch('/cm/copilot/chat', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json', 'X-CSRF-Token': {{.CSRFToken}}},
                    body: JSON.stringify({messages: messages})
                });
                const data = await res.json();
                if (!res.ok) throw new Error(data.error || res.statusText);
                thinking.innerHTML = md(data.reply || '(no reply)') + actionChips(data.actions);
                messages.push({role: 'assistant', content: data.reply || ''});
                renderLog[renderLog.length - 1] = {role: 'assistant', html: thinking.innerHTML};
                saveSession();
            } catch (err) {
                thinking.innerHTML = '<span style="color:#d9534f;">Error: ' + esc(err.message) + '</span>';
                renderLog[renderLog.length - 1] = {role: 'assistant', html: thinking.innerHTML};
            }
            timers.forEach(clearTimeout);
            send.disabled = false;
            input.focus();
        }
        send.addEventListener('click', submit);
        input.addEventListener('keydown', e => {
            if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); }
        });
    })();
    </script>
    {{end}}
</body>
</html>`
