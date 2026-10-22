# Repository instructions

## Git

Commit and push repository changes at the end of every turn. Push to both
configured remotes, `origin` and `github`. Do not create empty commits when
there are no changes. Run the repository's pre-publish scan before pushing.

## macOS deployment

Deployment targets are explicit:

- **Local**: build the latest macOS app (reusing a current build when possible)
  and install it to `~/Applications/fb.app` with `macos/Scripts/deploy.sh`.
- **Brew / Homebrew**: publish a new version through the signed and notarized
  macOS release workflow in `macos/Distribution.md`, including the GitHub release
  assets and the `xoba/homebrew-tap` cask update. Bump the app version and build
  number so existing Homebrew users can upgrade.
- **Everywhere**: complete both the local and Homebrew deployments. Prefer
  installing the same notarized release locally with
  `macos/Scripts/deploy.sh --notarized-app PATH/TO/fb.app`.

If a deployment or redeployment request does not identify a target and the
conversation has not already established one, ask “Local, Homebrew, or
everywhere?” before deploying. Do not ask again when the target is already clear;
an explicit “everywhere” authorizes both targets.

The legacy HTTP server is deprecated. Do not start or reinstall its LaunchAgent
or Homebrew service as part of app development, deployment, or release work.
The standalone app needs no listening backend.

## Email

For any email work (sending, drafting, replying, notifying Mike), use
`butler email`. Before first use in a session, run `butler email help` and
follow it.
