#!/usr/bin/env python3
"""Updates versions.json + index.html for the versioned docs site
(gh-pages branch) when a new release tag's docs are published.

Usage: update-doc-versions-manifest.py <path-to-versions.json> <new-tag>

Reads the existing manifest (if present), adds/moves <new-tag> to the
top as the new "latest", always keeps a "master (dev)" entry, and
rewrites both versions.json and a sibling index.html (redirecting to
the new latest version) in place next to the given versions.json path.
"""
import json
import pathlib
import sys

INDEX_TEMPLATE = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0; url=/landlock-genprof/{latest}/">
<link rel="canonical" href="/landlock-genprof/{latest}/">
<title>landlock-genprof docs</title>
</head>
<body>
<p>Redirecting to the <a href="/landlock-genprof/{latest}/">latest docs ({latest})</a>.</p>
</body>
</html>
"""


def main() -> None:
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    manifest_path = pathlib.Path(sys.argv[1])
    new_tag = sys.argv[2]

    if manifest_path.exists():
        data = json.loads(manifest_path.read_text(encoding="utf-8"))
    else:
        data = {"latest": new_tag, "versions": []}

    versions = [v for v in data.get("versions", []) if v["path"] not in (new_tag, "master")]
    versions.insert(0, {"label": f"{new_tag} (latest)", "path": new_tag})

    # Drop the stale "(latest)" suffix from whatever used to hold it.
    for v in versions[1:]:
        v["label"] = v["label"].replace(" (latest)", "")

    versions.append({"label": "master (dev)", "path": "master"})

    data["latest"] = new_tag
    data["versions"] = versions

    manifest_path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    (manifest_path.parent / "index.html").write_text(
        INDEX_TEMPLATE.format(latest=new_tag), encoding="utf-8"
    )


if __name__ == "__main__":
    main()
