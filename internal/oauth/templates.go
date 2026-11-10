package oauth

var oauthTemplates = map[string]string{
	"authorize": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authorize - LightCMS</title>
    <link rel="icon" type="image/x-icon" href="/static/images/favicon.ico">
    <link rel="icon" type="image/png" sizes="16x16" href="/static/images/favicon-16x16.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/static/images/favicon-32x32.png">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
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
        .consent-info {
            background: rgba(15, 23, 42, 0.5);
            border: 1px solid rgba(99, 102, 241, 0.2);
            border-radius: 12px;
            padding: 1.25rem;
            margin-bottom: 1.5rem;
        }
        .consent-info p {
            color: #cbd5e1;
            font-size: 0.9rem;
            line-height: 1.6;
        }
        .consent-info .client-name {
            color: #a5b4fc;
            font-weight: 600;
        }
        .consent-info .access-note {
            margin-top: 0.5rem;
            color: #94a3b8;
            font-size: 0.85rem;
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
            margin-bottom: 0.5rem;
        }
        input[type="email"]:focus, input[type="password"]:focus {
            outline: none;
            border-color: #6366f1;
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }
        .btn {
            display: block;
            width: 100%;
            padding: 0.875rem;
            border: none;
            border-radius: 12px;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
            margin-top: 1rem;
        }
        .btn-primary {
            background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
            color: white;
        }
        .btn-primary:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
        }
        .btn-deny {
            background: transparent;
            border: 1px solid rgba(239, 68, 68, 0.4);
            color: #f87171;
            margin-top: 0.75rem;
        }
        .btn-deny:hover {
            background: rgba(239, 68, 68, 0.1);
        }
        .btn.loading {
            opacity: 0.7;
            cursor: wait;
            pointer-events: none;
            position: relative;
        }
        .btn.loading::after {
            content: '';
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid rgba(255,255,255,0.3);
            border-top-color: white;
            border-radius: 50%;
            animation: spin 0.6s linear infinite;
            margin-left: 8px;
            vertical-align: middle;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="logo"><img src="/static/images/lightcms-logo.png" alt="LightCMS"></div>
        {{if .ShowLogin}}
            <p class="subtitle">Sign in to authorize access</p>
            {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
            <form method="POST" action="/oauth/authorize">
                <input type="hidden" name="client_id" value="{{.ClientID}}">
                <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
                <input type="hidden" name="response_type" value="{{.ResponseType}}">
                <input type="hidden" name="state" value="{{.State}}">
                <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
                <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
                <input type="hidden" name="resource" value="{{.Resource}}">
                <input type="hidden" name="scope" value="{{.Scope}}">
                <input type="hidden" name="action" value="login">
                <label for="email">Email</label>
                <input type="email" id="email" name="email" required autofocus>
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
                <button type="submit" class="btn btn-primary">Sign In</button>
            </form>
        {{else}}
            <p class="subtitle">Authorize application access</p>
            <div class="consent-info">
                <p><span class="client-name">{{.ClientName}}</span> wants to connect to this site as <strong>{{.UserEmail}}</strong>.</p>
                <p class="access-note">After you approve, your browser goes to <strong>{{.RedirectHost}}</strong>. The app chose its own name, so continue only if you started this connection and you recognise that address.</p>
                <p class="access-note">It will act with your {{.UserRole}} role: {{.RoleSummary}}. You can disconnect it at any time under Connected Apps.</p>
            </div>
            <form method="POST" action="/oauth/authorize">
                <input type="hidden" name="client_id" value="{{.ClientID}}">
                <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
                <input type="hidden" name="response_type" value="{{.ResponseType}}">
                <input type="hidden" name="state" value="{{.State}}">
                <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
                <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
                <input type="hidden" name="resource" value="{{.Resource}}">
                <input type="hidden" name="scope" value="{{.Scope}}">
                <input type="hidden" name="login_user" value="{{.LoginUser}}">
                <input type="hidden" name="login_proof" value="{{.LoginProof}}">
                <input type="hidden" name="login_ts" value="{{.LoginTS}}">
                <input type="hidden" name="action" value="approve">
                <button type="submit" class="btn btn-primary">Allow Access</button>
                <button type="submit" name="action" value="deny" class="btn btn-deny">Deny</button>
            </form>
        {{end}}
    </div>
    <script>
    document.querySelectorAll('form').forEach(function(form) {
        form.addEventListener('submit', function() {
            var btn = form.querySelector('button[type="submit"]:focus') || form.querySelector('.btn-primary');
            if (btn && !btn.classList.contains('loading')) {
                var label = btn.textContent;
                btn.classList.add('loading');
                btn.textContent = btn.classList.contains('btn-deny') ? 'Denying...' :
                    label.includes('Sign') ? 'Signing in...' : 'Authorizing...';
            }
        });
    });
    </script>
</body>
</html>`,
}
