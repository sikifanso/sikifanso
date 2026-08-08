# sikifanso

The ubiquitous language of this repo. Definitions only — no implementation detail, no
mechanisms. Built up as terms are resolved rather than written in one pass, so absence from
this file means "not yet pinned down", not "has no name".

## Language

### Dependency governance

Four words in this repo have meant "we are not taking this update" at various times, hiding
a distinction that matters: whether a human decision is outstanding.

**Held**:
An update we have deliberately not taken while a human decision remains outstanding. A held
update must stay *visible* — the whole point is that someone will eventually decide.
_Avoid_: capped, blocked, frozen, pinned

**Derived**:
A version determined entirely by another dependency, so no independent decision about it
exists. Correctly invisible: there is no decision-maker to hide anything from.
_Avoid_: held, locked, transitive

**Pinned**:
Fixed to one exact version or commit, chosen deliberately rather than resolved.
_Avoid_: locked, fixed, vendored

**Version-locked**:
A set of dependencies that must move together because something outside our control couples
them. Distinct from *pinned*: the constraint is the relationship between them, not any one
version.
_Avoid_: grouped, pinned group

**Skew**:
The minor-version distance between the Kubernetes client libraries compiled into the CLI and
the k3s server it provisions. Unusual in that this tool controls both sides — it chooses the
client at build time and the server at cluster-create time — so skew here is a decision, not
an accident of someone else's environment.
_Avoid_: drift, mismatch, lag
