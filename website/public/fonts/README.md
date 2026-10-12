# Aeonik webfonts

Drop the licensed Aeonik files here so `src/styles.css` can use them:

- `Aeonik-Regular.woff2` (weight 400)
- `Aeonik-Medium.woff2` (weight 500)
- `Aeonik-Bold.woff2` (weight 700)

Vite serves everything under `public/` at the site root, so these resolve
as `/fonts/Aeonik-Regular.woff2` etc. Until the files land, the system font
stack in `styles.css` applies and nothing breaks.

Do NOT commit licensed font binaries unless the license allows
redistribution; if it does not, keep this folder local-only.
