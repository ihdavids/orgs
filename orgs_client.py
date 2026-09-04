"""
orgs_client.py - Python client for the Orgs REST API.

Connects to a running Orgs server and provides methods to query
and manipulate org-mode nodes, files, todos, and more.

Usage:
    client = OrgsClient("http://localhost:8010")
    files = client.files()
    todos = client.search("TODO")
    node = client.get_by_id("my-custom-id")
"""

import base64
import json
from dataclasses import dataclass, field
from typing import Any, Optional

import requests


@dataclass
class ResultMsg:
    ok: bool = False
    msg: str = ""
    pos: dict = field(default_factory=dict)
    end: dict = field(default_factory=dict)


@dataclass
class Todo:
    headline: str = ""
    tags: list[str] = field(default_factory=list)
    props: dict[str, str] = field(default_factory=dict)
    hash: str = ""
    status: str = ""
    filename: str = ""
    line_num: int = 0
    is_active: bool = False
    parent: str = ""
    level: int = 0

    @classmethod
    def from_dict(cls, d: dict) -> "Todo":
        return cls(
            headline=d.get("Headline", ""),
            tags=d.get("Tags") or [],
            props=d.get("Props") or {},
            hash=d.get("Hash", ""),
            status=d.get("Status", ""),
            filename=d.get("Filename", ""),
            line_num=d.get("LineNum", 0),
            is_active=d.get("IsActive", False),
            parent=d.get("Parent", ""),
            level=d.get("Level", 0),
        )


@dataclass
class FullTodo:
    headline: str = ""
    content: str = ""
    tags: list[str] = field(default_factory=list)
    props: dict[str, str] = field(default_factory=dict)
    hash: str = ""
    priority: str = ""

    @classmethod
    def from_dict(cls, d: dict) -> "FullTodo":
        return cls(
            headline=d.get("Headline", ""),
            content=d.get("Content", ""),
            tags=d.get("Tags") or [],
            props=d.get("Props") or {},
            hash=d.get("Hash", ""),
            priority=d.get("Priority", ""),
        )


