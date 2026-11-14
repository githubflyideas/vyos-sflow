# Security policy

## Reporting a vulnerability

Report it privately through GitHub: **Security › Report a vulnerability** on
this repository ([direct link](https://github.com/githubflyideas/vyos-sflow/security/advisories/new)).
Please do not open a public issue for it.

Say which version you tested (`vyos-sflow version`), what an attacker needs
(network access to the web UI or to UDP 6343, an account, local access), and
how to reproduce it. A proof of concept helps but is not required.

What happens next:

| Step | Within |
| --- | --- |
| We confirm we received the report | 7 days |
| We say whether we accept it and how serious we think it is | 14 days |
| A fixed release, for an accepted report | 90 days, sooner for serious ones |

We agree the publication date with you; the advisory is published with the
fixed release and credits you unless you ask us not to. We do not take legal
action against research done in good faith that respects this policy:
testing only your own installations, and not reading, changing or deleting
other people's data.

## Supported versions

Security fixes go into a new release of the latest version line. Only the
newest release is supported; upgrade to it before reporting.

## Verifying a download

Every release carries `SHA256SUMS`, a signature for it (`SHA256SUMS.sigstore.json`)
and a software bill of materials (`vyos-sflow-VERSION-linux-amd64.cdx.json`,
CycloneDX). The checksum alone proves a download is complete; the signature
proves the checksums were made by this repository's release workflow, so a
release altered by someone with access to the publishing account does not
verify. With [cosign](https://docs.sigstore.dev/cosign/system_config/installation/):

```sh
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github.com/githubflyideas/vyos-sflow/\.github/workflows/ci\.yml@refs/(heads/main|tags/v.*)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
```

## How the code is checked

Every change runs `govulncheck` (known vulnerabilities in the code and the
modules it calls) as part of the tests a release must pass; CodeQL analyses
the code, and Dependabot proposes updates for outdated modules and actions.
The bill of materials lists the modules linked into each release, so a
published vulnerability in one of them (DuckDB's bindings, for instance) can
be matched to the releases it affects.
