# Creating addons

Copy the [blank template](../examples/addons/template.json), replace its placeholders, and remove unused settings, action kinds, pages, or optional fields. Import the file through **Settings / addons / + Add addon**.

The template shows every supported field and variant. `your-tool` and `example.com` are placeholders, not a working integration.

```json
{
  "format_version": 1,
  "version": "1.0.0",
  "update_url": "",
  "id": "your-addon",
  "name": "Your addon",
  "description": "",
  "shortcut": "",
  "menu": false,
  "detect": {
    "files": [],
    "tools": []
  },
  "settings": [
    {"id": "text", "name": "Text setting", "type": "text", "default": ""},
    {"id": "number", "name": "Number setting", "type": "number", "default": 0},
    {"id": "toggle", "name": "Boolean setting", "type": "bool", "default": false},
    {"id": "choice", "name": "Choice setting", "type": "choice", "default": "option-1", "options": ["option-1", "option-2"]}
  ],
  "actions": [
    {
      "id": "cli-action",
      "name": "CLI action",
      "kind": "cli",
      "executable": "your-tool",
      "args": [],
      "detect": {"files": [], "tools": []}
    },
    {
      "id": "command-action",
      "name": "Command action",
      "kind": "command",
      "executable": "your-tool",
      "args": [],
      "detect": {"files": [], "tools": []}
    },
    {
      "id": "app-action",
      "name": "Application action",
      "kind": "app",
      "executable": "your-tool",
      "args": [],
      "detect": {"files": [], "tools": []}
    },
    {
      "id": "output-action",
      "name": "Output action",
      "kind": "output",
      "executable": "your-tool",
      "args": [],
      "detect": {"files": [], "tools": []}
    },
    {
      "id": "url-action",
      "name": "URL action",
      "kind": "url",
      "url": "https://example.com",
      "detect": {"files": [], "tools": []}
    }
  ],
  "pages": [
    {
      "id": "your-page",
      "name": "Your page",
      "title": "",
      "text": "",
      "detect": {"files": [], "tools": []},
      "actions": ["cli-action", "command-action", "app-action", "output-action", "url-action"],
      "sections": [
        {
          "title": "",
          "text": "",
          "actions": ["cli-action", "command-action", "app-action", "output-action", "url-action"]
        }
      ]
    }
  ]
}
```

## Manifest fields

A manifest is one JSON object, at most 1 MiB. Unknown fields are rejected.

| Field | Meaning |
| --- | --- |
| `format_version` | Required schema version; currently `1`. |
| `version` | Required addon SemVer string, without a `v` prefix. |
| `id`, `name` | Required identifier and display name. |
| `update_url` | Optional raw JSON manifest URL; empty disables checks for a new addon. |
| `description` | Optional descriptive text. |
| `shortcut` | Optional ASCII letter; empty assigns none. |
| `menu` | `true` keeps the action menu even when one action applies. |
| `detect` | Optional requirements shared by actions. |
| `settings` | Up to 100 setting definitions. |
| `actions`, `pages` | At least one is required; limits are 100 actions and 50 pages. |
| `metadata` | Optional live JSON data source and display templates (see below). |

IDs match `[a-z][a-z0-9-]{0,63}`. Keep the addon ID stable across versions. Action, setting, and page IDs must be unique within their respective arrays.

## Actions

Each action requires `id`, `name`, and `kind`. Process actions use an `executable` name on `PATH`, not a file path, and optional string `args`. They run in the selected project's directory. A `url` action uses only `url`; do not combine it with `executable` or `args`.

| Kind | Use |
| --- | --- |
| `cli` | Interactive tools; follows the CLI tools launch preference. |
| `command` | Project tasks; follows the Commands launch preference. |
| `app` | Starts an application without waiting for it to close. |
| `output` | Captures noninteractive output inside Rian; closes on success and retains failures. |
| `url` | Opens an HTTP(S) link. |
| `metadata-url` | Opens the link returned by this addon's metadata rules; requires `metadata` and no `url`, `executable`, or `args`. |

## Detection

`files` contains paths or globs relative to the saved project root; **any** match qualifies. `tools` lists executable names that must **all** exist on `PATH`. Empty arrays impose no requirements. Addon and action detection combine; page detection filters its project rows.

Detection matches files and directories without searching parent folders. It supports neither negative matches nor recursive `**`; paths containing `..` are rejected. Missing action executables exclude those actions automatically. Detection never runs commands.

## Settings and templates

Settings require `id`, `name`, `type`, and a suitable `default`. `text` uses strings, `number` uses finite numbers, and `bool` uses `true` or `false`. Only `choice` needs `options`: a nonempty string array containing its default. Command-specific constraints, such as integer ranges, remain the executable's responsibility. Settings apply across projects.

Arguments and URLs support `${project.path}`, `${project.name}`, and `${setting.id}`. Each argument stays one string, including spaces. Expansion is literal and not recursive: shell variables, wildcards, pipes, and command substitutions are not evaluated. Templates cannot change the executable or conditionally add flags; booleans become `true` or `false`. Use an executable script on `PATH` for multi-step logic. URL values are not automatically URL-encoded.

