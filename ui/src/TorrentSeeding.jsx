import React, { useEffect, useState } from "react";

export function TorrentSeeding({ gid, api, onChange, lifecycle }) {
  const [state, setState] = useState(null),
    [minutes, setMinutes] = useState("10"),
    [approved, setApproved] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    setApproved(false);
    setState(null);
    api("torrent-seeding?gid=" + encodeURIComponent(gid))
      .then((v) => {
        if (alive) setState(v);
      })
      .catch((e) => {
        if (alive) setError(e.message);
      });
    return () => {
      alive = false;
    };
  }, [gid, lifecycle]);
  async function save(value) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      setState(
        await api("torrent-seeding", {
          gid,
          minutes: value,
          approved: value > 0 && approved,
        }),
      );
      await onChange();
    } catch (e) {
      setError(e.message);
    } finally {
      setApproved(false);
      setBusy(false);
    }
  }
  return (
    <details className="torrent-seeding">
      <summary>Seeding policy</summary>
      <p>
        Downloads stop on completion by default. Seeding shares the downloaded
        torrent files with peers, which can see your network address. An
        approved session lasts at most 24 hours and retains its deadline across
        restarts. Existing torrent upload limits still apply.
      </p>
      {state && (
        <>
          {state.until > 0 && (
            <p role="status">
              {state.until * 1000 > Date.now()
                ? "Sharing deadline: "
                : "Previous sharing deadline: "}
              {new Date(state.until * 1000).toLocaleString()}
            </p>
          )}
          <p>
            {state.active
              ? "Currently sharing with peers."
              : state.ready
                ? "Complete and paused; ready for a reviewed sharing session."
                : "Pause a fully completed torrent before starting a sharing session."}
          </p>
          {state.ready && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                save(Number(minutes));
              }}
            >
              <label>
                Seeding duration (minutes)
                <input
                  type="number"
                  min="1"
                  max="1440"
                  step="1"
                  required
                  value={minutes}
                  disabled={busy}
                  onChange={(e) => {
                    setMinutes(e.target.value);
                    setApproved(false);
                  }}
                />
              </label>
              <label className="permission">
                <input
                  type="checkbox"
                  checked={approved}
                  disabled={busy}
                  onChange={(e) => setApproved(e.target.checked)}
                />{" "}
                Share this torrent’s downloaded files with its peers until the
                chosen deadline
              </label>
              <button disabled={busy || !approved}>
                Start approved seeding
              </button>
            </form>
          )}
          {(state.active || state.until * 1000 > Date.now()) && (
            <button type="button" disabled={busy} onClick={() => save(0)}>
              Stop sharing and clear deadline
            </button>
          )}
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