class OrgsClient:
    """Client for the Orgs REST API."""

    def __init__(self, url: str = "http://localhost:8010", verify_ssl: bool = True):
        self.url = url.rstrip("/")
        self.session = requests.Session()
        self.session.verify = verify_ssl

    def _get(self, endpoint: str, params: Optional[dict] = None) -> Any:
        resp = self.session.get(f"{self.url}/{endpoint}", params=params)
        resp.raise_for_status()
        return resp.json()

    def _post(self, endpoint: str, data: Any = None) -> Any:
        resp = self.session.post(f"{self.url}/{endpoint}", json=data)
        resp.raise_for_status()
        return resp.json()

    @staticmethod
    def _encode_hash(h: str) -> str:
        """Base64 URL-encode a hash for use in path parameters."""
        return base64.urlsafe_b64encode(h.encode()).decode()

    # ── File Operations ──────────────────────────────────────────────

    def files(self) -> list[str]:
        """Get list of all tracked org files."""
        return self._get("files")

    def org_file(self, filename: str) -> ResultMsg:
        """Get raw contents of an org file."""
        data = self._get("orgfile", {"filename": filename})
        return ResultMsg(**data) if isinstance(data, dict) else data

    def find_file(self, filename: str) -> ResultMsg:
        """Find a file in the database by name."""
        data = self._get("findfile", {"filename": filename})
        return ResultMsg(**data) if isinstance(data, dict) else data

    def file_as_html(self, filename: str, query: str = "") -> str:
        """Export a file as HTML."""
        params = {"filename": filename}
        if query:
            params["query"] = query
        return self._get("file/html", params)

    def headings(self, filename: str) -> list[Todo]:
        """Get all headings/todos in a file."""
        data = self._get("filecontents/headings", {"filename": filename})
        if isinstance(data, list):
            return [Todo.from_dict(d) for d in data]
        return data

    def refile_targets(self) -> Any:
        """Get list of available refile targets."""
        return self._get("refilefiles")

    # ── Search & Query ───────────────────────────────────────────────

    def search(self, query: str) -> list[Todo]:
        """Search todos by expression (e.g. 'TODO', 'DONE+:work:')."""
        data = self._get("search", {"query": query})
        if isinstance(data, list):
            return [Todo.from_dict(d) for d in data]
        return data

    def grep(self, query: str, delimiter: str = ":") -> Any:
        """Grep through all org files."""
        return self._get("grep", {"query": query, "delimeter": delimiter})

    # ── Node Lookup ──────────────────────────────────────────────────

    def get_by_hash(self, hash: str) -> Any:
        """Get a node by its dynamic hash."""
        return self._get(f"hash/{self._encode_hash(hash)}")

    def get_by_id(self, node_id: str) -> Any:
        """Get a node by its custom_id or id property."""
        return self._get(f"id/{node_id}")

    def get_full_todo(self, hash: str) -> FullTodo:
        """Get complete todo item with full details."""
        data = self._get(f"todofull/{self._encode_hash(hash)}")
        return FullTodo.from_dict(data) if isinstance(data, dict) else data

    def get_todo_html(self, hash: str) -> str:
        """Get a todo rendered as HTML."""
        return self._get(f"todohtml/{self._encode_hash(hash)}")

    def get_file_html(self, hash: str) -> str:
        """Get a file rendered as HTML (by hash of a node in it)."""
        return self._get(f"filehtml/{self._encode_hash(hash)}")

    # ── Node Navigation ──────────────────────────────────────────────

    def next_sibling(self, hash: str) -> Any:
        """Get the next sibling node."""
        return self._get(f"next/{self._encode_hash(hash)}")

    def prev_sibling(self, hash: str) -> Any:
        """Get the previous sibling node."""
        return self._get(f"prev/{self._encode_hash(hash)}")

    def last_child(self, hash: str) -> Any:
        """Get the last child node."""
        return self._get(f"child/{self._encode_hash(hash)}")

    def valid_statuses(self, hash: str) -> Any:
        """Get valid status transitions for a node."""
        return self._get(f"status/{self._encode_hash(hash)}")

    # ── Status & Property Modification ───────────────────────────────

    def change_status(self, hash: str, new_status: str) -> ResultMsg:
        """Change a node's TODO status (e.g. TODO -> DONE)."""
        data = self._post("status/change", {"Hash": hash, "Value": new_status})
        return ResultMsg(**data) if isinstance(data, dict) else data

    def change_property(self, hash: str, name: str, value: str) -> ResultMsg:
        """Change a property on a node (e.g. EFFORT)."""
        data = self._post("property", {"Hash": hash, "Name": name, "Value": value})
        return ResultMsg(**data) if isinstance(data, dict) else data

    def toggle_tags(self, hash: str, tags: str) -> ResultMsg:
        """Toggle tags on a node."""
        data = self._post("tags", {"Hash": hash, "Value": tags})
        return ResultMsg(**data) if isinstance(data, dict) else data

    # ── Capture ──────────────────────────────────────────────────────

    def capture(self, template: str, headline: str = "", content: str = "",
                tags: Optional[list[str]] = None,
                props: Optional[dict[str, str]] = None) -> ResultMsg:
        """Capture a new entry using a named template."""
        data = self._post("capture", {
            "Template": template,
            "NewNode": {
                "Headline": headline,
                "Content": content,
                "Tags": tags or [],
                "Props": props or {},
            },
        })
        return ResultMsg(**data) if isinstance(data, dict) else data

    def capture_templates(self) -> Any:
        """Get available capture templates."""
        return self._get("capture/templates")

    # ── Refile, Delete, Archive ──────────────────────────────────────

    def refile(self, from_target: dict, to_target: dict) -> ResultMsg:
        """Refile a node from one location to another.

        Targets are dicts with keys: Filename, Id, Type, Lvl
        Type can be: file+headline, id, customid, hash, file+line
        """
        data = self._post("refile", {"FromId": from_target, "ToId": to_target})
        return ResultMsg(**data) if isinstance(data, dict) else data

    def delete(self, target: dict) -> ResultMsg:
        """Delete a node. Target dict: {Filename, Id, Type}."""
        data = self._post("delete", target)
        return ResultMsg(**data) if isinstance(data, dict) else data

    def archive(self, target: dict) -> ResultMsg:
        """Archive a node. Target dict: {Filename, Id, Type}."""
        data = self._post("archive", target)
        return ResultMsg(**data) if isinstance(data, dict) else data

    # ── Day Pages ────────────────────────────────────────────────────

    def daypage(self, date: str) -> Any:
        """Get daypage for a specific date (format: YYYY-MM-DD)."""
        return self._get(f"daypage/{date}")

    def create_daypage(self, data: dict) -> ResultMsg:
        """Create a new daypage."""
        resp = self._post("daypage", data)
        return ResultMsg(**resp) if isinstance(resp, dict) else resp

    # ── Clock ────────────────────────────────────────────────────────

    def clock_status(self) -> Any:
        """Get current clock status."""
        return self._get("clock")

    def clock_in(self, target: dict) -> ResultMsg:
        """Clock in to a task. Target: {Filename, Id, Type}."""
        data = self._post("clockin", target)
        return ResultMsg(**data) if isinstance(data, dict) else data

    def clock_out(self) -> ResultMsg:
        """Clock out from current task."""
        data = self._post("clockout", {})
        return ResultMsg(**data) if isinstance(data, dict) else data

    # ── Tags & Filters ───────────────────────────────────────────────

    def all_tags(self) -> Any:
        """Get all tags used across files."""
        return self._get("alltags")

    def tag_groups(self) -> Any:
        """Get configured tag groups."""
        return self._get("taggroups")

    def filters(self) -> Any:
        """Get configured search filters."""
        return self._get("filters")

    # ── Tables ───────────────────────────────────────────────────────

    def table_names(self) -> Any:
        """Get list of all named tables."""
        return self._get("tablenames")

    def table_random_row(self, name: str) -> Any:
        """Get a random row from a named table."""
        return self._get("tablerandomget", {"name": name})