## Pages

Pages require `id` and `name`; `title`, `text`, `detect`, `actions`, and `sections` are optional. Sections accept `title`, `text`, and `actions`. Action lists reference IDs from the same manifest. An omitted or empty list uses all available actions. Choose a top-level `actions` list or `sections`: sections take precedence when both exist.

Page and section `text` support setting templates. Project templates belong in actions, which carry each row's project context. Pages contain static text and action rows; addons with `metadata` also display live project summaries. Viewing a page never executes actions, and a page can remain visible without matching projects. Text-only addons can omit `actions`.

## Live JSON metadata

An addon can read one project-relative JSON `file`, GET JSON from a fixed HTTP(S) `fetch` URL, or combine both. At least one source is required. All service-specific URLs, field mappings, and conditions stay in the addon file. No executable or project script runs. Metadata is fetched in the background only for enabled, visible addons whose detection matches, at startup and every five minutes. Up to four checks run at once, requests time out after eight seconds, and local/remote JSON is limited to 1 MiB each. Ctrl+R on a focused addon page refreshes metadata; rescanning a project invalidates its cached data.

```json
"metadata": {
  "file": "app.json",
  "fetch": "https://example.com/releases/${file.name}",
  "fetch_when": [{"path": "file.private", "op": "ne", "value": "true"}],
  "hidden_when": [{"path": "file.name", "op": "empty"}],
  "title": "RELEASE",
  "name": "${file.name}",
  "summary": "local ${file.version} / published ${remote.version}",
  "status": "Published",
  "statuses": [{"when": [{"path": "request.status", "op": "eq", "value": "404"}], "text": "Not published"}],
  "url": "https://example.com/projects/${file.name}",
  "url_when": [{"path": "request.status", "op": "eq", "value": "200"}],
  "fields": [{"label": "Local", "value": "${file.version}"}, {"label": "Published", "value": "${remote.version}"}]
}
```

Templates access scalar dotted paths under `file` and `remote`, plus `project.name`, `request.status`, `request.error`, and the resolved `status`. Arrays use numeric segments, for example `${remote.items.0.title}` or `${remote.0.title}`. Missing or nonscalar values expand to empty strings. URL substitutions are encoded as path segments; `${file.name|path}` preserves slashes between separately encoded segments. Schemes and hosts must be literal, and credentials are unsupported. Only `200` responses populate `remote`; `404` is available as a distinct status, while other HTTP errors, network failures, and malformed JSON populate `request.error`.

Conditions use `path` and `op`: `empty`, `present`, `eq`, `ne`, or `in`. Equality operations use `value`, which can contain templates; `in` uses a string array `values`. All conditions in a list must match; an empty list matches. `hidden_when` is ignored when omitted or empty. The first matching status rule wins; otherwise `status` supplies the default. Up to 20 fields, 20 status rules, and 100 conditions are supported. Empty field values are omitted.

Optional `visible_when` conditions run after fetching, so an addon can include only confirmed remote records. When present and nonempty, all conditions must match; unmatched records are hidden from overview rows, project details, and addon shortcuts. Pending records are hidden until checked, preventing unrelated projects from briefly appearing during discovery. Overview rows follow saved-project order: favorites, most recently used, then project name.

Set `filter_setting` to the ID of an addon boolean setting to let users toggle `visible_when` filtering. A `true` value applies the filter; `false` shows all candidates. The setting's default controls its initial state. File detection and `hidden_when` still apply in both modes.

Metadata appears in project details and in two-line overview rows. A `metadata-url` action opens the resulting `url` if its conditions match. Addon shortcuts also work on the selected row of a focused overview page. A missing link displays a notice.

## Test and import

Test commands from a target project directory before importing. Import validates fields, IDs, defaults, action templates, and page references. For missing actions, check enabled state, visibility, detection paths, and executable availability. For failed commands, test the same arguments in a terminal.

Rian stores a copy of the imported manifest; editing its source file does not change the installed copy. Removing an addon keeps the source file.

## Versions and updates

Keep `format_version: 1` and increase addon `version` for manifest changes, for example `"1.0.0"` to `"1.1.0"`. SemVer prerelease/build suffixes are supported. Equal or older precedence, including build-only changes, is rejected.

Reimport a newer version with the same `id` to upgrade. Enabled state, the chosen shortcut, and values accepted by matching setting IDs are preserved. Removed settings are discarded; incompatible values use new defaults.

For network updates, host the **complete manifest** at a stable raw HTTP(S) URL and set `update_url`; HTTPS is recommended. Replace placeholder URLs with your own endpoint. Serve the same addon `id` with a newer `version`.

Rian checks configured URLs at startup and hourly; installing an update requires selection. Validation failures, changed IDs, or payloads over 1 MiB leave the installed copy unchanged. An omitted or empty `update_url` in a newer manifest retains the current URL; a nonempty URL replaces it.

Working examples: [Git](../examples/addons/git.json), [npm](../examples/addons/npm.json).
