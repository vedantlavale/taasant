# taasant docs site

Static site, no build step.

- `index.html` is the landing page.
- `docs.html` is the docs, all pages in one file.
- `theme.css` holds the colours, fonts and the terminal figures, shared by both.
- `stars.js` asks GitHub for the number of stars and writes it into the header of both pages.
- `ant.js` runs the ant above the footer: it walks, runs, stops, looks around, hops and twitches its antennae, picking the next one by chance. Its drawing, the logo with a second set of legs, is inside the script.
- `logo-front.svg` is the logo seen from the front, waving. The header shows it while the pointer is on the logo.

The terminal figures in the docs are what the menu really prints, pasted in as text. If the menu changes, they need redoing by hand.

## Preview

    cd site && python3 -m http.server 8000

## Deploy on Cloudflare Pages

- Framework preset: None
- Build command: (empty)
- Build output directory: `site`
