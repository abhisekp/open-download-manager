import React, { useEffect, useState } from "react";
import { JobLogin, websiteOrigin } from "./SavedLogins.jsx";

const pending = (job) =>
  ["waiting", "paused", "scheduled", "retrying"].includes(job.status);
const label = {
  waiting: "Queued",
  paused: "Paused",
  scheduled: "Scheduled",
  retrying: "Waiting to retry",
  active: "Downloading",
  finalizing: "Finishing",
  error: "Failed",
  complete: "Complete",
  removed: "Removed",
};

export function HttpQueue({ api, refreshDownloads }) {
  const [data, setData] = useState(null),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const [concurrent, setConcurrent] = useState("3"),
    [perHost, setPerHost] = useState("3"),
    [url, setURL] = useState(""),
    [later, setLater] = useState(false),
    [start, setStart] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [singleStream, setSingleStream] = useState(false);
  const [retryLimit, setRetryLimit] = useState("3");
  const [logins, setLogins] = useState([]),
    [credentialRef, setCredentialRef] = useState("");
  useEffect(() => {
    let alive = true;
    const load = async (initial = false) => {
      try {
        const d = await api("http-queue");
        if (!alive) return;
        setData(d);
        if (initial) {
          setConcurrent(String(d.settings.concurrent));
          setPerHost(String(d.settings.perHost));
        }
        try {
          const saved = await api("logins");
          if (alive) setLogins(saved.logins);
        } catch {
          if (alive) setLogins([]);
        }
      } catch (e) {
        if (alive) setError(e.message);
      }
    };
    load(true);
    const timer = setInterval(() => {
      if (!document.hidden) load();
    }, 1500);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [attempt]);
  async function change(body, message, clearForm = false) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("http-queue", body);
      setNotice(message);
      if (clearForm) {
        setURL("");
        setStart("");
        setCredentialRef("");
      }
      setData(await api("http-queue"));
      await refreshDownloads();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function control(gid, action) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("control", { gid, action });
      setData(await api("http-queue"));
      await refreshDownloads();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  function add(event) {
    event.preventDefault();
    const when = later ? new Date(start).getTime() : 0;
    if (later && (!Number.isFinite(when) || when <= Date.now())) {
      setError("Choose a future local date and time.");
      return;
    }
    change(
      {
        action: "add",
        url: url.trim(),
        startAt: when,
        singleStream,
        credentialRef,
        retryLimit: Number(retryLimit),
      },
      later
        ? "Schedule saved. ODM must be running to start the download."
        : "Download added to the queue.",
      true,
    );
  }
  if (!data)
    return (
      <section className="queue-panel" aria-label="HTTP queue and schedules">
        {error ? (
          <>
            <p role="alert" className="alert">
              {error.trim()}
            </p>
            <button
              onClick={() => {
                setError("");
                setAttempt((n) => n + 1);
              }}
            >
              Retry queue
            </button>
          </>
        ) : (
          <p role="status">Loading queue…</p>
        )}
      </section>
    );
  return (
    <section className="queue-panel" aria-label="HTTP queue and schedules">
      <p>
        Manage HTTP and HTTPS downloads added here. Torrents and downloads added
        through other engines have separate queues.
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
      <form
        onSubmit={(event) => {
          event.preventDefault();
          change(
            {
              action: "settings",
              settings: {
                concurrent: Number(concurrent),
                perHost: Number(perHost),
              },
            },
            "Queue limits saved. Active downloads finish without interruption.",
          );
        }}
      >
        <h2>Queue limits</h2>
        <div className="queue-fields">
          <label>
            Active downloads
            <input
              type="number"
              min="1"
              max="8"
              required
              value={concurrent}
              onChange={(e) => setConcurrent(e.target.value)}
            />
          </label>
          <label>
            Downloads per host
            <input
              type="number"
              min="1"
              max={Math.min(3, Number(concurrent) || 1)}
              required
              value={perHost}
              onChange={(e) => setPerHost(e.target.value)}
            />
          </label>
          <button disabled={busy} type="submit">
            Save queue limits
          </button>
        </div>
        <p>
          Limits apply when a new download starts. Lowering them lets current
          downloads finish without interruption.
        </p>
      </form>
      <form onSubmit={add}>
        <h2>Queue a download</h2>
        <label>
          HTTP or HTTPS link
          <input
            type="url"
            required
            maxLength="8192"
            value={url}
            onChange={(e) => {
              setURL(e.target.value);
              setCredentialRef("");
            }}
            placeholder="https://example.com/file.zip"
          />
        </label>
        {websiteOrigin(url) && (
          <div>
            <label htmlFor="queue-saved-login">Saved website login</label>
            <select
              id="queue-saved-login"
              aria-describedby="queue-login-help"
              value={credentialRef}
              onChange={(e) => setCredentialRef(e.target.value)}
            >
              <option value="">No saved login</option>
              {logins
                .filter(
                  (entry) =>
                    entry.state === "ready" &&
                    entry.origin === websiteOrigin(url),
                )
                .map((entry) => (
                  <option key={entry.id} value={entry.id}>
                    {entry.label}
                  </option>
                ))}
            </select>
            <small id="queue-login-help">
              Only logins for this HTTPS website are listed. Add them in
              Settings.
            </small>
          </div>
        )}
        <label>
          Automatic retries (0–5)
          <input
            type="number"
            min="0"
            max="5"
            required
            value={retryLimit}
            onChange={(event) => setRetryLimit(event.target.value)}
          />
        </label>
        <p>
          Retry temporary connection failures and busy servers. Pausing cancels
          pending retries. Login, certificate and invalid range errors require
          attention.
        </p>
        <label className="queue-check">
          <input
            type="checkbox"
            checked={singleStream}
            onChange={(e) => setSingleStream(e.target.checked)}
          />
          Use one connection (for servers that limit parallel requests)
        </label>
        <label className="queue-check">
          <input
            type="checkbox"
            checked={later}
            onChange={(e) => setLater(e.target.checked)}
          />{" "}
          Start at a chosen time
        </label>
        {later && (
          <label>
            Start time ({Intl.DateTimeFormat().resolvedOptions().timeZone})
            <input
              type="datetime-local"
              required
              value={start}
              onChange={(e) => setStart(e.target.value)}
            />
          </label>
        )}
        <p>
          Scheduled downloads start while ODM is running. If a start is missed,
          it runs when ODM next opens. Times use this computer’s local timezone.
          Pausing cancels a scheduled start; Start now adds it to the queue
          immediately.
        </p>
        <button className="primary" disabled={busy} type="submit">
          {later ? "Save schedule" : "Add to HTTP queue"}
        </button>
      </form>
      <h2>Queue order and schedules</h2>
      {!data ? (
        <p role="status">Loading queue…</p>
      ) : !data.jobs.some(
          (j) => !["removed", "complete"].includes(j.status),
        ) ? (
        <p>No pending HTTP downloads.</p>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Download</th>
                <th>Status</th>
                <th>Next start</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.jobs
                .filter((j) => !["removed", "complete"].includes(j.status))
                .map((job) => (
                  <tr key={job.id}>
                    <td>{job.name}</td>
                    <td>
                      {label[job.status] || job.status}
                      {job.error && (
                        <small className="queue-retry-reason">
                          {job.error}
                        </small>
                      )}
                      {job.retryLimit > 0 && (
                        <small>
                          {" "}
                          · {job.retryCount}/{job.retryLimit} retries used
                        </small>
                      )}
                    </td>
                    <td>
                      {job.status === "scheduled" || job.status === "retrying"
                        ? new Date(
                            job.status === "retrying"
                              ? job.retryAt
                              : job.startAt,
                          ).toLocaleString()
                        : "—"}
                    </td>
                    <td>
                      <div className="queue-actions">
                        {pending(job) && (
                          <>
                            <button
                              disabled={busy}
                              aria-label={"Move " + job.name + " first"}
                              onClick={() =>
                                change(
                                  {
                                    action: "move",
                                    gid: job.id,
                                    position: "first",
                                  },
                                  "Queue order saved.",
                                )
                              }
                            >
                              First
                            </button>
                            <button
                              disabled={busy}
                              aria-label={"Move " + job.name + " last"}
                              onClick={() =>
                                change(
                                  {
                                    action: "move",
                                    gid: job.id,
                                    position: "last",
                                  },
                                  "Queue order saved.",
                                )
                              }
                            >
                              Last
                            </button>
                          </>
                        )}
                        {[
                          "waiting",
                          "scheduled",
                          "active",
                          "retrying",
                        ].includes(job.status) && (
                          <button
                            disabled={busy}
                            aria-label={"Pause " + job.name}
                            onClick={() => control(job.id, "pause")}
                          >
                            Pause
                          </button>
                        )}
                        {["paused", "scheduled", "error", "retrying"].includes(
                          job.status,
                        ) && (
                          <button
                            disabled={busy}
                            aria-label={"Start " + job.name + " now"}
                            onClick={() => control(job.id, "resume")}
                          >
                            Start now
                          </button>
                        )}
                      </div>
                      {["paused", "error"].includes(job.status) && (
                        <JobLogin
                          job={job}
                          logins={logins}
                          busy={busy}
                          change={change}
                        />
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
