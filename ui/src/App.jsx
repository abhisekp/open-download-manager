import React, { useState, useEffect } from "react";
import { BrowserConnections } from "./BrowserConnections.jsx";
import { AddonManager } from "./AddonManager.jsx";
import "bootstrap-icons/font/bootstrap-icons.css";
const I = ({ name }) => <i aria-hidden="true" className={"bi bi-" + name} />;
const size = (n) => {
  n = Number(n) || 0;
  return n >= 1073741824
    ? (n / 1073741824).toFixed(2) + " GB"
    : n >= 1048576
      ? (n / 1048576).toFixed(1) + " MB"
      : (n / 1024).toFixed(1) + " KB";
};
const pct = (d) =>
  Number(d.totalLength)
    ? Math.min(100, (100 * Number(d.completedLength)) / Number(d.totalLength))
    : 0;
const name = (d) =>
  d.bittorrent?.info?.name ||
  d.files?.[0]?.path?.split(/[\\/]/).pop() ||
  d.files?.[0]?.uris?.[0]?.uri?.split("/").pop()?.split("?")[0] ||
  "Retrieving metadata…";
const status = {
  active: "Downloading",
  waiting: "Queued",
  paused: "Paused",
  complete: "Complete",
  error: "Failed",
  removed: "Removed",
};
function Modal({ title, onClose, children }) {
  const ref = React.useRef(null);
  useEffect(() => {
    ref.current?.querySelector("input,button")?.focus();
  }, []);
  return (
    <div className="scrim">
      <section
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="dialog"
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
          if (e.key === "Tab") {
            const nodes = [
              ...e.currentTarget.querySelectorAll("button,input,textarea"),
            ].filter((n) => !n.disabled);
            const first = nodes[0],
              last = nodes[nodes.length - 1];
            if (e.shiftKey && document.activeElement === first) {
              e.preventDefault();
              last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
              e.preventDefault();
              first.focus();
            }
          }
        }}
      >
        <h2>{title}</h2>
        {children}
      </section>
    </div>
  );
}
export function App() {
  const [verifyTarget, setVerifyTarget] = useState(null),
    [checksum, setChecksum] = useState(""),
    [removeTarget, setRemoveTarget] = useState(null);
  const [token] = useState(() => {
    const t =
      location.hash.slice(1) || sessionStorage.getItem("odm-token") || "";
    if (t) sessionStorage.setItem("odm-token", t);
    history.replaceState(null, "", location.pathname);
    return t;
  });
  const [data, setData] = useState({ downloads: [], stats: {} }),
    [error, setError] = useState(""),
    [view, setView] = useState("Downloads"),
    [selected, setSelected] = useState(""),
    [dialog, setDialog] = useState(false),
    [url, setUrl] = useState(""),
    [busy, setBusy] = useState(false),
    [limit, setLimit] = useState("0"),
    [grant, setGrant] = useState(false),
    [trackers, setTrackers] = useState([]),
    [notice, setNotice] = useState(""),
    [search, setSearch] = useState("");
  async function api(path, body) {
    const res = await fetch("/api/" + path, {
      method: body ? "POST" : "GET",
      headers: {
        Authorization: "Bearer " + token,
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) throw new Error(await res.text());
    return res.json();
  }
  async function refresh() {
    try {
      const d = await api("status");
      setData({ ...d, downloads: d.downloads || [] });
    } catch (e) {
      setError(e.message);
    }
  }
  useEffect(() => {
    refresh();
    const t = setInterval(() => {
      if (!document.hidden) refresh();
    }, 1500);
    return () => clearInterval(t);
  }, []);
  async function act(path, body) {
    setBusy(true);
    setNotice("");
    setError("");
    try {
      await api(path, body);
      await refresh();
      return true;
    } catch (e) {
      setError(e.message);
      return false;
    } finally {
      setBusy(false);
    }
  }
  async function add(e) {
    e.preventDefault();
    const urls = [
      ...new Set(
        url
          .split(/\r?\n/)
          .map((u) => u.trim())
          .filter(Boolean),
      ),
    ];
    if (urls.length > 1) {
      setBusy(true);
      setError("");
      try {
        const out = await api("batch", { urls });
        const failed = out.results.filter((r) => r.error).length;
        await refresh();
        if (failed) {
          setError(
            `${failed} links failed; successful links are already queued. Remove those links before retrying.`,
          );
        } else {
          setDialog(false);
          setUrl("");
          setNotice(`${out.results.length} links added.`);
        }
      } catch (e) {
        setError(e.message);
      } finally {
        setBusy(false);
      }
      return;
    }
    if (await act("add", { url })) {
      setDialog(false);
      setUrl("");
      setNotice("Download added to the queue.");
    }
  }
  const rows = data.downloads.filter(
      (d) =>
        (view !== "Torrents" || d.bittorrent || d.infoHash) &&
        name(d).toLowerCase().includes(search.toLowerCase()),
    ),
    d = rows.find((d) => d.gid === selected) || rows[0];
  const count = (s) => data.downloads.filter((d) => d.status === s).length;
  async function importTorrent(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > 4 * 1024 * 1024) {
      setError("Torrent metadata must be 4 MiB or less.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/torrent-file", {
        method: "POST",
        headers: {
          Authorization: "Bearer " + token,
          "Content-Type": "application/x-bittorrent",
        },
        body: file,
      });
      if (!res.ok) throw new Error(await res.text());
      await refresh();
      setDialog(false);
      setNotice("Torrent added to the native engine.");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function verify(e) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const out = await api("verify", {
        gid: verifyTarget.gid,
        checksum: checksum.trim(),
      });
      if (out.matches) {
        setNotice("SHA-256 matches. The file is intact.");
        setVerifyTarget(null);
      } else setError("Checksum mismatch. Actual SHA-256: " + out.sha256);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  function controls(d) {
    return (
      <>
        {d.status === "complete" && (
          <button
            disabled={busy}
            onClick={() => {
              setChecksum("");
              setError("");
              setVerifyTarget(d);
            }}
          >
            <I name="shield-check" />
            Verify checksum
          </button>
        )}
        {["active", "waiting"].includes(d.status) && (
          <button
            disabled={busy}
            onClick={() => act("control", { gid: d.gid, action: "pause" })}
          >
            <I name="pause-fill" />
            Pause
          </button>
        )}
        {d.status === "paused" && (
          <button
            disabled={busy}
            onClick={() => act("control", { gid: d.gid, action: "resume" })}
          >
            <I name="play-fill" />
            Resume
          </button>
        )}
        <button
          disabled={busy}
          onClick={() => {
            setError("");
            setRemoveTarget(d);
          }}
        >
          <I name="x-lg" />
          Remove
        </button>
      </>
    );
  }
  return (
    <div className="app">
      <aside inert={Boolean(dialog || verifyTarget || removeTarget)}>
        <div className="brand">
          <img src="/assets/odm-icon.png" alt="" width="46" height="46" />
          <div>
            Open Download<small>Manager</small>
          </div>
        </div>
        <nav aria-label="Main navigation">
          {[
            ["Downloads", "download"],
            ["Torrents", "magnet"],
            ["Extensions", "puzzle"],
            ["Settings", "gear"],
          ].map(([v, i]) => (
            <button
              key={v}
              className={"nav " + (view === v ? "active" : "")}
              onClick={() => setView(v)}
            >
              <I name={i} />
              {v}
              {v === "Downloads" && (
                <b className="badge">{data.downloads.length}</b>
              )}
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          Local. Open. Yours.
          <small>Early development · {data.version || "0.1.0-dev"}</small>
        </div>
      </aside>
      <main inert={Boolean(dialog || verifyTarget || removeTarget)}>
        <header>
          <div>
            <h1>{view}</h1>
            <p>
              {["Downloads", "Torrents"].includes(view)
                ? `${rows.length} transfers · ${count("active")} active, ${count("paused")} paused, ${count("complete")} complete`
                : view === "Extensions"
                  ? "Add capabilities, with your permission."
                  : "Make downloads work for you."}
            </p>
          </div>
          <button className="primary" onClick={() => setDialog(true)}>
            <I name="plus-lg" />
            Add download
          </button>
        </header>
        {error && (
          <div role="alert" className="alert">
            {error.trim()} <button onClick={refresh}>Retry</button>
          </div>
        )}
        {notice && (
          <div role="status" className="notice">
            {notice}
          </div>
        )}
        {["Downloads", "Torrents"].includes(view) && (
          <>
            <div className="toolbar">
              <label>
                <I name="search" />
                <input
                  aria-label="Search downloads"
                  placeholder="Search downloads"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </label>
              <span>
                <I name="shield-check" /> Files stay on this device
              </span>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Status</th>
                    <th>Progress</th>
                    <th>Size</th>
                    <th>Speed</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr
                      key={row.gid}
                      className={d?.gid === row.gid ? "selected" : ""}
                    >
                      <td>
                        <button
                          className="file"
                          onClick={() => setSelected(row.gid)}
                        >
                          <I
                            name={row.bittorrent ? "magnet" : "file-earmark"}
                          />
                          <strong>{name(row)}</strong>
                        </button>
                      </td>
                      <td className={"status " + row.status}>
                        <I
                          name={
                            row.status === "complete"
                              ? "check-lg"
                              : row.status === "paused"
                                ? "pause-fill"
                                : "arrow-down-short"
                          }
                        />
                        {status[row.status] || row.status}
                      </td>
                      <td>
                        <div className="row-progress">
                          <span>{pct(row).toFixed(0)}%</span>
                          <progress
                            max="100"
                            value={pct(row)}
                            aria-label={"Progress for " + name(row)}
                          />
                        </div>
                      </td>
                      <td>{size(row.totalLength)}</td>
                      <td>
                        {row.status === "active"
                          ? size(row.downloadSpeed) + "/s"
                          : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!rows.length && (
              <section className="empty">
                <I name="download" />
                <h2>
                  {search ? "No matching downloads" : "Ready when you are"}
                </h2>
                <p>Add a link and keep every transfer in one place.</p>
                <button className="primary" onClick={() => setDialog(true)}>
                  Add your first download
                </button>
              </section>
            )}
            {d && (
              <section className="details">
                <div className="detail-heading">
                  <div className="detail-file">
                    <I name={d.bittorrent ? "magnet" : "file-earmark"} />
                    <div>
                      <h2>{name(d)}</h2>
                      <p>
                        {status[d.status]} · {size(d.totalLength)}
                      </p>
                    </div>
                  </div>
                  <div className="actions">{controls(d)}</div>
                </div>
                <div className="big-progress">
                  <strong>{pct(d).toFixed(0)}%</strong>
                  <span>
                    {size(d.completedLength)} of {size(d.totalLength)}
                  </span>
                </div>
                <progress
                  max="100"
                  value={pct(d)}
                  aria-label="Selected download progress"
                />
                <dl>
                  <div>
                    <dt>Speed</dt>
                    <dd>{size(d.downloadSpeed)}/s</dd>
                  </div>
                  <div>
                    <dt>Remaining</dt>
                    <dd>
                      {Number(d.downloadSpeed)
                        ? Math.ceil(
                            (Number(d.totalLength) -
                              Number(d.completedLength)) /
                              Number(d.downloadSpeed),
                          ) + " sec"
                        : "—"}
                    </dd>
                  </div>
                  <div>
                    <dt>Downloaded</dt>
                    <dd>{size(d.completedLength)}</dd>
                  </div>
                  <div>
                    <dt>{d.native ? "Peers" : "Connections"}</dt>
                    <dd>{d.connections || 0}</dd>
                  </div>
                </dl>
                {d.native && (
                  <p>
                    Native BitTorrent � {d.torrentState} �{" "}
                    {d.private ? "Private torrent" : "Public torrent"}
                  </p>
                )}
                <p className="destination">
                  <I name="folder2" /> {data.directory}
                </p>
                {d.errorMessage && (
                  <p role="alert">Transfer error: {d.errorMessage}</p>
                )}
              </section>
            )}
          </>
        )}
        {view === "Extensions" && (
          <section className="settings">
            <AddonManager api={api} token={token} onAdded={refresh} />
            <div className="extension-heading">
              <I name="puzzle" />
              <div>
                <h2>Live tracker list</h2>
                <p>newTrackon · v1.0.0 · Built-in extension</p>
              </div>
            </div>
            <p>
              Fetch responding public trackers for review. This extension never
              changes torrents automatically.
            </p>
            <label className="permission">
              <input
                type="checkbox"
                checked={grant}
                onChange={(e) => {
                  setGrant(e.target.checked);
                  setTrackers([]);
                }}
              />
              Allow this extension to connect to newtrackon.com
            </label>
            <button
              className="primary"
              disabled={!grant || busy}
              onClick={async () => {
                setBusy(true);
                try {
                  const out = await api("trackers", { enabled: grant });
                  setTrackers(out.trackers);
                  setNotice("Live trackers fetched.");
                } catch (e) {
                  setError(e.message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              <I name="arrow-clockwise" />
              Fetch live trackers
            </button>
            {trackers.length > 0 && (
              <>
                <p>
                  {trackers.length} trackers · Do not add public trackers to
                  private torrents.
                </p>
                <textarea
                  aria-label="Live trackers"
                  readOnly
                  rows="12"
                  value={trackers.join("\n\n")}
                />
                <button
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(
                        trackers.join("\n\n"),
                      );
                      setNotice("Trackers copied.");
                    } catch {
                      setNotice("Select and copy the text above.");
                    }
                  }}
                >
                  Copy list
                </button>
              </>
            )}
            <div className="boundary">
              <h3>Extension API v1</h3>
              <p>
                Signed local URL resolvers run in an isolated worker with
                reviewed host permissions. The built-in tracker feed has a
                separate network grant. Shell commands and unrestricted scripts
                are not supported.
              </p>
            </div>
          </section>
        )}
        {view === "Settings" && (
          <section className="settings">
            <h2>Transfer preferences</h2>
            <BrowserConnections api={api} />
            <label className="field">
              HTTP/FTP/SFTP download limit (bytes per second)
              <input
                type="number"
                min="0"
                step="1"
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
              />
            </label>
            <p>
              Use 0 for unlimited. Applies to aria2 transfers in this session;
              native torrents have separate limits.
            </p>
            <button
              className="primary"
              disabled={busy}
              onClick={async () => {
                if (await act("limits", { limit }))
                  setNotice("Speed limit updated.");
              }}
            >
              Save speed limit
            </button>
            <div className="boundary">
              <h3>Download folder</h3>
              <p>{data.directory}</p>
              <p>
                Change the folder at startup with the downloads option. Files
                are never automatically opened.
              </p>
              <h3>Privacy & torrent defaults</h3>
              <p>
                No telemetry or cloud account. BitTorrent runs inside ODM. DHT
                and peer exchange are disabled. Seeding stops on completion.
              </p>
              <h3>Session recovery</h3>
              <p>
                Unfinished transfers are saved every 10 seconds. Closing the
                browser leaves downloads running. Stop the application with
                Ctrl+C in its terminal to save and exit.
              </p>
            </div>
          </section>
        )}
      </main>
      <footer inert={Boolean(dialog || verifyTarget || removeTarget)}>
        <I name="arrow-left-right" />
        <span>Transfers: {data.downloads.length}</span>
        <span>Downloading: {size(data.stats?.downloadSpeed)}/s</span>
        <span>Uploading: {size(data.stats?.uploadSpeed)}/s</span>
        <span className="footer-right">
          <I name="shield-check" /> Local session
        </span>
      </footer>
      {verifyTarget && (
        <Modal title="Verify checksum" onClose={() => setVerifyTarget(null)}>
          <p>
            Verify the first file of {name(verifyTarget)} against a published
            SHA-256 checksum. Supports files up to 1 GiB.
          </p>
          <form onSubmit={verify}>
            {error && (
              <p className="alert" role="alert">
                {error}
              </p>
            )}
            <label className="field">
              Expected SHA-256
              <input
                required
                minLength={64}
                maxLength={64}
                pattern="[a-fA-F0-9]{64}"
                value={checksum}
                onChange={(e) => setChecksum(e.target.value)}
              />
            </label>
            <div className="dialog-actions">
              <button type="button" onClick={() => setVerifyTarget(null)}>
                Cancel
              </button>
              <button className="primary" disabled={busy}>
                {busy ? "Verifying�" : "Verify"}
              </button>
            </div>
          </form>
        </Modal>
      )}
      {removeTarget && (
        <Modal title="Remove transfer" onClose={() => setRemoveTarget(null)}>
          <p>
            Remove {name(removeTarget)} from the list? Downloaded files will
            remain on disk.
          </p>
          {error && (
            <p role="alert" className="alert">
              {error}
            </p>
          )}
          <div className="dialog-actions">
            <button onClick={() => setRemoveTarget(null)}>Cancel</button>
            <button
              className="primary"
              disabled={busy}
              onClick={async () => {
                if (
                  await act("control", {
                    gid: removeTarget.gid,
                    action: ["complete", "error", "removed"].includes(
                      removeTarget.status,
                    )
                      ? "forget"
                      : "remove",
                  })
                )
                  setRemoveTarget(null);
              }}
            >
              Remove transfer
            </button>
          </div>
        </Modal>
      )}
      {dialog && (
        <div className="scrim">
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="add-title"
            className="dialog"
            onKeyDown={(e) => {
              if (e.key === "Escape") setDialog(false);
              if (e.key === "Tab") {
                const nodes = [
                  ...e.currentTarget.querySelectorAll("button,input,textarea"),
                ].filter((n) => !n.disabled);
                const first = nodes[0],
                  last = nodes[nodes.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                  e.preventDefault();
                  last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                  e.preventDefault();
                  first.focus();
                }
              }
            }}
          >
            <button
              className="close"
              aria-label="Close dialog"
              onClick={() => setDialog(false)}
            >
              <I name="x-lg" />
            </button>
            <h2 id="add-title">Add a download</h2>
            <p>Paste one link per line, up to 50 links.</p>
            <label className="field">
              Or open a torrent file
              <input
                type="file"
                accept=".torrent,application/x-bittorrent"
                disabled={busy}
                onChange={importTorrent}
              />
            </label>
            <form onSubmit={add}>
              {error && (
                <p role="alert" className="alert">
                  {error}
                </p>
              )}
              <label className="field">
                Download link
                <textarea
                  autoFocus
                  required
                  rows="4"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://example.com/file.zip"
                />
              </label>
              <p className="hint">
                HTTP, HTTPS, FTP, SFTP and BitTorrent magnets. Links can contact
                servers on your local network.
              </p>
              <div className="dialog-actions">
                <button type="button" onClick={() => setDialog(false)}>
                  Cancel
                </button>
                <button type="submit" className="primary" disabled={busy}>
                  {busy ? "Adding…" : "Add download"}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
    </div>
  );
}
