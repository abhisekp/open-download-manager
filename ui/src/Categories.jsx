import React, { useEffect, useState } from "react";

function CategoryRow({ category, revision, busy, change }) {
  const [name, setName] = useState(category.name),
    [editRevision, setEditRevision] = useState(null);
  useEffect(() => {
    if (editRevision === null) setName(category.name);
  }, [category.name, editRevision]);
  return (
    <li>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (
            await change({
              action: "rename",
              id: category.id,
              name,
              revision: editRevision ?? revision,
            })
          )
            setEditRevision(null);
        }}
      >
        <label>
          Category name: {category.name}
          <input
            aria-label={"Rename category " + category.name}
            maxLength={48}
            required
            value={name}
            disabled={busy}
            onChange={(e) => {
              if (editRevision === null) setEditRevision(revision);
              setName(e.target.value);
            }}
          />
        </label>
        <button disabled={busy || name === category.name}>Save name</button>
        <button
          type="button"
          disabled={busy}
          onClick={() => {
            setName(category.name);
            setEditRevision(null);
          }}
        >
          Reset name
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={() => change({ action: "delete", id: category.id })}
        >
          Remove category {category.name}
        </button>
      </form>
    </li>
  );
}

export function CategoriesManager({ state, api, onChange }) {
  const [name, setName] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  async function change(input) {
    if (busy || !state) return false;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("categories", { revision: state.revision, ...input });
      await onChange();
      setNotice("Category labels saved. Downloaded files were not changed.");
      return true;
    } catch (e) {
      setError(e.message);
      await onChange();
      return false;
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="Download categories" className="category-manager">
      <h3>Download categories</h3>
      <p>
        Create up to 32 category labels and use them to filter your downloads.
        Removing a category clears its assignments; it never removes transfers
        or files.
      </p>
      {state === undefined ? (
        <p role="status">Loading categories…</p>
      ) : state === null ? (
        <p>Saved categories are unavailable. Downloads remain usable.</p>
      ) : (
        <>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              if (await change({ action: "create", name: name.trim() }))
                setName("");
            }}
          >
            <label>
              New category name
              <input
                maxLength={48}
                required
                value={name}
                disabled={busy}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <button
              disabled={busy || state.categories.length >= 32 || !name.trim()}
            >
              Create category
            </button>
          </form>
          {state.categories.length ? (
            <ul>
              {state.categories.map((c) => (
                <CategoryRow
                  key={c.id}
                  category={c}
                  revision={state.revision}
                  busy={busy}
                  change={change}
                />
              ))}
            </ul>
          ) : (
            <p>No categories yet.</p>
          )}
        </>
      )}
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
