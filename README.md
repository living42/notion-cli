# notion-cli

A command-line tool to work with your Notion。

---

## Installation

Install the latest pre-built binary for your platform (macOS / Linux, amd64 / arm64) with one command:

```bash
curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash
```

Options:

```bash
# Install a specific version
curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash -s -- --version v1.2.3

# Install into a custom directory
curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash -s -- --bin-dir ~/bin
```

Alternatively, build from source with Go 1.24+:

```bash
go install github.com/living42/notion-cli@latest
```

---

## Quick Start

```bash
notion configure                              # set your Notion integration token
notion find "meeting notes"                   # search the workspace
notion read 3c90c3cc0d444b5088888dd25736052a  # read a page as Markdown
notion write "Notes" --parent 3c90c3cc…       # create a page (body from --content/--content-file/stdin)
notion mkdb "Tracker" --parent 3c90c3cc…      # create a database
notion mv 3c90c3cc… --parent fed321cb…        # move a page to a new parent
notion rm 3c90c3cc…                           # move a resource to trash
```

---

## License

MIT License

Copyright (c) 2025

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
