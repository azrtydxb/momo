# Fix every open GitHub issue (#86 to #108), the Kuvryn Orbit port's v0.7.0 findings, and close the reworded carried stories

Status: active
Created: 2026-11-11

## Goal

Kuvryn Orbit can move all of its database code onto momo with no workarounds. Every GitHub issue open on 2026-11-11 (#86 to #108) is fixed, with a regression test, and shipped in a release:

- **Naming:** initialisms in snake_case, `-ss` singulars, and the naming preset used by plain-struct `Query`.
- **`--from-db`:** generated columns, inverse relations, and no objects of unmodeled tables.
- **Migrations:** the importer, history, partition-index inspection, and `momo.yaml` as library options.
- **Queries and expressions:** the missing operators and constructs, plus LISTEN/NOTIFY, `RawRows`, `Date` infinity, `Numeric`, and a trusted-SQL marker for momovet.

The carried plan stories that the user decided to reword (048, 049, 054, 090, 098, 099, 101 and 236) are closed with CI evidence.