def _make_target(filename: str = "", node_id: str = "",
                 target_type: str = "hash", level: int = 0) -> dict:
    """Helper to build a Target dict.

    target_type options: file+headline, id, customid, hash, file+line
    """
    return {
        "Filename": filename,
        "Id": node_id,
        "Type": target_type,
        "Lvl": level,
    }


# ── Example Usage ────────────────────────────────────────────────────

if __name__ == "__main__":
    client = OrgsClient("http://localhost:8010")

    # List all files
    print("=== Files ===")
    try:
        files = client.files()
        for f in files[:10]:
            print(f"  {f}")
        if len(files) > 10:
            print(f"  ... and {len(files) - 10} more")
    except requests.ConnectionError:
        print("  Could not connect to server. Is orgs running?")
        exit(1)

    # Search for TODOs
    print("\n=== Active TODOs ===")
    todos = client.search("TODO")
    for t in todos[:10]:
        tags = f" :{':'.join(t.tags)}:" if t.tags else ""
        print(f"  [{t.status}] {t.headline}{tags}")
        print(f"         {t.filename}:{t.line_num}")

    # Get all tags
    print("\n=== Tags ===")
    tags = client.all_tags()
    if isinstance(tags, dict) and "Vals" in tags:
        print(f"  {', '.join(tags['Vals'][:20])}")

    # Get capture templates
    print("\n=== Capture Templates ===")
    templates = client.capture_templates()
    if isinstance(templates, list):
        for tmpl in templates:
            print(f"  {tmpl.get('name', 'unnamed')}: {tmpl.get('type', '')}")
