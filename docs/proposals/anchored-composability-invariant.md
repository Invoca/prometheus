# `anchored`: document and test the composability invariant for adjacent ranges

> **Status:** Draft for internal Invoca review. Target repo once finalized:
> `prometheus/prometheus` (new issue) and/or `prometheus/proposals`
> (amendment to [`0052-extended-range-selectors-semantics`](https://github.com/prometheus/proposals/blob/main/proposals/0052-extended-range-selectors-semantics.md)).
>
> **Proposed labels (upstream):** `kind/enhancement`, `kind/documentation`, `component/promql`

---

## Summary

PROM-52 ([`0052-extended-range-selectors-semantics`](https://github.com/prometheus/proposals/blob/main/proposals/0052-extended-range-selectors-semantics.md), implemented in [prometheus/prometheus#16457](https://github.com/prometheus/prometheus/pull/16457), released in 3.7 behind `promql-extended-range-selectors`) introduces `anchored` and `smoothed`. By combining a left-open / right-closed range `(start, end]` with a separate baseline *anchor* — the latest sample with timestamp `≤ start`, fetched from outside the range within `lookback_delta` — `anchored + increase/rate/delta` has an important mathematical property that the proposal demonstrates informally but never states or tests:

> For any series `m` and any three timestamps `T₀ < T₁ < T₂`, let:
>
> - `r₁` = the length of `(T₀, T₁]`, subject to `r₁ ≤ lookback_delta`
> - `r₂` = the length of `(T₁, T₂]`
>
> The three anchored windows below cover these intervals. (Recall the PromQL rule: `m[duration] anchored @ T` covers `(T − duration, T]` — the `@` instant is always the *right* edge, and the range extends backward from it.)
>
> | Window | Covers |
> |---|---|
> | `m[r₁]      anchored  @ T₁` | `(T₀, T₁]` |
> | `m[r₂]      anchored  @ T₂` | `(T₁, T₂]` |
> | `m[r₁ + r₂] anchored  @ T₂` | `(T₀, T₂]` |
>
> and:
>
> ```promql
> increase(m[r₁]      anchored)  @ T₁
>   +
> increase(m[r₂]      anchored)  @ T₂
>   =
> increase(m[r₁ + r₂] anchored)  @ T₂
> ```

This invariant is what makes anchored `increase` safely composable across contiguous time windows — splitting a wider range into adjacent sub-ranges and summing the results gives the same answer as asking for the wider range directly. Plain `rate`/`increase` doesn't satisfy this; the proposal itself notes on line 119 that *"two consecutive range selectors therefore fail to capture the increase."*

## Why it matters

This is the formal expression of the "composability" goal the proposal mentions on line 147 ("offer greater composability and flexibility") and line 222 ("Improved composability across range boundaries"), but neither of those phrasings is pinned down. Making it explicit:

- Gives users a precise, testable reason to prefer `anchored` for any workflow that splits or stitches time ranges (recording rules that roll up, alerts on windowed counter increases, dashboards that zoom between panel ranges, etc.).
- Locks the invariant into the engine's contract via tests so future refactors can't silently regress it.
- Resolves a subtle ambiguity at the shared boundary `T₁`: a sample whose timestamp equals `T₁` is a *member* of the earlier range (right-closed) but is *not* a member of the later range `(T₁, T₂]` (left-open). The same sample is nonetheless reached by `anchor(T₁)` for the later range — the anchor lookup ("latest sample with `t ≤ T₁`") reaches backward from, and including, the range's left boundary, even though the boundary itself is not in the range. That dual role — "last in the earlier range" *and* "anchor of the later range," rather than double-membership — is what makes additivity exact.

## Why `anchored` satisfies it

<details>
<summary>Proof sketch</summary>

Let `last_in(s, e]` denote the latest sample whose timestamp lies in `(s, e]`, `anchor(t)` the latest sample with timestamp `≤ t` within `lookback_delta`, and `resets(s, e]` the counter-reset correction accumulated from samples in `(s, e]`.

The three anchored windows in the invariant cover `(T₀, T₁]`, `(T₁, T₂]`, and `(T₀, T₂]`, respectively. So:

```
f₀₁ = last_in(T₀, T₁] − anchor(T₀) + resets(T₀, T₁]
f₁₂ = last_in(T₁, T₂] − anchor(T₁) + resets(T₁, T₂]
f₀₂ = last_in(T₀, T₂] − anchor(T₀) + resets(T₀, T₂]
```

`f₀₁ + f₁₂ = f₀₂` iff all of:

1. `resets(T₀, T₁] + resets(T₁, T₂] = resets(T₀, T₂]` — holds because `(T₀, T₁]` and `(T₁, T₂]` partition `(T₀, T₂]`; every reset is counted exactly once.
2. `last_in(T₀, T₁] = anchor(T₁)` — these are *different* lookups that agree in value. `last_in(T₀, T₁]` is the latest sample *in* the earlier range (right-closed membership, so a sample at exactly `T₁` qualifies). `anchor(T₁)` is a separate lookup *outside* the later range `(T₁, T₂]`: the latest sample with `t ≤ T₁`, within `lookback_delta`. A sample at exactly `t = T₁` is reached by both — as the last member of the earlier range, and as the anchor for the later one — without ever being a member of both ranges. Provided `r₁ ≤ lookback_delta` (so the anchor lookup doesn't time-out before reaching `T₀`), these values coincide.
3. `last_in(T₁, T₂] = last_in(T₀, T₂]` — trivially true when at least one sample exists in `(T₁, T₂]`; handled by the empty-range convention otherwise.

The proposal's partial-dataset example (lines 231–237) — *"The first window slightly underestimates while the second window slightly overestimates the actual increase"* — is this invariant in action. The per-window numbers aren't coincidentally canceling; they're composing.

</details>

## Related prior art

This property is the design goal of the Invoca `yrate` family (referenced in the PROM-52 "Other docs or links" as [Prometheus y-rate](https://docs.google.com/document/d/1CF5jhyxSD437c2aU2wHcvg88i8CjSPO3kMHsEaDRe2w/edit)). Invoca has run `yrate`-semantics in production for ~5 years with this invariant as a documented contract; `anchored + increase` converges on the same behavior via a cleaner modifier-based syntax that avoids function proliferation.

## Proposed additions

1. **User-facing docs** (`docs/feature_flags.md` and the `anchored` entry under `docs/querying/functions.md`): a short callout stating the invariant.
2. **Proposal amendment** (`prometheus/proposals#…`): one paragraph under "How" linking the invariant to the "composability" goal.
3. **Regression tests** (`promql/promqltest/testdata/…`): paired-window cases asserting the invariant across:
   - Regular scrape cadence, boundary timestamps aligned with sample cadence
   - Same, but shifted off-cadence (to exercise the `last_in` / `anchor` coincidence)
   - Ranges containing counter resets on both sides of `T₁`
   - Partial datasets with missing scrapes straddling `T₁`

## Happy to contribute

I can split this into two PRs — docs first for quick review, then tests — and port test-data patterns from our internal `yrate` suite that have exercised this invariant for years. Please let me know if a proposal amendment is preferred before or after the docs/tests land.

---

## Reviewer notes (Invoca-internal, strip before sending upstream)

- **Tone:** aiming for "colleague flagging a missing piece," not "outsider demanding a spec change." Happy to soften or sharpen.
- **Attribution:** we can lean harder into the `yrate`-as-prior-art framing, or pull it back to just a link — depending on how forward we want to be about our involvement.
- **Scope:** current draft bundles docs + tests + proposal amendment. Could split into a pure docs issue first to test reception before offering code.
- **Venue:** open as an issue on `prometheus/prometheus` (lower friction, invites discussion) vs. open as a proposal amendment PR on `prometheus/proposals` (more formal, commits us to a spec change). My recommendation is the issue first, referencing PROM-52, and let maintainers route it.
- **Timing:** `anchored` is still behind `promql-extended-range-selectors` as of 3.11.2. Filing before GA gives us a chance to shape the contract before it solidifies; filing after GA lets us point at existing production adoption. Leaning toward "before GA."
