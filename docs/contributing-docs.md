# Contributing to Docs

This site is built with [Zensical](https://zensical.org), a static site generator compatible with MkDocs Material. Source files live in `docs/` as Markdown, configured by `mkdocs.yml` at the repo root.

Pushing to `main` triggers a GitHub Actions workflow that builds and deploys to GitHub Pages automatically.

## Local preview

```bash
# One-time setup
python3 -m venv .venv
.venv/bin/pip install zensical

# Serve with live reload
.venv/bin/zensical serve
# Open http://127.0.0.1:8000
```

Edit any `.md` file under `docs/` and the browser refreshes automatically.

## Build

```bash
.venv/bin/zensical build
```

Output goes to `site/` (gitignored).
