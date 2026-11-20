import React, { useEffect, useState } from "react";

export function websiteOrigin(raw) {
  try {
    const site = new URL(raw);
    if (site.protocol !== "https:" || site.username || site.password) return "";
    site.hostname = site.hostname.replace(/\.$/, "");
    return site.origin;
  } catch {
    return "";
  }
}

export function SavedLogins({ api }) {
  const [data, setData] = useState(null),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const [label, setLabel] = useState(""),
    [site, setSite] = useState(""),
    [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [approved, setApproved] = useState(false),
    [removeID, setRemoveID] = useState("");
  const origin = websiteOrigin(site);
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const d = await api("logins");
        if (alive) setData(d);
      } catch (e) {
        if (alive) setError(e.message);
      }
    };
    load();
    const timer = setInterval(() => {
      if (!document.hidden) load();
    }, 1500);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, []);
  function edit(setter, value) {
    setter(value);
    setApproved(false);
  }
  async function submit(event) {
    event.preventDefault();
    if (!origin || !approved) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("logins", {
        action: "save",
        label: label.trim(),
        origin,
        username,
        password,
        approved: true,
      });
      setPassword("");
      setUsername("");
      setLabel("");
      setApproved(false);
      setNotice(
        "Saving to the system keyring. Complete any local unlock prompt, then check the status below.",
      );
      setData(await api("logins"));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function act(id, action) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("logins", {
        id,
        action,
        ...(action === "remove" ? { approved: true } : {}),
      });
      setRemoveID("");
      setData(await api("logins"));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="saved-logins" aria-label="Saved website logins">
      <h2>Saved website logins</h2>
      <p>
        For websites using HTTPS basic authentication. Passwords stay in this
        computer’s system keyring; download files and transfer records contain
        only a login reference. Select a saved login when adding a download in
        Queue & schedules.
      </p>
      {error && (
        <p role="alert" className="alert">
          {error.trim()}
        </p>
      )}
      {notice && (
        <p role="status" className="notice">
          {notice}
        </p>
      )}
      {!data ? (
        <p role="status">Loading saved logins…</p>
      ) : (
        <>
          <form onSubmit={submit} autoComplete="off">
            <label className="field">
              Login label
              <input
                required
                maxLength="128"
                value={label}
                onChange={(e) => edit(setLabel, e.target.value)}
                placeholder="Work downloads"
              />
            </label>
            <label className="field">
              HTTPS website
              <input
                required
                type="url"
                maxLength="2048"
                value={site}
                onChange={(e) => edit(setSite, e.target.value)}
                placeholder="https://downloads.example.com"
              />
            </label>
            <p>
              Website scope:{" "}
              <strong>
                {origin ||
                  "Enter an HTTPS website without a username or password in its address."}
              </strong>
            </p>
            <label className="field">
              Website username
              <input
                required
                autoComplete="off"
                value={username}
                onChange={(e) => edit(setUsername, e.target.value)}
              />
            </label>
            <label className="field">
              Website password
              <input
                required
                type="password"
                autoComplete="off"
                value={password}
                onChange={(e) => edit(setPassword, e.target.value)}
              />
            </label>
            <label className="queue-check">
              <input
                type="checkbox"
                checked={approved}
                onChange={(e) => setApproved(e.target.checked)}
                disabled={!origin}
              />{" "}
              Save this login for the HTTPS website shown above.
            </label>
            <p>
              Other websites, ports and insecure redirects never receive this
              login. Browser cookies, FTP/SFTP, OAuth and Windows integrated
              sign-in are separate features.
            </p>
            <button
              className="primary"
              disabled={busy || data.busy || !approved || !origin}
            >
              Save website login
            </button>
          </form>
          <h3>Stored logins</h3>
          {!data.logins.length ? (
            <p>No saved website logins.</p>
          ) : (
            <ul className="login-list">
              {data.logins.map((entry) => (
                <li key={entry.id}>
                  <strong>{entry.label}</strong> <span>{entry.origin}</span>
                  <p role="status">
                    {entry.state === "ready"
                      ? "Ready"
                      : entry.state === "error"
                        ? "Could not finish. Unlock the system keyring and retry."
                        : entry.state === "deleting"
                          ? "Removing login…"
                          : "Waiting for the system keyring…"}
                  </p>
                  {entry.action !== "remove" && (
                    <button
                      disabled={busy || data.busy}
                      onClick={() => act(entry.id, "check")}
                    >
                      Check {entry.label}
                    </button>
                  )}
                  <label className="queue-check">
                    <input
                      type="checkbox"
                      checked={removeID === entry.id}
                      onChange={(e) =>
                        setRemoveID(e.target.checked ? entry.id : "")
                      }
                    />{" "}
                    Confirm removal of {entry.label}
                  </label>
                  <button
                    disabled={busy || data.busy || removeID !== entry.id}
                    onClick={() => act(entry.id, "remove")}
                  >
                    Remove {entry.label}
                  </button>
                </li>
              ))}
            </ul>
          )}
          <p>
            Pause affected downloads before removing a login. Active connections
            may continue; later retries will need another login. If the system
            keyring is locked or unavailable, passwords are never saved to a
            plain file instead.
          </p>
        </>
      )}
    </section>
  );
}

export function JobLogin({ job, logins, busy, change }) {
  const [selected, setSelected] = useState(job.credentialRef || ""),
    [approved, setApproved] = useState(false);
  const choices = logins.filter(
    (entry) =>
      entry.state === "ready" && entry.origin === websiteOrigin(job.url),
  );
  return (
    <details className="job-login">
      <summary>Change login</summary>
      <label>
        Saved login
        <select
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setApproved(false);
          }}
        >
          <option value="">No saved login</option>
          {job.credentialRef &&
            !choices.some((entry) => entry.id === job.credentialRef) && (
              <option value={job.credentialRef}>Unavailable saved login</option>
            )}
          {choices.map((entry) => (
            <option key={entry.id} value={entry.id}>
              {entry.label}
            </option>
          ))}
        </select>
      </label>
      <label className="queue-check">
        <input
          type="checkbox"
          checked={approved}
          onChange={(e) => setApproved(e.target.checked)}
        />{" "}
        Restart this partial download with the chosen login.
      </label>
      <button
        disabled={busy || !approved || selected === (job.credentialRef || "")}
        onClick={async () => {
          await change(
            {
              action: "login",
              gid: job.id,
              credentialRef: selected,
              approved: true,
            },
            "Login changed. Partial progress was reset; choose Start now to download again.",
          );
          setApproved(false);
        }}
      >
        Apply login
      </button>
    </details>
  );
}
