# journal

A personal CLI for running your day in 1.5-hour focus blocks, tracking sleep/feel,
and keeping weekly goals — backed by a local SQLite file (`journal.db`).

## Build

```bash
go build -o journal .
```


## Commands

### Blocks — the core loop

| Command | What it does |
|---|---|
| `journal start` | Start a new block. Prompts for project, outcome, context reload. |
| `journal update` | Mid-block check-in on the currently open block. Prompts for done notes, deliverable, files/links — all optional, appends to existing values. |
| `journal close` | Close the currently open block. Prompts for done, not done, next step (required), files/links and a tweak (optional), and a focus quality rating (1–10). |
| `journal block list [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--project name]` | List blocks, optionally filtered by date range or project. Not limited to the current week. |
| `journal block show <id>` | Show full detail for one block by its id (as printed by `block list` or `start`). |

### Sleep / daily check-in

| Command | What it does |
|---|---|
| `journal sleep log [--hours N] [--quality N] [--feel N] [--day YYYY-MM-DD] [--notes "..."]` | Log sleep hours, sleep quality (1–10), and feel (1–10) for a day. Defaults to today; omitted flags are prompted interactively. Re-running for the same day updates that day's entry. |

### Weekly goals

| Command | What it does |
|---|---|
| `journal goal add <text> [--day mon\|tue\|...\|sun]` | Add a goal for the current week. Defaults to today's day. |
| `journal goal list` | List this week's goals with reference numbers (`#`). |
| `journal goal done <n>` | Mark goal `#n` (from `goal list`) as done. |
| `journal goal edit <n> <new text>` | Edit goal `#n`'s text. |
| `journal goal delete <n>` | Delete goal `#n`. |

### Projects

| Command | What it does |
|---|---|
| `journal project add <name>` | Add a new project/type. |
| `journal project list` | List all projects with their ids. |
| `journal project rename <old> <new>` | Rename a project. |
| `journal project delete <name>` | Delete a project. Fails if any blocks reference it — rename it or reassign those blocks first. |

### Review

| Command | What it does |
|---|---|
| `journal week` | This week's goals plus all blocks logged this week. |
| `journal metrics week` | Block count and average focus quality per project, this week. |
| `journal metrics sleep` | Daily sleep/quality/feel log and weekly averages, this week. |
| `journal metrics correlate` | Pearson correlation between sleep hours/quality/feel and average daily focus quality, across all days with paired data. Needs at least 3 paired days (14+ recommended). |

## Example

A morning check-in, one focus block, and a look back at the week:

```bash
$ journal sleep log
Sleep hours (0-24): 7.5
Sleep quality (1-10): 8
Feel (1-10): 7

Checkin saved for 2026-08-16: sleep=7.5h quality=8 feel=7

$ journal goal add "Ship the block-review commands" --day mon
Goal added for Mon.

$ journal start
Project:
  1) work
  2) side-project
Choose number: 1
Outcome: Ship journal block list/show
Context reload: Picked up from yesterday's plan to add block review commands
First action: Write cmd/block.go
Block #1 started (id=14)

$ journal update
Done notes (leave blank to skip): block list working, filters tested
Deliverable/checkpoint (leave blank to skip): 
Files/links (leave blank to skip): cmd/block.go
Block #1 updated (id=14)

$ journal close
Done: block list and block show both working
Not done: haven't wired up project delete safety check yet
Exact next step to start with: add the blocks-referencing-project guard
Files/links (leave blank to skip): 
Focus quality (1-10): 8
One tweak for next block (leave blank to skip): write the guard clause first, before the happy path

Block #1 closed (id=14)

$ journal block show 14
Block #1 (id=14) — 2026-08-16 (Sun)
Project:         work
Outcome:         Ship journal block list/show
Context reload:  Picked up from yesterday's plan to add block review commands
First action:    Write cmd/block.go
Deliverable:     -
Done:            block list working, filters tested | block list and block show both working
Not done:        haven't wired up project delete safety check yet
Next step:       add the blocks-referencing-project guard
Files/links:     cmd/block.go
Focus quality:   4
Tweak:           write the guard clause first, before the happy path
Status:          closed at 2026-08-16 14:32:07
Created:         2026-08-16 13:01:22

$ journal week
=== Weekly Goals ===
1) [ ] Mon  Ship the block-review commands

=== Blocks ===
2026-08-16 #1  focus:4  Ship journal block list/show  -> next: add the blocks-referencing-project guard

$ journal metrics correlate
Only 1 paired days found. Need at least 3 (ideally 14+) for a meaningful correlation.
```
