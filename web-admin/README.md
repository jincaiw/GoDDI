# GoDDI Web Admin

The GoDDI management console: Vue 3 + TypeScript + Vite, built on the
**TDesign Vue Next Starter** theme.

## Development

```bash
pnpm install
pnpm dev        # dev server
pnpm lint       # eslint
pnpm typecheck  # vue-tsc
pnpm build      # production build -> dist/ (copied to ../web/dist by make web-build)
```

## Notes

- The production bundle is embedded into the Go binary from `../web/dist`
  (see `assets.go`); `make web-build` builds and copies it.
- The previous SoybeanAdmin starter README and changelogs were removed as
  unrelated template leftovers. See the repository `CHANGELOG.md` and `README.md`
  for product-level history and scope.
