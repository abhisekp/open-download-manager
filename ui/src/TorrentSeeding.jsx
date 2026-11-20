import React, { useEffect, useRef, useState } from "react";

export function TorrentSeeding({ gid, api, onChange, lifecycle }) {
  const epoch = useRef(0);
  const [state, setState] = useState(null),
    [minutes, setMinutes] = useState("10"),
    [ratio, setRatio] = useState("0"),
    [approved, setApproved] = useState(false),
    [busy, setBusy] = useState(false),
    [refresh, setRefresh] = useState(0),
    [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    epoch.current++;
    setApproved(false);
    setState(null);
    setBusy(false);
    setError("");
    api("torrent-seeding?gid=" + encodeURIComponent(gid))
      .then((v) => {
        if (alive) setState(v);
      })
      .catch((e) => {
        if (alive) setError(e.message);
      });
    return () => {
      alive = false;
      epoch.current++;
    };
  }, [gid, lifecycle, refresh]);
  async function save(value) {
    if (busy) return;
    setBusy(true);
    setError("");
    const requestEpoch = epoch.current;
    try {
      const result = await api("torrent-seeding", {
        gid,
        minutes: value,
        ratioPercent: value > 0 ? Math.round(Number(ratio) * 100) : 0,
        approved: value > 0 && approved,
      });
      if (requestEpoch !== epoch.current) return;
      setState(result);
      await onChange();
    } catch (e) {
      if (requestEpoch === epoch.current) setError(e.message);
    } finally {
      if (requestEpoch === epoch.current) {
        setApproved(false);
        setBusy(false);
      }
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
      <p>
        An optional lifetime upload ratio can stop sharing earlier. The ratio is
        uploaded payload divided by this torrent’s content size. It is a target,
        not an exact traffic quota: in-flight data and crash checkpoints can
        exceed it. The chosen time limit always applies.
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
          {state.ratioPercent > 0 && (
            <p role="status">
              Lifetime ratio goal: {state.ratioPercent / 100}. Uploaded payload
              at last policy check: {state.uploadedBytes.toLocaleString()} bytes
              {" / "}
              {state.targetBytes.toLocaleString()} bytes.
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
              <label>
                Stop at upload ratio (0 for time only)
                <input
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  required
                  value={ratio}
                  disabled={busy}
                  onChange={(e) => {
                    setRatio(e.target.value);
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
                chosen deadline or lifetime upload-ratio goal, whichever comes
                first
              </label>
              <button disabled={busy || !approved}>
                Start approved seeding
              </button>
            </form>
          )}
          {(state.active || state.until * 1000 > Date.now()) && (
            <button type="button" disabled={busy} onClick={() => save(0)}>
              Stop sharing and clear policy
            </button>
          )}
        </>
      )}
      <button
        type="button"
        disabled={busy || !state}
        onClick={() => {
          setApproved(false);
          setRefresh((v) => v + 1);
        }}
      >
        Refresh seeding status
      </button>
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
