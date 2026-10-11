# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| Latest  | Yes       |
| < Latest | No       |

Only the most recent release of [xp-tracker](https://github.com/kanzifucius/xp-tracker) receives security updates. Users are encouraged to stay on the latest version.

## Reporting a Vulnerability

**Do NOT open a public issue for security vulnerabilities.**

To report a security vulnerability, use GitHub's private vulnerability reporting feature:

1. Navigate to the [xp-tracker repository](https://github.com/kanzifucius/xp-tracker)
2. Go to **Settings > Security > Report a vulnerability**
3. Fill out the report with the following information:
   - A clear description of the vulnerability
   - Steps to reproduce the issue
   - Affected versions
   - Potential impact and severity assessment

### Response Timeline

- **Acknowledgement**: Within 7 business days of your report.
- **Resolution**: Depends on the severity and complexity of the vulnerability. Critical issues will be prioritized.

You will be kept informed of progress toward a fix and any relevant decisions throughout the process.

## Disclosure Policy

This project follows a coordinated disclosure model:

1. The vulnerability is reported privately using the process described above.
2. The maintainers investigate and develop a fix.
3. A fix is released before any public announcement is made.
4. After the fix is available, a public advisory is published with appropriate credit to the reporter (unless anonymity is requested).

We ask that reporters refrain from publicly disclosing the vulnerability until a fix has been released.

## Container image provenance and SBOM

Release tags and `main` pushes publish multi-arch images to
`ghcr.io/kanzifucius/xp-tracker` with GitHub Artifact Attestations:

- **SLSA build provenance** (Sigstore keyless OIDC via GitHub Actions — no
  long-lived signing secrets)
- **SPDX SBOM** generated with Syft (`anchore/sbom-action`) and attached as a
  signed attestation (also uploaded as a release asset on tagged releases)

Attestations are written to the GitHub attestations API and pushed to GHCR as
OCI referrers (Cosign bundle layout).

### Verify provenance

Requires [GitHub CLI](https://cli.github.com/) `gh` ≥ 2.49 with attestation
support, and a local Docker credential for GHCR if the image is private:

```bash
docker login ghcr.io

# Replace the tag with a release version or digest
gh attestation verify oci://ghcr.io/kanzifucius/xp-tracker:1.2.3 \
  -R kanzifucius/xp-tracker
```

### Verify the SBOM attestation

```bash
gh attestation verify oci://ghcr.io/kanzifucius/xp-tracker:1.2.3 \
  -R kanzifucius/xp-tracker \
  --predicate-type https://spdx.dev/Document/v2.3
```

To inspect the SBOM predicate:

```bash
gh attestation verify oci://ghcr.io/kanzifucius/xp-tracker:1.2.3 \
  -R kanzifucius/xp-tracker \
  --predicate-type https://spdx.dev/Document/v2.3 \
  --format json \
  --jq '.[].verificationResult.statement.predicate'
```

Tagged releases also attach `xp-tracker-sbom.spdx.json` as a GitHub Release
asset for offline inspection.
