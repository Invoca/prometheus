# `anchored`: document and test the linearity invariant that makes adjacent ranges compose

> **Status:** Draft for internal Invoca review. Target repo once finalized:
> `prometheus/prometheus` (new issue) and/or `prometheus/proposals`
> (amendment to [`0052-extended-range-selectors-semantics`](https://github.com/prometheus/proposals/blob/main/proposals/0052-extended-range-selectors-semantics.md)).
>
> **Proposed labels (upstream):** `kind/enhancement`, `kind/documentation`, `component/promql`

---

## Summary

PROM-52 ([`0052-extended-range-selectors-semantics`](https://github.com/prometheus/proposals/blob/main/proposals/0052-extended-range-selectors-semantics.md), implemented in [prometheus/prometheus#16457](https://github.com/prometheus/prometheus/pull/16457), released in 3.7 behind `promql-extended-range-selectors`) introduces `anchored` and `smoothed`. By specifying left-open / right-closed range semantics `(start, end]` plus "last sample at or before `start` as the boundary anchor," `anchored + increase/rate/delta` has an important mathematical property that the proposal demonstrates informally but never states or tests:

> For any series `m`, times `T₁ ≤ T₂ ≤ T₃`, and durations `Δ₁₂ = T₂−T₁`, `Δ₂₃ = T₃−T₂`, `Δ₁₃ = T₃−T₁`:
>
> ```promql
> increase(m[Δ₁₂] anchored)  @ T₂
>   +
> increase(m[Δ₂₃] anchored)  @ T₃
>   =
> increase(m[Δ₁₃] anchored)  @ T₃
> ```
>
> (assuming `Δ₁₂ ≤ lookback_delta` so the anchor lookup succeeds).

This invariant is what makes anchored `increase` safely composable across contiguous time windows — splitting a wider range into adjacent sub-ranges and summing the results gives the same answer as asking for the wider range directly. Plain `rate`/`increase` doesn't satisfy this; the proposal itself notes on line 119 that *"two consecutive range selectors therefore fail to capture the increase."*

## Why it matters

This is the formal expression of the "composability" goal the proposal mentions on line 147 ("offer greater composability and flexibility") and line 222 ("Improved composability across range boundaries"), but neither of those phrasings is pinned down. Making it explicit:

- Gives users a precise, testable reason to prefer `anchored` for any workflow that splits or stitches time ranges (recording rules that roll up, alerts on windowed counter increases, dashboards that zoom between panel ranges, etc.).
- Locks the invariant into the engine's contract via tests so future refactors can't silently regress it.
- Resolves a subtle ambiguity at the `T₂` boundary: because the range is `(T₁, T₂]`, a sample whose timestamp equals `T₂` is attributed to the earlier range, and the same sample is returned by `anchor(T₂)` for the later range. That symmetry is what makes additivity exact — not a rounding coincidence.

## Why `anchored` satisfies it

<details>
<summary>Proof sketch</summary>

Let `last_in(T_a, T_b]` denote the latest sample whose timestamp lies in `(T_a, T_b]`, `anchor(T)` the latest sample with timestamp `≤ T` within the lookback delta, and `resets(T_a, T_b]` the counter-reset correction accumulated from samples in `(T_a, T_b]`.

Under `anchored + increase`:

```
f(T₁, T₂] = last_in(T₁, T₂] − anchor(T₁) + resets(T₁, T₂]
f(T₂, T₃] = last_in(T₂, T₃] − anchor(T₂) + resets(T₂, T₃]
f(T₁, T₃] = last_in(T₁, T₃] − anchor(T₁) + resets(T₁, T₃]
```

Adding the first two gives the third iff all of:

1. `resets(T₁, T₂] + resets(T₂, T₃] = resets(T₁, T₃]` — holds because `(T₁, T₂]` and `(T₂, T₃]` partition `(T₁, T₃]`; every reset is counted exactly once.
2. `last_in(T₁, T₂] = anchor(T₂)` — both expressions denote "the latest sample with `t ≤ T₂`." The right-closed boundary on `(T₁, T₂]` includes `T₂` itself; the anchor definition "at or before `T₂`" matches. Provided `Δ₁₂ ≤ lookback_delta` (so `anchor(T₂)` doesn't fall back to the first in-range sample), these coincide.
3. `last_in(T₂, T₃] = last_in(T₁, T₃]` — trivially true when at least one sample exists in `(T₂, T₃]`; handled by the empty-range convention otherwise.

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
   - Ranges containing counter resets on both sides of `T₂`
   - Partial datasets with missing scrapes straddling `T₂`

## Happy to contribute

I can split this into two PRs — docs first for quick review, then tests — and port test-data patterns from our internal `yrate` suite that have exercised this invariant for years. Please let me know if a proposal amendment is preferred before or after the docs/tests land.

---

## Reviewer notes (Invoca-internal, strip before sending upstream)

- **Tone:** aiming for "colleague flagging a missing piece," not "outsider demanding a spec change." Happy to soften or sharpen.
- **Attribution:** we can lean harder into the `yrate`-as-prior-art framing, or pull it back to just a link — depending on how forward we want to be about our involvement.
- **Scope:** current draft bundles docs + tests + proposal amendment. Could split into a pure docs issue first to test reception before offering code.
- **Venue:** open as an issue on `prometheus/prometheus` (lower friction, invites discussion) vs. open as a proposal amendment PR on `prometheus/proposals` (more formal, commits us to a spec change). My recommendation is the issue first, referencing PROM-52, and let maintainers route it.
- **Timing:** `anchored` is still behind `promql-extended-range-selectors` as of 3.11.2. Filing before GA gives us a chance to shape the contract before it solidifies; filing after GA lets us point at existing production adoption. Leaning toward "before GA."
