# Evidence acceptance

The executor establishes a claim, Gadget attachment, active event stream,
and trace-ready barrier. Activity then produces raw events; normalization and
attribution produce typed capability facts. The observation persists source
qualification and evidence state. Candidate generation consumes the
canonical evidence/provenance contract, and proposal generation remains
bound to the candidate and observation lineage.

Facts are not synonymous with evidence state. AVAILABLE, EMPTY, and UNKNOWN
remain distinct. UNKNOWN is never converted into success or silently treated
as EMPTY. `NOT_AVAILABLE / attribution COMPLETED / UNKNOWN` is represented as
an incomplete historical/partial proof state with forensic details preserved.
