# Tmux Manager

A tmux manager that lets you create, rename and delete tmux sessions and create custom templates for future use.

## Features

- **Interactive Session Management**: Browse, create, rename, attach to, and delete tmux sessions
- **Session Templates**: Create reusable session templates with multiple panes and custom commands
- **Visual Pane Editor**: Interactive editor for designing multi-pane layouts
- **Keyboard-driven**: Efficient navigation with vim-like keybindings

## Prerequisites

- [tmux](https://github.com/tmux/tmux) installed and accessible in PATH
- [Go](https://golang.org/) 1.19 or later (for building from source)
- A supported terminal emulator (kitty, alacritty, gnome-terminal, xterm, konsole, terminator, tilix)

## Installation

### From Source

```bash
git clone https://github.com/yourusername/lazytmux.git
cdlazytmux
go mod tidy
go build -o lazytmux
```

### Usage

Run the program with default terminal:

```bash
.lazytmux/
```

Run with a specific terminal:

```bash
./lazytmux -t alacritty
./lazytmux -t gnome-terminal
./lazytmux -t xterm
```

Show help and supported options:

```bash
./lazytmux -h
```

### Command Line Options

| Flag            | Description               | Example        |
| --------------- | ------------------------- | -------------- |
| `-t <terminal>` | Specify terminal emulator | `-t alacritty` |
| `-h`            | Show help message         |                |
| `-v`            | Show version information  |                |

### Supported Terminals

The program supports the following terminal emulators by default, this is only for attaching the session to that terminal emulator,
the default attach command is {selected terminal} -e tmux attach-session -t {session name}:

- **kitty** (default)
- **alacritty**
- **gnome-terminal**
- **xterm**
- **konsole**
- **terminator**
- **tilix**

## Key Features

### Session Management

- **View Sessions**: See all active tmux sessions with status, window count, and creation time
- **Create Sessions**: Create new sessions with auto-generated names or custom names
- **Template Integration**: Create sessions from templates by using template names
- **Attach/Detach**: Seamlessly attach to existing sessions
- **Bulk Operations**: Delete individual sessions or kill all sessions at once

### Template System

- **Create Templates**: Design multi-pane layouts with custom commands for each pane
- **Visual Editor**: Interactive grid-based editor for arranging panes
- **Flexible Layouts**: Support for horizontal and vertical splits with custom percentages
- **Persistent Storage**: Templates are saved in `~/.config/lazytmux/templates.json`

## Keyboard Shortcuts

### Main Session View

| Key           | Action              |
| ------------- | ------------------- |
| `↑/k`         | Move up             |
| `↓/j`         | Move down           |
| `g`           | Go to top           |
| `G`           | Go to bottom        |
| `Enter/Space` | Attach to session   |
| `n/c`         | Create new session  |
| `t`           | Browse templates    |
| `r`           | Rename session      |
| `d`           | Delete session      |
| `D`           | Delete ALL sessions |
| `Ctrl+R/F5`   | Refresh sessions    |
| `a`           | Toggle auto-refresh |
| `?/h`         | Toggle help         |
| `q/Ctrl+C`    | Quit                |

### Template Browser

| Key           | Action                       |
| ------------- | ---------------------------- |
| `↑/k, ↓/j`    | Navigate templates           |
| `Enter/Space` | Create session from template |
| `n/c`         | Create new template          |
| `e`           | Edit template                |
| `d`           | Delete template              |
| `p`           | Toggle preview               |
| `Esc`         | Back to sessions             |

### Template Editor

| Key        | Action                   |
| ---------- | ------------------------ |
| `↑/k, ↓/j` | Navigate panes           |
| `Enter/e`  | Edit pane command        |
| `H`        | Add pane to the left     |
| `J`        | Add pane below           |
| `K`        | Add pane above           |
| `L`        | Add pane to the right    |
| `d`        | Delete selected pane     |
| `s`        | Save template            |
| `Esc`      | Back to template browser |

## Template Format

Templates are stored as JSON files with the following structure:

```json
{
  "name": "template-name",
  "description": "Optional description",
  "panes": [
    {
      "id": 1,
      "command": "htop",
      "position": "main",
      "parent": 0,
      "split_percent": 50
    },
    {
      "id": 2,
      "command": "tail -f /var/log/syslog",
      "position": "right",
      "parent": 1,
      "split_percent": 30
    }
  ]
}
```

### Pane Properties

- `id`: Unique identifier for the pane
- `command`: Command to run in the pane (optional)
- `position`: Split direction - "main", "left", "right", "up", "down"
- `parent`: ID of the parent pane to split from
- `split_percent`: Percentage of space for the new pane (1-99)

## Configuration

Configuration files are stored in `~/.config/lazytmux/`:

- `templates.json`: Session templates

The configuration directory is created automatically on first run.

### Environment Variables

You can set these environment variables to configure behavior:

- `LAYTMUX_TERMINAL`: Your preferred terminal emulator
- `TERMINAL`: System-wide terminal preference (fallback)

Examples:

```bash
# Set in your shell profile (.bashrc, .zshrc, etc.)
export LAYTMUX_TERMINAL=alacritty

# Or use for a single session
LAYTMUX_TERMINAL=kitty .lazytmux/
```

## Dependencies

This project uses the following Go modules:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components

## License

This project is licensed under the WTFPL - see the LICENSE file for details.
