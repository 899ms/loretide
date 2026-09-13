# Loretide UI development

All new and changed Web pages must fully reuse Multica's existing components, page layouts and typography conventions. This includes diagnostics and temporary development screens.

- Use existing components from `packages/ui/components/` and shared layouts/compositions from `packages/views/`. Do not hand-build replacement controls or page layouts.
- Inherit the existing font setup, user preferences and semantic design tokens from `packages/ui/styles/tokens.css`. Do not add font families, hardcoded colors or font sizes, local token overrides, or a separate design system.
- Reuse the original component variants, spacing, typography roles, surfaces, focus states and responsive behavior. Using matching colors alone is not sufficient.
- Select a comparable native Multica page before implementation. For settings and management screens, inspect `packages/views/settings/components/settings-page.tsx` and its child views.
- If an existing composition cannot cover a requirement, document the concrete gap and closest upstream component; do not silently create a replacement design system or layout.
- Verify against the native page in the browser under the same theme, zoom and font preferences. Type checking alone is not visual acceptance.

The full Chinese policy is `docs/design/README.md` in the parent Loretide documentation repository. That repository and this application checkout are separate Git repositories. Record implementation evidence and remaining gaps in the documentation repository's `records/`.

Confirmed by the user on 2026-09-13. The current diagnostics page requires remediation; this policy does not certify that remediation is complete.
