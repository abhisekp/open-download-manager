import React, { useEffect, useState } from "react";

export function AddonManager({ api, token, onAdded }) {
  const [items, setItems] = useState([]);
  const [file, setFile] = useState(null);
  const [preview, setPreview] = useState(null);
  const [approved, setApproved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [selected, setSelected] = useState("");
  const [url, setUrl] = useState("");
  const [links, setLinks] = useState([]);
  const [networkApproved, setNetworkApproved] = useState(false);
  const [trackerSelected, setTrackerSelected] = useState("");
  const [trackerApproved, setTrackerApproved] = useState(false);
  const [trackerResult, setTrackerResult] = useState(null);
  const providers = items.filter(
    (item) => item.enabled && item.manifest.capability === "tracker-provider",
  );
  const currentProvider = providers.find(
    (item) => item.manifest.id === trackerSelected,
  );
  const resolvers = items.filter(
    (item) => item.enabled && item.manifest.capability === "url-resolver",
  );
  const currentAddon = items.find((item) => item.manifest.id === selected);
  const networkHosts = currentAddon?.manifest.network_hosts || [];
  useEffect(() => {
    setNetworkApproved(false);
  }, [selected, url, currentAddon?.digest]);
  useEffect(() => {
    setTrackerApproved(false);
    setTrackerResult(null);
  }, [trackerSelected, currentProvider?.digest]);
  useEffect(() => {
    if (selected && !resolvers.some((item) => item.manifest.id === selected)) {
      setSelected("");
      setLinks([]);
    }
  }, [items, selected]);
  async function reload() {
    setItems(await api("addons"));
  }
  useEffect(() => {
    reload().catch((e) => setError(e.message));
  }, []);
  async function perform(action) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function upload(path, packageFile, digest) {
    const response = await fetch("/api/addons/" + path, {
      method: "POST",
      body: packageFile,
      headers: {
        Authorization: "Bearer " + token,
        "Content-Type": "application/octet-stream",
        ...(digest ? { "X-ODM-Approve-SHA256": digest } : {}),
      },
    });
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }
  return (
    <section className="addon-manager" aria-labelledby="addons-title">
      <h2 id="addons-title">Application add-ons</h2>
      <p>
        Install a signed .odmx package from a publisher you trust. Add-ons
        cannot access your files. Provider requests require declared public
        HTTPS hosts and separate approval for each resolution or tracker fetch.
      </p>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      <label>
        Choose an add-on package
        <input
          type="file"
          accept=".odmx"
          disabled={busy}
          onChange={(event) => {
            const next = event.target.files?.[0];
            setFile(next || null);
            setPreview(null);
            setApproved(false);
            setNetworkApproved(false);
            setTrackerApproved(false);
            setTrackerResult(null);
            if (next)
              perform(async () => {
                if (next.size > 8 * 1024 * 1024)
                  throw new Error("Packages must be no larger than 8 MiB.");
                setPreview(await upload("inspect", next));
              });
          }}
        />
      </label>
      {preview && (
        <section
          className="addon-review"
          aria-label="Review add-on permissions"
        >
          <h3>
            {preview.manifest.name} · {preview.manifest.version}
          </h3>
          <p>{preview.manifest.description}</p>
          <dl>
            <dt>Capability</dt>
            <dd>
              {preview.manifest.capability === "tracker-provider"
                ? "Tracker suggestions (no torrent changes)"
                : "Download link resolver"}
            </dd>
            <dt>
              {preview.manifest.capability === "tracker-provider"
                ? "Signed feed URL"
                : "Receives full links from"}
            </dt>
            <dd>
              {preview.manifest.capability === "tracker-provider" ? (
                <code>{preview.manifest.feed_url}</code>
              ) : (
                preview.manifest.input_hosts.join(", ")
              )}
            </dd>
            <dt>May suggest links to</dt>
            <dd>
              {preview.manifest.capability === "tracker-provider"
                ? "Tracker URLs across hosts; separately review before contacting them"
                : (preview.manifest.output_hosts || []).join(", ")}
            </dd>
            <dt>Public HTTPS provider hosts</dt>
            <dd>
              {preview.manifest.network_hosts?.length
                ? preview.manifest.network_hosts.join(", ")
                : "None — no provider network permission"}
            </dd>
            <dt>Publisher key fingerprint</dt>
            <dd>
              <code>{preview.publisher}</code>
            </dd>
            <dt>Package fingerprint</dt>
            <dd>
              <code>{preview.digest}</code>
            </dd>
          </dl>
          <p>
            A valid signature verifies the key, not the publisher’s identity.
            {preview.manifest.capability === "tracker-provider"
              ? "Suggested trackers can use other hosts and ports. Fetching never contacts them."
              : "Input/output link permissions cover all ports on the listed hosts."}
            Provider requests use HTTPS port 443 only. Existing add-on grants
            are revoked by an update.
          </p>
          <details>
            <summary>Included license and notices</summary>
            <pre style={{ whiteSpace: "pre-wrap" }}>{preview.license}</pre>
          </details>
          <label className="permission">
            <input
              type="checkbox"
              checked={approved}
              disabled={busy}
              onChange={(e) => setApproved(e.target.checked)}
            />
            I trust this publisher and approve these permissions.
          </label>
          <button
            className="primary"
            disabled={!approved || busy || !file}
            onClick={() =>
              perform(async () => {
                await upload("install", file, preview.digest);
                await reload();
                setPreview(null);
                setApproved(false);
                setMessage(
                  "Installed. Enable the add-on when you want to use it.",
                );
              })
            }
          >
            Install reviewed package
          </button>
        </section>
      )}
      <h3>Installed add-ons</h3>
      {!items.length && <p>No application add-ons installed.</p>}
      <ul className="addon-list">
        {items.map((item) => (
          <li key={item.manifest.id}>
            <div>
              <strong>{item.manifest.name}</strong> · {item.manifest.version}
              <p>{item.manifest.description}</p>
            </div>
            <label className="permission">
              <input
                type="checkbox"
                checked={item.enabled}
                disabled={busy}
                onChange={(e) => {
                  const enabled = e.target.checked;
                  setLinks([]);
                  setNetworkApproved(false);
                  setTrackerApproved(false);
                  setTrackerResult(null);
                  const previous = items;
                  setItems((current) =>
                    current.map((row) =>
                      row.manifest.id === item.manifest.id
                        ? { ...row, enabled }
                        : row,
                    ),
                  );
                  perform(async () => {
                    try {
                      await api("addons/enable", {
                        id: item.manifest.id,
                        enabled,
                      });
                    } catch (error) {
                      setItems(previous);
                      throw error;
                    }
                    await reload();
                  });
                }}
              />
              Enabled
            </label>
            <button
              disabled={busy}
              onClick={() =>
                perform(async () => {
                  await api("addons/uninstall", { id: item.manifest.id });
                  setLinks([]);
                  await reload();
                })
              }
            >
              Uninstall {item.manifest.name}
            </button>
          </li>
        ))}
      </ul>
      {resolvers.length > 0 && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setLinks([]);
            perform(async () => {
              try {
                const result = await api("addons/resolve", {
                  id: selected,
                  url,
                  networkApproved,
                  digest: currentAddon?.digest,
                });
                setLinks(result.urls);
              } finally {
                setNetworkApproved(false);
              }
            });
          }}
        >
          <h3>Resolve a download link</h3>
          <label>
            Add-on
            <select
              value={selected}
              required
              disabled={busy}
              onChange={(e) => {
                setSelected(e.target.value);
                setLinks([]);
                setNetworkApproved(false);
              }}
            >
              <option value="">Choose an enabled add-on</option>
              {resolvers.map((item) => (
                <option key={item.manifest.id} value={item.manifest.id}>
                  {item.manifest.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Link
            <input
              type="url"
              required
              value={url}
              disabled={busy}
              onChange={(e) => {
                setUrl(e.target.value);
                setLinks([]);
                setNetworkApproved(false);
              }}
            />
          </label>
          {networkHosts.length > 0 && (
            <>
              <p>
                This add-on can send the entire entered link, including private
                query parameters and fragments, to {networkHosts.join(", ")}. No
                account passwords or cookies are supplied.
              </p>
              <label className="permission">
                <input
                  type="checkbox"
                  disabled={busy}
                  checked={networkApproved}
                  onChange={(e) => setNetworkApproved(e.target.checked)}
                />
                Allow provider requests to these hosts for this link and this
                exact package
              </label>
            </>
          )}
          <button
            disabled={
              busy || !selected || (networkHosts.length > 0 && !networkApproved)
            }
            type="submit"
          >
            Review resolved links
          </button>
        </form>
      )}
      {providers.length > 0 && (
        <section aria-label="Tracker-provider add-ons">
          <h3>Fetch tracker suggestions</h3>
          <p>
            Provider add-ons fetch their signed feed URL. Suggestions do not
            contact trackers or change any torrent. Review and apply selected
            URLs separately in a public torrent’s tracker controls.
          </p>
          <label>
            Tracker provider
            <select
              value={trackerSelected}
              disabled={busy}
              onChange={(e) => {
                setTrackerSelected(e.target.value);
                setTrackerApproved(false);
                setTrackerResult(null);
              }}
            >
              <option value="">Choose an enabled tracker provider</option>
              {providers.map((item) => (
                <option key={item.manifest.id} value={item.manifest.id}>
                  {item.manifest.name}
                </option>
              ))}
            </select>
          </label>
          {currentProvider && (
            <>
              <p>
                Feed: <code>{currentProvider.manifest.feed_url}</code>
              </p>
              <p>
                This fetch may contact{" "}
                {(currentProvider.manifest.network_hosts || []).join(", ")}. The
                provider can see your network address. No download link, torrent
                identifier, cookies or saved account credentials are supplied.
              </p>
              <label className="permission">
                <input
                  type="checkbox"
                  checked={trackerApproved}
                  disabled={busy}
                  onChange={(e) => setTrackerApproved(e.target.checked)}
                />
                Allow this tracker fetch for this exact package
              </label>
            </>
          )}
          <button
            disabled={busy || !currentProvider || !trackerApproved}
            onClick={() =>
              perform(async () => {
                const reviewed = currentProvider;
                setTrackerResult(null);
                try {
                  const result = await api("addons/trackers", {
                    id: reviewed.manifest.id,
                    networkApproved: trackerApproved,
                    digest: reviewed.digest,
                  });
                  if (
                    result.packageDigest !== reviewed.digest ||
                    result.addonId !== reviewed.manifest.id
                  )
                    throw new Error(
                      "Tracker response does not match the reviewed package",
                    );
                  setTrackerResult({
                    name: reviewed.manifest.name,
                    digest: reviewed.digest,
                    trackers: result.trackers,
                  });
                } finally {
                  setTrackerApproved(false);
                }
              })
            }
          >
            Fetch reviewed tracker suggestions
          </button>
          {trackerResult && (
            <section aria-label="Fetched tracker suggestions">
              <p role="status">
                {trackerResult.trackers.length} tracker suggestions from{" "}
                {trackerResult.name}. No torrents changed.
              </p>
              <details>
                <summary>Fetched package fingerprint</summary>
                <code>{trackerResult.digest}</code>
              </details>
              <label>
                Tracker suggestions
                <textarea
                  readOnly
                  rows={8}
                  value={trackerResult.trackers.join("\n")}
                />
              </label>
              <p>
                Copy only the URLs you want to review in a public torrent’s
                tracker controls.
              </p>
            </section>
          )}
        </section>
      )}
      {!!links.length && (
        <section aria-label="Resolved links">
          <ul>
            {links.map((link) => (
              <li key={link}>
                <code>{link}</code>
              </li>
            ))}
          </ul>
          <p>No download starts until you choose to add these links.</p>
          <button
            className="primary"
            disabled={busy}
            onClick={() =>
              perform(async () => {
                const result = await api("batch", { urls: links });
                const failed = result.results.filter((row) => row.error).length;
                if (failed)
                  throw new Error(
                    `${failed} link(s) could not be added. Check the download engine and review the list.`,
                  );
                setLinks([]);
                setMessage("Resolved links added to downloads.");
                await onAdded();
              })
            }
          >
            Add reviewed links to downloads
          </button>
        </section>
      )}
    </section>
  );
}
