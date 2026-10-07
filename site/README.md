# taasant docs site

Static site, no build step.

- `index.html` is the landing page.
- `docs.html` is the docs, all pages in one file.
- `theme.css` holds the colours, fonts and the terminal figures, shared by both.

The terminal figures in the docs are what the menu really prints, pasted in as text. If the menu changes, they need redoing by hand.

## Preview

    cd site && python3 -m http.server 8000

## Deploy on Cloudflare Pages

- Framework preset: None
- Build command: (empty)
- Build output directory: `site`
