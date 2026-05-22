// Login screen — self-hosted admin vibe. No photo mosaic, no marketing copy.
// Server diagnostics up top, tight form below, build/source info in the footer.

const {useState: _useStateL, useEffect: _useEffectL} = React;

function Login({onLogin, iconStyle, logoVariant = 'swirl', logoParams}) {
  const [user, setUser] = _useStateL('');
  const [pass, setPass] = _useStateL('');
  const [err, setErr] = _useStateL('');
  const [busy, setBusy] = _useStateL(false);

  // Minimal, non-sensitive info — version/build is the most you'd expose
  // on an unauthenticated /health endpoint.
  const version = 'v0.4.2';
  const build = '4f7a91d';

  const submit = (e) => {
    e && e.preventDefault();
    if (!user || !pass) { setErr('username and password required.'); return; }
    setErr(''); setBusy(true);
    setTimeout(() => {
      if (pass === 'wrong') { setErr('authentication failed. check the server log: journalctl -u gooru'); setBusy(false); return; }
      setBusy(false);
      onLogin(user);
    }, 500);
  };

  return (
    <div className="login-v2">
      <div className="login-v2-card">
        {/* Identity strip */}
        <div className="login-v2-id">
          <div className="login-v2-mark">
            <Logo variant={logoVariant} size={logoIsWordmark(logoVariant) ? 16 : 20} color="var(--accent)" {...(logoParams || {})}/>
            {!logoIsWordmark(logoVariant) && <span className="login-v2-wordmark">gooru</span>}
          </div>
          <div className="login-v2-id-meta">
            <span>{version}</span>
            <span className="sep">·</span>
            <span>{build}</span>
            <span className="sep">·</span>
            <span>gpl-3.0</span>
          </div>
        </div>

        {/* Form */}
        <form className="login-v2-form" onSubmit={submit}>
          <div className="login-v2-field">
            <label htmlFor="lg-user">user</label>
            <input id="lg-user" className="g-input" autoComplete="username"
                   spellCheck="false" autoCapitalize="off"
                   value={user} onChange={(e) => setUser(e.target.value)} autoFocus/>
          </div>
          <div className="login-v2-field">
            <label htmlFor="lg-pass">password</label>
            <input id="lg-pass" className="g-input" type="password" autoComplete="current-password"
                   value={pass} onChange={(e) => setPass(e.target.value)}/>
          </div>

          {err && (
            <div className="login-v2-error">
              <span className="prompt">!</span>
              <span>{err}</span>
            </div>
          )}

          <button type="submit" className="g-btn g-btn-primary" disabled={busy}
                  style={{height: 36, justifyContent: 'center', width: '100%'}}>
            {busy ? 'authenticating…' : 'sign in'}
          </button>
        </form>

        {/* Footer */}
        <div className="login-v2-foot">
          <div>
            <span className="mono">first run?</span>
            <span> on the server: </span>
            <code>gooru auth init</code>
          </div>
          <div className="login-v2-foot-links">
            <a href="#" tabIndex={-1}>docs</a>
            <span className="sep">·</span>
            <a href="#" tabIndex={-1}>source</a>
            <span className="sep">·</span>
            <a href="#" tabIndex={-1}>changelog</a>
          </div>
        </div>
      </div>

      {/* Background: subtle dotted grid, no photos */}
      <div className="login-v2-bg" aria-hidden="true"/>
    </div>
  );
}

Object.assign(window, {Login});
