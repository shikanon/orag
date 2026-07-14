# Vendored Swagger UI assets

This directory contains the browser distribution files from `swagger-ui-dist@5.32.8`, used to keep the runtime `/docs` page self-contained and reproducible.

- Upstream: <https://github.com/swagger-api/swagger-ui>
- Package: <https://www.npmjs.com/package/swagger-ui-dist/v/5.32.8>
- License: Apache-2.0; see `LICENSE` and `NOTICE` in this directory.

To refresh the assets, update the pinned version in `api/embed.go`, replace `swagger-ui.css`, `swagger-ui-bundle.js`, `LICENSE`, and `NOTICE` from the same npm package, then run `make agent-gate` and verify `/docs` in a browser.
