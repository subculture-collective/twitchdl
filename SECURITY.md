# Security Policy

Report security issues privately to the repository maintainers rather than in a
public issue.

Version 1 handles public Twitch URLs and channel names only. It does not accept
or store Twitch client secrets, OAuth tokens, browser cookies, or subscriber-only
content. Do not add secrets to command arguments, configuration, fixtures, logs,
or issue reports.

Remote titles and channel names are untrusted input and must be sanitized before
becoming filesystem paths. External tools must always be invoked with argument
arrays, never by concatenating user input into a shell command. Failed media
finalization must retain the raw recording instead of deleting recoverable data.

## Reporting a vulnerability

Do not publish sensitive vulnerabilities in public issues.

Report privately through GitHub: open the repository's [Security tab](https://github.com/subculture-collective/twitchdl/security/advisories/new) and choose "Report a vulnerability".

Include:

- affected repository and commit
- impact summary
- reproduction steps
- affected configuration or deployment path
- suggested mitigation, if known

## Secret handling

Never commit:

- `.env` or `.env.*` files
- API tokens or OAuth secrets
- SSH keys or deploy keys
- private certificates or signing keys
- webhook secrets

Use placeholders in docs and examples. Report only secret key names and presence, never secret values.

## Dependency and supply-chain hygiene

Contributors should:

- commit `go.sum`
- review `yt-dlp`, Streamlink, Cobra, and Go updates deliberately
- document media-tool compatibility changes
- run dependency and static-analysis checks
- keep generated caches out of git

## Open Pilot safety

Open Pilot tasks should include a deterministic `Test Command` and clear scope limits. Do not queue tasks that require secret access, production credentials, destructive data changes, or ambiguous deployment actions.

For risky work, require human review before adding `agent:queued`.
